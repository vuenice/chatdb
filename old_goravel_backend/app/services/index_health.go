package services

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DuplicateIndexes finds different btree indexes on the same table with identical indkey (common duplicate pattern).
func DuplicateIndexes(ctx context.Context, pool *pgxpool.Pool) (json.RawMessage, error) {
	q := `
SELECT
  n.nspname AS schema,
  c.relname AS table_name,
  ia.relname AS index_a,
  ib.relname AS index_b
FROM pg_index a
JOIN pg_index b ON a.indrelid = b.indrelid AND a.indkey = b.indkey AND a.indexrelid < b.indexrelid
JOIN pg_class c ON c.oid = a.indrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_class ia ON ia.oid = a.indexrelid
JOIN pg_class ib ON ib.oid = b.indexrelid
JOIN pg_am ama ON ama.oid = ia.relam
JOIN pg_am amb ON amb.oid = ib.relam
WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND ama.amname = 'btree' AND amb.amname = 'btree'
ORDER BY n.nspname, c.relname, ia.relname
`
	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type item struct {
		Schema string `json:"schema"`
		Table  string `json:"table"`
		IndexA string `json:"index_a"`
		IndexB string `json:"index_b"`
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.Schema, &it.Table, &it.IndexA, &it.IndexB); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(items)
	return json.RawMessage(b), err
}

func HasPgStatStatements(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
	var ok bool
	err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_stat_statements')`).Scan(&ok)
	return ok, err
}

func SlowQueries(ctx context.Context, pool *pgxpool.Pool, limit int) (json.RawMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := pool.Query(ctx, `
SELECT queryid::text, LEFT(query, 4000) AS query, calls,
       round(mean_exec_time::numeric, 3)::float8 AS mean_ms,
       round(total_exec_time::numeric, 3)::float8 AS total_ms
FROM pg_stat_statements
ORDER BY mean_exec_time DESC NULLS LAST
LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type row struct {
		QueryID string  `json:"queryid"`
		Query   string  `json:"query"`
		Calls   int64   `json:"calls"`
		MeanMs  float64 `json:"mean_ms"`
		TotalMs float64 `json:"total_ms"`
	}
	var out []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.QueryID, &r.Query, &r.Calls, &r.MeanMs, &r.TotalMs); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(out)
	return json.RawMessage(b), err
}
