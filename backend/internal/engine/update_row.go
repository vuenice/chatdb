package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"chatdb/internal/config"
)

// UpdateTableRow runs UPDATE ... SET ... WHERE using original cell values to identify the row.
// original should contain a full row snapshot (used for WHERE). values may contain only the
// columns to update. Uses the write pool / role semantics
// via executeParameterizedWithOptionalRole (write pool + optional Postgres ROLE).
func UpdateTableRow(ctx context.Context, eng Engine, role, schema, table string, original, values map[string]any) (*QueryResult, error) {
	if len(original) == 0 || len(values) == 0 {
		return nil, errors.New("original and values are required")
	}
	if _, ok := original["id"]; !ok {
		return nil, errors.New("original.id is required")
	}
	sch := effectiveSchemaForSQL(eng, schema)
	if !SafeIdent(table) || !SafeIdent(sch) {
		return nil, errors.New("invalid schema or table identifier")
	}

	cols, err := eng.ListColumns(ctx, schema, table)
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, errors.New("no columns")
	}
	colSet := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		colSet[c.Column] = struct{}{}
	}
	if _, ok := colSet["id"]; !ok {
		return nil, errors.New(`table must contain an "id" column`)
	}
	for k := range values {
		if _, ok := colSet[k]; !ok {
			return nil, fmt.Errorf("values contains unknown column %q", k)
		}
	}

	sqlText, args, err := buildUpdateRowSQL(eng.Driver(), sch, table, cols, original, values)
	if err != nil {
		return nil, err
	}
	return executeParameterizedWithOptionalRole(ctx, eng, role, sqlText, args)
}

func effectiveSchemaForSQL(eng Engine, schema string) string {
	if me, ok := eng.(*myEngine); ok {
		if schema == "" || schema == "public" {
			return me.database
		}
		return schema
	}
	if schema == "" {
		return "public"
	}
	return schema
}

func quoteTableRef(drv config.Driver, schema, table string) string {
	if drv == config.DriverMySQL {
		return fmt.Sprintf("`%s`.`%s`", schema, table)
	}
	return fmt.Sprintf(`"%s"."%s"`, strings.ReplaceAll(schema, `"`, `""`), strings.ReplaceAll(table, `"`, `""`))
}

func quoteColIdent(drv config.Driver, col string) string {
	if drv == config.DriverMySQL {
		return "`" + strings.ReplaceAll(col, "`", "``") + "`"
	}
	return `"` + strings.ReplaceAll(col, `"`, `""`) + `"`
}

func buildUpdateRowSQL(drv config.Driver, schema, table string, cols []ColumnMeta, original, values map[string]any) (string, []any, error) {
	var args []any
	n := 1
	nextPH := func() string {
		if drv == config.DriverMySQL {
			return "?"
		}
		s := fmt.Sprintf("$%d", n)
		n++
		return s
	}

	setParts := make([]string, 0, len(values))
	for _, c := range cols {
		v, ok := values[c.Column]
		if !ok {
			continue
		}
		if !SafeIdent(c.Column) {
			return "", nil, fmt.Errorf("invalid column name %q", c.Column)
		}
		ph := nextPH()
		setParts = append(setParts, quoteColIdent(drv, c.Column)+" = "+ph)
		args = append(args, v)
	}
	if len(setParts) == 0 {
		return "", nil, errors.New("no columns to update")
	}

	idVal, ok := original["id"]
	if !ok {
		return "", nil, errors.New("original.id is required")
	}
	if idVal == nil {
		return "", nil, errors.New("original.id cannot be null")
	}
	whereParts := []string{quoteColIdent(drv, "id") + " = " + nextPH()}
	args = append(args, idVal)

	q := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s",
		quoteTableRef(drv, schema, table),
		strings.Join(setParts, ", "),
		strings.Join(whereParts, " AND "),
	)
	return q, args, nil
}

func (e *pgEngine) execParameterizedMutation(ctx context.Context, sqlText string, args []any) (int64, error) {
	res, err := e.db.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (e *pgEngine) execParameterizedMutationWithLocalRole(ctx context.Context, role, sqlText string, args []any) (int64, error) {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SET LOCAL ROLE "+quotePgIdent(role)); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

func (e *myEngine) execParameterizedMutation(ctx context.Context, sqlText string, args []any) (int64, error) {
	res, err := e.db.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
