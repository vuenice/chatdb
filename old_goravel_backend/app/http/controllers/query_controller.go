package controllers

import (
	"time"

	"github.com/goravel/framework/contracts/http"
	"github.com/spf13/cast"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
)

type QueryController struct{}

func NewQueryController() *QueryController {
	return &QueryController{}
}

func (r *QueryController) Index(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	connID := connectionID(ctx)
	var list []models.SavedQuery
	var listErr error
	saved := ctx.Request().Input("saved", "") == "1" || ctx.Request().Input("saved", "") == "true"
	if saved {
		listErr = facades.Orm().Query().Where("user_id", user.ID).Where("connection_id", connID).
			Where("is_saved", true).Order("updated_at desc").Limit(200).Get(&list)
	} else {
		listErr = facades.Orm().Query().Where("user_id", user.ID).Where("connection_id", connID).
			Order("last_run_at desc").Order("updated_at desc").Limit(200).Get(&list)
	}
	if listErr != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": listErr.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"queries": list})
}

type saveQueryRequest struct {
	ID    uint   `json:"id"`
	Title string `json:"title"`
	SQL   string `json:"sql"`
	Saved bool   `json:"is_saved"`
}

func (r *QueryController) Store(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	connID := connectionID(ctx)
	if _, err := loadOwnedConnection(ctx, connID); err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	var req saveQueryRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	if req.ID == 0 {
		sq := models.SavedQuery{
			UserID:       user.ID,
			ConnectionID: connID,
			Title:        req.Title,
			Sql:          req.SQL,
			IsSaved:      req.Saved,
		}
		if err := facades.Orm().Query().Create(&sq); err != nil {
			return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
		}
		return ctx.Response().Success().Json(http.Json{"query": sq})
	}
	var sq models.SavedQuery
	if err := facades.Orm().Query().Where("id", req.ID).Where("user_id", user.ID).Where("connection_id", connID).First(&sq); err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	sq.Title = req.Title
	sq.Sql = req.SQL
	sq.IsSaved = req.Saved
	if err := facades.Orm().Query().Save(&sq); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"query": sq})
}

type touchRunRequest struct {
	SQL string `json:"sql"`
}

func (r *QueryController) TouchRun(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	connID := connectionID(ctx)
	if _, err := loadOwnedConnection(ctx, connID); err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	var req touchRunRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	now := time.Now()
	sq := models.SavedQuery{
		UserID:       user.ID,
		ConnectionID: connID,
		Title:        "",
		Sql:          req.SQL,
		IsSaved:      false,
		LastRunAt:    &now,
	}
	if err := facades.Orm().Query().Create(&sq); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"query": sq})
}

func (r *QueryController) Running(ctx http.Context) http.Response {
	if _, err := currentUser(ctx); err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	if _, err := loadOwnedConnection(ctx, connectionID(ctx)); err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	runs := services.Runs.ListForConnection(connectionID(ctx))
	return ctx.Response().Success().Json(http.Json{"runs": runs})
}

func (r *QueryController) Destroy(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	connID := connectionID(ctx)
	qid := queryID(ctx)
	var sq models.SavedQuery
	if err := facades.Orm().Query().Where("id", qid).Where("user_id", user.ID).Where("connection_id", connID).First(&sq); err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	res, err := facades.Orm().Query().Where("id", qid).Where("user_id", user.ID).Where("connection_id", connID).Delete(&models.SavedQuery{})
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
	}
	if res.RowsAffected == 0 {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	return ctx.Response().Success().Json(http.Json{"deleted": true})
}
