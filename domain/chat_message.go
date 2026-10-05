package domain

import (
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ChatMessage struct {
	ID            string         `gorm:"column:id;primaryKey" json:"id"`
	RoomID        string         `gorm:"column:room_id;not null;index" json:"roomId"`
	SenderID      string         `gorm:"column:sender_id;not null;index" json:"senderId"`
	SenderName    string         `gorm:"column:sender_name;type:varchar(100)" json:"senderName"`
	SenderAvatar  string         `gorm:"column:sender_avatar;type:varchar(500)" json:"senderAvatar"`
	Content       string         `gorm:"column:content;not null;type:text" json:"content"`
	Type          string         `gorm:"column:type;default:'TEXT';type:varchar(20)" json:"type"` // TEXT, SYSTEM, IDIOM, REACTION
	IsHighlighted bool           `gorm:"column:is_highlighted;default:false" json:"isHighlighted"`
	Reactions     datatypes.JSON `gorm:"column:reactions;type:jsonb" json:"reactions"` // ["👍", "❤️", "💡"]
	CreatedAt     time.Time      `gorm:"column:created_at;autoCreateTime" json:"timestamp"`

	Sender        User           `gorm:"foreignKey:SenderID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"sender,omitempty"`
}

func (ChatMessage) TableName() string {
	return "chat_messages"
}

func (cm *ChatMessage) BeforeCreate(tx *gorm.DB) (err error) {
	if cm.ID == "" {
		cm.ID = cuid.New()
	}
	if len(cm.Reactions) == 0 {
		cm.Reactions = datatypes.JSON([]byte(`[]`))
	}
	return nil
}
