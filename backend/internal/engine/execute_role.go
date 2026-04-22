package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ExecuteWithOptionalRole runs sqlText on eng. For Postgres with a non-empty role,
// uses SET LOCAL ROLE inside a transaction. Other drivers ignore role (MySQL has no equivalent).
func ExecuteWithOptionalRole(ctx context.Context, eng Engine, role, sqlText string, maxRows int) (*QueryResult, error) {
	r := strings.TrimSpace(role)
	if r == "" {
		return eng.Execute(ctx, sqlText, maxRows)
	}
	if pe, ok := eng.(*pgEngine); ok {
		return pe.executeWithLocalRole(ctx, r, sqlText, maxRows)
	}
	return eng.Execute(ctx, sqlText, maxRows)
}

// executeParameterizedWithOptionalRole runs a parameterized UPDATE (or similar) with optional Postgres ROLE.
func executeParameterizedWithOptionalRole(ctx context.Context, eng Engine, role, sqlText string, args []any) (*QueryResult, error) {
	r := strings.TrimSpace(role)
	var n int64
	var err error
	switch e := eng.(type) {
	case *pgEngine:
		if r != "" {
			n, err = e.execParameterizedMutationWithLocalRole(ctx, r, sqlText, args)
		} else {
			n, err = e.execParameterizedMutation(ctx, sqlText, args)
		}
	case *myEngine:
		n, err = e.execParameterizedMutation(ctx, sqlText, args)
	default:
		return nil, errors.New("unsupported SQL engine")
	}
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("no row updated (row may have changed or no longer exists)")
	}
	return &QueryResult{Message: fmt.Sprintf("%d rows affected", n)}, nil
}
