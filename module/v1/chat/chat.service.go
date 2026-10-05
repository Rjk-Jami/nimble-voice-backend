package chat

import (
	"encoding/json"
	"net/http"
	"time"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/server"
	"nimble-voice-backend/utils/httpx"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ChatServiceInterface interface {
	GetMessages(roomID string, query *ChatQuery) httpx.APIResponse
	SendMessage(roomID, userID string, dto *SendMessageDto) httpx.APIResponse
}

type ChatService struct {
	db           *gorm.DB
	socketServer *server.SocketServer
}

func NewChatService(db *gorm.DB, sockServer ...*server.SocketServer) ChatServiceInterface {
	var srv *server.SocketServer
	if len(sockServer) > 0 {
		srv = sockServer[0]
	}
	return &ChatService{
		db:           db,
		socketServer: srv,
	}
}

func (s *ChatService) GetMessages(roomID string, query *ChatQuery) httpx.APIResponse {
	limit := query.Limit
	if limit < 1 || limit > 100 {
		limit = 50
	}

	dbQuery := s.db.Model(&domain.ChatMessage{}).Where("room_id = ?", roomID)

	if query.Before != "" {
		var beforeMsg domain.ChatMessage
		if err := s.db.Where("id = ?", query.Before).First(&beforeMsg).Error; err == nil {
			dbQuery = dbQuery.Where("created_at < ?", beforeMsg.CreatedAt)
		}
	}

	var messages []domain.ChatMessage
	err := dbQuery.Order("created_at ASC").Limit(limit).Find(&messages).Error
	if err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to retrieve messages", err.Error())
	}

	return httpx.SendData(http.StatusOK, "Messages retrieved successfully", ChatMessagesResponse{
		Messages: messages,
	})
}

func (s *ChatService) SendMessage(roomID, userID string, dto *SendMessageDto) httpx.APIResponse {
	// Fetch sender user info
	var user domain.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return httpx.SendData(http.StatusUnauthorized, "User not found")
	}

	msgType := dto.Type
	if msgType == "" {
		msgType = "TEXT"
	}

	emptyReactions, _ := json.Marshal([]string{})
	message := domain.ChatMessage{
		RoomID:        roomID,
		SenderID:      userID,
		SenderName:    user.Name,
		SenderAvatar:  user.AvatarURL,
		Content:       dto.Content,
		Type:          msgType,
		IsHighlighted: msgType == "IDIOM",
		Reactions:     datatypes.JSON(emptyReactions),
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.db.Create(&message).Error; err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to send message", err.Error())
	}

	// Trigger real-time broadcast to room channel via Socket.IO
	if s.socketServer != nil {
		s.socketServer.BroadcastToRoom(roomID, "chat:new-message", message)
	}

	return httpx.SendData(http.StatusCreated, "Message sent successfully", map[string]interface{}{
		"success": true,
		"message": message,
	})
}
