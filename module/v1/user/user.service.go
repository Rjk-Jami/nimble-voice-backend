package user

import (
	"nimble-voice-backend/domain"
	"nimble-voice-backend/utils/httpx"
	"nimble-voice-backend/utils/jwt"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UserServiceInterface interface {
	Register(dto *RegisterUserDto) httpx.APIResponse
	Login(dto *LoginUserDto) httpx.APIResponse
	GetProfile(userID string) httpx.APIResponse
	UpdateAvatar(userID, avatarURL string) httpx.APIResponse
}

type UserService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) UserServiceInterface {
	return &UserService{db: db}
}

func (s *UserService) Register(dto *RegisterUserDto) httpx.APIResponse {
	var count int64
	s.db.Model(&domain.User{}).Where("email = ?", dto.Email).Count(&count)
	if count > 0 {
		return httpx.SendData(400, "User already exists with this email")
	}

	role := dto.Role
	if role == "" {
		role = "user"
	}

	user := domain.User{
		Name:     dto.Name,
		Email:    dto.Email,
		Password: dto.Password,
		Role:     role,
		StoreID:  dto.StoreID,
		BranchID: dto.BranchID,
	}

	if err := s.db.Create(&user).Error; err != nil {
		return httpx.SendData(500, "Failed to create user", err.Error())
	}
	return httpx.SendData(201, "Registration successful", user)
}

func (s *UserService) Login(dto *LoginUserDto) httpx.APIResponse {
	var user domain.User
	if err := s.db.Where("email = ?", dto.Email).First(&user).Error; err != nil {
		return httpx.SendData(401, "Invalid email or password")
	}

	if !user.ComparePassword(dto.Password) {
		return httpx.SendData(401, "Invalid email or password")
	}

	token, err := jwt.GenerateToken(user.ID, user.Email, user.Role, user.StoreID, user.BranchID)
	if err != nil {
		return httpx.SendData(500, "Failed to generate token")
	}

	return httpx.SendData(200, "Login successful", LoginResponseDto{Token: token, User: user})
}

func (s *UserService) GetProfile(userID string) httpx.APIResponse {
	var user domain.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return httpx.SendData(404, "User not found")
	}
	return httpx.SendData(200, "Profile fetched", user)
}

func (s *UserService) UpdateAvatar(userID, avatarURL string) httpx.APIResponse {
	if err := s.db.Model(&domain.User{}).Where("id = ?", userID).Update("avatar_url", avatarURL).Error; err != nil {
		return httpx.SendData(500, "Failed to update user avatar", err.Error())
	}

	return httpx.SendData(200, "Avatar updated successfully", gin.H{
		"avatar_url": avatarURL,
	})
}
