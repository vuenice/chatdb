package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"chatdb/internal/auth"
	"chatdb/internal/config"
	"chatdb/internal/engine"
	"chatdb/internal/store"
)

// effectivePhysicalDatabase returns the DB name to connect to: optional query/body override, else stored default.
func effectivePhysicalDatabase(c *store.DbConnection, override string) (string, error) {
	o := strings.TrimSpace(override)
	if o == "" {
		return c.Database, nil
	}
	if o == c.Database {
		return o, nil
	}
	if !safePhysicalDatabaseName(o) {
		return "", errors.New("invalid database name")
	}
	return o, nil
}

func safePhysicalDatabaseName(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '_', '-', '.', '$':
			continue
		default:
			return false
		}
	}
	return true
}

// resolveEngine loads the connection and returns a pooled engine for the optional ?database= override.
func (s *Server) resolveEngine(r *http.Request, write bool) (engine.Engine, *store.DbConnection, error) {
	return s.resolveEngineWithDB(r, write, strings.TrimSpace(r.URL.Query().Get("database")))
}

// resolveEngineWithDB is like resolveEngine but accepts a database override from a JSON body field.
func (s *Server) resolveEngineWithDB(r *http.Request, write bool, databaseOverride string) (engine.Engine, *store.DbConnection, error) {
	uid, _ := auth.UserID(r)
	id, err := parseConnID(r)
	if err != nil {
		return nil, nil, err
	}
	c, err := s.Store.GetConnection(r.Context(), uid, id)
	if err != nil {
		return nil, nil, errors.New("not found")
	}

	dbName, err := effectivePhysicalDatabase(c, databaseOverride)
	if err != nil {
		return nil, nil, err
	}

	username := c.ReadUsername
	encPass := c.ReadPassword
	if write && c.WriteUsername != "" {
		username = c.WriteUsername
		encPass = c.WritePassword
	}
	password, err := s.Crypter.Decrypt(encPass)
	if err != nil {
		return nil, nil, err
	}

	build := func() (engine.Engine, error) {
		switch config.Driver(c.Driver) {
		case config.DriverMySQL:
			return engine.OpenMySQL(c.Host, c.Port, username, password, dbName)
		default:
			return engine.OpenPostgres(c.Host, c.Port, username, password, dbName, c.SslMode)
		}
	}
	eng, err := s.Pools.GetOrCreate(c.ID, write, dbName, build)
	if err != nil {
		return nil, nil, err
	}
	return eng, c, nil
}

func intParam(r *http.Request, key string, def, max int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return def
	}
	if max > 0 && n > max {
		return max
	}
	return n
}
