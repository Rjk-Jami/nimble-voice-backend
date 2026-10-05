package room

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/utils/httpx"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RoomServiceInterface interface {
	CreateRoom(hostID string, dto *CreateRoomDto) httpx.APIResponse
	GetRoomList(query *RoomQuery) httpx.APIResponse
	GetRoomByID(roomID string) httpx.APIResponse
	UpdateRoom(roomID, hostID string, dto *UpdateRoomDto) httpx.APIResponse
	DeleteRoom(roomID, hostID string) httpx.APIResponse
	JoinRoom(roomID, userID string, dto *JoinRoomDto) httpx.APIResponse
	LeaveRoom(roomID, userID string) httpx.APIResponse
}

type RoomService struct {
	db *gorm.DB
}

func NewRoomService(db *gorm.DB) RoomServiceInterface {
	return &RoomService{db: db}
}

func getLanguageFlag(lang string) string {
	switch strings.ToLower(lang) {
	case "english":
		return "🇬🇧"
	case "spanish":
		return "🇪🇸"
	case "french":
		return "🇫🇷"
	case "german":
		return "🇩🇪"
	case "japanese":
		return "🇯🇵"
	case "chinese":
		return "🇨🇳"
	case "italian":
		return "🇮🇹"
	case "portuguese":
		return "🇧🇷"
	case "korean":
		return "🇰🇷"
	default:
		return "🌐"
	}
}

func (s *RoomService) CreateRoom(hostID string, dto *CreateRoomDto) httpx.APIResponse {
	maxSlots := 5
	if dto.MaxSlots >= 2 && dto.MaxSlots <= 20 {
		maxSlots = dto.MaxSlots
	}

	cefrLevel := "B1"
	if dto.CEFRLevel != "" {
		cefrLevel = dto.CEFRLevel
	}

	levelLabel := dto.LevelLabel
	if levelLabel == "" {
		levelLabel = fmt.Sprintf("Intermediate %s", cefrLevel)
	}

	topic := dto.Topic
	if topic == "" && dto.TopicTag != "" {
		topic = dto.TopicTag
	} else if topic == "" {
		topic = "Casual Conversation"
	}

	isBeginner := true
	if dto.IsBeginnerFriendly != nil {
		isBeginner = *dto.IsBeginnerFriendly
	}

	tagsList := dto.Tags
	if len(tagsList) == 0 {
		if dto.TopicTag != "" {
			tagsList = []string{dto.TopicTag}
		} else {
			tagsList = []string{"General", "Casual & Life"}
		}
	}
	tagsBytes, _ := json.Marshal(tagsList)

	room := domain.Room{
		Title:              dto.Title,
		Topic:              topic,
		Language:           dto.Language,
		Flag:               getLanguageFlag(dto.Language),
		CEFRLevel:          cefrLevel,
		LevelLabel:         levelLabel,
		MaxSlots:           maxSlots,
		CurrentSlots:       1,
		TopicTag:           dto.TopicTag,
		Tags:               datatypes.JSON(tagsBytes),
		Status:             "LIVE",
		IsBeginnerFriendly: isBeginner,
		HasFreeSeats:       maxSlots > 1,
		HasNativeSpeaker:   false,
		IsLive:             true,
		HostID:             hostID,
		RoomKey:            dto.RoomKey,
		StartedAt:          time.Now().UTC(),
	}

	// ACID Transaction: Create room and enroll host as first participant
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&room).Error; err != nil {
			return err
		}

		hostParticipant := domain.RoomParticipant{
			RoomID:     room.ID,
			UserID:     hostID,
			Role:       "host",
			IsHost:     true,
			IsMuted:    false,
			IsDeafened: false,
			HandRaised: false,
			JoinedAt:   room.CreatedAt,
		}
		return tx.Create(&hostParticipant).Error
	})

	if err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to create voice room", err.Error())
	}

	// Preload Host & Participants
	s.db.Preload("Host").Preload("Participants.User").Where("id = ?", room.ID).First(&room)

	return httpx.SendData(http.StatusCreated, "Voice room created successfully", map[string]interface{}{
		"success": true,
		"room":    room,
	})
}

