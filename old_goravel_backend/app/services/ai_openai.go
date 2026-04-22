package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

type AIClient struct {
	APIKey  string
	BaseURL string
	Model   string
}

func NewAIClientFromEnv() *AIClient {
	return &AIClient{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: strings.TrimSuffix(getenv("OPENAI_BASE_URL", "https://api.openai.com/v1"), "/"),
		Model:   getenv("OPENAI_MODEL", "gpt-4o-mini"),
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

type chatReq struct {
	Model    string         `json:"model"`
	Messages []chatMessage  `json:"messages"`
	Temp     float64        `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

var codeFence = regexp.MustCompile("(?s)```(?:sql)?\n(.*?)```")

func (c *AIClient) GenerateSQL(ctx context.Context, schemaJSON string, userQuestion string) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is not set")
	}
	system := `You are a PostgreSQL expert. You receive ONLY JSON describing database schema (tables/columns/types). You must NOT assume unseen columns or tables.
Return a single SELECT statement only (CTEs allowed). No comments outside the query. Wrap the SQL in a markdown fenced code block with language sql.`

	user := "Schema (JSON, no row data):\n" + schemaJSON + "\n\nQuestion:\n" + userQuestion

	body, _ := json.Marshal(chatReq{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temp: 0.2,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	var cr chatResp
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("decode response: %w; body=%s", err, truncate(string(raw), 500))
	}
	if cr.Error != nil {
		return "", fmt.Errorf("openai: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	content := strings.TrimSpace(cr.Choices[0].Message.Content)
	if m := codeFence.FindStringSubmatch(content); len(m) == 2 {
		return strings.TrimSpace(m[1]), nil
	}
	// fallback: whole content
	return strings.TrimSpace(content), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ValidateGeneratedSQL performs light checks for viewer-safe queries.
func ValidateGeneratedSQL(sql string, allowedSchemas []string) error {
	if err := ValidateSingleStatement(sql); err != nil {
		return err
	}
	u := strings.ToUpper(strings.TrimSpace(sql))
	if strings.HasPrefix(u, "INSERT") || strings.HasPrefix(u, "UPDATE") || strings.HasPrefix(u, "DELETE") ||
		strings.HasPrefix(u, "DROP") || strings.HasPrefix(u, "ALTER") || strings.HasPrefix(u, "TRUNCATE") ||
		strings.HasPrefix(u, "GRANT") || strings.HasPrefix(u, "REVOKE") {
		return fmt.Errorf("only read queries are allowed for this role")
	}
	if len(allowedSchemas) == 0 {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, s := range allowedSchemas {
		allowed[strings.TrimSpace(s)] = struct{}{}
	}
	// Very small heuristic: forbid explicit references to non-allowed schemas like other_schema.table
	for _, tok := range strings.FieldsFunc(sql, func(r rune) bool {
		return r == ' ' || r == ',' || r == '(' || r == ')' || r == '\n' || r == '\t'
	}) {
		tok = strings.Trim(tok, `"'`)
		if !strings.Contains(tok, ".") {
			continue
		}
		parts := strings.SplitN(tok, ".", 2)
		sch := strings.Trim(parts[0], `"`)
		if sch == "public" || sch == "" {
			continue
		}
		if _, ok := allowed[sch]; !ok {
			// allow pg_catalog for casts is rare in SELECT - skip if starts with pg_
			if strings.HasPrefix(sch, "pg_") {
				continue
			}
			return fmt.Errorf("schema %q is not in the allowlist for this user", sch)
		}
	}
	return nil
}
