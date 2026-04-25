package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const localMetadataUsername = "__chatdb_local@internal"

// Store wraps the SQLite metadata DB and exposes typed queries against the
// `users` and `db_connections` tables.
type Store struct {
	DB *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{DB: db}
}

// User represents a registered application user.
type User struct {
	ID           int64
	Username     string
	DbUsername   string
	DbPassword   string // encrypted
	PasswordHash string // may be empty if NULL in DB
	Name         string
	CreatedAt    time.Time
}

func (s *Store) CreateUser(ctx context.Context, username, passwordHash, name, dbUsername, dbPasswordEnc string) (*User, error) {
	id, err := s.insertReturningID(ctx,
		`INSERT INTO users (username, db_username, db_password, password_hash, name)
		 VALUES (?, ?, ?, ?, ?)`,
		username, dbUsername, dbPasswordEnc, passwordHashOrNull(passwordHash), name,
	)
	if err != nil {
		return nil, err
	}
	return s.UserByID(ctx, id)
}

func passwordHashOrNull(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT id, username, db_username, db_password, password_hash, name, created_at
		 FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *Store) UserByUsername(ctx context.Context, username string) (*User, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT id, username, db_username, db_password, password_hash, name, created_at
		 FROM users WHERE username = ?`, username)
	return scanUser(row)
}

// DeleteUser removes a user row (used to roll back failed registration).
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	var ph sql.NullString
	var created sql.NullTime
	if err := row.Scan(&u.ID, &u.Username, &u.DbUsername, &u.DbPassword, &ph, &u.Name, &created); err != nil {
		return nil, err
	}
	if ph.Valid {
		u.PasswordHash = ph.String
	}
	if created.Valid {
		u.CreatedAt = created.Time
	}
	return &u, nil
}

// CountHumanUsers returns how many real accounts exist (excludes legacy local-only rows).
func (s *Store) CountHumanUsers(ctx context.Context) (int64, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE username != ?`, localMetadataUsername)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// DbConnection mirrors the legacy Goravel model.
type DbConnection struct {
	ID             int64
	UserID         int64
	Name           string
	Driver         string
	Host           string
	Port           int
	Database       string
	SslMode        string
	ReadUsername   string
	ReadPassword   string // encrypted
	WriteUsername  string
	WritePassword  string // encrypted
	AllowedSchemas string // JSON
	CreatedAt      time.Time
}

func (s *Store) CreateConnection(ctx context.Context, c *DbConnection) error {
	id, err := s.insertReturningID(ctx,
		`INSERT INTO db_connections
			(user_id, name, driver, host, port, "database", ssl_mode,
			 read_username, read_password, write_username, write_password, allowed_schemas)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.UserID, c.Name, c.Driver, c.Host, c.Port, c.Database, c.SslMode,
		c.ReadUsername, c.ReadPassword, c.WriteUsername, c.WritePassword, c.AllowedSchemas,
	)
	if err != nil {
		return err
	}
	c.ID = id
	return nil
}

const connectionColumns = `id, user_id, name, driver, host, port, "database", ssl_mode,
	read_username, read_password, write_username, write_password, allowed_schemas, created_at`

// ConnectionCount returns how many connections belong to the user.
func (s *Store) ConnectionCount(ctx context.Context, userID int64) (int64, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM db_connections WHERE user_id = ?`, userID)
	var n int64
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) ListConnections(ctx context.Context, userID int64) ([]DbConnection, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+connectionColumns+` FROM db_connections WHERE user_id = ? ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DbConnection
	for rows.Next() {
		var c DbConnection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Driver, &c.Host, &c.Port, &c.Database,
			&c.SslMode, &c.ReadUsername, &c.ReadPassword, &c.WriteUsername, &c.WritePassword,
			&c.AllowedSchemas, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetConnection(ctx context.Context, userID, id int64) (*DbConnection, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT `+connectionColumns+` FROM db_connections WHERE id = ? AND user_id = ?`, id, userID)
	var c DbConnection
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.Driver, &c.Host, &c.Port, &c.Database,
		&c.SslMode, &c.ReadUsername, &c.ReadPassword, &c.WriteUsername, &c.WritePassword,
		&c.AllowedSchemas, &c.CreatedAt); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) UpdateConnection(ctx context.Context, c *DbConnection) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE db_connections
		 SET name = ?, driver = ?, host = ?, port = ?, "database" = ?, ssl_mode = ?,
		     read_username = ?, read_password = ?, write_username = ?, write_password = ?, allowed_schemas = ?
		 WHERE id = ? AND user_id = ?`,
		c.Name, c.Driver, c.Host, c.Port, c.Database, c.SslMode,
		c.ReadUsername, c.ReadPassword, c.WriteUsername, c.WritePassword, c.AllowedSchemas,
		c.ID, c.UserID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteConnection(ctx context.Context, userID, id int64) error {
	res, err := s.DB.ExecContext(ctx,
		`DELETE FROM db_connections WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) insertReturningID(ctx context.Context, q string, args ...any) (int64, error) {
	res, err := s.DB.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, errors.New("no insert id returned")
	}
	return id, nil
}
