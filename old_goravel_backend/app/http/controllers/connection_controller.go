package controllers

import (
	"encoding/json"
	"strings"

	"github.com/goravel/framework/contracts/http"

	"goravel/app/facades"
	"goravel/app/models"
	"goravel/app/services"
)

type ConnectionController struct{}

func NewConnectionController() *ConnectionController {
	return &ConnectionController{}
}

type connectionDTO struct {
	ID             uint     `json:"id"`
	Name           string   `json:"name"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	Database       string   `json:"database"`
	SslMode        string   `json:"ssl_mode"`
	ReadUsername   string   `json:"read_username"`
	WriteUsername  string   `json:"write_username"`
	AllowedSchemas []string `json:"allowed_schemas"`
}

type connectionCreateRequest struct {
	Name           string   `json:"name"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	Database       string   `json:"database"`
	SslMode        string   `json:"ssl_mode"`
	ReadUsername   string   `json:"read_username"`
	ReadPassword   string   `json:"read_password"`
	WriteUsername  string   `json:"write_username"`
	WritePassword  string   `json:"write_password"`
	AllowedSchemas []string `json:"allowed_schemas"`
}

func (r *ConnectionController) toDTO(c models.DbConnection) connectionDTO {
	var schemas []string
	if strings.TrimSpace(c.AllowedSchemas) != "" {
		_ = json.Unmarshal([]byte(c.AllowedSchemas), &schemas)
	}
	return connectionDTO{
		ID:             c.ID,
		Name:           c.Name,
		Host:           c.Host,
		Port:           c.Port,
		Database:       c.Database,
		SslMode:        c.SslMode,
		ReadUsername:   c.ReadUsername,
		WriteUsername:  c.WriteUsername,
		AllowedSchemas: schemas,
	}
}

func (r *ConnectionController) Index(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	var list []models.DbConnection
	if err := facades.Orm().Query().Where("user_id", user.ID).Order("id desc").Get(&list); err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
	}
	out := make([]connectionDTO, 0, len(list))
	for _, c := range list {
		out = append(out, r.toDTO(c))
	}
	return ctx.Response().Success().Json(http.Json{"connections": out})
}

func (r *ConnectionController) Store(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	var req connectionCreateRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	if req.Name == "" || req.Host == "" || req.Database == "" || req.ReadUsername == "" || req.WriteUsername == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": "missing required fields"})
	}
	if req.Port == 0 {
		req.Port = 5432
	}
	if req.SslMode == "" {
		req.SslMode = "disable"
	}
	rp, err := facades.Crypt().EncryptString(req.ReadPassword)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
	}
	wp, err := facades.Crypt().EncryptString(req.WritePassword)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
	}
	schemaJSON, _ := json.Marshal(req.AllowedSchemas)
	conn := models.DbConnection{
		UserID:         user.ID,
		Name:           req.Name,
		Host:           req.Host,
		Port:           req.Port,
		Database:       req.Database,
		SslMode:        req.SslMode,
		ReadUsername:   req.ReadUsername,
		ReadPassword:   rp,
		WriteUsername:  req.WriteUsername,
		WritePassword:  wp,
		AllowedSchemas: string(schemaJSON),
	}
	if err := facades.Orm().Query().Create(&conn); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	services.Pools.Invalidate(conn.ID)
	return ctx.Response().Success().Json(http.Json{"connection": r.toDTO(conn)})
}

func (r *ConnectionController) Show(ctx http.Context) http.Response {
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	return ctx.Response().Success().Json(http.Json{"connection": r.toDTO(*c)})
}

func (r *ConnectionController) Update(ctx http.Context) http.Response {
	c, err := loadOwnedConnection(ctx, connectionID(ctx))
	if err != nil {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	var req connectionCreateRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	if req.Name != "" {
		c.Name = req.Name
	}
	if req.Host != "" {
		c.Host = req.Host
	}
	if req.Port != 0 {
		c.Port = req.Port
	}
	if req.Database != "" {
		c.Database = req.Database
	}
	if req.SslMode != "" {
		c.SslMode = req.SslMode
	}
	if req.ReadUsername != "" {
		c.ReadUsername = req.ReadUsername
	}
	if req.ReadPassword != "" {
		rp, err := facades.Crypt().EncryptString(req.ReadPassword)
		if err != nil {
			return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
		}
		c.ReadPassword = rp
	}
	if req.WriteUsername != "" {
		c.WriteUsername = req.WriteUsername
	}
	if req.WritePassword != "" {
		wp, err := facades.Crypt().EncryptString(req.WritePassword)
		if err != nil {
			return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
		}
		c.WritePassword = wp
	}
	if req.AllowedSchemas != nil {
		b, _ := json.Marshal(req.AllowedSchemas)
		c.AllowedSchemas = string(b)
	}
	if err := facades.Orm().Query().Save(c); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	services.Pools.Invalidate(c.ID)
	return ctx.Response().Success().Json(http.Json{"connection": r.toDTO(*c)})
}

func (r *ConnectionController) Destroy(ctx http.Context) http.Response {
	user, err := currentUser(ctx)
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	id := connectionID(ctx)
	res, err := facades.Orm().Query().Where("id", id).Where("user_id", user.ID).Delete(&models.DbConnection{})
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
	}
	if res.RowsAffected == 0 {
		return ctx.Response().Json(http.StatusNotFound, http.Json{"error": "not found"})
	}
	services.Pools.Invalidate(id)
	return ctx.Response().Success().Json(http.Json{"deleted": true})
}
