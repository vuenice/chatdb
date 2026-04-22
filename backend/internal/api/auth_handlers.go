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
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
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
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, errors.New("email and password required"))
		return
	}
	if req.Host == "" || req.Database == "" || req.ReadUsername == "" || req.ReadPassword == "" {
		writeErr(w, http.StatusBadRequest, errors.New("host, database, read_username, and read_password are required"))
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
		connName = strings.TrimSpace(req.Host)
		if connName == "" {
			connName = "default"
		}
	}

	if existing, err := s.Store.UserByEmail(r.Context(), req.Email); err == nil && existing != nil {
		writeErr(w, http.StatusConflict, errors.New("email already registered"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := pingTargetDB(ctx, driver, req.Host, port, req.Database, sslMode, req.ReadUsername, req.ReadPassword); err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("database connection failed: "+err.Error()))
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	displayName := strings.TrimSpace(req.Name)
	if displayName == "" {
		displayName = strings.Split(req.Email, "@")[0]
	}
	user, err := s.Store.CreateUser(r.Context(), req.Email, hash, displayName)
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
		"user":  userPayload(user.ID, user.Email, user.Name),
	})
}

// userPayload includes the role the frontend expects. v1 grants every user
// the engineer role so both read and write pools are usable.
func userPayload(id int64, email, name string) map[string]any {
	return map[string]any{"id": id, "email": email, "name": name, "role": "engineer"}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	user, err := s.Store.UserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusUnauthorized, errors.New("invalid credentials"))
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		writeErr(w, http.StatusUnauthorized, errors.New("invalid credentials"))
		return
	}
	tok, err := s.JWT.Issue(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": tok,
		"user":  userPayload(user.ID, user.Email, user.Name),
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
		"user": userPayload(user.ID, user.Email, user.Name),
	})
}
