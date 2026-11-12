package contracts_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"nimble-voice-backend/internal/contracts"
)

func TestCanonicalServerEvents(t *testing.T) {
	events := contracts.CanonicalServerEvents()
	if len(events) != 22 {
		t.Fatalf("len(CanonicalServerEvents()) = %d, want 22", len(events))
	}

	seen := make(map[string]struct{}, len(events))
	for _, ev := range events {
		if ev == "" {
			t.Fatal("CanonicalServerEvents() contains empty event name")
		}
		if strings.Contains(ev, ":") {
			t.Fatalf("canonical event %q contains legacy colon delimiter", ev)
		}
		if !strings.Contains(ev, ".") {
			t.Fatalf("canonical event %q is missing dot namespace delimiter", ev)
		}
		if _, dup := seen[ev]; dup {
			t.Fatalf("duplicate canonical event %q", ev)
		}
		seen[ev] = struct{}{}

		if !contracts.IsCanonicalServerEvent(ev) {
			t.Fatalf("IsCanonicalServerEvent(%q) = false, want true", ev)
		}
	}

	for _, invalid := range []string{"", "room:joined", "chat:send", "command.ack", "unknown.event"} {
		if contracts.IsCanonicalServerEvent(invalid) {
			t.Fatalf("IsCanonicalServerEvent(%q) = true, want false", invalid)
		}
	}

	// Verify returned slice is defensive copy.
	events[0] = "mutated"
	if contracts.CanonicalServerEvents()[0] == "mutated" {
		t.Fatal("CanonicalServerEvents() returned backing slice instead of a copy")
	}

	// Verify canonical client commands contain zero colons or duplicates.
	cmds := contracts.CanonicalClientCommands()
	if len(cmds) != 17 {
		t.Fatalf("len(CanonicalClientCommands()) = %d, want 17", len(cmds))
	}
	cmdSeen := make(map[string]struct{}, len(cmds))
	for _, cmd := range cmds {
		if strings.Contains(cmd, ":") || !strings.Contains(cmd, ".") {
			t.Fatalf("invalid canonical client command %q", cmd)
		}
		if _, dup := cmdSeen[cmd]; dup {
			t.Fatalf("duplicate canonical client command %q", cmd)
		}
		cmdSeen[cmd] = struct{}{}
		if !contracts.IsCanonicalClientCommand(cmd) {
			t.Fatalf("IsCanonicalClientCommand(%q) = false, want true", cmd)
		}
	}
}

func TestNewServerEventEnvelopeAndConcurrency(t *testing.T) {
	env := contracts.NewServerEventEnvelope(
		contracts.EventPresenceSnapshot,
		"",
		42,
		contracts.PresenceDTO{OnlineCount: 7},
	)

	if env.EventID == "" {
		t.Fatal("expected non-empty EventID")
	}
	if contracts.SchemaVersion != 1 {
		t.Fatalf("contracts.SchemaVersion = %d, want 1", contracts.SchemaVersion)
	}
	if env.SchemaVersion != contracts.SchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", env.SchemaVersion, contracts.SchemaVersion)
	}
	if env.Event != contracts.EventPresenceSnapshot {
		t.Fatalf("Event = %q, want %q", env.Event, contracts.EventPresenceSnapshot)
	}
	if _, err := time.Parse(time.RFC3339Nano, env.OccurredAt); err != nil {
		t.Fatalf("OccurredAt %q is not valid RFC3339Nano: %v", env.OccurredAt, err)
	}
	if env.Version != 42 {
		t.Fatalf("Version = %d, want 42", env.Version)
	}

	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal(env) failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if _, hasRoomID := decoded["roomId"]; hasRoomID {
		t.Fatalf("expected empty roomId to be omitted from global presence envelope, got %v", decoded["roomId"])
	}
	for _, requiredKey := range []string{"eventId", "event", "schemaVersion", "occurredAt", "version", "data"} {
		if _, ok := decoded[requiredKey]; !ok {
			t.Fatalf("missing required envelope key %q in %s", requiredKey, string(raw))
		}
	}

	// Non-empty roomId must be present in JSON.
	roomEnv := contracts.NewServerEventEnvelope(
		contracts.EventRoomDeleted,
		"room-123",
		3,
		contracts.RoomDeletedDTO{RoomID: "room-123", Reason: "host_closed"},
	)
	roomRaw, err := json.Marshal(roomEnv)
	if err != nil {
		t.Fatalf("json.Marshal(roomEnv) failed: %v", err)
	}
	var roomDecoded map[string]any
	if err := json.Unmarshal(roomRaw, &roomDecoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if roomDecoded["roomId"] != "room-123" {
		t.Fatalf("roomId = %v, want %q", roomDecoded["roomId"], "room-123")
	}

	// Concurrent goroutines must generate collision-free eventIds.
	const workers = 32
	const perWorker = 50
	idCh := make(chan string, workers*perWorker)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				e := contracts.NewServerEventEnvelope(
					contracts.EventVoiceSpeakingChanged,
					"room-conc",
					0,
					contracts.VoiceSpeakingChangedDTO{RoomID: "room-conc", UserID: "u1", IsSpeaking: true, Level: 0.5},
				)
				idCh <- e.EventID
			}
		}()
	}
	wg.Wait()
	close(idCh)

	ids := make(map[string]struct{}, workers*perWorker)
	for id := range idCh {
		if id == "" {
			t.Fatal("generated empty EventID under concurrency")
		}
		if _, exists := ids[id]; exists {
			t.Fatalf("duplicate EventID %q generated under concurrency", id)
		}
		ids[id] = struct{}{}
	}
	if len(ids) != workers*perWorker {
		t.Fatalf("got %d unique EventIDs, want %d", len(ids), workers*perWorker)
	}
}

