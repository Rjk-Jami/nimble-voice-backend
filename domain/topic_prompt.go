package domain

import (
	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

type TopicPrompt struct {
	ID       string `gorm:"column:id;primaryKey" json:"id"`
	Category string `gorm:"column:category;not null;index;type:varchar(100)" json:"category"`
	Icon     string `gorm:"column:icon;type:varchar(50)" json:"icon"`
	Title    string `gorm:"column:title;not null;type:text" json:"title"`
	Level    string `gorm:"column:level;default:'Any';type:varchar(20)" json:"level"` // A1-A2, B1-B2, Any
	Tag      string `gorm:"column:tag;type:varchar(50)" json:"tag"`
}

func (TopicPrompt) TableName() string {
	return "topic_prompts"
}

func (tp *TopicPrompt) BeforeCreate(tx *gorm.DB) (err error) {
	if tp.ID == "" {
		tp.ID = cuid.New()
	}
	return nil
}
