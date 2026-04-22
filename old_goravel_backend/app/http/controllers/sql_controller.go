package controllers

import (
	"context"
	"strings"
	"time"

	"github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
)

type SqlController struct{}

func NewSqlController() *SqlController {
	return &SqlController{}
}

type executeRequest struct {
	SQL     string `json:"sql"`
	Pool    string `json:"pool"` // read | write
	MaxRows int    `json:"max_rows"`
}

type cancelRequest struct {
	RunID string `json:"run_id"`
}

func (r *SqlController) audit(user *models.User, connID uint, action, pool, snippet string) {
	uid := user.ID
	cid := connID
	_ = facades.Orm().Query().Create(&models.AuditLog{
		UserID:       &uid,
		ConnectionID: &cid,
		Action:       action,
		Pool:         pool,
		SqlSnippet:   truncateStr(snippet, 2000),
	})
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (r *SqlController) Execute(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	var req executeRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	wantWrite := strings.EqualFold(strings.TrimSpace(req.Pool), "write")
	pool, label, err := resolvePool(ctx, c, wantWrite)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}

	execCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	runID := services.Runs.Register(cancel, c.ID, req.SQL)
	defer services.Runs.Done(runID)

	res, err := services.Execute(execCtx, pool, req.SQL, req.MaxRows)
	if err != nil {
		r.audit(user, c.ID, "sql_error", label, req.SQL)
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error(), "run_id": runID})
	}
	r.audit(user, c.ID, "sql_ok", label, req.SQL)
	return ctx.Response().Success().Json(http.Json{
		"run_id": runID,
		"pool":   label,
		"result": res,
	})
}

func (r *SqlController) Cancel(ctx http.Context) http.Response {
	var req cancelRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	if req.RunID == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": "run_id required"})
	}
	ok := services.Runs.Cancel(req.RunID)
	return ctx.Response().Success().Json(http.Json{"cancelled": ok})
}

type explainRequest struct {
	SQL  string `json:"sql"`
	Pool string `json:"pool"`
}

func (r *SqlController) Explain(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	var req explainRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	wantWrite := strings.EqualFold(strings.TrimSpace(req.Pool), "write")
	pool, label, err := resolvePool(ctx, c, wantWrite)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	execCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	raw, err := services.ExplainJSON(execCtx, pool, req.SQL)
	if err != nil {
		r.audit(user, c.ID, "explain_error", label, req.SQL)
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	r.audit(user, c.ID, "explain_ok", label, req.SQL)
	return ctx.Response().Success().Json(http.Json{"pool": label, "plan": raw})
}
