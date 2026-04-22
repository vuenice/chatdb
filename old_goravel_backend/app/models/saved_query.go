package models

import (
	"time"

	"github.com/goravel/framework/database/orm"
)

type SavedQuery struct {
	orm.Model
	UserID       uint
	ConnectionID uint

	Title    string
	Sql      string `gorm:"type:text"`
	IsSaved  bool
	LastRunAt *time.Time
}

func (SavedQuery) TableName() string {
	return "saved_queries"
}
