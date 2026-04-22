package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"chatdb/internal/engine"
)

type executeReq struct {
	SQL      string `json:"sql"`
	Pool     string `json:"pool"`
	MaxRows  int    `json:"max_rows"`
	Database string `json:"database"`
	Role     string `json:"role"`
}

type cancelReq struct {
	RunID string `json:"run_id"`
}

// runRegistry tracks in-flight queries so a `cancel` request can stop them.
type runRegistry struct {
	mu   sync.Mutex
	runs map[string]context.CancelFunc
}

var runs = &runRegistry{runs: map[string]context.CancelFunc{}}

func (r *runRegistry) register(cancel context.CancelFunc) string {
	id := uuid.NewString()
	r.mu.Lock()
	r.runs[id] = cancel
	r.mu.Unlock()
	return id
}

func (r *runRegistry) done(id string) {
	r.mu.Lock()
	delete(r.runs, id)
	r.mu.Unlock()
}

func (r *runRegistry) cancel(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cancel, ok := r.runs[id]
	if !ok {
		return false
	}
	cancel()
	delete(r.runs, id)
	return true
}

func (s *Server) handleSQLExecute(w http.ResponseWriter, r *http.Request) {
	var req executeReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	wantWrite := strings.EqualFold(strings.TrimSpace(req.Pool), "write")
	eng, _, err := s.resolveEngineWithDB(r, wantWrite, strings.TrimSpace(req.Database))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	runID := runs.register(cancel)
	defer runs.done(runID)

	res, err := engine.ExecuteWithOptionalRole(ctx, eng, strings.TrimSpace(req.Role), req.SQL, req.MaxRows)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "run_id": runID})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": runID,
		"pool":   poolLabel(wantWrite),
		"result": res,
	})
}

func poolLabel(write bool) string {
	if write {
		return "write"
	}
	return "read"
}

func (s *Server) handleSQLCancel(w http.ResponseWriter, r *http.Request) {
	var req cancelReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ok := runs.cancel(req.RunID)
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": ok})
}
