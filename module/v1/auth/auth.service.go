package auth

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/utils/httpx"
	"nimble-voice-backend/utils/jwt"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type AuthServiceInterface interface {
	CreateGuest(dto *GuestAuthDto) httpx.APIResponse
	GetSession(userID string) httpx.APIResponse
	GetProfile(userID string) httpx.APIResponse
	Register(dto *RegisterDto) httpx.APIResponse
	Login(dto *LoginDto) httpx.APIResponse
}

type AuthService struct {
	db *gorm.DB
}

func NewAuthService(db *gorm.DB) AuthServiceInterface {
	return &AuthService{db: db}
}

func (s *AuthService) CreateGuest(dto *GuestAuthDto) httpx.APIResponse {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	randomNum := r.Intn(9000) + 1000

	name := dto.Name
	if name == "" {
		name = fmt.Sprintf("Guest #%d", randomNum)
	}

	nativeLang := dto.NativeLanguage
	if nativeLang == "" {
		nativeLang = "English"
	}

	learningLang := dto.LearningLanguage
	if learningLang == "" {
		learningLang = "Spanish"
	}

	portfolioMap := map[string]string{
		nativeLang:   "NATIVE",
		learningLang: "A1",
	}
	portfolioBytes, _ := json.Marshal(portfolioMap)

	guestUser := domain.User{
		Name:             name,
		AvatarURL:        fmt.Sprintf("https://api.dicebear.com/7.x/bottts/svg?seed=%s_%d", name, randomNum),
		Location:         "Global",
		NativeLanguage:   nativeLang,
		LearningLanguage: learningLang,
		IsVerified:       false,
		IsGuest:          true,
		Role:             "user",
		Karma:            0,
		HoursSpoken:      0,
		Streak:           1,
		CEFRPortfolio:    datatypes.JSON(portfolioBytes),
	}

	if err := s.db.Create(&guestUser).Error; err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to create guest user", err.Error())
	}

	token, err := jwt.GenerateToken(guestUser.ID, "", guestUser.Role, "", "")
	if err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to generate auth token")
	}

	return httpx.SendData(http.StatusCreated, "Guest session created successfully", AuthResponse{
		Token: token,
		User:  guestUser,
	})
}

func (s *AuthService) GetSession(userID string) httpx.APIResponse {
	if userID == "" {
		return httpx.SendData(http.StatusOK, "No active session", SessionResponse{
			Authenticated: false,
			User:          nil,
		})
	}

	var user domain.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return httpx.SendData(http.StatusOK, "Session expired or invalid", SessionResponse{
			Authenticated: false,
			User:          nil,
		})
	}

	return httpx.SendData(http.StatusOK, "Active session retrieved", SessionResponse{
		Authenticated: true,
		User:          user,
	})
}

func (s *AuthService) GetProfile(userID string) httpx.APIResponse {
	var user domain.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return httpx.SendData(http.StatusNotFound, "User profile not found")
	}

	var totalRoomsJoined int64
	s.db.Model(&domain.RoomParticipant{}).Where("user_id = ?", userID).Count(&totalRoomsJoined)

	return httpx.SendData(http.StatusOK, "Profile retrieved successfully", map[string]interface{}{
		"id":                   user.ID,
		"name":                 user.Name,
		"email":                user.Email,
		"avatarUrl":            user.AvatarURL,
		"location":             user.Location,
		"nativeLanguage":       user.NativeLanguage,
		"learningLanguage":     user.LearningLanguage,
		"isVerified":           user.IsVerified,
		"isGuest":              user.IsGuest,
		"role":                 user.Role,
		"karma":                user.Karma,
		"hoursSpoken":          user.HoursSpoken,
		"streak":               user.Streak,
		"cefrPortfolio":        user.CEFRPortfolio,
		"totalRoomsJoined":     totalRoomsJoined,
		"frequentPartnersCount": 6,
	})
}

func (s *AuthService) Register(dto *RegisterDto) httpx.APIResponse {
	var count int64
	s.db.Model(&domain.User{}).Where("email = ?", dto.Email).Count(&count)
	if count > 0 {
		return httpx.SendData(http.StatusBadRequest, "An account with this email already exists")
	}

	nativeLang := dto.NativeLanguage
	if nativeLang == "" {
		nativeLang = "English"
	}
	learningLang := dto.LearningLanguage
	if learningLang == "" {
		learningLang = "Spanish"
	}

	portfolioMap := map[string]string{
		nativeLang:   "NATIVE",
		learningLang: "A1",
	}
	portfolioBytes, _ := json.Marshal(portfolioMap)

	email := dto.Email
	password := dto.Password
	user := domain.User{
		Name:             dto.Name,
		Email:            &email,
		Password:         &password,
		AvatarURL:        fmt.Sprintf("https://api.dicebear.com/7.x/bottts/svg?seed=%s", dto.Name),
		NativeLanguage:   nativeLang,
		LearningLanguage: learningLang,
		IsVerified:       true,
		IsGuest:          false,
		Role:             "user",
		Karma:            10,
		HoursSpoken:      0,
		Streak:           1,
		CEFRPortfolio:    datatypes.JSON(portfolioBytes),
	}

	if err := s.db.Create(&user).Error; err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to register user", err.Error())
	}

	token, err := jwt.GenerateToken(user.ID, *user.Email, user.Role, "", "")
	if err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to generate auth token")
	}

	return httpx.SendData(http.StatusCreated, "User registered successfully", AuthResponse{
		Token: token,
		User:  user,
	})
}

func (s *AuthService) Login(dto *LoginDto) httpx.APIResponse {
	var user domain.User
	if err := s.db.Where("email = ?", dto.Email).First(&user).Error; err != nil {
		return httpx.SendData(http.StatusUnauthorized, "Invalid email or password")
	}

	if !user.ComparePassword(dto.Password) {
		return httpx.SendData(http.StatusUnauthorized, "Invalid email or password")
	}

	emailStr := ""
	if user.Email != nil {
		emailStr = *user.Email
	}
	token, err := jwt.GenerateToken(user.ID, emailStr, user.Role, "", "")
	if err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to generate token")
	}

	return httpx.SendData(http.StatusOK, "Login successful", AuthResponse{
		Token: token,
		User:  user,
	})
}
