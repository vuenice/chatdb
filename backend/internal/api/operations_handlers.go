package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
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
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("multipart parse: %w", err))
		return
	}

	format := strings.TrimSpace(r.FormValue("format"))

	eng, conn, err := s.resolveEngine(r, true)
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

	dbName, err := effectivePhysicalDatabase(conn, strings.TrimSpace(r.URL.Query().Get("database")))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	switch conn.Driver {
	case "mysql":
		if format != "" && (strings.EqualFold(format, importFormatPsql) || strings.EqualFold(format, importFormatPgdump)) {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("psql/pg_dump archive import applies to PostgreSQL only"))
			return
		}
		content, err := io.ReadAll(file)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		_, err = eng.Execute(r.Context(), string(content), 10000)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, fmt.Errorf("failed to import SQL: %w", err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	case "postgres":
		imFmt := strings.ToLower(format)
		if imFmt == "" {
			imFmt = importFormatPsql
		}
		if imFmt != importFormatPsql && imFmt != importFormatPgdump {
			writeErr(w, http.StatusBadRequest, errWrongImportFormat)
			return
		}
		tmpPath, err := saveUploadToTemp(file, "chatdb-import-*")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		defer os.Remove(tmpPath)

		user, pass, err := s.decryptedConnAuth(conn, true)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}

		ctx, cancel := exportContext(r.Context())
		defer cancel()

		switch imFmt {
		case importFormatPsql:
			psqlPath, err := lookPostgresTool("psql")
			if err != nil {
				writeErr(w, http.StatusServiceUnavailable, err)
				return
			}
			if err := runPsqlFile(ctx, psqlPath, conn, dbName, user, pass, tmpPath); err != nil {
				writeErr(w, http.StatusInternalServerError, fmt.Errorf("psql import failed: %v", err))
				return
			}
		case importFormatPgdump:
			restorePath, err := lookPostgresTool("pg_restore")
			if err != nil {
				writeErr(w, http.StatusServiceUnavailable, err)
				return
			}
			if err := runPgRestoreFile(ctx, restorePath, conn, dbName, user, pass, tmpPath); err != nil {
				writeErr(w, http.StatusInternalServerError, fmt.Errorf("pg_restore failed: %v", err))
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("unsupported driver: %s", conn.Driver))
	}
}

// handleExportSQL exports the database as SQL
func (s *Server) handleExportSQL(w http.ResponseWriter, r *http.Request) {
	eng, conn, err := s.resolveEngine(r, false)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	if conn.Driver != "postgres" {
		dbLabel, err := effectivePhysicalDatabase(conn, strings.TrimSpace(r.URL.Query().Get("database")))
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
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
			output = fmt.Sprintf("-- Database: %s\n-- Exported from ChatDB", dbLabel)
		}

		fn := sanitizedExportBase(dbLabel) + ".sql"
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fn))
		_, _ = w.Write([]byte(output))
		return
	}

	exFmt := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if exFmt == "" {
		exFmt = exportFormatPlain
	}
	if exFmt != exportFormatPlain && exFmt != exportFormatArchive {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("export format must be plain (SQL) or archive (pg_dump custom)"))
		return
	}

	dbName, err := effectivePhysicalDatabase(conn, strings.TrimSpace(r.URL.Query().Get("database")))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	user, pass, err := s.decryptedConnAuth(conn, false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	pgDumpPath, err := lookPostgresTool("pg_dump")
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}

	ctx, cancel := exportContext(r.Context())
	defer cancel()

	tmpFile, err := os.CreateTemp("", "chatdb-export-*")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	customFormat := exFmt == exportFormatArchive
	if err := runPgDump(ctx, pgDumpPath, conn, dbName, user, pass, customFormat, tmpPath); err != nil {
		writeErr(w, http.StatusInternalServerError, fmt.Errorf("pg_dump failed: %v", err))
		return
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	base := sanitizedExportBase(dbName)
	var ext, ctype string
	if customFormat {
		ext = ".dump"
		ctype = "application/octet-stream"
	} else {
		ext = ".sql"
		ctype = "text/plain; charset=utf-8"
	}
	dlName := base + ext

	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", dlName))
	http.ServeContent(w, r, dlName, fi.ModTime(), f)
}
