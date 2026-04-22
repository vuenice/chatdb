package controllers

import (
	"strings"

	"github.com/goravel/framework/contracts/http"
	"github.com/spf13/cast"

	"goravel/app/facades"
	"goravel/app/models"
)

type AuthController struct{}

func NewAuthController() *AuthController {
	return &AuthController{}
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r *AuthController) Register(ctx http.Context) http.Response {
	var req registerRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": "email and password are required"})
	}
	role := req.Role
	if role == "" {
		role = models.RoleViewer
	}
	if role != models.RoleViewer && role != models.RoleAnalyst && role != models.RoleEngineer {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": "invalid role"})
	}
	hashed, err := facades.Hash().Make(req.Password)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
	}
	user := models.User{
		Name:     req.Name,
		Email:    req.Email,
		Password: hashed,
		Role:     role,
	}
	if err := facades.Orm().Query().Create(&user); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	token, err := facades.Auth(ctx).LoginUsingID(user.ID)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
	}
	return ctx.Response().Header("Authorization", "Bearer "+token).Success().Json(http.Json{
		"token": token,
		"user":  user,
	})
}

func (r *AuthController) Login(ctx http.Context) http.Response {
	var req loginRequest
	if err := ctx.Request().Bind(&req); err != nil {
		return ctx.Response().Json(http.StatusBadRequest, http.Json{"error": err.Error()})
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	var user models.User
	if err := facades.Orm().Query().Where("email", req.Email).First(&user); err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": "invalid credentials"})
	}
	if !facades.Hash().Check(req.Password, user.Password) {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": "invalid credentials"})
	}
	token, err := facades.Auth(ctx).LoginUsingID(user.ID)
	if err != nil {
		return ctx.Response().Json(http.StatusInternalServerError, http.Json{"error": err.Error()})
	}
	return ctx.Response().Header("Authorization", "Bearer "+token).Success().Json(http.Json{
		"token": token,
		"user":  user,
	})
}

func (r *AuthController) Me(ctx http.Context) http.Response {
	var user models.User
	if err := facades.Auth(ctx).User(&user); err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	id, err := facades.Auth(ctx).ID()
	if err != nil {
		return ctx.Response().Json(http.StatusUnauthorized, http.Json{"error": err.Error()})
	}
	return ctx.Response().Success().Json(http.Json{
		"id":   cast.ToUint(id),
		"user": user,
	})
}
