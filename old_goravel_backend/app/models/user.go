package models

import "github.com/goravel/framework/database/orm"

const (
	RoleViewer   = "viewer"
	RoleAnalyst  = "analyst"
	RoleEngineer = "engineer"
)

type User struct {
	orm.Model
	Name     string
	Email    string
	Password string `json:"-"`
	Role     string `gorm:"default:viewer"`
}

func (User) TableName() string {
	return "users"
}
