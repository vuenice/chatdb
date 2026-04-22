package migrate

import (
	"context"
	"database/sql"
	"fmt"
)

// Bootstrap creates the chatdb metadata tables in the SQLite database if they
// do not already exist. Safe to call on every startup.
func Bootstrap(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			email           TEXT    NOT NULL UNIQUE,
			password_hash   TEXT    NOT NULL,
			name            TEXT    NOT NULL DEFAULT '',
			created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS db_connections (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name            TEXT    NOT NULL,
			driver          TEXT    NOT NULL DEFAULT 'postgres',
			host            TEXT    NOT NULL,
			port            INTEGER NOT NULL,
			"database"      TEXT    NOT NULL,
			ssl_mode        TEXT    NOT NULL DEFAULT 'disable',
			read_username   TEXT    NOT NULL,
			read_password   TEXT    NOT NULL,
			write_username  TEXT    NOT NULL DEFAULT '',
			write_password  TEXT    NOT NULL DEFAULT '',
			allowed_schemas TEXT    NOT NULL DEFAULT '[]',
			created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS db_connections_user_id_idx ON db_connections(user_id)`,
		// One connection per ChatDB user: remove older duplicates then enforce uniqueness.
		`DELETE FROM db_connections WHERE id IN (
			SELECT d1.id FROM db_connections d1
			WHERE EXISTS (
				SELECT 1 FROM db_connections d2
				WHERE d2.user_id = d1.user_id AND d2.id > d1.id
			)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS db_connections_one_per_user ON db_connections(user_id)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("sqlite bootstrap: %w (sql=%s)", err, s)
		}
	}
	return nil
}
