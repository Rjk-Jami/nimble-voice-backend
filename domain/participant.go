package domain

import (
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

type RoomParticipant struct {
	ID         string     `gorm:"column:id;primaryKey" json:"id"`
	RoomID     string     `gorm:"column:room_id;not null;index" json:"room_id"`
	UserID     string     `gorm:"column:user_id;not null;index" json:"user_id"`
	Role       string     `gorm:"column:role;default:'listener';type:varchar(20)" json:"role"` // host, speaker, listener
	IsHost     bool       `gorm:"column:is_host;default:false" json:"is_host"`
	IsSpeaking bool       `gorm:"column:is_speaking;default:false" json:"is_speaking"`
	IsMuted    bool       `gorm:"column:is_muted;default:false" json:"is_muted"`
	IsDeafened bool       `gorm:"column:is_deafened;default:false" json:"is_deafened"`
	HandRaised bool       `gorm:"column:hand_raised;default:false" json:"hand_raised"`
	JoinedAt   time.Time  `gorm:"column:joined_at;autoCreateTime" json:"joined_at"`
	LeftAt     *time.Time `gorm:"column:left_at" json:"left_at,omitempty"`

	User       User       `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" json:"user"`
}

func (RoomParticipant) TableName() string {
	return "room_participants"
}

func (rp *RoomParticipant) BeforeCreate(tx *gorm.DB) (err error) {
	if rp.ID == "" {
		rp.ID = cuid.New()
	}
	return nil
}
