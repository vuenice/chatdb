package api

import (
	"fmt"
	"io"
	"net/http"
)

// handleDeleteDatabase drops the entire database
func (s *Server) handleDeleteDatabase(w http.ResponseWriter, r *http.Request) {
	eng, conn, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	db := conn.Database
	if db == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no database specified"))
		return
	}

	// Different syntax for different drivers
	var query string
	switch conn.Driver {
	case "postgres":
		// PostgreSQL: terminate all connections first, then drop
		_, err = eng.Execute(r.Context(), fmt.Sprintf("SELECT pg_terminate_backend(pg_stat_activity.pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()", db), 100)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("failed to terminate connections: %w", err))
			return
		}
		query = fmt.Sprintf("DROP DATABASE IF EXISTS \"%s\"", db)
	case "mysql":
		query = fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", db)
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unsupported driver: %s", conn.Driver))
		return
	}

	_, err = eng.Execute(r.Context(), query, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("failed to delete database: %w", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "database": db})
}

// handleRenameDatabase renames a database
func (s *Server) handleRenameDatabase(w http.ResponseWriter, r *http.Request) {
	eng, conn, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	var req struct {
		NewName string `json:"new_name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if req.NewName == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("new_name is required"))
		return
	}

	oldName := conn.Database
	if oldName == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no database specified"))
		return
	}

	// Different syntax for different drivers
	var query string
	switch conn.Driver {
	case "postgres":
		query = fmt.Sprintf("ALTER DATABASE \"%s\" RENAME TO \"%s\"", oldName, req.NewName)
	case "mysql":
		query = fmt.Sprintf("ALTER DATABASE `%s` RENAME TO `%s`", oldName, req.NewName)
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unsupported driver: %s", conn.Driver))
		return
	}

	_, err = eng.Execute(r.Context(), query, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("failed to rename database: %w", err))
		return
	}

writeJSON(w, http.StatusOK, map[string]any{"ok": true, "old_name": oldName, "new_name": req.NewName})
}

// handleTruncateDatabase truncates all tables in the database
func (s *Server) handleTruncateDatabase(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	// Get all tables
	tables, err := eng.ListTables(r.Context(), "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	// Truncate each table
	var truncated int
	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM `%s`", table.Name)
		_, err := eng.Execute(r.Context(), query, 1000)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("failed to truncate %s: %w", table.Name, err))
			return
		}
		truncated++
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "truncated": truncated})
}

// handleImportSQL imports SQL from a file upload
func (s *Server) handleImportSQL(w http.ResponseWriter, r *http.Request) {
	eng, _, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no file uploaded"))
		return
	}
	defer file.Close()

	// Read the file content
	content, err := io.ReadAll(file)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	// Execute the SQL content
	_, err = eng.Execute(r.Context(), string(content), 10000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("failed to import SQL: %w", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleExportSQL exports the database as SQL
func (s *Server) handleExportSQL(w http.ResponseWriter, r *http.Request) {
	eng, conn, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	// Export as text format
	var output string
	tables, err := eng.ListTables(r.Context(), "")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	for _, table := range tables {
		output += fmt.Sprintf("-- Table: %s\n", table.Name)
		output += "-- Data exported from ChatDB\n\n"
	}

	if output == "" {
		output = fmt.Sprintf("-- Database: %s\n-- Exported from ChatDB", conn.Database)
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.sql", conn.Database))
	_, _ = w.Write([]byte(output))
}