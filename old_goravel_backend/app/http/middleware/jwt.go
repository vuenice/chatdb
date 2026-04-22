package middleware

import (
	"errors"
	"strings"

	"github.com/goravel/framework/auth"
	"github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
)

func Jwt() http.Middleware {
	return func(ctx http.Context) {
		guard := facades.Config().GetString("auth.defaults.guard")
		if ctx.Request().Header("Guard") != "" {
			guard = ctx.Request().Header("Guard")
		}

		token := strings.TrimSpace(ctx.Request().Header("Authorization", ""))
		if token == "" {
			_ = ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": "Authorization header required"})
			return
		}

		if _, err := facades.Auth(ctx).Guard(guard).Parse(token); err != nil {
			if errors.Is(err, auth.ErrorTokenExpired) {
				refreshed, err := facades.Auth(ctx).Guard(guard).Refresh()
				if err != nil {
					_ = ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": "token expired"})
					return
				}
				token = "Bearer " + refreshed
				if _, err := facades.Auth(ctx).Guard(guard).Parse(token); err != nil {
					_ = ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": "invalid token"})
					return
				}
			} else {
				_ = ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": "invalid token"})
				return
			}
		}

		ctx.Response().Header("Authorization", token)
		ctx.Request().Next()
	}
}
