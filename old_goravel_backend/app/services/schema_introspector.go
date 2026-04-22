package services

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ColumnMeta struct {
	Schema     string `json:"schema"`
	Table      string `json:"table"`
	Column     string `json:"column"`
	DataType   string `json:"data_type"`
	IsNullable string `json:"is_nullable"`
}

type TableMeta struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Kind   string `json:"kind"` // table | view
}

func ListDatabases(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

func ListTables(ctx context.Context, pool *pgxpool.Pool, schema string) ([]TableMeta, error) {
	q := `
SELECT table_schema, table_name, table_type
FROM information_schema.tables
WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
`
	args := []any{}
	if schema != "" {
		q += ` AND table_schema = $1`
		args = append(args, schema)
	}
	q += ` ORDER BY table_schema, table_name`

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableMeta
	for rows.Next() {
		var sch, name, ttype string
		if err := rows.Scan(&sch, &name, &ttype); err != nil {
			return nil, err
		}
		kind := "table"
		if ttype == "VIEW" {
			kind = "view"
		}
		out = append(out, TableMeta{Schema: sch, Name: name, Kind: kind})
	}
	return out, rows.Err()
}

func TableColumns(ctx context.Context, pool *pgxpool.Pool, schema, table string) ([]ColumnMeta, error) {
	rows, err := pool.Query(ctx, `
SELECT table_schema, table_name, column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_schema = $1 AND table_name = $2
ORDER BY ordinal_position
`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ColumnMeta
	for rows.Next() {
		var c ColumnMeta
		if err := rows.Scan(&c.Schema, &c.Table, &c.Column, &c.DataType, &c.IsNullable); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func SchemaGraphJSON(ctx context.Context, pool *pgxpool.Pool) (json.RawMessage, error) {
	rows, err := pool.Query(ctx, `
SELECT table_schema, table_name, column_name, data_type, is_nullable
FROM information_schema.columns
WHERE table_schema NOT IN ('pg_catalog', 'information_schema')
ORDER BY table_schema, table_name, ordinal_position
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type row struct {
		Schema, Table, Column, DataType, Nullable string
	}
	var list []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.Schema, &r.Table, &r.Column, &r.DataType, &r.Nullable); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(list)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}
