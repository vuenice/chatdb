package controllers

import (
	"strings"

	"github.com/goravel/framework/contracts/http"

	"goravel/app/models"
	"goravel/app/services"
)

type AIController struct{}

func NewAIController() *AIController {
	return &AIController{}
}

type aiChatRequest struct {
	Message string `json:"message"`
}

func (r *AIController) Chat(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	pool, _, err := resolvePool(ctx, c, false)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	var req aiChatRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	if strings.TrimSpace(req.Message) == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": "message required"})
	}
	schemaJSON, err := services.SchemaGraphJSON(ctx, pool)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	ai := services.NewAIClientFromEnv()
	sqlText, err := ai.GenerateSQL(ctx, string(schemaJSON), req.Message)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}

	allowed := allowedSchemaList(c)
	if user.Role == models.RoleViewer || user.Role == models.RoleAnalyst {
		if err := services.ValidateGeneratedSQL(sqlText, allowed); err != nil {
			return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error(), "sql": sqlText})
		}
	} else if len(allowed) > 0 {
		if err := services.ValidateGeneratedSQL(sqlText, allowed); err != nil {
			return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error(), "sql": sqlText})
		}
	}

	return ctx.Response().Success().Json(http.Json{
		"sql": sqlText,
	})
}
