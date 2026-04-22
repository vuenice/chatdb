package controllers

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/spf13/cast"

	"goravel/app/services"
)

type SchemaController struct{}

func NewSchemaController() *SchemaController {
	return &SchemaController{}
}

func (r *SchemaController) Databases(ctx http.Context) http.Response {
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	pool, _, err := resolvePool(ctx, c, false)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	dbs, err := services.ListDatabases(ctx, pool)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"databases": dbs})
}

func (r *SchemaController) Tables(ctx http.Context) http.Response {
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	pool, _, err := resolvePool(ctx, c, false)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	schema := ctx.Request().Input("schema", "")
	tables, err := services.ListTables(ctx, pool, schema)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"tables": tables})
}

func (r *SchemaController) Columns(ctx http.Context) http.Response {
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	pool, _, err := resolvePool(ctx, c, false)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	schema := ctx.Request().Input("schema", "public")
	table := ctx.Request().Input("table", "")
	if table == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": "table required"})
	}
	cols, err := services.TableColumns(ctx, pool, schema, table)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"columns": cols})
}

func (r *SchemaController) Rows(ctx http.Context) http.Response {
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	pool, _, err := resolvePool(ctx, c, false)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	schema := ctx.Request().Input("schema", "public")
	table := ctx.Request().Input("table", "")
	if table == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": "table required"})
	}
	limit := cast.ToInt(ctx.Request().Input("limit", "100"))
	offset := cast.ToInt(ctx.Request().Input("offset", "0"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	// Identifier quoting: allow simple names only
	if !safeIdent(schema) || !safeIdent(table) {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": "invalid schema or table"})
	}
	q := `SELECT * FROM "` + schema + `"."` + table + `" LIMIT $1 OFFSET $2`
	res, err := services.Execute(ctx, pool, q, limit)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"result": res})
}

func safeIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func (r *SchemaController) SchemaGraph(ctx http.Context) http.Response {
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	pool, _, err := resolvePool(ctx, c, false)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	raw, err := services.SchemaGraphJSON(ctx, pool)
	if err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{"schema_json": raw})
}
