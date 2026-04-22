package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultMaxRows = 500

var multiStmtPattern = regexp.MustCompile(`;\s*\S`)

type ExecuteResult struct {
	Columns  []string `json:"columns"`
	Rows     [][]any  `json:"rows"`
	RowCount int      `json:"row_count"`
	Message  string   `json:"message,omitempty"`
}

func ValidateSingleStatement(sql string) error {
	s := strings.TrimSpace(sql)
	if s == "" {
		return fmt.Errorf("empty sql")
	}
	if multiStmtPattern.MatchString(s) {
		return fmt.Errorf("multiple statements are not allowed")
	}
	return nil
}

func NormalizeCell(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case json.RawMessage:
		return json.RawMessage(x)
	default:
		return x
	}
}

func usesRowCursor(sql string) bool {
	s := strings.TrimSpace(sql)
	u := strings.ToUpper(s)
	if strings.HasPrefix(u, "SELECT") || strings.HasPrefix(u, "WITH") ||
		strings.HasPrefix(u, "SHOW") || strings.HasPrefix(u, "EXPLAIN") ||
		strings.HasPrefix(u, "TABLE") || strings.HasPrefix(u, "VALUES") {
		return true
	}
	return strings.Contains(u, "INSERT") && strings.Contains(u, "RETURNING")
}

func Execute(ctx context.Context, pool *pgxpool.Pool, sql string, maxRows int) (*ExecuteResult, error) {
	if err := ValidateSingleStatement(sql); err != nil {
		return nil, err
	}
	if maxRows <= 0 {
		maxRows = defaultMaxRows
	}

	if !usesRowCursor(sql) {
		tag, err := pool.Exec(ctx, sql)
		if err != nil {
			return nil, err
		}
		return &ExecuteResult{
			Columns:  []string{},
			Rows:     [][]any{},
			RowCount: 0,
			Message:  tag.String(),
		}, nil
	}

	rows, err := pool.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	desc := rows.FieldDescriptions()
	cols := make([]string, len(desc))
	for i, d := range desc {
		cols[i] = string(d.Name)
	}
	out := [][]any{}
	n := 0
	for rows.Next() {
		if n >= maxRows {
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]any, len(vals))
		for i, v := range vals {
			row[i] = NormalizeCell(v)
		}
		out = append(out, row)
		n++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &ExecuteResult{Columns: cols, Rows: out, RowCount: len(out)}, nil
}

func ExplainJSON(ctx context.Context, pool *pgxpool.Pool, sql string) (json.RawMessage, error) {
	if err := ValidateSingleStatement(sql); err != nil {
		return nil, err
	}
	explain := "EXPLAIN (FORMAT JSON) " + sql
	rows, err := pool.Query(ctx, explain)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, fmt.Errorf("no explain result")
	}
	var raw []byte
	if err := rows.Scan(&raw); err != nil {
		return nil, err
	}
	return json.RawMessage(raw), rows.Err()
}
