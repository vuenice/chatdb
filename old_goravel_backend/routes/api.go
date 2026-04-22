package routes

import (
	"github.com/goravel/framework/contracts/http"
	"github.com/goravel/framework/contracts/route"

	"goravel/app/facades"
	"goravel/app/http/controllers"
	"goravel/app/http/middleware"
)

func Api() {
	auth := controllers.NewAuthController()

	facades.Route().Prefix("api").Group(func(route route.Router) {
		route.Post("/register", auth.Register)
		route.Post("/login", auth.Login)

		route.Middleware(middleware.Jwt()).Group(func(r route.Router) {
			r.Get("/me", auth.Me)

			cc := controllers.NewConnectionController()
			r.Get("/connections", cc.Index)
			r.Post("/connections", cc.Store)
			r.Get("/connections/{id}", cc.Show)
			r.Put("/connections/{id}", cc.Update)
			r.Delete("/connections/{id}", cc.Destroy)

			sc := controllers.NewSchemaController()
			r.Get("/connections/{id}/databases", sc.Databases)
			r.Get("/connections/{id}/tables", sc.Tables)
			r.Get("/connections/{id}/columns", sc.Columns)
			r.Get("/connections/{id}/rows", sc.Rows)
			r.Get("/connections/{id}/schema_graph", sc.SchemaGraph)

			sqlc := controllers.NewSqlController()
			r.Post("/connections/{id}/sql/execute", sqlc.Execute)
			r.Post("/connections/{id}/sql/cancel", sqlc.Cancel)
			r.Post("/connections/{id}/sql/explain", sqlc.Explain)

			mc := controllers.NewMonitoringController()
			r.Get("/connections/{id}/monitoring/duplicate_indexes", mc.DuplicateIndexes)
			r.Get("/connections/{id}/monitoring/slow_queries", mc.PgStatStatements)

			ac := controllers.NewAIController()
			r.Post("/connections/{id}/ai/chat", ac.Chat)

			qc := controllers.NewQueryController()
			r.Get("/connections/{id}/queries", qc.Index)
			r.Post("/connections/{id}/queries", qc.Store)
			r.Post("/connections/{id}/queries/touch_run", qc.TouchRun)
			r.Get("/connections/{id}/queries/running", qc.Running)
			r.Delete("/connections/{id}/queries/{query_id}", qc.Destroy)
		})

		route.Get("/health", func(ctx http.Context) http.Response {
			return ctx.Response().Success().Json(http.Json{"ok": true})
		})
	})
}
