package controllers

import (
	"encoding/json"
	"strings"

	"github.com/goravel/framework/contracts/http"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cast"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
)

func currentUser(ctx http.Context) (*models.User, error) {
	var user models.User
	if err := facades.Auth(ctx).User(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func loadOwnedConnection(ctx http.Context, connID uint) (*models.DbConnection, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	var c models.DbConnection
	if err := facades.Orm().Query().Where("id", connID).Where("user_id", user.ID).First(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func allowedSchemaList(c *models.DbConnection) []string {
	var schemas []string
	if strings.TrimSpace(c.AllowedSchemas) != "" {
		_ = json.Unmarshal([]byte(c.AllowedSchemas), &schemas)
	}
	return schemas
}

// resolvePool picks read vs write Postgres credentials based on app role and wantWrite.
func resolvePool(ctx http.Context, c *models.DbConnection, wantWrite bool) (*pgxpool.Pool, string, error) {
	user, err := currentUser(ctx)
	if err != nil {
		return nil, "", err
	}
	switch user.Role {
	case models.RoleViewer, models.RoleAnalyst:
		wantWrite = false
	case models.RoleEngineer:
	default:
		wantWrite = false
	}

	useReadPool := !wantWrite
	dbUser := c.ReadUsername
	pass, err := facades.Crypt().DecryptString(c.ReadPassword)
	if err != nil {
		return nil, "", err
	}
	label := "read"
	if wantWrite {
		dbUser = c.WriteUsername
		pass, err = facades.Crypt().DecryptString(c.WritePassword)
		if err != nil {
			return nil, "", err
		}
		label = "write"
	}

	pool, err := services.Pools.GetOrCreate(ctx, c.ID, useReadPool, c.Host, c.Port, c.Database, dbUser, pass, c.SslMode)
	if err != nil {
		return nil, "", err
	}
	return pool, label, nil
}

func connectionID(ctx http.Context) uint {
	if v := ctx.Request().Route("id"); v != "" {
		return cast.ToUint(v)
	}
	return cast.ToUint(ctx.Request().Input("id"))
}

func queryID(ctx http.Context) uint {
	if v := ctx.Request().Route("query_id"); v != "" {
		return cast.ToUint(v)
	}
	return cast.ToUint(ctx.Request().Input("query_id"))
}
