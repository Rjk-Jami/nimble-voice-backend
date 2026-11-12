package chat

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/internal/contracts"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type broadcastCall struct {
	roomID  string
	event   string
	version int64
	data    any
}

type mockBroadcaster struct {
	roomCalls  []broadcastCall
	lobbyCalls []broadcastCall
}

func (m *mockBroadcaster) BroadcastRoomEvent(roomID, event string, version int64, data any) {
	m.roomCalls = append(m.roomCalls, broadcastCall{
		roomID:  roomID,
		event:   event,
		version: version,
		data:    data,
	})
}

func (m *mockBroadcaster) BroadcastLobbyEvent(event, roomID string, version int64, data any) {
	m.lobbyCalls = append(m.lobbyCalls, broadcastCall{
		roomID:  roomID,
		event:   event,
		version: version,
		data:    data,
	})
}

type mockChatStore struct {
	rooms   map[string]*domain.Room
	users   map[string]*domain.User
	created []*domain.ChatMessage
}

func (m *mockChatStore) FindActiveRoom(roomID string) (*domain.Room, error) {
	room, ok := m.rooms[roomID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return room, nil
}

func (m *mockChatStore) FindUser(userID string) (*domain.User, error) {
	user, ok := m.users[userID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return user, nil
}

func (m *mockChatStore) CreateMessage(msg *domain.ChatMessage) error {
	m.created = append(m.created, msg)
	return nil
}

func TestChatService_SendMessageValidationRejectsSystemAndUnknownTypes(t *testing.T) {
	broadcaster := &mockBroadcaster{}
	store := &mockChatStore{
		rooms: map[string]*domain.Room{
			"room-1": {ID: "room-1", Status: "LIVE"},
		},
		users: map[string]*domain.User{
			"user-1": {ID: "user-1", Name: "Alice"},
		},
	}
	svc := newChatServiceWithStore(store, broadcaster)

	for _, badType := range []string{"SYSTEM", "system", "ADMIN", "NOTICE", "UNKNOWN"} {
		res := svc.SendMessage("room-1", "user-1", &SendMessageDto{
			Content: "System takeover attempt",
			Type:    badType,
		})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("SendMessage(type=%q) status = %d, want 400", badType, res.StatusCode)
		}
	}

	if len(broadcaster.roomCalls) != 0 {
		t.Fatalf("expected 0 BroadcastRoomEvent calls on rejected types, got %d", len(broadcaster.roomCalls))
	}
	if len(store.created) != 0 {
		t.Fatalf("expected 0 persisted messages on rejected types, got %d", len(store.created))
	}
}

func TestChatService_SendMessageRejectsEmptyOrWhitespaceContent(t *testing.T) {
	broadcaster := &mockBroadcaster{}
	store := &mockChatStore{
		rooms: map[string]*domain.Room{
			"room-1": {ID: "room-1", Status: "LIVE"},
		},
		users: map[string]*domain.User{
			"user-1": {ID: "user-1", Name: "Alice"},
		},
	}
	svc := newChatServiceWithStore(store, broadcaster)

	for _, content := range []string{"", "   ", "\t\n  "} {
		res := svc.SendMessage("room-1", "user-1", &SendMessageDto{
			Content: content,
			Type:    "TEXT",
		})
		if res.StatusCode != http.StatusBadRequest {
			t.Fatalf("SendMessage(content=%q) status = %d, want 400", content, res.StatusCode)
		}
	}

	if len(broadcaster.roomCalls) != 0 {
		t.Fatalf("expected 0 BroadcastRoomEvent calls on empty content, got %d", len(broadcaster.roomCalls))
	}
}

func TestChatService_MapChatMessageDTO(t *testing.T) {
	ts := time.Date(2026, 11, 12, 10, 30, 0, 123456000, time.UTC)
	msg := domain.ChatMessage{
		ID:            "msg-cuid-1",
		RoomID:        "room-42",
		SenderID:      "user-7",
		SenderName:    "Amina",
		SenderAvatar:  "https://cdn.example.com/a.png",
		Content:       "Break a leg!",
		Type:          "IDIOM",
		IsHighlighted: true,
		Reactions:     datatypes.JSON([]byte(`[]`)), // legacy array format
		CreatedAt:     ts,
	}

	dto := ToChatMessageDTO(msg, "client-req-99")
	if dto.ID != "msg-cuid-1" || dto.ClientMessageID != "client-req-99" || dto.RoomID != "room-42" {
		t.Fatalf("unexpected DTO identifiers: %+v", dto)
	}
	if dto.Sender.ID != "user-7" || dto.Sender.Name != "Amina" || dto.Sender.AvatarURL != "https://cdn.example.com/a.png" {
		t.Fatalf("unexpected DTO sender: %+v", dto.Sender)
	}
	if dto.CreatedAt != ts.Format(time.RFC3339Nano) {
		t.Fatalf("CreatedAt = %q, want %q", dto.CreatedAt, ts.Format(time.RFC3339Nano))
	}
	if dto.Reactions == nil || len(dto.Reactions) != 0 {
		t.Fatalf("expected empty non-nil reactions map, got %+v", dto.Reactions)
	}

	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("json.Marshal(ChatMessageDTO) failed: %v", err)
	}
	raw := string(encoded)
	if strings.Contains(raw, `"timestamp"`) {
		t.Fatalf("DTO JSON must use createdAt instead of timestamp: %s", raw)
	}
	if !strings.Contains(raw, `"createdAt":"2026-11-12T10:30:00.123456Z"`) {
		t.Fatalf("DTO JSON missing expected createdAt: %s", raw)
	}
	if !strings.Contains(raw, `"reactions":{}`) {
		t.Fatalf("DTO JSON must serialize reactions as {}: %s", raw)
	}
	if !strings.Contains(raw, `"clientMessageId":"client-req-99"`) {
		t.Fatalf("DTO JSON missing clientMessageId: %s", raw)
	}
}

