package chat

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/internal/contracts"
	"nimble-voice-backend/utils/httpx"

	"github.com/lucsky/cuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ChatServiceInterface interface {
	GetMessages(roomID string, query *ChatQuery) httpx.APIResponse
	SendMessage(roomID, userID string, dto *SendMessageDto) httpx.APIResponse
}

type chatStore interface {
	FindActiveRoom(roomID string) (*domain.Room, error)
	FindUser(userID string) (*domain.User, error)
	CreateMessage(msg *domain.ChatMessage) error
}

type gormChatStore struct {
	db *gorm.DB
}

func (g *gormChatStore) FindActiveRoom(roomID string) (*domain.Room, error) {
	var room domain.Room
	if err := g.db.Where("id = ?", roomID).First(&room).Error; err != nil {
		return nil, err
	}
	return &room, nil
}

func (g *gormChatStore) FindUser(userID string) (*domain.User, error) {
	var user domain.User
	if err := g.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (g *gormChatStore) CreateMessage(msg *domain.ChatMessage) error {
	return g.db.Create(msg).Error
}

type ChatService struct {
	db          *gorm.DB
	broadcaster contracts.RoomBroadcaster
	store       chatStore
}

func NewChatService(db *gorm.DB, broadcasters ...contracts.RoomBroadcaster) ChatServiceInterface {
	var broadcaster contracts.RoomBroadcaster
	if len(broadcasters) > 0 && broadcasters[0] != nil {
		broadcaster = broadcasters[0]
	}
	var store chatStore
	if db != nil {
		store = &gormChatStore{db: db}
	}
	return &ChatService{
		db:          db,
		broadcaster: broadcaster,
		store:       store,
	}
}

func newChatServiceWithStore(store chatStore, broadcaster contracts.RoomBroadcaster) *ChatService {
	return &ChatService{
		broadcaster: broadcaster,
		store:       store,
	}
}

// ToChatMessageDTO converts a persisted domain.ChatMessage into the canonical camelCase ChatMessageDTO.
func ToChatMessageDTO(msg domain.ChatMessage, clientMessageID string) contracts.ChatMessageDTO {
	createdAt := msg.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return contracts.NewChatMessageDTO(contracts.ChatMessageDTO{
		ID:              msg.ID,
		ClientMessageID: strings.TrimSpace(clientMessageID),
		RoomID:          msg.RoomID,
		Sender: contracts.ChatSenderDTO{
			ID:        msg.SenderID,
			Name:      msg.SenderName,
			AvatarURL: msg.SenderAvatar,
		},
		Content:       msg.Content,
		Type:          msg.Type,
		IsHighlighted: msg.IsHighlighted,
		Reactions:     contracts.NormalizeReactionsMap(msg.Reactions),
		CreatedAt:     createdAt.UTC().Format(time.RFC3339Nano),
	})
}

func isAllowedClientMessageType(msgType string) bool {
	switch msgType {
	case "TEXT", "IMAGE", "IDIOM", "REACTION":
		return true
	default:
		return false
	}
}

func (s *ChatService) GetMessages(roomID string, query *ChatQuery) httpx.APIResponse {
	if s.db == nil {
		return httpx.InternalError("chat.get_messages", errors.New("database is not configured"), "Failed to retrieve messages")
	}

	limit := 50
	if query != nil && query.Limit >= 1 && query.Limit <= 100 {
		limit = query.Limit
	}

	dbQuery := s.db.Model(&domain.ChatMessage{}).Where("room_id = ?", roomID)

	if query != nil && query.Before != "" {
		var beforeMsg domain.ChatMessage
		if err := s.db.Where("id = ?", query.Before).First(&beforeMsg).Error; err == nil {
			dbQuery = dbQuery.Where("created_at < ?", beforeMsg.CreatedAt)
		}
	}

	var messages []domain.ChatMessage
	err := dbQuery.Order("created_at ASC").Limit(limit).Find(&messages).Error
	if err != nil {
		return httpx.InternalError("chat.get_messages", err, "Failed to retrieve messages")
	}

	dtos := make([]contracts.ChatMessageDTO, 0, len(messages))
	for _, msg := range messages {
		dtos = append(dtos, ToChatMessageDTO(msg, ""))
	}

	return httpx.SendData(http.StatusOK, "Messages retrieved successfully", ChatMessagesResponse{
		Messages: dtos,
	})
}

func (s *ChatService) SendMessage(roomID, userID string, dto *SendMessageDto) httpx.APIResponse {
	if dto == nil {
		return httpx.SendData(http.StatusBadRequest, "Invalid message payload")
	}

	cleanRoomID := strings.TrimSpace(roomID)
	cleanUserID := strings.TrimSpace(userID)
	if cleanRoomID == "" {
		return httpx.SendData(http.StatusBadRequest, "roomId is required")
	}
	if cleanUserID == "" {
		return httpx.SendData(http.StatusUnauthorized, "User not found")
	}

	msgType := strings.ToUpper(strings.TrimSpace(dto.Type))
	if msgType == "" {
		msgType = "TEXT"
	}
	if !isAllowedClientMessageType(msgType) {
		return httpx.SendData(http.StatusBadRequest, "Invalid message type")
	}

	content := strings.TrimSpace(dto.Content)
	mediaURL := strings.TrimSpace(dto.MediaURL)
	mediaType := strings.TrimSpace(dto.MediaType)
	clientMessageID := strings.TrimSpace(dto.ClientMessageID)

	if msgType != "IMAGE" && content == "" {
		return httpx.SendData(http.StatusBadRequest, "Message content is required")
	}
	if msgType == "IMAGE" && content == "" && mediaURL == "" {
		return httpx.SendData(http.StatusBadRequest, "Message content or mediaUrl is required")
	}
	if len(content) > 500 {
		return httpx.SendData(http.StatusBadRequest, "Message content exceeds 500 characters")
	}

	if s.store == nil {
		return httpx.InternalError("chat.send_message", errors.New("chat store is not configured"), "Failed to send message")
	}

	room, err := s.store.FindActiveRoom(cleanRoomID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.SendData(http.StatusNotFound, "Room not found")
		}
		return httpx.InternalError("chat.send_message.room", err, "Failed to verify room")
	}
	if room == nil || strings.EqualFold(room.Status, "ENDED") {
		return httpx.SendData(http.StatusNotFound, "Room not found or has ended")
	}

	user, err := s.store.FindUser(cleanUserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.SendData(http.StatusUnauthorized, "User not found")
		}
		return httpx.InternalError("chat.send_message.user", err, "Failed to verify user")
	}
	if user == nil {
		return httpx.SendData(http.StatusUnauthorized, "User not found")
	}

	message := domain.ChatMessage{
		ID:            cuid.New(),
		RoomID:        cleanRoomID,
		SenderID:      cleanUserID,
		SenderName:    user.Name,
		SenderAvatar:  user.AvatarURL,
		Content:       content,
		Type:          msgType,
		IsHighlighted: msgType == "IDIOM",
		Reactions:     datatypes.JSON([]byte("{}")),
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.store.CreateMessage(&message); err != nil {
		return httpx.InternalError("chat.send_message", err, "Failed to send message")
	}

	msgDTO := ToChatMessageDTO(message, clientMessageID)
	msgDTO.MediaURL = mediaURL
	msgDTO.MediaType = mediaType

	if s.broadcaster != nil {
		s.broadcaster.BroadcastRoomEvent(cleanRoomID, contracts.EventChatMessageCreated, 0, msgDTO)
	}

	return httpx.SendData(http.StatusCreated, "Message sent successfully", map[string]interface{}{
		"success": true,
		"message": msgDTO,
	})
}