func TestCommandAckSerialization(t *testing.T) {
	okAck := contracts.NewAckOK("req-123", contracts.LobbySubscriptionAckDTO{Subscribed: true})
	okRaw, err := json.Marshal(okAck)
	if err != nil {
		t.Fatalf("json.Marshal(okAck) failed: %v", err)
	}
	var okMap map[string]any
	if err := json.Unmarshal(okRaw, &okMap); err != nil {
		t.Fatalf("json.Unmarshal(okRaw) failed: %v", err)
	}
	if okMap["ok"] != true || okMap["requestId"] != "req-123" {
		t.Fatalf("unexpected okAck JSON: %s", string(okRaw))
	}
	if _, hasErr := okMap["error"]; hasErr {
		t.Fatalf("okAck unexpectedly serialized error field: %s", string(okRaw))
	}
	dataMap, ok := okMap["data"].(map[string]any)
	if !ok || dataMap["subscribed"] != true {
		t.Fatalf("okAck data = %v, want {subscribed: true}", okMap["data"])
	}

	errAck := contracts.NewAckError("req-456", contracts.ErrCodeForbidden, "only the host can lock this room")
	errRaw, err := json.Marshal(errAck)
	if err != nil {
		t.Fatalf("json.Marshal(errAck) failed: %v", err)
	}
	var errMap map[string]any
	if err := json.Unmarshal(errRaw, &errMap); err != nil {
		t.Fatalf("json.Unmarshal(errRaw) failed: %v", err)
	}
	if errMap["ok"] != false || errMap["requestId"] != "req-456" {
		t.Fatalf("unexpected errAck JSON: %s", string(errRaw))
	}
	if _, hasData := errMap["data"]; hasData {
		t.Fatalf("errAck unexpectedly serialized data field: %s", string(errRaw))
	}
	ackErr, ok := errMap["error"].(map[string]any)
	if !ok || ackErr["code"] != contracts.ErrCodeForbidden || ackErr["message"] != "only the host can lock this room" {
		t.Fatalf("errAck error = %v, want {code: FORBIDDEN, message: ...}", errMap["error"])
	}
}

