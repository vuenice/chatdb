package models

import "github.com/goravel/framework/database/orm"

type AuditLog struct {
	orm.Model
	UserID       *uint
	ConnectionID *uint
	Action       string
	Pool         string // read | write
	SqlSnippet   string `gorm:"type:text"`
	Meta         string `gorm:"type:text"`
}

func (AuditLog) TableName() string {
	return "audit_logs"
}
