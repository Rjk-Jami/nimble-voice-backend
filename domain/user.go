package domain

import (
	"time"

	"github.com/lucsky/cuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type User struct {
	ID               string         `gorm:"column:id;primaryKey" json:"id"`
	Name             string         `gorm:"column:name;not null;type:varchar(100)" json:"name"`
	Email            *string        `gorm:"column:email;uniqueIndex;type:varchar(255)" json:"email,omitempty"`
	Password         *string        `gorm:"column:password" json:"-"`
	AvatarURL        string         `gorm:"column:avatar_url;type:varchar(500)" json:"avatar_url"`
	Location         string         `gorm:"column:location;type:varchar(100)" json:"location"`
	NativeLanguage   string         `gorm:"column:native_language;default:'English';type:varchar(50)" json:"native_language"`
	LearningLanguage string         `gorm:"column:learning_language;default:'Spanish';type:varchar(50)" json:"learning_language"`
	IsVerified       bool           `gorm:"column:is_verified;default:false" json:"is_verified"`
	IsGuest          bool           `gorm:"column:is_guest;default:false" json:"is_guest"`
	Role             string         `gorm:"column:role;default:'user';type:varchar(20)" json:"role"`
	Karma            int            `gorm:"column:karma;default:0" json:"karma"`
	HoursSpoken      float64        `gorm:"column:hours_spoken;default:0" json:"hours_spoken"`
	Streak           int            `gorm:"column:streak;default:1" json:"streak"`
	CEFRPortfolio    datatypes.JSON `gorm:"column:cefr_portfolio;type:jsonb" json:"cefr_portfolio"`
	CreatedAt        time.Time      `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt        time.Time      `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}

// BeforeCreate hook generates CUID and hashes password if provided
func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == "" {
		u.ID = cuid.New()
	}
	if u.Password != nil && *u.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*u.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		hashed := string(hash)
		u.Password = &hashed
	}
	if len(u.CEFRPortfolio) == 0 {
		u.CEFRPortfolio = datatypes.JSON([]byte(`{"English":"NATIVE","Spanish":"A1"}`))
	}
	return nil
}

// ComparePassword verifies raw password against the stored bcrypt hash
func (u *User) ComparePassword(raw string) bool {
	if u.Password == nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(*u.Password), []byte(raw)) == nil
}
