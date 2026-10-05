package domain

import (
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

type BaseModel struct {
	ID        string    `gorm:"column:id;primaryKey" json:"id"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (b *BaseModel) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == "" {
		b.ID = cuid.New()
	}
	return nil
}