func TestDTOCamelCaseAndNonNilCollections(t *testing.T) {
	msg := contracts.NewChatMessageDTO(contracts.ChatMessageDTO{
		ID:              "msg-1",
		ClientMessageID: "client-uuid-1",
		RoomID:          "room-1",
		Sender: contracts.ChatSenderDTO{
			ID:        "user-1",
			Name:      "Alice",
			AvatarURL: "https://example.com/a.png",
		},
		Content:       "Hello!",
		Type:          "TEXT",
		IsHighlighted: false,
		Reactions:     contracts.NormalizeReactionsMap([]byte(`[]`)),
		CreatedAt:     "2026-11-11T12:00:00Z",
	})

	msgRaw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal(msg) failed: %v", err)
	}
	var msgMap map[string]any
	if err := json.Unmarshal(msgRaw, &msgMap); err != nil {
		t.Fatalf("json.Unmarshal(msgRaw) failed: %v", err)
	}

	if _, hasTimestamp := msgMap["timestamp"]; hasTimestamp {
		t.Fatalf("ChatMessageDTO must NOT serialize legacy 'timestamp' key: %s", string(msgRaw))
	}
	if msgMap["createdAt"] != "2026-11-11T12:00:00Z" {
		t.Fatalf("ChatMessageDTO createdAt = %v, want RFC3339 string", msgMap["createdAt"])
	}
	if msgMap["clientMessageId"] != "client-uuid-1" {
		t.Fatalf("ChatMessageDTO clientMessageId = %v, want %q", msgMap["clientMessageId"], "client-uuid-1")
	}
	reactionsMap, ok := msgMap["reactions"].(map[string]any)
	if !ok || len(reactionsMap) != 0 {
		t.Fatalf("ChatMessageDTO reactions = %#v, want empty JSON object {}", msgMap["reactions"])
	}

	// Verify NormalizeReactionsMap preserves valid map entries and normalizes nil/legacy slices.
	validReactions := contracts.NormalizeReactionsMap([]byte(`{"👍":["user-1","user-2"],"❤️":null}`))
	if len(validReactions["👍"]) != 2 || validReactions["❤️"] == nil || len(validReactions["❤️"]) != 0 {
		t.Fatalf("unexpected NormalizeReactionsMap result: %#v", validReactions)
	}

	// RoomSnapshotDTO must serialize peers as [] (never null), and RoomSummaryDTO tags/cefrPortfolio as [] / {}.
	room := contracts.NewRoomSummaryDTO(contracts.RoomSummaryDTO{
		ID:           "room-1",
		Title:        "English Cafe",
		Topic:        "Daily chat",
		Language:     "English",
		Flag:         "🇺🇸",
		CEFRLevel:    "B1",
		LevelLabel:   "Intermediate",
		MaxSlots:     6,
		CurrentSlots: 1,
		Status:       "LIVE",
		StartedAt:    "2026-11-11T12:00:00Z",
		HasFreeSeats: true,
		IsLive:       true,
		Version:      1,
		Host: contracts.UserSummaryDTO{
			ID:   "user-1",
			Name: "Alice",
		},
	})
	snap := contracts.NewRoomSnapshotDTO("room-1", &room, nil, false)
	snapRaw, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal(snap) failed: %v", err)
	}
	var snapMap map[string]any
	if err := json.Unmarshal(snapRaw, &snapMap); err != nil {
		t.Fatalf("json.Unmarshal(snapRaw) failed: %v", err)
	}
	peersSlice, ok := snapMap["peers"].([]any)
	if !ok || len(peersSlice) != 0 {
		t.Fatalf("RoomSnapshotDTO peers = %#v, want empty array []", snapMap["peers"])
	}
	roomObj := snapMap["room"].(map[string]any)
	tagsSlice, ok := roomObj["tags"].([]any)
	if !ok || len(tagsSlice) != 0 {
		t.Fatalf("RoomSummaryDTO tags = %#v, want empty array []", roomObj["tags"])
	}
	hostObj := roomObj["host"].(map[string]any)
	cefrObj, ok := hostObj["cefrPortfolio"].(map[string]any)
	if !ok || len(cefrObj) != 0 {
		t.Fatalf("UserSummaryDTO cefrPortfolio = %#v, want empty object {}", hostObj["cefrPortfolio"])
	}

	// Ensure zero snake_case keys in marshaled JSON across envelope + DTOs.
	for _, blob := range []string{string(msgRaw), string(snapRaw)} {
		if strings.Contains(blob, "room_id") || strings.Contains(blob, "user_id") || strings.Contains(blob, "created_at") || strings.Contains(blob, "is_locked") {
			t.Fatalf("unexpected snake_case JSON key found in %s", blob)
		}
	}
}
