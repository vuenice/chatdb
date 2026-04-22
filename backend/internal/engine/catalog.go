package engine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"chatdb/internal/config"
)

func catalogDB(eng Engine) (*sql.DB, config.Driver, error) {
	switch e := eng.(type) {
	case *pgEngine:
		return e.db, config.DriverPostgres, nil
	case *myEngine:
		return e.db, config.DriverMySQL, nil
	default:
		return nil, "", errors.New("unsupported engine")
	}
}

// ListCatalogRoleNames returns pg_roles.rolname or DISTINCT mysql.user.User.
func ListCatalogRoleNames(ctx context.Context, eng Engine) ([]string, error) {
	db, d, err := catalogDB(eng)
	if err != nil {
		return nil, err
	}
	switch d {
	case config.DriverPostgres:
		rows, err := db.QueryContext(ctx, `SELECT rolname FROM pg_roles ORDER BY rolname`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanStringCol(rows)
	default:
		rows, err := db.QueryContext(ctx, `SELECT DISTINCT User FROM mysql.user ORDER BY User`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanStringCol(rows)
	}
}

// CatalogLoginRow is a human-facing login principal on the target server.
type CatalogLoginRow struct {
	Name string `json:"name"`
	Host string `json:"host,omitempty"`
}

// ListCatalogLoginRows returns roles that can log in (Postgres) or mysql.user rows.
func ListCatalogLoginRows(ctx context.Context, eng Engine) ([]CatalogLoginRow, error) {
	db, d, err := catalogDB(eng)
	if err != nil {
		return nil, err
	}
	switch d {
	case config.DriverPostgres:
		rows, err := db.QueryContext(ctx, `SELECT rolname FROM pg_roles WHERE rolcanlogin ORDER BY rolname`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []CatalogLoginRow
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return nil, err
			}
			out = append(out, CatalogLoginRow{Name: n})
		}
		return out, rows.Err()
	default:
		rows, err := db.QueryContext(ctx, `SELECT User, Host FROM mysql.user ORDER BY User, Host`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []CatalogLoginRow
		for rows.Next() {
			var u, h string
			if err := rows.Scan(&u, &h); err != nil {
				return nil, err
			}
			out = append(out, CatalogLoginRow{Name: u, Host: h})
		}
		return out, rows.Err()
	}
}

// CreateCatalogLoginUser creates a login user and optionally grants an existing role.
// username and assignRole must be simple identifiers (letters, digits, underscore).
func CreateCatalogLoginUser(ctx context.Context, eng Engine, username, password, assignRole string) error {
	if !safeCatalogIdent(username) {
		return errors.New("invalid username")
	}
	if assignRole != "" && !safeCatalogIdent(assignRole) {
		return errors.New("invalid role name")
	}
	if password == "" {
		return errors.New("password is required")
	}
	db, d, err := catalogDB(eng)
	if err != nil {
		return err
	}
	switch d {
	case config.DriverPostgres:
		pw := strings.ReplaceAll(password, "'", "''")
		create := fmt.Sprintf("CREATE USER %s WITH LOGIN PASSWORD '%s'", quotePgIdent(username), pw)
		if _, err := db.ExecContext(ctx, create); err != nil {
			return err
		}
		if assignRole != "" {
			grant := fmt.Sprintf("GRANT %s TO %s", quotePgIdent(assignRole), quotePgIdent(username))
			if _, err := db.ExecContext(ctx, grant); err != nil {
				return fmt.Errorf("user created but grant failed: %w", err)
			}
		}
		return nil
	default:
		// MySQL: identifier-quoted user; password via driver placeholder.
		qUser := "`" + strings.ReplaceAll(username, "`", "``") + "`"
		create := fmt.Sprintf("CREATE USER %s@'%%' IDENTIFIED BY ?", qUser)
		if _, err := db.ExecContext(ctx, create, password); err != nil {
			return err
		}
		if assignRole != "" {
			rq := "`" + strings.ReplaceAll(assignRole, "`", "``") + "`"
			grant := fmt.Sprintf("GRANT %s TO %s@'%%'", rq, qUser)
			if _, err := db.ExecContext(ctx, grant); err != nil {
				return fmt.Errorf("user created but grant failed: %w", err)
			}
		}
		return nil
	}
}

func safeCatalogIdent(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func scanStringCol(rows *sql.Rows) ([]string, error) {
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
