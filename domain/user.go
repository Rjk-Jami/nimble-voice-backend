package domain

import (
	"time"

	"github.com/lucsky/cuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID        string    `gorm:"column:id;primaryKey" json:"id"`
	Name      string    `gorm:"column:name;not null" json:"name"`
	Email     string    `gorm:"column:email;uniqueIndex;not null" json:"email"`
	Password  string    `gorm:"column:password;not null" json:"-"`
	AvatarURL string    `gorm:"column:avatar_url" json:"avatar_url"`
	Role      string    `gorm:"column:role;default:user" json:"role"`
	StoreID   string    `gorm:"column:store_id" json:"store_id"`
	BranchID  string    `gorm:"column:branch_id" json:"branch_id"`
	Status    string    `gorm:"column:status;default:active" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}

// BeforeCreate hook generates CUID and hashes password if not already hashed
func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == "" {
		u.ID = cuid.New()
	}
	if u.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Password = string(hash)
	}
	return nil
}

// ComparePassword verifies raw password against the stored bcrypt hash
func (u *User) ComparePassword(raw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(raw)) == nil
}