func (s *RoomService) GetRoomList(query *RoomQuery) httpx.APIResponse {
	page := query.Page
	if page < 1 {
		page = 1
	}
	limit := query.Limit
	if limit < 1 || limit > 50 {
		limit = 20
	}
	offset := (page - 1) * limit

	dbQuery := s.db.Model(&domain.Room{}).Where("status != ?", "ENDED")

	// Language filter
	if query.Lang != "" && strings.ToLower(query.Lang) != "all" {
		dbQuery = dbQuery.Where("LOWER(language) = ?", strings.ToLower(query.Lang))
	}

	// Search query in title or topic
	if query.Query != "" {
		searchTerm := "%" + strings.ToLower(query.Query) + "%"
		dbQuery = dbQuery.Where("LOWER(title) LIKE ? OR LOWER(topic) LIKE ? OR LOWER(topic_tag) LIKE ?", searchTerm, searchTerm, searchTerm)
	}

	// Quick filter
	switch strings.ToLower(query.Filter) {
	case "free-seats":
		dbQuery = dbQuery.Where("current_slots < max_slots")
	case "beginner":
		dbQuery = dbQuery.Where("is_beginner_friendly = true")
	case "native":
		dbQuery = dbQuery.Where("has_native_speaker = true")
	case "active":
		dbQuery = dbQuery.Where("status = 'LIVE'")
	}

	var total int64
	dbQuery.Count(&total)

	var rooms []domain.Room
	err := dbQuery.Preload("Host").
		Preload("Participants", "left_at IS NULL").
		Preload("Participants.User").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&rooms).Error

	if err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to fetch voice rooms", err.Error())
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if totalPages == 0 {
		totalPages = 1
	}

	return httpx.SendData(http.StatusOK, "Voice rooms retrieved successfully", RoomListResponse{
		Rooms: rooms,
		Pagination: PaginationMeta{
			Total:      total,
			Page:       page,
			Limit:      limit,
			TotalPages: totalPages,
		},
	})
}

func (s *RoomService) GetRoomByID(roomID string) httpx.APIResponse {
	var room domain.Room
	err := s.db.Preload("Host").
		Preload("Participants", "left_at IS NULL").
		Preload("Participants.User").
		Where("id = ?", roomID).
		First(&room).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return httpx.SendData(http.StatusNotFound, "Room not found or has ended")
		}
		return httpx.SendData(http.StatusInternalServerError, "Failed to retrieve room details", err.Error())
	}

	return httpx.SendData(http.StatusOK, "Room details retrieved successfully", room)
}

func (s *RoomService) UpdateRoom(roomID, hostID string, dto *UpdateRoomDto) httpx.APIResponse {
	var room domain.Room
	if err := s.db.Where("id = ?", roomID).First(&room).Error; err != nil {
		return httpx.SendData(http.StatusNotFound, "Room not found")
	}

	if room.HostID != hostID {
		return httpx.SendData(http.StatusForbidden, "Only the host can modify room properties")
	}

	updates := make(map[string]interface{})
	if dto.Title != nil && *dto.Title != "" {
		updates["title"] = *dto.Title
	}
	if dto.Topic != nil && *dto.Topic != "" {
		updates["topic"] = *dto.Topic
	}
	if dto.Status != nil && *dto.Status != "" {
		updates["status"] = *dto.Status
		if *dto.Status == "ENDED" {
			updates["is_live"] = false
		}
	}
	if dto.MaxSlots != nil && *dto.MaxSlots >= 2 {
		updates["max_slots"] = *dto.MaxSlots
		updates["has_free_seats"] = room.CurrentSlots < *dto.MaxSlots
	}

	if len(updates) > 0 {
		if err := s.db.Model(&room).Updates(updates).Error; err != nil {
			return httpx.SendData(http.StatusInternalServerError, "Failed to update room", err.Error())
		}
	}

	s.db.Preload("Host").Preload("Participants", "left_at IS NULL").Where("id = ?", roomID).First(&room)
	return httpx.SendData(http.StatusOK, "Room updated successfully", room)
}

func (s *RoomService) DeleteRoom(roomID, hostID string) httpx.APIResponse {
	var room domain.Room
	if err := s.db.Where("id = ?", roomID).First(&room).Error; err != nil {
		return httpx.SendData(http.StatusNotFound, "Room not found")
	}

	if room.HostID != hostID {
		return httpx.SendData(http.StatusForbidden, "Only the room host can close this room")
	}

	now := time.Now().UTC()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Mark room ended
		if err := tx.Model(&room).Updates(map[string]interface{}{
			"status":  "ENDED",
			"is_live": false,
		}).Error; err != nil {
			return err
		}

		// Mark all remaining participants as left
		return tx.Model(&domain.RoomParticipant{}).
			Where("room_id = ? AND left_at IS NULL", roomID).
			Update("left_at", now).Error
	})

	if err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to close room", err.Error())
	}

	return httpx.SendData(http.StatusOK, "Room closed successfully", map[string]interface{}{
		"success": true,
		"message": "Room closed successfully",
	})
}

