package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"chatdb/internal/auth"
	"chatdb/internal/config"
	"chatdb/internal/store"
)

type connectionDTO struct {
	ID             int64    `json:"id"`
	Name           string   `json:"name"`
	Driver         string   `json:"driver"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	Database       string   `json:"database"`
	SslMode        string   `json:"ssl_mode"`
	ReadUsername   string   `json:"read_username"`
	WriteUsername  string   `json:"write_username"`
	AllowedSchemas []string `json:"allowed_schemas"`
}

type connectionWriteReq struct {
	Name           string   `json:"name"`
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

func toDTO(c *store.DbConnection) connectionDTO {
	var schemas []string
	if strings.TrimSpace(c.AllowedSchemas) != "" {
		_ = json.Unmarshal([]byte(c.AllowedSchemas), &schemas)
	}
	return connectionDTO{
		ID:             c.ID,
		Name:           c.Name,
		Driver:         c.Driver,
		Host:           c.Host,
		Port:           c.Port,
		Database:       c.Database,
		SslMode:        c.SslMode,
		ReadUsername:   c.ReadUsername,
		WriteUsername:  c.WriteUsername,
		AllowedSchemas: schemas,
	}
}

func parseConnID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func (s *Server) handleConnectionsIndex(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	list, err := s.Store.ListConnections(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]connectionDTO, 0, len(list))
	for i := range list {
		out = append(out, toDTO(&list[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

func (s *Server) handleConnectionStore(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	var req connectionWriteReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" || req.Host == "" || req.Database == "" || req.ReadUsername == "" {
		writeErr(w, http.StatusBadRequest, errors.New("name, host, database, read_username are required"))
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
	if req.Port == 0 {
		if driver == string(config.DriverPostgres) {
			req.Port = 5432
		} else {
			req.Port = 3306
		}
	}
	if req.SslMode == "" {
		req.SslMode = "disable"
	}
	n, err := s.Store.ConnectionCount(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if n > 0 {
		writeErr(w, http.StatusConflict, errors.New("a connection already exists for this account"))
		return
	}
	rp, err := s.Crypter.Encrypt(req.ReadPassword)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	wp, err := s.Crypter.Encrypt(req.WritePassword)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	schemas, _ := json.Marshal(req.AllowedSchemas)
	c := &store.DbConnection{
		UserID: uid, Name: req.Name, Driver: driver, Host: req.Host, Port: req.Port,
		Database: req.Database, SslMode: req.SslMode,
		ReadUsername: req.ReadUsername, ReadPassword: rp,
		WriteUsername: req.WriteUsername, WritePassword: wp,
		AllowedSchemas: string(schemas),
	}
	if err := s.Store.CreateConnection(r.Context(), c); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.Pools.Invalidate(c.ID)
	writeJSON(w, http.StatusOK, map[string]any{"connection": toDTO(c)})
}

func (s *Server) handleConnectionShow(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	id, err := parseConnID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	c, err := s.Store.GetConnection(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connection": toDTO(c)})
}

func (s *Server) handleConnectionUpdate(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	id, err := parseConnID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	c, err := s.Store.GetConnection(r.Context(), uid, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	var req connectionWriteReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.Name != "" {
		c.Name = req.Name
	}
	if req.Driver != "" {
		c.Driver = strings.ToLower(req.Driver)
	}
	if req.Host != "" {
		c.Host = req.Host
	}
	if req.Port != 0 {
		c.Port = req.Port
	}
	if req.Database != "" {
		c.Database = req.Database
	}
	if req.SslMode != "" {
		c.SslMode = req.SslMode
	}
	if req.ReadUsername != "" {
		c.ReadUsername = req.ReadUsername
	}
	if req.WriteUsername != "" {
		c.WriteUsername = req.WriteUsername
	}
	if req.ReadPassword != "" {
		enc, err := s.Crypter.Encrypt(req.ReadPassword)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		c.ReadPassword = enc
	}
	if req.WritePassword != "" {
		enc, err := s.Crypter.Encrypt(req.WritePassword)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		c.WritePassword = enc
	}
	if req.AllowedSchemas != nil {
		schemas, _ := json.Marshal(req.AllowedSchemas)
		c.AllowedSchemas = string(schemas)
	}
	if err := s.Store.UpdateConnection(r.Context(), c); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.Pools.Invalidate(c.ID)
	writeJSON(w, http.StatusOK, map[string]any{"connection": toDTO(c)})
}

func (s *Server) handleConnectionDestroy(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	id, err := parseConnID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.DeleteConnection(r.Context(), uid, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, errors.New("not found"))
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.Pools.Invalidate(id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
