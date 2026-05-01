package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"chatdb/internal/engine"
)

// BulkOperationRequest represents the frontend request payload.
type BulkOperationRequest struct {
	Operation string   `json:"operation"` // "delete", "truncate", "drop", "analyze", "optimize", "repair", "check"
	Tables    []string `json:"tables"`    // array of "schema.table" strings
}

// BulkOperationResponse represents the response to the frontend.
type BulkOperationResponse struct {
	Ok      bool   `json:"ok"`
	Message string `json:"message"`
}

// handleBulkOperation processes bulk table operations.
func (s *Server) handleBulkOperation(w http.ResponseWriter, r *http.Request) {
	// Resolve the database connection and get engine
	eng, _, err := s.resolveEngineWithDB(r, true, r.URL.Query().Get("database"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	// Decode request
	var req BulkOperationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	if len(req.Tables) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no tables specified"))
		return
	}

	// Execute the bulk operation
	result, err := executeBulkOperation(r.Context(), eng, req.Operation, req.Tables)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// executeBulkOperation runs the bulk operation on the given tables.
func executeBulkOperation(ctx context.Context, eng engine.Engine, operation string, tables []string) (*BulkOperationResponse, error) {
	driver := string(eng.Driver())
	var results []string
	var lastErr error

	for _, table := range tables {
		// Parse schema.table format
		parts := strings.SplitN(table, ".", 2)
		if len(parts) != 2 {
			results = append(results, fmt.Sprintf("%s: invalid format", table))
			lastErr = fmt.Errorf("invalid table format: %s", table)
			continue
		}
		schema := parts[0]
		tblName := parts[1]

		// Build the SQL statement based on operation
		sqlStr, err := buildBulkSQL(operation, schema, tblName, driver)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: %v", table, err))
			lastErr = err
			continue
		}

		// Execute the SQL using the engine's Execute method
		_, err = eng.Execute(ctx, sqlStr, 0)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: %v", table, err))
			lastErr = err
			continue
		}

		results = append(results, fmt.Sprintf("%s: OK", table))
	}

	if lastErr != nil {
		return &BulkOperationResponse{Ok: false, Message: strings.Join(results, "\n")}, nil
	}

	return &BulkOperationResponse{Ok: true, Message: strings.Join(results, "\n")}, nil
}

// buildBulkSQL builds the SQL statement for the given bulk operation.
func buildBulkSQL(operation, schema, table, driver string) (string, error) {
	// Quote identifiers based on driver
	schemaQ := quoteIdent(driver, schema)
	tableQ := quoteIdent(driver, table)

	switch operation {
	case "drop":
		// DROP TABLE requires MySQL to include cascade for foreign keys
		if driver == "mysql" {
			return fmt.Sprintf("DROP TABLE IF EXISTS %s.%s CASCADE", schemaQ, tableQ), nil
		}
		return fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", schemaQ, tableQ), nil

	case "truncate":
		if driver == "mysql" {
			return fmt.Sprintf("TRUNCATE TABLE %s.%s", schemaQ, tableQ), nil
		}
		return fmt.Sprintf("TRUNCATE TABLE %s.%s RESTART IDENTITY", schemaQ, tableQ), nil

	case "analyze":
		if driver == "mysql" {
			return fmt.Sprintf("ANALYZE TABLE %s.%s", schemaQ, tableQ), nil
		}
		return fmt.Sprintf("ANALYZE %s.%s", schemaQ, tableQ), nil

	case "check":
		if driver == "mysql" {
			return fmt.Sprintf("CHECK TABLE %s.%s", schemaQ, tableQ), nil
		}
		// PostgreSQL doesn't have CHECK TABLE, use VACUUM VERIFY
		return fmt.Sprintf("VACUUM VERBOSE %s.%s", schemaQ, tableQ), nil

	case "optimize":
		if driver == "mysql" {
			return fmt.Sprintf("OPTIMIZE TABLE %s.%s", schemaQ, tableQ), nil
		}
		// PostgreSQL uses VACUUM and REINDEX
		return fmt.Sprintf("VACUUM %s.%s", schemaQ, tableQ), nil

	case "repair":
		if driver == "mysql" {
			return fmt.Sprintf("REPAIR TABLE %s.%s", schemaQ, tableQ), nil
		}
		// PostgreSQL: use REINDEX
		return fmt.Sprintf("REINDEX TABLE %s.%s", schemaQ, tableQ), nil

	default:
		return "", fmt.Errorf("unsupported operation: %s", operation)
	}
}

// quoteIdent wraps an identifier in quotes based on the database driver.
func quoteIdent(driver, ident string) string {
	switch driver {
	case "mysql":
		return "`" + ident + "`"
	case "postgres", "postgresql":
		return "\"" + ident + "\""
	default:
		return ident
	}
}