func (s *RoomService) JoinRoom(roomID, userID string, dto *JoinRoomDto) httpx.APIResponse {
	var room domain.Room
	peerToken := "sig_tok_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// ACID Row lock on room record to prevent exceeding max capacity
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status != 'ENDED'", roomID).
			First(&room).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("room not found or is no longer active")
			}
			return err
		}

		// Check private key if configured
		if room.RoomKey != nil && *room.RoomKey != "" {
			if dto == nil || dto.Password == nil || *dto.Password != *room.RoomKey {
				return errors.New("incorrect room password")
			}
		}

		// Count active participants
		var currentCount int64
		if err := tx.Model(&domain.RoomParticipant{}).
			Where("room_id = ? AND left_at IS NULL", roomID).
			Count(&currentCount).Error; err != nil {
			return err
		}

		// Check if user is already in the room
		var existing domain.RoomParticipant
		err := tx.Where("room_id = ? AND user_id = ?", roomID, userID).First(&existing).Error
		if err == nil && existing.LeftAt == nil {
			// Already active participant, refresh
			return nil
		}

		if currentCount >= int64(room.MaxSlots) {
			return errors.New("room is already at full capacity")
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			newParticipant := domain.RoomParticipant{
				RoomID:     roomID,
				UserID:     userID,
				Role:       "listener",
				IsHost:     room.HostID == userID,
				IsMuted:    true,
				IsDeafened: false,
				HandRaised: false,
				JoinedAt:   time.Now().UTC(),
			}
			if err := tx.Create(&newParticipant).Error; err != nil {
				return err
			}
		} else if err == nil {
			if err := tx.Model(&existing).Updates(map[string]interface{}{
				"left_at":     nil,
				"is_muted":    true,
				"is_deafened": false,
				"hand_raised": false,
				"joined_at":   time.Now().UTC(),
			}).Error; err != nil {
				return err
			}
		}

		newSlots := int(currentCount) + 1
		hasFreeSeats := newSlots < room.MaxSlots
		return tx.Model(&room).Updates(map[string]interface{}{
			"current_slots":  newSlots,
			"has_free_seats": hasFreeSeats,
		}).Error
	})

	if err != nil {
		if strings.Contains(err.Error(), "capacity") {
			return httpx.SendData(http.StatusConflict, err.Error())
		}
		if strings.Contains(err.Error(), "not found") {
			return httpx.SendData(http.StatusNotFound, err.Error())
		}
		return httpx.SendData(http.StatusBadRequest, err.Error())
	}

	s.db.Where("id = ?", roomID).First(&room)

	return httpx.SendData(http.StatusOK, "Joined room successfully", JoinRoomResponse{
		Success:      true,
		RoomID:       room.ID,
		PeerToken:    peerToken,
		CurrentSlots: room.CurrentSlots,
		MaxSlots:     room.MaxSlots,
	})
}

func (s *RoomService) LeaveRoom(roomID, userID string) httpx.APIResponse {
	var remainingCount int64

	err := s.db.Transaction(func(tx *gorm.DB) error {
		var participant domain.RoomParticipant
		if err := tx.Where("room_id = ? AND user_id = ? AND left_at IS NULL", roomID, userID).First(&participant).Error; err != nil {
			return nil
		}

		now := time.Now().UTC()
		if err := tx.Model(&participant).Update("left_at", now).Error; err != nil {
			return err
		}

		if err := tx.Model(&domain.RoomParticipant{}).Where("room_id = ? AND left_at IS NULL", roomID).Count(&remainingCount).Error; err != nil {
			return err
		}

		var room domain.Room
		if err := tx.Where("id = ?", roomID).First(&room).Error; err != nil {
			return nil
		}

		updates := map[string]interface{}{
			"current_slots":  int(remainingCount),
			"has_free_seats": int(remainingCount) < room.MaxSlots,
		}

		// Auto-close room if host left and room is now empty
		if remainingCount == 0 && room.HostID == userID {
			updates["status"] = "ENDED"
			updates["is_live"] = false
		}

		return tx.Model(&room).Updates(updates).Error
	})

	if err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to leave room", err.Error())
	}

	return httpx.SendData(http.StatusOK, "Left room successfully", LeaveRoomResponse{
		Success:        true,
		RoomID:         roomID,
		RemainingSlots: int(remainingCount),
	})
}
