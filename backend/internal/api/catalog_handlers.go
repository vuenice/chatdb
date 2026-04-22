package api

import (
	"net/http"

	"chatdb/internal/engine"
)

func (s *Server) handleCatalogRoles(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	roles, err := engine.ListCatalogRoleNames(r.Context(), eng)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roles": roles})
}

func (s *Server) handleCatalogLoginUsers(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	users, err := engine.ListCatalogLoginRows(r.Context(), eng)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

type createCatalogUserReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (s *Server) handleCatalogCreateUser(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, true)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var req createCatalogUserReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := engine.CreateCatalogLoginUser(r.Context(), eng, req.Username, req.Password, req.Role); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
