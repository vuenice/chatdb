package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"chatdb/internal/config"
	"chatdb/internal/store"
)

type myEngine struct {
	db       *sql.DB
	database string // default schema for queries when none is given
}

// OpenMySQL creates a new MySQL/MariaDB-backed engine.
func OpenMySQL(host string, port int, user, password, database string) (Engine, error) {
	dsn := store.MySQLDSN(host, port, user, password, database)
	db, err := sql.Open("mysql", dsn)
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
	return &myEngine{db: db, database: database}, nil
}

func (e *myEngine) Driver() config.Driver        { return config.DriverMySQL }
func (e *myEngine) Close()                       { _ = e.db.Close() }
func (e *myEngine) Ping(ctx context.Context) error { return e.db.PingContext(ctx) }

func (e *myEngine) ListDatabases(ctx context.Context) ([]string, error) {
	// Physical databases (not information_schema schemata).
	rows, err := e.db.QueryContext(ctx, `SHOW DATABASES`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	skip := map[string]struct{}{
		"information_schema": {},
		"mysql":              {},
		"performance_schema": {},
		"sys":                {},
	}
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if _, bad := skip[s]; bad {
			continue
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListSchemas returns logical schemas within the current database.
func (e *myEngine) ListSchemas(ctx context.Context) ([]string, error) {
	rows, err := e.db.QueryContext(ctx,
		`SELECT schema_name FROM information_schema.schemata
		 WHERE schema_name NOT IN ('mysql','information_schema','performance_schema','sys')
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

func (e *myEngine) ListTables(ctx context.Context, schema string) ([]TableMeta, error) {
	// UI defaults to "public" (Postgres); MySQL uses the current DB name as table_schema.
	if schema == "" || schema == "public" {
		schema = e.database
	}
	rows, err := e.db.QueryContext(ctx,
		`SELECT table_schema, table_name, table_type
		 FROM information_schema.tables
		 WHERE table_schema = ?
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

func (e *myEngine) ListColumns(ctx context.Context, schema, table string) ([]ColumnMeta, error) {
	if schema == "" || schema == "public" {
		schema = e.database
	}
	rows, err := e.db.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable
		 FROM information_schema.columns
		 WHERE table_schema = ? AND table_name = ?
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

func (e *myEngine) ListIndexes(ctx context.Context, schema, table string) ([]IndexMeta, error) {
	if schema == "" || schema == "public" {
		schema = e.database
	}
	if !SafeIdent(schema) || !SafeIdent(table) {
		return nil, errors.New("invalid schema or table identifier")
	}
	rows, err := e.db.QueryContext(ctx,
		`SELECT DISTINCT index_name
		 FROM information_schema.statistics
		 WHERE table_schema = ? AND table_name = ?
		 ORDER BY index_name`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IndexMeta
	for rows.Next() {
		var im IndexMeta
		if err := rows.Scan(&im.Name); err != nil {
			return nil, err
		}
		out = append(out, im)
	}
	return out, rows.Err()
}

func (e *myEngine) PreviewRows(ctx context.Context, schema, table string, limit, offset int) (*QueryResult, error) {
	if schema == "" || schema == "public" {
		schema = e.database
	}
	if !SafeIdent(schema) || !SafeIdent(table) {
		return nil, errors.New("invalid schema or table identifier")
	}
	q := fmt.Sprintf("SELECT * FROM `%s`.`%s` LIMIT ? OFFSET ?", schema, table)
	rows, err := e.db.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAll(rows, limit)
}

func (e *myEngine) Execute(ctx context.Context, sqlText string, maxRows int) (*QueryResult, error) {
	if maxRows <= 0 || maxRows > 5000 {
		maxRows = 1000
	}
	rows, err := e.db.QueryContext(ctx, sqlText)
	if err != nil {
		res, execErr := e.db.ExecContext(ctx, sqlText)
		if execErr != nil {
			return nil, err
		}
		n, _ := res.RowsAffected()
		return &QueryResult{Message: fmt.Sprintf("%d rows affected", n)}, nil
	}
	defer rows.Close()
	return scanAll(rows, maxRows)
}
