package api

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"chatdb/internal/auth"
	"chatdb/internal/config"
	"chatdb/internal/engine"
	"chatdb/internal/security"
	"chatdb/internal/store"
)

// Server holds all wired dependencies for HTTP handlers.
type Server struct {
	Cfg         *config.Config
	Store       *store.Store
	Crypter     *security.Crypter
	JWT         *auth.Issuer
	Pools       *engine.Manager
	Static fs.FS // built SPA, may be nil during dev
}

// requireAuth enforces JWT on all protected routes.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return s.JWT.Middleware(next)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.CountHumanUsers(r.Context())
	hasUsers := true
	if err == nil {
		hasUsers = n > 0
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "has_users": hasUsers})
}

// Router returns the configured chi router with API + (optional) SPA mounted.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	r.Route("/api", func(r chi.Router) {
		r.Post("/register", s.handleRegister)
		r.Post("/login", s.handleLogin)
		r.Get("/health", s.handleHealth)
		r.Get("/connection-labels", s.handleConnectionLabels)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/me", s.handleMe)

			r.Get("/connections", s.handleConnectionsIndex)
			r.Post("/connections", s.handleConnectionStore)
			r.Get("/connections/{id}", s.handleConnectionShow)
			r.Put("/connections/{id}", s.handleConnectionUpdate)
			r.Delete("/connections/{id}", s.handleConnectionDestroy)

			r.Get("/connections/{id}/databases", s.handleListDatabases)
			r.Get("/connections/{id}/catalog/roles", s.handleCatalogRoles)
			r.Get("/connections/{id}/catalog/login_users", s.handleCatalogLoginUsers)
			r.Post("/connections/{id}/catalog/users", s.handleCatalogCreateUser)
			r.Get("/connections/{id}/tables", s.handleListTables)
			r.Get("/connections/{id}/columns", s.handleListColumns)
			r.Get("/connections/{id}/indexes", s.handleListIndexes)
			r.Get("/connections/{id}/rows", s.handlePreviewRows)
			r.Post("/connections/{id}/rows/update", s.handleUpdateTableRow)

			r.Post("/connections/{id}/sql/execute", s.handleSQLExecute)
			r.Post("/connections/{id}/sql/cancel", s.handleSQLCancel)

			// Deferred endpoints — return empty payloads so the UI doesn't choke.
			r.Get("/connections/{id}/schema_graph", stub(map[string]any{"nodes": []any{}, "edges": []any{}}))
			r.Get("/connections/{id}/queries", stub(map[string]any{"queries": []any{}}))
			r.Post("/connections/{id}/queries", stub(map[string]any{"saved": false}))
			r.Get("/connections/{id}/queries/running", stub(map[string]any{"runs": []any{}}))
			r.Post("/connections/{id}/queries/touch_run", stub(map[string]any{"ok": true}))
			r.Delete("/connections/{id}/queries/{qid}", stub(map[string]any{"ok": true}))
			r.Get("/connections/{id}/monitoring/duplicate_indexes", stub(map[string]any{"unavailable": true}))
			r.Get("/connections/{id}/monitoring/slow_queries", stub(map[string]any{"unavailable": true}))
			r.Post("/connections/{id}/ai/chat", stub(map[string]any{"sql": "", "error": "AI disabled in this build"}))
			r.Post("/connections/{id}/sql/explain", stub(map[string]any{"plan": nil, "error": "EXPLAIN disabled in this build"}))
		})
	})

	if s.Static != nil {
		s.mountSPA(r)
	}

	return r
}

func (s *Server) mountSPA(r chi.Router) {
	fileServer := http.FileServer(http.FS(s.Static))
	indexBytes, _ := fs.ReadFile(s.Static, "index.html")

	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			http.NotFound(w, req)
			return
		}
		// Serve real files (assets, favicon, etc.) when present; fall back to
		// index.html so the Vue router can handle client-side routes.
		if req.URL.Path != "/" {
			if _, err := fs.Stat(s.Static, strings.TrimPrefix(req.URL.Path, "/")); err == nil {
				fileServer.ServeHTTP(w, req)
				return
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexBytes)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func stub(payload map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, payload)
	}
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(dst)
}
