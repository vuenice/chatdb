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
	"chatdb/internal/store"
)

func toSavedQueryJSON(q *store.SavedQuery) map[string]any {
	out := map[string]any{
		"id":         q.ID,
		"title":      q.Title,
		"sql":        q.Sql,
		"is_saved":   q.IsSaved,
		"updated_at": q.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		"created_at": q.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if q.LastRunAt != nil {
		s := q.LastRunAt.UTC().Format("2006-01-02T15:04:05.000Z")
		out["last_run_at"] = s
	}
	return out
}

type saveQueryRequest struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	SQL     string `json:"sql"`
	IsSaved *bool  `json:"is_saved"`
}

type touchRunRequest struct {
	SQL string `json:"sql"`
}

func (s *Server) handleQueriesIndex(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	connID, err := parseConnID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.Store.GetConnection(r.Context(), uid, connID); err != nil {
		writeErr(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	savedOnly := r.URL.Query().Get("saved") == "1" || strings.EqualFold(r.URL.Query().Get("saved"), "true")
	var list []store.SavedQuery
	if savedOnly {
		list, err = s.Store.ListSavedQueriesForConnection(r.Context(), uid, connID)
	} else {
		list, err = s.Store.ListAllQueriesForConnection(r.Context(), uid, connID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	queries := make([]map[string]any, 0, len(list))
	for i := range list {
		queries = append(queries, toSavedQueryJSON(&list[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"queries": queries})
}

func (s *Server) handleQueriesStore(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	connID, err := parseConnID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.Store.GetConnection(r.Context(), uid, connID); err != nil {
		writeErr(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	var req saveQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req.SQL = strings.TrimSpace(req.SQL)
	if req.SQL == "" {
		writeErr(w, http.StatusBadRequest, errors.New("sql is required"))
		return
	}
	saved := true
	if req.IsSaved != nil {
		saved = *req.IsSaved
	}
	if req.ID == 0 {
		q := &store.SavedQuery{
			UserID:       uid,
			ConnectionID: connID,
			Title:        req.Title,
			Sql:          req.SQL,
			IsSaved:      saved,
		}
		if err := s.Store.CreateSavedQuery(r.Context(), q); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"query": toSavedQueryJSON(q)})
		return
	}
	if err := s.Store.UpdateSavedQuery(r.Context(), uid, connID, req.ID, req.Title, req.SQL, saved); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, errors.New("not found"))
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	updated, err := s.Store.GetSavedQuery(r.Context(), uid, connID, req.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": toSavedQueryJSON(updated)})
}

func (s *Server) handleQueriesDestroy(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	connID, err := parseConnID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	qid, err := strconv.ParseInt(chi.URLParam(r, "qid"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.Store.DeleteSavedQuery(r.Context(), uid, connID, qid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, errors.New("not found"))
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": true})
}

func (s *Server) handleQueriesRecent(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	connID, err := parseConnID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.Store.GetConnection(r.Context(), uid, connID); err != nil {
		writeErr(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	only := r.URL.Query().Get("only_update") == "1" || strings.EqualFold(r.URL.Query().Get("only_update"), "true")
	list, err := s.Store.ListRecentRunsForConnection(r.Context(), uid, connID, only, 200)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	queries := make([]map[string]any, 0, len(list))
	for i := range list {
		queries = append(queries, toSavedQueryJSON(&list[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"queries": queries})
}

func (s *Server) handleQueriesTouchRun(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	connID, err := parseConnID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.Store.GetConnection(r.Context(), uid, connID); err != nil {
		writeErr(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	var req touchRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	req.SQL = strings.TrimSpace(req.SQL)
	if req.SQL == "" {
		writeErr(w, http.StatusBadRequest, errors.New("sql is required"))
		return
	}
	q, err := s.Store.TouchRun(r.Context(), uid, connID, req.SQL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": toSavedQueryJSON(q)})
}
