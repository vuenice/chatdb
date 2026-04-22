package services

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ActiveRun struct {
	RunID        string    `json:"run_id"`
	ConnectionID uint      `json:"connection_id"`
	Started      time.Time `json:"started"`
	SQLSnippet   string    `json:"sql_snippet"`
}

type runEntry struct {
	cancel       context.CancelFunc
	connectionID uint
	started      time.Time
	snippet      string
}

type RunRegistry struct {
	mu   sync.Mutex
	runs map[string]*runEntry
}

func NewRunRegistry() *RunRegistry {
	return &RunRegistry{runs: make(map[string]*runEntry)}
}

func (r *RunRegistry) Register(cancel context.CancelFunc, connectionID uint, sql string) string {
	id := uuid.NewString()
	snippet := sql
	if len(snippet) > 200 {
		snippet = snippet[:200]
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[id] = &runEntry{
		cancel:       cancel,
		connectionID: connectionID,
		started:      time.Now().UTC(),
		snippet:      snippet,
	}
	return id
}

func (r *RunRegistry) Cancel(runID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.runs[runID]; ok {
		e.cancel()
		delete(r.runs, runID)
		return true
	}
	return false
}

func (r *RunRegistry) Done(runID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.runs, runID)
}

func (r *RunRegistry) ListForConnection(connectionID uint) []ActiveRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []ActiveRun
	for id, e := range r.runs {
		if e.connectionID != connectionID {
			continue
		}
		out = append(out, ActiveRun{
			RunID:        id,
			ConnectionID: e.connectionID,
			Started:      e.started,
			SQLSnippet:   e.snippet,
		})
	}
	return out
}
