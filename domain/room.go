package domain

import (
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Room struct {
	ID                 string            `gorm:"column:id;primaryKey" json:"id"`
	Title              string            `gorm:"column:title;not null;type:varchar(200)" json:"title"`
	Topic              string            `gorm:"column:topic;type:varchar(200)" json:"topic"`
	Language           string            `gorm:"column:language;not null;type:varchar(50)" json:"language"`
	Flag               string            `gorm:"column:flag;type:varchar(10)" json:"flag"`
	CEFRLevel          string            `gorm:"column:cefr_level;default:'ANY';type:varchar(10)" json:"cefr_level"`
	LevelLabel         string            `gorm:"column:level_label;type:varchar(50)" json:"level_label"`
	MaxSlots           int               `gorm:"column:max_slots;default:5" json:"max_slots"`
	CurrentSlots       int               `gorm:"column:current_slots;default:1" json:"current_slots"`
	TopicTag           string            `gorm:"column:topic_tag;type:varchar(100)" json:"topic_tag"`
	Tags               datatypes.JSON    `gorm:"column:tags;type:jsonb" json:"tags"`
	Status             string            `gorm:"column:status;default:'LIVE';type:varchar(20)" json:"status"` // LIVE, WAITING, ENDED
	IsBeginnerFriendly bool              `gorm:"column:is_beginner_friendly;default:true" json:"is_beginner_friendly"`
	HasFreeSeats       bool              `gorm:"column:has_free_seats;default:true" json:"has_free_seats"`
	HasNativeSpeaker   bool              `gorm:"column:has_native_speaker;default:false" json:"has_native_speaker"`
	IsLive             bool              `gorm:"column:is_live;default:true" json:"is_live"`
	HostID             string            `gorm:"column:host_id;not null;index" json:"host_id"`
	RoomKey            *string           `gorm:"column:room_key;type:varchar(100)" json:"-"`
	StartedAt          time.Time         `gorm:"column:started_at;autoCreateTime" json:"started_at"`
	CreatedAt          time.Time         `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time         `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	Host               User              `gorm:"foreignKey:HostID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"host"`
	Participants       []RoomParticipant `gorm:"foreignKey:RoomID;constraint:OnDelete:CASCADE" json:"participants,omitempty"`
}

func (Room) TableName() string {
	return "rooms"
}

func (r *Room) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == "" {
		r.ID = cuid.New()
	}
	if len(r.Tags) == 0 {
		r.Tags = datatypes.JSON([]byte(`[]`))
	}
	return nil
}