func TestChatService_SendMessagePersistsAndBroadcastsOnce(t *testing.T) {
	broadcaster := &mockBroadcaster{}
	store := &mockChatStore{
		rooms: map[string]*domain.Room{
			"room-live":  {ID: "room-live", Status: "LIVE"},
			"room-ended": {ID: "room-ended", Status: "ENDED"},
		},
		users: map[string]*domain.User{
			"user-1": {ID: "user-1", Name: "Nadia", AvatarURL: "https://cdn.example.com/n.png"},
		},
	}
	svc := newChatServiceWithStore(store, broadcaster)

	// Ended room returns 404
	endedRes := svc.SendMessage("room-ended", "user-1", &SendMessageDto{Content: "hello", Type: "TEXT"})
	if endedRes.StatusCode != http.StatusNotFound {
		t.Fatalf("SendMessage to ENDED room status = %d, want 404", endedRes.StatusCode)
	}

	// Missing room returns 404
	missingRes := svc.SendMessage("room-missing", "user-1", &SendMessageDto{Content: "hello", Type: "TEXT"})
	if missingRes.StatusCode != http.StatusNotFound {
		t.Fatalf("SendMessage to missing room status = %d, want 404", missingRes.StatusCode)
	}

	// Valid send persists once and broadcasts once
	res := svc.SendMessage("room-live", "user-1", &SendMessageDto{
		Content:         "  Hello room!  ",
		Type:            "",
		ClientMessageID: "cid-123",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("SendMessage status = %d, want 201 (msg=%s)", res.StatusCode, res.Message)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 persisted message, got %d", len(store.created))
	}
	if store.created[0].Content != "Hello room!" || store.created[0].Type != "TEXT" {
		t.Fatalf("unexpected persisted message: %+v", store.created[0])
	}

	if len(broadcaster.roomCalls) != 1 {
		t.Fatalf("expected exactly 1 BroadcastRoomEvent call, got %d", len(broadcaster.roomCalls))
	}
	call := broadcaster.roomCalls[0]
	if call.roomID != "room-live" || call.event != contracts.EventChatMessageCreated || call.version != 0 {
		t.Fatalf("unexpected broadcast call args: %+v", call)
	}
	dto, ok := call.data.(contracts.ChatMessageDTO)
	if !ok {
		t.Fatalf("broadcast data type = %T, want contracts.ChatMessageDTO", call.data)
	}
	if dto.ClientMessageID != "cid-123" || dto.Sender.ID != "user-1" || dto.Sender.Name != "Nadia" {
		t.Fatalf("unexpected broadcast ChatMessageDTO: %+v", dto)
	}
	if dto.Reactions == nil || len(dto.Reactions) != 0 {
		t.Fatalf("expected empty non-nil reactions map in broadcast DTO, got %+v", dto.Reactions)
	}
}
