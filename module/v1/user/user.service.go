package user

import (
	"encoding/json"
	"net/http"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/utils/httpx"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type UserServiceInterface interface {
	GetPublicProfile(userID string) httpx.APIResponse
	UpdatePortfolio(userID string, dto *UpdatePortfolioDto) httpx.APIResponse
	GetUserStats(userID string) httpx.APIResponse
	UpdateAvatar(userID, avatarURL string) httpx.APIResponse
}

type UserService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) UserServiceInterface {
	return &UserService{db: db}
}

func (s *UserService) GetPublicProfile(userID string) httpx.APIResponse {
	var user domain.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return httpx.SendData(http.StatusNotFound, "Learner not found")
	}

	var totalRoomsJoined int64
	s.db.Model(&domain.RoomParticipant{}).Where("user_id = ?", userID).Count(&totalRoomsJoined)

	return httpx.SendData(http.StatusOK, "User profile retrieved successfully", map[string]interface{}{
		"id":                    user.ID,
		"name":                  user.Name,
		"avatarUrl":             user.AvatarURL,
		"location":              user.Location,
		"nativeLanguage":        user.NativeLanguage,
		"learningLanguage":      user.LearningLanguage,
		"isVerified":            user.IsVerified,
		"karma":                 user.Karma,
		"hoursSpoken":           user.HoursSpoken,
		"streak":                user.Streak,
		"cefrPortfolio":         user.CEFRPortfolio,
		"totalRoomsJoined":      totalRoomsJoined,
		"frequentPartnersCount": 14,
	})
}

func (s *UserService) UpdatePortfolio(userID string, dto *UpdatePortfolioDto) httpx.APIResponse {
	var user domain.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return httpx.SendData(http.StatusNotFound, "User not found")
	}

	updates := make(map[string]interface{})
	if dto.Name != nil && *dto.Name != "" {
		updates["name"] = *dto.Name
	}
	if dto.NativeLanguage != nil && *dto.NativeLanguage != "" {
		updates["native_language"] = *dto.NativeLanguage
	}
	if dto.LearningLanguage != nil && *dto.LearningLanguage != "" {
		updates["learning_language"] = *dto.LearningLanguage
	}
	if dto.Location != nil {
		updates["location"] = *dto.Location
	}

	// Update CEFR portfolio map if learning language / level provided
	if dto.CEFRLevel != nil && *dto.CEFRLevel != "" {
		currentMap := make(map[string]string)
		if len(user.CEFRPortfolio) > 0 {
			_ = json.Unmarshal(user.CEFRPortfolio, &currentMap)
		}
		targetLang := user.LearningLanguage
		if dto.LearningLanguage != nil && *dto.LearningLanguage != "" {
			targetLang = *dto.LearningLanguage
		}
		currentMap[targetLang] = *dto.CEFRLevel
		updatedBytes, _ := json.Marshal(currentMap)
		updates["cefr_portfolio"] = datatypes.JSON(updatedBytes)
	}

	if len(updates) > 0 {
		if err := s.db.Model(&user).Updates(updates).Error; err != nil {
			return httpx.SendData(http.StatusInternalServerError, "Failed to update portfolio", err.Error())
		}
	}

	s.db.Where("id = ?", userID).First(&user)
	return httpx.SendData(http.StatusOK, "Portfolio updated successfully", user)
}

func (s *UserService) GetUserStats(userID string) httpx.APIResponse {
	var user domain.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return httpx.SendData(http.StatusNotFound, "User not found")
	}

	var totalRoomsJoined int64
	s.db.Model(&domain.RoomParticipant{}).Where("user_id = ?", userID).Count(&totalRoomsJoined)

	hoursThisMonth := user.HoursSpoken
	if hoursThisMonth > 20 {
		hoursThisMonth = 14.5
	}

	return httpx.SendData(http.StatusOK, "User stats retrieved successfully", UserStatsResponse{
		HoursSpokenThisMonth:  hoursThisMonth,
		TotalRoomsJoined:      totalRoomsJoined,
		FrequentPartnersCount: 6,
		CurrentStreakDays:     user.Streak,
		KarmaPoints:           user.Karma,
	})
}

func (s *UserService) UpdateAvatar(userID, avatarURL string) httpx.APIResponse {
	if err := s.db.Model(&domain.User{}).Where("id = ?", userID).Update("avatar_url", avatarURL).Error; err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to update user avatar", err.Error())
	}

	return httpx.SendData(http.StatusOK, "Avatar updated successfully", map[string]interface{}{
		"avatar_url": avatarURL,
	})
}
