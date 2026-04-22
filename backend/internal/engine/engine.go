package engine

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"chatdb/internal/config"
)

// TableMeta describes a table or view returned by ListTables.
type TableMeta struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
}

// ColumnMeta describes a column returned by ListColumns.
type ColumnMeta struct {
	Column     string `json:"column"`
	DataType   string `json:"data_type"`
	IsNullable string `json:"is_nullable"`
}

// IndexMeta describes an index on a table.
type IndexMeta struct {
	Name       string `json:"name"`
	Definition string `json:"definition,omitempty"`
}

// QueryResult is the canonical row-set shape returned to the frontend.
type QueryResult struct {
	Columns  []string `json:"columns"`
	Rows     [][]any  `json:"rows"`
	RowCount int      `json:"row_count"`
	Message  string   `json:"message,omitempty"`
}

// Engine is the dialect-agnostic interface implemented per driver.
type Engine interface {
	Driver() config.Driver
	Ping(ctx context.Context) error
	ListDatabases(ctx context.Context) ([]string, error)
	ListSchemas(ctx context.Context) ([]string, error)
	ListTables(ctx context.Context, schema string) ([]TableMeta, error)
	ListColumns(ctx context.Context, schema, table string) ([]ColumnMeta, error)
	ListIndexes(ctx context.Context, schema, table string) ([]IndexMeta, error)
	PreviewRows(ctx context.Context, schema, table string, limit, offset int) (*QueryResult, error)
	Execute(ctx context.Context, sql string, maxRows int) (*QueryResult, error)
	Close()
}

var safeIdentRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// SafeIdent reports whether name is a simple identifier that can be safely
// quoted into SQL (no embedded quotes, dots, or whitespace).
func SafeIdent(name string) bool {
	return safeIdentRE.MatchString(name)
}

// scanAll reads every row of rs into a QueryResult, capping at maxRows when
// maxRows > 0.
func scanAll(rs *sql.Rows, maxRows int) (*QueryResult, error) {
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	out := &QueryResult{Columns: cols, Rows: make([][]any, 0, 64)}
	for rs.Next() {
		if maxRows > 0 && len(out.Rows) >= maxRows {
			break
		}
		row := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range row {
			ptrs[i] = &row[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range row {
			row[i] = normalize(v)
		}
		out.Rows = append(out.Rows, row)
	}
	if err := rs.Err(); err != nil {
		return nil, err
	}
	out.RowCount = len(out.Rows)
	return out, nil
}

// normalize converts driver-native types into JSON-friendly forms.
func normalize(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	}
	return v
}

// Pool wraps a *sql.DB plus dialect metadata so handlers don't need to know.
type Pool struct {
	DB     *sql.DB
	Driver config.Driver
}

// Manager caches Engines per (connectionID, mode) so we don't reopen per request.
type Manager struct {
	mu      sync.Mutex
	engines map[string]Engine
}

func NewManager() *Manager {
	return &Manager{engines: map[string]Engine{}}
}

func poolKey(connID int64, write bool, database string) string {
	w := "read"
	if write {
		w = "write"
	}
	// Prefix "c:<id>:" so Invalidate(connID) cannot match a longer numeric id.
	return fmt.Sprintf("c:%d:%s:%s", connID, w, database)
}

// GetOrCreate returns a cached engine or builds one with the given factory.
// database is the effective physical database name (must be non-empty).
func (m *Manager) GetOrCreate(connID int64, write bool, database string, build func() (Engine, error)) (Engine, error) {
	k := poolKey(connID, write, database)
	m.mu.Lock()
	if e, ok := m.engines[k]; ok {
		m.mu.Unlock()
		return e, nil
	}
	m.mu.Unlock()

	e, err := build()
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.engines[k]; ok {
		e.Close()
		return existing, nil
	}
	m.engines[k] = e
	return e, nil
}

// Invalidate closes and forgets all engines for connID (any database variant).
func (m *Manager) Invalidate(connID int64) {
	prefix := fmt.Sprintf("c:%d:", connID)
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.engines {
		if strings.HasPrefix(k, prefix) {
			e.Close()
			delete(m.engines, k)
		}
	}
}

// CloseAll closes all engines (for graceful shutdown).
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.engines {
		e.Close()
		delete(m.engines, k)
	}
}
