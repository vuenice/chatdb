package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"chatdb/internal/config"
	"chatdb/internal/store"
)

type pgEngine struct {
	db *sql.DB
}

// OpenPostgres creates a new Postgres-backed engine.
func OpenPostgres(host string, port int, user, password, database, sslmode string) (Engine, error) {
	dsn := store.PostgresDSN(host, port, user, password, database, sslmode)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &pgEngine{db: db}, nil
}

func (e *pgEngine) Driver() config.Driver        { return config.DriverPostgres }
func (e *pgEngine) Close()                       { _ = e.db.Close() }
func (e *pgEngine) Ping(ctx context.Context) error { return e.db.PingContext(ctx) }

func (e *pgEngine) ListDatabases(ctx context.Context) ([]string, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT datname FROM pg_database WHERE NOT datistemplate ORDER BY datname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (e *pgEngine) ListSchemas(ctx context.Context) ([]string, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT schema_name FROM information_schema.schemata
		 WHERE schema_name NOT IN ('pg_catalog','information_schema')
		   AND schema_name NOT LIKE 'pg_toast%'
		   AND schema_name NOT LIKE 'pg_temp%'
		 ORDER BY schema_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (e *pgEngine) ListTables(ctx context.Context, schema string) ([]TableMeta, error) {
	if schema == "" {
		schema = "public"
	}
	rows, err := e.db.QueryContext(ctx,
		`SELECT table_schema, table_name, table_type
		 FROM information_schema.tables
		 WHERE table_schema = $1
		 ORDER BY table_name`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TableMeta
	for rows.Next() {
		var t TableMeta
		if err := rows.Scan(&t.Schema, &t.Name, &t.Kind); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (e *pgEngine) ListColumns(ctx context.Context, schema, table string) ([]ColumnMeta, error) {
	if schema == "" {
		schema = "public"
	}
	rows, err := e.db.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable
		 FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2
		 ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ColumnMeta
	for rows.Next() {
		var c ColumnMeta
		if err := rows.Scan(&c.Column, &c.DataType, &c.IsNullable); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (e *pgEngine) ListIndexes(ctx context.Context, schema, table string) ([]IndexMeta, error) {
	if schema == "" {
		schema = "public"
	}
	if !SafeIdent(schema) || !SafeIdent(table) {
		return nil, errors.New("invalid schema or table identifier")
	}
	rows, err := e.db.QueryContext(ctx,
		`SELECT indexname, indexdef FROM pg_indexes WHERE schemaname = $1 AND tablename = $2 ORDER BY indexname`,
		schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IndexMeta
	for rows.Next() {
		var im IndexMeta
		if err := rows.Scan(&im.Name, &im.Definition); err != nil {
			return nil, err
		}
		out = append(out, im)
	}
	return out, rows.Err()
}

func (e *pgEngine) PreviewRows(ctx context.Context, schema, table string, limit, offset int) (*QueryResult, error) {
	if schema == "" {
		schema = "public"
	}
	if !SafeIdent(schema) || !SafeIdent(table) {
		return nil, errors.New("invalid schema or table identifier")
	}
	q := fmt.Sprintf(`SELECT * FROM "%s"."%s" LIMIT $1 OFFSET $2`, schema, table)
	rows, err := e.db.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows, limit)
}

func quotePgIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

type pgQueryExec interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func executeSQL(ctx context.Context, db pgQueryExec, sqlText string, maxRows int) (*QueryResult, error) {
	if maxRows <= 0 || maxRows > 5000 {
		maxRows = 1000
	}
	rows, err := db.QueryContext(ctx, sqlText)
	if err != nil {
		res, execErr := db.ExecContext(ctx, sqlText)
		if execErr != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		return &QueryResult{Message: fmt.Sprintf("%d rows affected", n)}, nil
	}
	defer rows.Close()
	return scanAll(rows, maxRows)
}

func (e *pgEngine) Execute(ctx context.Context, sqlText string, maxRows int) (*QueryResult, error) {
	return executeSQL(ctx, e.db, sqlText, maxRows)
}

func (e *pgEngine) executeWithLocalRole(ctx context.Context, role, sqlText string, maxRows int) (*QueryResult, error) {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SET LOCAL ROLE "+quotePgIdent(role)); err != nil {
		return nil, err
	}
	res, err := executeSQL(ctx, tx, sqlText, maxRows)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return res, nil
}
