package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"chatdb/internal/engine"
)

func (s *Server) handleListDatabases(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	dbs, err := eng.ListDatabases(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"databases": dbs})
}

func (s *Server) handleListTables(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	schema := r.URL.Query().Get("schema")
	tables, err := eng.ListTables(r.Context(), schema)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
}

func (s *Server) handleListColumns(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")
	if table == "" {
		writeErr(w, http.StatusBadRequest, errors.New("table is required"))
		return
	}
	cols, err := eng.ListColumns(r.Context(), schema, table)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": cols})
}

func (s *Server) handleListIndexes(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")
	if table == "" {
		writeErr(w, http.StatusBadRequest, errors.New("table is required"))
		return
	}
	idx, err := eng.ListIndexes(r.Context(), schema, table)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"indexes": idx})
}

func (s *Server) handlePreviewRows(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	schema := r.URL.Query().Get("schema")
	table := r.URL.Query().Get("table")
	if table == "" {
		writeErr(w, http.StatusBadRequest, errors.New("table is required"))
		return
	}
	limit := intParam(r, "limit", 100, 500)
	offset := intParam(r, "offset", 0, 0)
	if offset < 0 {
		offset = 0
	}
	res, err := eng.PreviewRows(r.Context(), schema, table, limit, offset)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res})
}

type updateTableRowReq struct {
	Schema   string         `json:"schema"`
	Table    string         `json:"table"`
	Database string         `json:"database"`
	Role     string         `json:"role"`
	Original map[string]any `json:"original"`
	Values   map[string]any `json:"values"`
}

func (s *Server) handleUpdateTableRow(w http.ResponseWriter, r *http.Request) {
	var req updateTableRowReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Schema) == "" || strings.TrimSpace(req.Table) == "" {
		writeErr(w, http.StatusBadRequest, errors.New("schema and table are required"))
		return
	}
	eng, _, err := s.resolveEngineWithDB(r, true, strings.TrimSpace(req.Database))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	res, err := engine.UpdateTableRow(ctx, eng, strings.TrimSpace(req.Role), req.Schema, req.Table, req.Original, req.Values)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": res})
}
