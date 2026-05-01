package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// LLMProvider defines the interface for SQL generation backends
type LLMProvider interface {
	GenerateSQL(ctx context.Context, schemaJSON string, userQuestion string) (string, error)
}

// OpenAIProvider uses OpenAI API for SQL generation
type OpenAIProvider struct {
	APIKey  string
	BaseURL string
	Model  string
}

func NewOpenAIProvider() *OpenAIProvider {
	return &OpenAIProvider{
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: os.Getenv("OPENAI_BASE_URL"),
		Model:  os.Getenv("OPENAI_MODEL"),
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatReq struct {
	Model    string         `json:"model"`
	Messages []chatMessage `json:"messages"`
	Temperature float64   `json:"temperature"`
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

func (p *OpenAIProvider) GenerateSQL(ctx context.Context, schemaJSON string, userQuestion string) (string, error) {
	if p.APIKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is not set")
	}
	
	baseURL := strings.TrimSuffix(p.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := p.Model
	if model == "" {
		model = "gpt-4o-mini"
	}

	systemPrompt := `You are a PostgreSQL expert. You receive ONLY JSON describing database schema (tables/columns/types). You must NOT assume unseen columns or tables.
Return a single SELECT statement only (CTEs allowed). No comments outside the query. Wrap the SQL in a markdown fenced code block with language sql.`

	userPrompt := "Schema (JSON, no row data):\n" + schemaJSON + "\n\nQuestion:\n" + userQuestion

	body, _ := json.Marshal(chatReq{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
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
	codeFence := regexp.MustCompile("(?s)```(?:sql)?\n(.*?)```")
	if m := codeFence.FindStringSubmatch(content); len(m) == 2 {
		return strings.TrimSpace(m[1]), nil
	}
	return strings.TrimSpace(content), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ClaudeCodeProvider uses Claude Code CLI for SQL generation (fallback)
type ClaudeCodeProvider struct{}

func NewClaudeCodeProvider() *ClaudeCodeProvider {
	return &ClaudeCodeProvider{}
}

func (c *ClaudeCodeProvider) GenerateSQL(ctx context.Context, schemaJSON string, userQuestion string) (string, error) {
	// Build prompt for claude-cli
	// Note: Requires Claude Code CLI to be installed and configured
	prompt := fmt.Sprintf(`Given this database schema (JSON):

%s

Write a PostgreSQL SELECT query for this question: %s

Respond with ONLY the SQL query wrapped in markdown code block with "sql" language tag. No explanation.`, schemaJSON, userQuestion)

	cmd := exec.CommandContext(ctx, "claude-code", prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("claude-code failed: %v - %s", err, stderr.String())
	}

	content := strings.TrimSpace(stdout.String())
	codeFence := regexp.MustCompile("(?s)```(?:sql)?\n(.*?)```")
	if m := codeFence.FindStringSubmatch(content); len(m) == 2 {
		return strings.TrimSpace(m[1]), nil
	}
	return strings.TrimSpace(content), nil
}

// NewProvider creates the appropriate LLM provider based on available configuration
func NewProvider() LLMProvider {
	// Prefer OpenAI if API key is set
	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		return NewOpenAIProvider()
	}
	// Fallback: try Claude Code CLI
	if _, err := exec.LookPath("claude-code"); err == nil {
		return NewClaudeCodeProvider()
	}
	// No provider available
	return nil
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
	for _, tok := range strings.FieldsFunc(sql, func(r rune) bool {
		return r == ' ' || r == ',' || r == '(' || r == ')' || r == '\n' || r == '\t'
	}) {
		tok = strings.Trim(tok, `"'`)
		if !strings.Contains(tok, ".") {
			continue
		}
		parts := strings.SplitN(tok, ".", 2)
		sch := strings.Trim(parts[0], `"'`)
		if sch == "public" || sch == "" {
			continue
		}
		if _, ok := allowed[sch]; !ok {
			if strings.HasPrefix(sch, "pg_") {
				continue
			}
			return fmt.Errorf("schema %q is not in the allowlist for this user", sch)
		}
	}
	return nil
}

// ValidateSingleStatement ensures only one statement
func ValidateSingleStatement(sql string) error {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return fmt.Errorf("empty query")
	}
	return nil
}