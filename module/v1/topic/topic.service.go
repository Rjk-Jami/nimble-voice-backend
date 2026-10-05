package topic

import (
	"net/http"
	"strings"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/utils/httpx"

	"gorm.io/gorm"
)

type TopicServiceInterface interface {
	GetPrompts(query *TopicQuery) httpx.APIResponse
}

type TopicService struct {
	db *gorm.DB
}

func NewTopicService(db *gorm.DB) TopicServiceInterface {
	return &TopicService{db: db}
}

func (s *TopicService) GetPrompts(query *TopicQuery) httpx.APIResponse {
	dbQuery := s.db.Model(&domain.TopicPrompt{})

	if query.Category != "" {
		dbQuery = dbQuery.Where("LOWER(category) = ?", strings.ToLower(query.Category))
	}
	if query.Level != "" {
		dbQuery = dbQuery.Where("LOWER(level) = ? OR level = 'Any'", strings.ToLower(query.Level))
	}

	var prompts []domain.TopicPrompt
	if err := dbQuery.Find(&prompts).Error; err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to retrieve prompts", err.Error())
	}

	// Group by category
	categoryMap := make(map[string]*CategoryDeck)
	categoryOrder := []string{}

	for _, p := range prompts {
		if _, exists := categoryMap[p.Category]; !exists {
			categoryOrder = append(categoryOrder, p.Category)
			categoryMap[p.Category] = &CategoryDeck{
				Category: p.Category,
				Icon:     p.Icon,
				Prompts:  []PromptItem{},
			}
		}
		categoryMap[p.Category].Prompts = append(categoryMap[p.Category].Prompts, PromptItem{
			Title: p.Title,
			Level: p.Level,
			Tag:   p.Tag,
		})
	}

	result := []CategoryDeck{}
	for _, cat := range categoryOrder {
		result = append(result, *categoryMap[cat])
	}

	return httpx.SendData(http.StatusOK, "Topic prompts retrieved successfully", result)
}
