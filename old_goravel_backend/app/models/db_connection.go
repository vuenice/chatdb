package models

import "github.com/goravel/framework/database/orm"

type DbConnection struct {
	orm.Model
	UserID uint

	Name     string
	Host     string
	Port     int
	Database string
	SslMode  string `gorm:"column:ssl_mode;default:disable"`

	ReadUsername  string `gorm:"column:read_username"`
	ReadPassword  string `gorm:"column:read_password"` // encrypted
	WriteUsername string `gorm:"column:write_username"`
	WritePassword string `gorm:"column:write_password"` // encrypted

	// AllowedSchemas JSON array of schema names; empty = all (engineers only enforced in app)
	AllowedSchemas string `gorm:"type:text"`
}

func (DbConnection) TableName() string {
	return "db_connections"
}
