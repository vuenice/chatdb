package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// PostgresDSN builds a libpq URL.
func PostgresDSN(host string, port int, user, password, database, sslmode string) string {
	if sslmode == "" {
		sslmode = "disable"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/" + database,
	}
	q := u.Query()
	q.Set("sslmode", sslmode)
	u.RawQuery = q.Encode()
	return u.String()
}

// MySQLDSN builds a go-sql-driver/mysql DSN.
func MySQLDSN(host string, port int, user, password, database string) string {
	if database == "" {
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/?parseTime=true&charset=utf8mb4&loc=UTC&multiStatements=false",
			user, password, host, port)
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&loc=UTC&multiStatements=false",
		user, password, host, port, database)
}

// OpenMetadataDB opens (and pings) the chatdb metadata SQLite database at path.
// It enables foreign keys and a busy timeout so concurrent API writes don't
// trip SQLITE_BUSY under normal load.
func OpenMetadataDB(path string) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("metadata sqlite path is empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve metadata path: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", abs)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// SQLite serializes writes; a small pool keeps things predictable.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}
