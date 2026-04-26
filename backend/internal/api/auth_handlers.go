package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"chatdb/internal/auth"
	"chatdb/internal/config"
	"chatdb/internal/store"
)

type registerReq struct {
	// Target DB connection (required for registration)
	ConnName       string   `json:"connection_name"`
	Driver         string   `json:"driver"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	Database       string   `json:"database"`
	SslMode        string   `json:"ssl_mode"`
	ReadUsername   string   `json:"read_username"`
	ReadPassword   string   `json:"read_password"`
	WriteUsername  string   `json:"write_username"`
	WritePassword  string   `json:"write_password"`
	AllowedSchemas []string `json:"allowed_schemas"`
}

type loginReq struct {
	ConnectionName string `json:"connection_name"`
	Username       string `json:"username"`
	Password       string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Host == "" || req.Database == "" || req.ReadUsername == "" {
		writeErr(w, http.StatusBadRequest, errors.New("host, database, and read_username are required"))
		return
	}
	driver := strings.ToLower(strings.TrimSpace(req.Driver))
	if driver == "" {
		driver = string(config.DriverPostgres)
	}
	if driver != string(config.DriverPostgres) && driver != string(config.DriverMySQL) {
		writeErr(w, http.StatusBadRequest, errors.New("driver must be postgres or mysql"))
		return
	}
	port := req.Port
	if port == 0 {
		if driver == string(config.DriverPostgres) {
			port = 5432
		} else {
			port = 3306
		}
	}
	sslMode := req.SslMode
	if sslMode == "" {
		sslMode = "disable"
	}
	connName := strings.TrimSpace(req.ConnName)
	if connName == "" {
		writeErr(w, http.StatusBadRequest, errors.New("connection_name is required"))
		return
	}

	username := strings.TrimSpace(req.ReadUsername)
	if username == "" {
		writeErr(w, http.StatusBadRequest, errors.New("read_username is required"))
		return
	}
	if _, err := s.Store.UserByUsernameAndConnectionName(r.Context(), username, connName); err == nil {
		writeErr(w, http.StatusConflict, errors.New("username already registered for this connection label"))
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := pingTargetDB(ctx, driver, req.Host, port, req.Database, sslMode, req.ReadUsername, req.ReadPassword); err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("database connection failed: "+err.Error()))
		return
	}

	// Account credentials intentionally mirror DB credentials.
	// If password is empty, leave password_hash NULL (login will fail until a password is set).
	hash := ""
	if strings.TrimSpace(req.ReadPassword) != "" {
		var err error
		hash, err = auth.HashPassword(req.ReadPassword)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}
	displayName := username

	// Persist DB credentials into SQLite users table (encrypted at rest).
	dbp := ""
	if strings.TrimSpace(req.ReadPassword) != "" {
		enc, err := s.Crypter.Encrypt(req.ReadPassword)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		dbp = enc
	}
	user, err := s.Store.CreateUser(r.Context(), username, connName, hash, displayName, req.ReadUsername, dbp)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	rp, err := s.Crypter.Encrypt(req.ReadPassword)
	if err != nil {
		_ = s.Store.DeleteUser(r.Context(), user.ID)
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	wp, err := s.Crypter.Encrypt(req.WritePassword)
	if err != nil {
		_ = s.Store.DeleteUser(r.Context(), user.ID)
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	schemas := []byte("[]")
	if req.AllowedSchemas != nil {
		schemas, _ = json.Marshal(req.AllowedSchemas)
	}
	c := &store.DbConnection{
		UserID: user.ID, Name: connName, Driver: driver, Host: req.Host, Port: port,
		Database: req.Database, SslMode: sslMode,
		ReadUsername: req.ReadUsername, ReadPassword: rp,
		WriteUsername: req.WriteUsername, WritePassword: wp,
		AllowedSchemas: string(schemas),
	}
	if err := s.Store.CreateConnection(r.Context(), c); err != nil {
		_ = s.Store.DeleteUser(r.Context(), user.ID)
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.Pools.Invalidate(c.ID)

	tok, err := s.JWT.Issue(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": tok,
		"user":  userPayload(user.ID, user.Username, user.Name),
	})
}

// userPayload includes the role the frontend expects. v1 grants every user
// the engineer role so both read and write pools are usable.
func userPayload(id int64, username, name string) map[string]any {
	return map[string]any{"id": id, "username": username, "name": name, "role": "engineer"}
}

func (s *Server) handleConnectionLabels(w http.ResponseWriter, r *http.Request) {
	labels, err := s.Store.ListConnectionNames(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if labels == nil {
		labels = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"labels": labels})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req.ConnectionName = strings.TrimSpace(req.ConnectionName)
	if req.ConnectionName == "" {
		writeErr(w, http.StatusBadRequest, errors.New("connection_name is required"))
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	user, err := s.Store.UserByUsernameAndConnectionName(r.Context(), req.Username, req.ConnectionName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusUnauthorized, errors.New("invalid credentials"))
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if strings.TrimSpace(user.PasswordHash) == "" {
		if strings.TrimSpace(req.Password) != "" {
			writeErr(w, http.StatusUnauthorized, errors.New("invalid credentials"))
			return
		}
	} else {
		if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
			writeErr(w, http.StatusUnauthorized, errors.New("invalid credentials"))
			return
		}
	}
	tok, err := s.JWT.Issue(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": tok,
		"user":  userPayload(user.ID, user.Username, user.Name),
	})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	user, err := s.Store.UserByID(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": userPayload(user.ID, user.Username, user.Name),
	})
}
