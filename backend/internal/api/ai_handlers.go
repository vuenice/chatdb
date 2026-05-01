package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"chatdb/internal/ai"
	"chatdb/internal/auth"
)

type chatRequest struct {
	Schema   string `json:"schema"`
	Question string `json:"question"`
}

type chatResponse struct {
	SQL   string `json:"sql,omitempty"`
	Error string `json:"error,omitempty"`
}

// handleAIChat handles the /connections/{id}/ai/chat endpoint
func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserID(r)
	connID, err := parseConnID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	// Verify connection exists (will 404 if not found)
	if _, err := s.Store.GetConnection(r.Context(), uid, connID); err != nil {
		writeErr(w, http.StatusNotFound, errors.New("connection not found"))
		return
	}

	// Parse request
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, errors.New("invalid request body"))
		return
	}

	if req.Question == "" {
		writeErr(w, http.StatusBadRequest, errors.New("question is required"))
		return
	}

	// If no schema provided in request, we need to fetch from database
	schemaJSON := req.Schema
	if schemaJSON == "" {
		writeErr(w, http.StatusBadRequest, errors.New("schema is required - click on a table first to load its schema"))
		return
	}

	// Get LLM provider
	provider := ai.NewProvider()
	if provider == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("no AI provider configured. Set OPENAI_API_KEY environment variable."))
		return
	}

	// Generate SQL with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	sql, err := provider.GenerateSQL(ctx, schemaJSON, req.Question)
	if err != nil {
		writeJSON(w, http.StatusOK, chatResponse{
			Error: err.Error(),
		})
		return
	}

	// Validate the generated SQL (empty allowedSchemas for now - could be enhanced)
	if err := ai.ValidateGeneratedSQL(sql, nil); err != nil {
		writeJSON(w, http.StatusOK, chatResponse{
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, chatResponse{
		SQL: sql,
	})
}