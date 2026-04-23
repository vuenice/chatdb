package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"time"
)

// SavedQuery is a stored SQL row (ad-hoc history and/or an explicitly saved query).
type SavedQuery struct {
	ID           int64
	UserID       int64
	ConnectionID int64
	Title        string
	Sql          string
	IsSaved      bool
	LastRunAt    *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

const savedQueryCols = `id, user_id, connection_id, title, sql, is_saved, last_run_at, created_at, updated_at`

func scanSavedQueryRow(row *sql.Row) (*SavedQuery, error) {
	var q SavedQuery
	var lastRun sql.NullTime
	var isInt int
	err := row.Scan(
		&q.ID, &q.UserID, &q.ConnectionID, &q.Title, &q.Sql, &isInt, &lastRun, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	q.IsSaved = isInt != 0
	if lastRun.Valid {
		t := lastRun.Time
		q.LastRunAt = &t
	}
	return &q, nil
}

func scanSavedQueryFromRows(rows *sql.Rows) (*SavedQuery, error) {
	var q SavedQuery
	var lastRun sql.NullTime
	var isInt int
	err := rows.Scan(
		&q.ID, &q.UserID, &q.ConnectionID, &q.Title, &q.Sql, &isInt, &lastRun, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	q.IsSaved = isInt != 0
	if lastRun.Valid {
		t := lastRun.Time
		q.LastRunAt = &t
	}
	return &q, nil
}

// ListSavedQueriesForConnection returns explicit saves (is_saved=1) for a connection, newest first.
func (s *Store) ListSavedQueriesForConnection(ctx context.Context, userID, connectionID int64) ([]SavedQuery, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+savedQueryCols+` FROM saved_queries
		WHERE user_id = ? AND connection_id = ? AND is_saved = 1
		ORDER BY updated_at DESC
		LIMIT 200
	`, userID, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAllSavedQueryRows(rows)
}

// SQLContainsUpdateOrAlter reports whether sql contains an UPDATE or ALTER statement keyword.
var sqlUpdateOrAlterRE = regexp.MustCompile(`(?is)\b(UPDATE|ALTER)\b`)

func SQLContainsUpdateOrAlter(sql string) bool {
	return sqlUpdateOrAlterRE.MatchString(sql)
}

// ListRecentRunsForConnection returns executed queries (rows with last_run_at), newest first.
// If onlyUpdateOrAlter is true, only rows whose SQL contains UPDATE or ALTER are returned (up to limit).
func (s *Store) ListRecentRunsForConnection(ctx context.Context, userID, connectionID int64, onlyUpdateOrAlter bool, limit int) ([]SavedQuery, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+savedQueryCols+` FROM saved_queries
		WHERE user_id = ? AND connection_id = ? AND last_run_at IS NOT NULL
		ORDER BY last_run_at DESC, id DESC
		LIMIT 2000
	`, userID, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedQuery
	for rows.Next() {
		q, err := scanSavedQueryFromRows(rows)
		if err != nil {
			return nil, err
		}
		if q == nil {
			continue
		}
		if onlyUpdateOrAlter && !SQLContainsUpdateOrAlter(q.Sql) {
			continue
		}
		out = append(out, *q)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

// ListAllQueriesForConnection returns history and saves for a connection (Conversations), newest first.
func (s *Store) ListAllQueriesForConnection(ctx context.Context, userID, connectionID int64) ([]SavedQuery, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+savedQueryCols+` FROM saved_queries
		WHERE user_id = ? AND connection_id = ?
		ORDER BY COALESCE(last_run_at, updated_at) DESC, id DESC
		LIMIT 200
	`, userID, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAllSavedQueryRows(rows)
}

func scanAllSavedQueryRows(rows *sql.Rows) ([]SavedQuery, error) {
	var out []SavedQuery
	for rows.Next() {
		q, err := scanSavedQueryFromRows(rows)
		if err != nil {
			return nil, err
		}
		if q == nil {
			continue
		}
		out = append(out, *q)
	}
	return out, rows.Err()
}

// GetSavedQuery fetches a single row if owned by the user and connection.
func (s *Store) GetSavedQuery(ctx context.Context, userID, connectionID, id int64) (*SavedQuery, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT `+savedQueryCols+` FROM saved_queries
		WHERE id = ? AND user_id = ? AND connection_id = ?
	`, id, userID, connectionID)
	return scanSavedQueryRow(row)
}

// CreateSavedQuery inserts a new row and sets q.ID, CreatedAt, UpdatedAt.
func (s *Store) CreateSavedQuery(ctx context.Context, q *SavedQuery) error {
	now := time.Now().UTC()
	isInt := 0
	if q.IsSaved {
		isInt = 1
	}
	id, err := s.insertReturningID(ctx, `
		INSERT INTO saved_queries
			(user_id, connection_id, title, sql, is_saved, last_run_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		q.UserID, q.ConnectionID, q.Title, q.Sql, isInt, nullableTime(q.LastRunAt), now, now,
	)
	if err != nil {
		return err
	}
	q.ID = id
	q.CreatedAt = now
	q.UpdatedAt = now
	return nil
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

// UpdateSavedQuery updates title, SQL body, is_saved, and updated_at.
func (s *Store) UpdateSavedQuery(ctx context.Context, userID, connectionID, id int64, title, sqlText string, isSaved bool) error {
	isInt := 0
	if isSaved {
		isInt = 1
	}
	now := time.Now().UTC()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE saved_queries
		SET title = ?, sql = ?, is_saved = ?, updated_at = ?
		WHERE id = ? AND user_id = ? AND connection_id = ?
	`, title, sqlText, isInt, now, id, userID, connectionID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteSavedQuery removes a row for the user/connection.
func (s *Store) DeleteSavedQuery(ctx context.Context, userID, connectionID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `
		DELETE FROM saved_queries WHERE id = ? AND user_id = ? AND connection_id = ?
	`, id, userID, connectionID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// TouchRun inserts a non-saved history entry with last_run_at = now.
func (s *Store) TouchRun(ctx context.Context, userID, connectionID int64, sqlText string) (*SavedQuery, error) {
	if sqlText == "" {
		return nil, errors.New("sql is required")
	}
	now := time.Now().UTC()
	q := &SavedQuery{
		UserID:       userID,
		ConnectionID: connectionID,
		Title:        "",
		Sql:          sqlText,
		IsSaved:      false,
		LastRunAt:    &now,
	}
	if err := s.CreateSavedQuery(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}
