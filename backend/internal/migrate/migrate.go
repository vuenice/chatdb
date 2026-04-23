package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Bootstrap creates the chatdb metadata tables in the SQLite database if they
// do not already exist. Safe to call on every startup.
func Bootstrap(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id                INTEGER PRIMARY KEY AUTOINCREMENT,
			username          TEXT    NOT NULL,
			connection_label  TEXT    NOT NULL,
			db_username       TEXT    NOT NULL DEFAULT '',
			db_password       TEXT    NOT NULL DEFAULT '',
			password_hash     TEXT    NULL,
			name              TEXT    NOT NULL DEFAULT '',
			created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (username, connection_label)
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
		`CREATE TABLE IF NOT EXISTS saved_queries (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			connection_id   INTEGER NOT NULL REFERENCES db_connections(id) ON DELETE CASCADE,
			title           TEXT    NOT NULL DEFAULT '',
			sql             TEXT    NOT NULL DEFAULT '',
			is_saved        INTEGER NOT NULL DEFAULT 0,
			last_run_at     DATETIME,
			created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
			updated_at      DATETIME NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX IF NOT EXISTS saved_queries_list_idx ON saved_queries(
			user_id, connection_id, is_saved, last_run_at DESC, updated_at DESC)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("sqlite bootstrap: %w (sql=%s)", err, s)
		}
	}
	return nil
}

// Upgrade migrates older metadata layouts (e.g. users.username unique alone) to
// UNIQUE(username, connection_label) and renames local MySQL root connections
// to the label "mysql_local".
func Upgrade(ctx context.Context, db *sql.DB) error {
	has, err := tableHasColumn(ctx, db, "users", "connection_label")
	if err != nil {
		return err
	}
	if !has {
		_, err = db.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
		if err != nil {
			return fmt.Errorf("pragma foreign_keys: %w", err)
		}
		defer func() { _, _ = db.ExecContext(ctx, "PRAGMA foreign_keys = ON") }()
		_, err = db.ExecContext(ctx, `
			CREATE TABLE users__migrated (
				id                INTEGER PRIMARY KEY AUTOINCREMENT,
				username          TEXT    NOT NULL,
				connection_label  TEXT    NOT NULL,
				db_username       TEXT    NOT NULL DEFAULT '',
				db_password       TEXT    NOT NULL DEFAULT '',
				password_hash     TEXT    NULL,
				name              TEXT    NOT NULL DEFAULT '',
				created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				UNIQUE (username, connection_label)
			)`)
		if err != nil {
			return fmt.Errorf("create users__migrated: %w", err)
		}
		_, err = db.ExecContext(ctx, `
			INSERT INTO users__migrated
				(id, username, connection_label, db_username, db_password, password_hash, name, created_at)
			SELECT
				u.id, u.username,
				COALESCE((SELECT d.name FROM db_connections d WHERE d.user_id = u.id LIMIT 1), ''),
				u.db_username, u.db_password, u.password_hash, u.name, u.created_at
			FROM users u`)
		if err != nil {
			return fmt.Errorf("fill users__migrated: %w", err)
		}
		_, err = db.ExecContext(ctx, "DROP TABLE users")
		if err != nil {
			return fmt.Errorf("drop users: %w", err)
		}
		_, err = db.ExecContext(ctx, "ALTER TABLE users__migrated RENAME TO users")
		if err != nil {
			return fmt.Errorf("rename users: %w", err)
		}
	}
	var uv int
	if e := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&uv); e != nil {
		return e
	}
	if uv < 1 {
		_, err = db.ExecContext(ctx, `
			UPDATE db_connections
			SET name = 'mysql_local'
			WHERE LOWER(TRIM(read_username)) = 'root'`)
		if err != nil {
			if !strings.Contains(strings.ToLower(err.Error()), "no such table") {
				return fmt.Errorf("label mysql_local for root: %w", err)
			}
		}
		if _, err = db.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
			return err
		}
	}
	_, err = db.ExecContext(ctx, `
		UPDATE users
		SET connection_label = (
			SELECT d.name FROM db_connections d WHERE d.user_id = users.id LIMIT 1
		)
		WHERE EXISTS (SELECT 1 FROM db_connections d WHERE d.user_id = users.id)
	`)
	if err != nil {
		return fmt.Errorf("sync connection_label: %w", err)
	}
	return nil
}

func tableHasColumn(ctx context.Context, db *sql.DB, table, col string) (bool, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name, ctype string
			notnull    int
			dfltValue  any
			pk         int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == col {
			return true, nil
		}
	}
	return false, rows.Err()
}
