package controllers

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/spf13/cast"

	"goravel/app/services"
)

type MonitoringController struct{}

func NewMonitoringController() *MonitoringController {
	return &MonitoringController{}
}

func (r *MonitoringController) DuplicateIndexes(ctx http.Context) http.Response {
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	pool, _, err := resolvePool(ctx, c, false)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	raw, err := services.DuplicateIndexes(ctx, pool)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"duplicates": raw})
}

func (r *MonitoringController) PgStatStatements(ctx http.Context) http.Response {
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	pool, _, err := resolvePool(ctx, c, false)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	ok, err := services.HasPgStatStatements(ctx, pool)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	if !ok {
		return ctx.Response().Success().Json(http.Json{"enabled": false, "slow_queries": nil})
	}
	limit := cast.ToInt(ctx.Request().Input("limit", "20"))
	raw, err := services.SlowQueries(ctx, pool, limit)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"enabled": true, "slow_queries": raw})
}
