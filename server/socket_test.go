package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nimble-voice-backend/internal/contracts"
	"nimble-voice-backend/utils/jwt"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/shishir1290/gsocketio"
)

const testJWTSecret = "test-socket-jwt-secret-key-at-least-32-chars"

// Helper to extract session ID from Engine.IO v4 handshake response: 0{"sid":"...","upgrades":[...],...}
func extractSID(body string) string {
	if len(body) < 2 || body[0] != '0' {
		return ""
	}
	var data struct {
		SID string `json:"sid"`
	}
	if err := json.Unmarshal([]byte(body[1:]), &data); err != nil {
		return ""
	}
	return data.SID
}

func setupTestServer(t *testing.T) (*SocketServer, *httptest.Server) {
	t.Helper()
	jwt.SetSecretKeyForTest(testJWTSecret)
	opts := &gsocketio.Options{
		PingInterval: 5 * time.Second,
		PingTimeout:  3 * time.Second,
		MaxPayload:   512 * 1024,
	}

	sockServer, err := NewSocketServer(opts)
	if err != nil {
		t.Fatalf("Failed to create SocketServer: %v", err)
	}

	go func() {
		_ = sockServer.Serve()
	}()

	ts := httptest.NewServer(sockServer)
	return sockServer, ts
}

func makeTestJWT(t *testing.T, userID, email, role string) string {
	t.Helper()
	tok, err := jwt.GenerateToken(userID, email, role, "", "")
	if err != nil {
		t.Fatalf("jwt.GenerateToken failed: %v", err)
	}
	return tok
}

type wsTestPacket struct {
	Raw     string
	Prefix  string
	Event   string
	Payload json.RawMessage
}

type wsTestClient struct {
	t       *testing.T
	conn    *websocket.Conn
	writeMu sync.Mutex
	mu      sync.Mutex
	buf     []wsTestPacket
	packets chan wsTestPacket
	done    chan struct{}
	once    sync.Once
}

func dialWSSocketClient(t *testing.T, baseURL string, connectAuthPayload string) (*wsTestClient, wsTestPacket) {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/socket.io/?EIO=4&transport=websocket"
	hdr := http.Header{}
	hdr.Set("Origin", "http://localhost:3000")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}

	// Expect Engine.IO open packet '0{...}'
	_, openMsg, err := conn.ReadMessage()
	if err != nil {
		_ = conn.Close()
		t.Fatalf("Failed reading Engine.IO open packet: %v", err)
	}
	if !strings.HasPrefix(string(openMsg), "0{") {
		_ = conn.Close()
		t.Fatalf("Expected Engine.IO open packet, got %q", string(openMsg))
	}

	c := &wsTestClient{
		t:       t,
		conn:    conn,
		packets: make(chan wsTestPacket, 128),
		done:    make(chan struct{}),
	}

	go c.readLoop()

	if err := c.sendRaw("40" + connectAuthPayload); err != nil {
		c.close()
		t.Fatalf("Failed sending Socket.IO CONNECT packet: %v", err)
	}

	// Read CONNECT ack ('40{...}') or CONNECT_ERROR ('44{...}')
	pkt, err := c.nextControlPacket(2 * time.Second)
	if err != nil {
		c.close()
		t.Fatalf("Timed out waiting for Socket.IO CONNECT response: %v", err)
	}
	return c, pkt
}

func (c *wsTestClient) readLoop() {
	defer c.close()
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		text := string(msg)
		if text == "2" {
			_ = c.sendRaw("3")
			continue
		}
		if strings.HasPrefix(text, "42") {
			var arr []json.RawMessage
			if err := json.Unmarshal([]byte(text[2:]), &arr); err == nil && len(arr) >= 1 {
				var ev string
				_ = json.Unmarshal(arr[0], &ev)
				var payload json.RawMessage
				if len(arr) >= 2 {
					payload = arr[1]
				}
				select {
				case c.packets <- wsTestPacket{Raw: text, Prefix: "42", Event: ev, Payload: payload}:
				case <-c.done:
					return
				}
			}
			continue
		}
		if strings.HasPrefix(text, "40") || strings.HasPrefix(text, "44") || strings.HasPrefix(text, "41") {
			select {
			case c.packets <- wsTestPacket{Raw: text, Prefix: text[:2]}:
			case <-c.done:
				return
			}
		}
	}
}

func (c *wsTestClient) sendRaw(raw string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, []byte(raw))
}

func (c *wsTestClient) emitCommand(cmd string, payload any) error {
	b, err := json.Marshal([]any{cmd, payload})
	if err != nil {
		return err
	}
	return c.sendRaw("42" + string(b))
}

func (c *wsTestClient) takeMatching(timeout time.Duration, match func(wsTestPacket) bool) (wsTestPacket, bool) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		c.mu.Lock()
		for i, pkt := range c.buf {
			if match(pkt) {
				c.buf = append(c.buf[:i], c.buf[i+1:]...)
				c.mu.Unlock()
				return pkt, true
			}
		}
		c.mu.Unlock()

		select {
		case pkt, ok := <-c.packets:
			if !ok {
				return wsTestPacket{}, false
			}
			if match(pkt) {
				return pkt, true
			}
			c.mu.Lock()
			c.buf = append(c.buf, pkt)
			c.mu.Unlock()
		case <-deadline.C:
			return wsTestPacket{}, false
		}
	}
}

func (c *wsTestClient) nextControlPacket(timeout time.Duration) (wsTestPacket, error) {
	pkt, ok := c.takeMatching(timeout, func(p wsTestPacket) bool {
		return p.Prefix == "40" || p.Prefix == "44"
	})
	if !ok {
		return wsTestPacket{}, fmt.Errorf("timeout waiting for control packet")
	}
	return pkt, nil
}

func (c *wsTestClient) waitForEvent(eventName string, timeout time.Duration) (wsTestPacket, bool) {
	return c.takeMatching(timeout, func(p wsTestPacket) bool {
		return p.Prefix == "42" && p.Event == eventName
	})
}

func (c *wsTestClient) waitForAck(requestID string, timeout time.Duration) (contracts.CommandAck[json.RawMessage], bool) {
	var matched contracts.CommandAck[json.RawMessage]
	_, ok := c.takeMatching(timeout, func(p wsTestPacket) bool {
		if p.Prefix != "42" || p.Event != contracts.EventCommandAck {
			return false
		}
		var ack contracts.CommandAck[json.RawMessage]
		if err := json.Unmarshal(p.Payload, &ack); err == nil && ack.RequestID == requestID {
			matched = ack
			return true
		}
		return false
	})
	return matched, ok
}

func (c *wsTestClient) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

func assertValidEnvelope(t *testing.T, raw json.RawMessage, expectedEvent string) contracts.ServerEventEnvelope[json.RawMessage] {
	t.Helper()
	var env contracts.ServerEventEnvelope[json.RawMessage]
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("failed to unmarshal ServerEventEnvelope for %s: %v (raw=%s)", expectedEvent, err, string(raw))
	}
	if env.SchemaVersion != contracts.SchemaVersion {
		t.Fatalf("envelope schemaVersion = %d, want %d", env.SchemaVersion, contracts.SchemaVersion)
	}
	if env.EventID == "" {
		t.Fatalf("envelope eventId must not be empty")
	}
	if env.Event != expectedEvent {
		t.Fatalf("envelope event = %q, want %q", env.Event, expectedEvent)
	}
	if _, err := time.Parse(time.RFC3339Nano, env.OccurredAt); err != nil {
		t.Fatalf("envelope occurredAt %q is not RFC3339Nano: %v", env.OccurredAt, err)
	}
	if len(env.Data) == 0 {
		t.Fatalf("envelope data must not be empty for %s", expectedEvent)
	}
	return env
}

type mockRoomAuthorizer struct {
	hosts   map[string]string
	members map[string]map[string]bool
}

func (m *mockRoomAuthorizer) IsRoomHost(roomID, userID string) (bool, error) {
	if m == nil {
		return false, nil
	}
	return m.hosts[roomID] == userID, nil
}

func (m *mockRoomAuthorizer) IsRoomMember(roomID, userID string) (bool, error) {
	if m == nil || m.members[roomID] == nil {
		return false, nil
	}
	return m.members[roomID][userID], nil
}

func TestSocketServer_HandshakeAndCORS(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Test CORS preflight on /socket.io/
	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/socket.io/?EIO=4&transport=polling", nil)
	if err != nil {
		t.Fatalf("Failed to create OPTIONS request: %v", err)
	}
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS request failed: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 204 or 200 for OPTIONS, got: %d", resp.StatusCode)
	}
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "http://localhost:3000" {
		t.Errorf("Expected Allow-Origin 'http://localhost:3000', got: %s", origin)
	}
	if creds := resp.Header.Get("Access-Control-Allow-Credentials"); creds != "true" {
		t.Errorf("Expected Allow-Credentials 'true', got: %s", creds)
	}

	// 2. Test Engine.IO v4 Handshake
	handshakeURL := ts.URL + "/socket.io/?EIO=4&transport=polling"
	req, _ = http.NewRequest(http.MethodGet, handshakeURL, nil)
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET handshake request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 OK for handshake, got %d: %s", resp.StatusCode, bodyStr)
	}
	if origin := resp.Header.Get("Access-Control-Allow-Origin"); origin != "http://localhost:3000" {
		t.Fatalf("Expected GET polling Access-Control-Allow-Origin 'http://localhost:3000', got %q", origin)
	}

	sid := extractSID(bodyStr)
	if sid == "" {
		t.Fatalf("Expected valid sid from Engine.IO handshake, got body: %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "pingInterval") || !strings.Contains(bodyStr, "pingTimeout") {
		t.Errorf("Expected pingInterval and pingTimeout in handshake, got: %s", bodyStr)
	}

	// 3. Test unlisted Origin rejection on OPTIONS and GET polling
	for _, method := range []string{http.MethodOptions, http.MethodGet} {
		badReq, err := http.NewRequest(method, handshakeURL, nil)
		if err != nil {
			t.Fatalf("Failed to build unlisted origin %s request: %v", method, err)
		}
		badReq.Header.Set("Origin", "https://evil.example")
		badResp, err := client.Do(badReq)
		if err != nil {
			t.Fatalf("Unlisted origin %s request failed: %v", method, err)
		}
		_ = badResp.Body.Close()

		if badResp.StatusCode != http.StatusForbidden {
			t.Fatalf("Expected 403 Forbidden for unlisted origin %s, got %d", method, badResp.StatusCode)
		}
		if gotOrigin := badResp.Header.Get("Access-Control-Allow-Origin"); gotOrigin != "" {
			t.Fatalf("Expected empty ACAO for unlisted origin %s, got %q", method, gotOrigin)
		}
	}

	// 4. Test real WebSocket upgrade: allowlisted succeeds, unlisted returns 403
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/socket.io/?EIO=4&transport=websocket"
	allowHdr := http.Header{}
	allowHdr.Set("Origin", "http://localhost:3000")
	wsConn, wsResp, err := websocket.DefaultDialer.Dial(wsURL, allowHdr)
	if err != nil {
		status := 0
		if wsResp != nil {
			status = wsResp.StatusCode
		}
		t.Fatalf("Expected WebSocket upgrade to succeed for allowlisted origin, got status=%d err=%v", status, err)
	}
	_, msg, err := wsConn.ReadMessage()
	_ = wsConn.Close()
	if err != nil {
		t.Fatalf("Failed reading Engine.IO open packet over WebSocket: %v", err)
	}
	if !strings.HasPrefix(string(msg), "0{") {
		t.Fatalf("Expected Engine.IO open packet over WebSocket, got %q", string(msg))
	}

	evilHdr := http.Header{}
	evilHdr.Set("Origin", "https://evil.example")
	badWSConn, badWSResp, err := websocket.DefaultDialer.Dial(wsURL, evilHdr)
	if err == nil {
		_ = badWSConn.Close()
		t.Fatalf("Expected WebSocket upgrade from unlisted origin to fail")
	}
	if badWSResp == nil || badWSResp.StatusCode != http.StatusForbidden {
		status := 0
		if badWSResp != nil {
			status = badWSResp.StatusCode
		}
		t.Fatalf("Expected 403 Forbidden on unlisted WebSocket origin, got %d (err: %v)", status, err)
	}
}

func TestSocketServer_JWTAuthAndDisconnectSafety(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	// Connect an authenticated lobby observer first to verify rejected handshakes emit zero presence.updated events
	obsToken := makeTestJWT(t, "observer-1", "obs@example.com", "USER")
	observer, pkt := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, obsToken))
	defer observer.close()
	if pkt.Prefix != "40" {
		t.Fatalf("expected observer connect ack 40, got %q", pkt.Raw)
	}
	if _, ok := observer.waitForEvent(contracts.EventPresenceSnapshot, time.Second); !ok {
		t.Fatalf("expected initial presence.snapshot for observer")
	}
	if err := observer.emitCommand(contracts.CmdLobbyJoin, map[string]string{"requestId": "obs-lobby"}); err != nil {
		t.Fatalf("observer lobby.join emit failed: %v", err)
	}
	if ack, ok := observer.waitForAck("obs-lobby", time.Second); !ok || !ack.Ok {
		t.Fatalf("expected observer lobby.join ack ok=true, got ok=%v ack=%+v", ok, ack)
	}
	// Drain snapshot emitted by lobby.join
	_, _ = observer.waitForEvent(contracts.EventPresenceSnapshot, 500*time.Millisecond)

	// Construct an unsigned alg=none JWT
	algNoneHeader := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	algNoneClaims := base64.RawURLEncoding.EncodeToString([]byte(`{"user_id":"forged-none","role":"ADMIN"}`))
	algNoneToken := algNoneHeader + "." + algNoneClaims + "."

	invalidPayloads := []struct {
		name    string
		payload string
	}{
		{name: "empty auth", payload: `{}`},
		{name: "forged user_id without token", payload: `{"user_id":"forged-user","role":"ADMIN"}`},
		{name: "invalid jwt string", payload: `{"token":"Bearer invalid.jwt.token"}`},
		{name: "alg none jwt", payload: fmt.Sprintf(`{"token":"Bearer %s"}`, algNoneToken)},
	}

	for _, tc := range invalidPayloads {
		t.Run(tc.name, func(t *testing.T) {
			badClient, respPkt := dialWSSocketClient(t, ts.URL, tc.payload)
			defer badClient.close()

			if respPkt.Prefix == "40" {
				// gsocketio sends 40{"sid":...} followed immediately by 44{"message":"Not authorized"}
				errPkt, err := badClient.nextControlPacket(time.Second)
				if err != nil {
					t.Fatalf("expected 44 error packet after 40 for %s, got err=%v", tc.name, err)
				}
				respPkt = errPkt
			}
			if respPkt.Prefix != "44" {
				t.Fatalf("expected 44 connect error packet for %s, got %q", tc.name, respPkt.Raw)
			}

			time.Sleep(40 * time.Millisecond)
			if got := sockServer.OnlineUserCount(); got != 1 {
				t.Fatalf("OnlineUserCount = %d, want 1 (observer only)", got)
			}
			if got := sockServer.TotalConnectionsCount(); got != 1 {
				t.Fatalf("TotalConnectionsCount = %d, want 1 (observer only)", got)
			}
			if ev, ok := observer.waitForEvent(contracts.EventPresenceUpdated, 80*time.Millisecond); ok {
				t.Fatalf("rejected connection must not broadcast presence.updated, got %s", ev.Raw)
			}
		})
	}

	// Forged user_id alongside a valid JWT must be ignored in favor of JWT claims
	realToken := makeTestJWT(t, "verified-user-99", "verified@example.com", "USER")
	realClient, realPkt := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s","user_id":"forged-admin","userId":"forged-admin","role":"ADMIN"}`, realToken))
	defer realClient.close()
	if realPkt.Prefix != "40" {
		t.Fatalf("expected 40 connect packet, got %q", realPkt.Raw)
	}

	if err := realClient.emitCommand(contracts.CmdRoomJoin, map[string]any{
		"requestId": "join-spoof-check",
		"roomId":    "spoof-check-room",
		"userId":    "forged-admin",
		"user_id":   "forged-admin",
		"role":      "host",
	}); err != nil {
		t.Fatalf("room.join failed: %v", err)
	}
	ack, ok := realClient.waitForAck("join-spoof-check", time.Second)
	if !ok || !ack.Ok {
		t.Fatalf("expected room.join ack ok=true, got ok=%v ack=%+v", ok, ack)
	}

	sockServer.userSocketMu.RLock()
	var recordedUID string
	for _, ps := range sockServer.peerStates["spoof-check-room"] {
		recordedUID = ps.UserID
	}
	sockServer.userSocketMu.RUnlock()
	if recordedUID != "verified-user-99" {
		t.Fatalf("peerState userId = %q, want %q (forged user_id must be ignored)", recordedUID, "verified-user-99")
	}
}

func TestSocketServer_RoomJoinSameUserDeduplicationAndGuestDisplayName(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	guestToken := makeTestJWT(t, "user20261113224", "", "USER")

	// Socket 1 joins room
	sock1, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, guestToken))
	defer sock1.close()
	_, _ = sock1.waitForEvent(contracts.EventPresenceSnapshot, time.Second)

	if err := sock1.emitCommand(contracts.CmdRoomJoin, map[string]interface{}{
		"requestId": "join-1",
		"roomId":    "room-dedup-test",
	}); err != nil {
		t.Fatalf("sock1 room.join failed: %v", err)
	}
	ack1, ok := sock1.waitForAck("join-1", time.Second)
	if !ok || !ack1.Ok {
		t.Fatalf("expected sock1 room.join ok, got ok=%v ack=%+v", ok, ack1)
	}

	var join1Data contracts.RoomJoinAckDTO
	if err := json.Unmarshal(*ack1.Data, &join1Data); err != nil {
		t.Fatalf("failed decoding RoomJoinAckDTO for sock1: %v", err)
	}

	// Verify displayName for guest user formatted as Guest #224
	sockServer.userSocketMu.RLock()
	ps1 := sockServer.peerStates["room-dedup-test"][join1Data.SocketID]
	roomCount1 := len(sockServer.socketRoom["room-dedup-test"])
	sockServer.userSocketMu.RUnlock()

	if ps1 == nil {
		t.Fatalf("expected peerState for sock1")
	}
	summary1 := buildUserSummaryDTO(ps1.UserID, ps1.Email)
	if summary1.Name != "Guest #224" || !summary1.IsGuest {
		t.Fatalf("summary Name=%q IsGuest=%v, want 'Guest #224' and true", summary1.Name, summary1.IsGuest)
	}
	if roomCount1 != 1 {
		t.Fatalf("socketRoom count = %d, want 1", roomCount1)
	}

	// Socket 2 connects for the SAME user (reconnect/reload) and joins the same room
	sock2, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, guestToken))
	defer sock2.close()
	_, _ = sock2.waitForEvent(contracts.EventPresenceSnapshot, time.Second)

	if err := sock2.emitCommand(contracts.CmdRoomJoin, map[string]interface{}{
		"requestId": "join-2",
		"roomId":    "room-dedup-test",
	}); err != nil {
		t.Fatalf("sock2 room.join failed: %v", err)
	}
	ack2, ok := sock2.waitForAck("join-2", time.Second)
	if !ok || !ack2.Ok {
		t.Fatalf("expected sock2 room.join ok, got ok=%v ack=%+v", ok, ack2)
	}
	var join2Data contracts.RoomJoinAckDTO
	if err := json.Unmarshal(*ack2.Data, &join2Data); err != nil {
		t.Fatalf("failed decoding RoomJoinAckDTO for sock2: %v", err)
	}

	// Verify socketRoom evicted sock1 and now only contains sock2
	sockServer.userSocketMu.RLock()
	roomCount2 := len(sockServer.socketRoom["room-dedup-test"])
	hasSock1 := sockServer.socketRoom["room-dedup-test"][join1Data.SocketID]
	hasSock2 := sockServer.socketRoom["room-dedup-test"][join2Data.SocketID]
	sockServer.userSocketMu.RUnlock()

	if roomCount2 != 1 {
		t.Fatalf("socketRoom count after rejoin = %d, want 1 (stale socket evicted)", roomCount2)
	}
	if hasSock1 {
		t.Fatalf("stale socket1 still present in socketRoom")
	}
	if !hasSock2 {
		t.Fatalf("active socket2 missing from socketRoom")
	}
}

func TestSocketServer_MultiTabPresenceAndDisconnect(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	// Observer subscribes to lobby
	obsToken := makeTestJWT(t, "lobby-observer", "obs@example.com", "USER")
	observer, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, obsToken))
	defer observer.close()
	_, _ = observer.waitForEvent(contracts.EventPresenceSnapshot, time.Second)
	_ = observer.emitCommand(contracts.CmdLobbyJoin, map[string]string{"requestId": "obs-1"})
	_, _ = observer.waitForAck("obs-1", time.Second)
	_, _ = observer.waitForEvent(contracts.EventPresenceSnapshot, time.Second)

	userToken := makeTestJWT(t, "multi-tab-user", "multi@example.com", "USER")

	// Tab 1 connects
	tab1, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, userToken))
	_, _ = tab1.waitForEvent(contracts.EventPresenceSnapshot, time.Second)

	evUp1, ok := observer.waitForEvent(contracts.EventPresenceUpdated, time.Second)
	if !ok {
		t.Fatalf("expected presence.updated when first tab connects")
	}
	env1 := assertValidEnvelope(t, evUp1.Payload, contracts.EventPresenceUpdated)
	var p1 contracts.PresenceDTO
	_ = json.Unmarshal(env1.Data, &p1)
	if p1.OnlineCount != 2 {
		t.Fatalf("expected onlineCount 2 after tab1 connects, got %d", p1.OnlineCount)
	}
	if sockServer.OnlineUserCount() != 2 || sockServer.TotalConnectionsCount() != 2 {
		t.Fatalf("unexpected counts after tab1: users=%d conns=%d", sockServer.OnlineUserCount(), sockServer.TotalConnectionsCount())
	}

	// Tab 2 connects for same user
	tab2, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, userToken))
	_, _ = tab2.waitForEvent(contracts.EventPresenceSnapshot, time.Second)

	if sockServer.OnlineUserCount() != 2 {
		t.Fatalf("OnlineUserCount = %d, want 2 with 2 tabs for same user", sockServer.OnlineUserCount())
	}
	if sockServer.TotalConnectionsCount() != 3 {
		t.Fatalf("TotalConnectionsCount = %d, want 3", sockServer.TotalConnectionsCount())
	}

	// Tab 1 disconnects -> user stays online
	_ = tab1.sendRaw("41")
	tab1.close()
	time.Sleep(60 * time.Millisecond)

	if sockServer.OnlineUserCount() != 2 {
		t.Fatalf("OnlineUserCount = %d after closing 1st tab, want 2", sockServer.OnlineUserCount())
	}
	if evUnexpected, ok := observer.waitForEvent(contracts.EventPresenceUpdated, 80*time.Millisecond); ok {
		t.Fatalf("did not expect presence.updated when user still has active tab2, got %s", evUnexpected.Raw)
	}

	// Tab 2 disconnects -> user drops offline and presence.updated broadcasts
	_ = tab2.sendRaw("41")
	tab2.close()

	evUp2, ok := observer.waitForEvent(contracts.EventPresenceUpdated, time.Second)
	if !ok {
		t.Fatalf("expected presence.updated after last tab disconnected")
	}
	env2 := assertValidEnvelope(t, evUp2.Payload, contracts.EventPresenceUpdated)
	var p2 contracts.PresenceDTO
	_ = json.Unmarshal(env2.Data, &p2)
	if p2.OnlineCount != 1 {
		t.Fatalf("expected onlineCount 1 after tab2 disconnects, got %d", p2.OnlineCount)
	}
	if sockServer.OnlineUserCount() != 1 {
		t.Fatalf("OnlineUserCount = %d, want 1", sockServer.OnlineUserCount())
	}
}

func TestSocketServer_CommandAck(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	tokA := makeTestJWT(t, "user-a", "alice@example.com", "USER")
	tokB := makeTestJWT(t, "user-b", "bob@example.com", "USER")

	clientA, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, tokA))
	defer clientA.close()
	snapA, ok := clientA.waitForEvent(contracts.EventPresenceSnapshot, time.Second)
	if !ok {
		t.Fatalf("expected initial presence.snapshot for clientA")
	}
	assertValidEnvelope(t, snapA.Payload, contracts.EventPresenceSnapshot)

	clientB, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, tokB))
	defer clientB.close()
	_, _ = clientB.waitForEvent(contracts.EventPresenceSnapshot, time.Second)

	// 1. lobby.join + presence.request
	_ = clientA.emitCommand(contracts.CmdLobbyJoin, map[string]string{"requestId": "req-lobby-join"})
	ackLobby, ok := clientA.waitForAck("req-lobby-join", time.Second)
	if !ok || !ackLobby.Ok {
		t.Fatalf("lobby.join ack failed: ok=%v ack=%+v", ok, ackLobby)
	}
	if ev, ok := clientA.waitForEvent(contracts.EventPresenceSnapshot, time.Second); !ok {
		t.Fatalf("expected presence.snapshot after lobby.join")
	} else {
		assertValidEnvelope(t, ev.Payload, contracts.EventPresenceSnapshot)
	}

	_ = clientA.emitCommand(contracts.CmdPresenceRequest, map[string]string{"requestId": "req-pres"})
	if ack, ok := clientA.waitForAck("req-pres", time.Second); !ok || !ack.Ok {
		t.Fatalf("presence.request ack failed: ok=%v ack=%+v", ok, ack)
	}

	// 2. room.join for A and B
	roomID := "canonical-room-1"
	_ = clientA.emitCommand(contracts.CmdRoomJoin, map[string]any{"requestId": "req-join-a", "roomId": roomID})
	ackJoinA, ok := clientA.waitForAck("req-join-a", time.Second)
	if !ok || !ackJoinA.Ok {
		t.Fatalf("room.join A failed: ok=%v ack=%+v", ok, ackJoinA)
	}
	// Ensure command.ack fires strictly once
	if dupAck, ok := clientA.waitForAck("req-join-a", 80*time.Millisecond); ok {
		t.Fatalf("duplicate command.ack received for req-join-a: %+v", dupAck)
	}

	if evSnapA, ok := clientA.waitForEvent(contracts.EventRoomSnapshot, time.Second); !ok {
		t.Fatalf("expected room.snapshot for clientA")
	} else {
		assertValidEnvelope(t, evSnapA.Payload, contracts.EventRoomSnapshot)
	}
	if evCreated, ok := clientA.waitForEvent(contracts.EventLobbyRoomCreated, time.Second); !ok {
		t.Fatalf("expected lobby.room_created when room count == 1")
	} else {
		assertValidEnvelope(t, evCreated.Payload, contracts.EventLobbyRoomCreated)
	}

	_ = clientB.emitCommand(contracts.CmdRoomJoin, map[string]any{"requestId": "req-join-b", "roomId": roomID})
	ackJoinB, ok := clientB.waitForAck("req-join-b", time.Second)
	if !ok || !ackJoinB.Ok {
		t.Fatalf("room.join B failed: ok=%v ack=%+v", ok, ackJoinB)
	}
	var joinBData contracts.RoomJoinAckDTO
	if err := json.Unmarshal(*ackJoinB.Data, &joinBData); err != nil {
		t.Fatalf("failed decoding RoomJoinAckDTO: %v", err)
	}
	if len(joinBData.Snapshot.Peers) != 1 || joinBData.Snapshot.Peers[0].UserID != "user-a" {
		t.Fatalf("expected clientA in clientB snapshot peers, got %+v", joinBData.Snapshot.Peers)
	}
	if evJoined, ok := clientA.waitForEvent(contracts.EventRoomMemberJoined, time.Second); !ok {
		t.Fatalf("expected room.member_joined on clientA when clientB joined")
	} else {
		assertValidEnvelope(t, evJoined.Payload, contracts.EventRoomMemberJoined)
	}

	// 3. voice.mute & voice.speaking
	_ = clientA.emitCommand(contracts.CmdVoiceMute, map[string]any{
		"requestId": "req-mute",
		"roomId":    roomID,
		"isMuted":   true,
	})
	if ack, ok := clientA.waitForAck("req-mute", time.Second); !ok || !ack.Ok {
		t.Fatalf("voice.mute ack failed: ok=%v ack=%+v", ok, ack)
	}
	if evMute, ok := clientB.waitForEvent(contracts.EventVoiceMuteChanged, time.Second); !ok {
		t.Fatalf("expected voice.mute_changed on clientB")
	} else {
		assertValidEnvelope(t, evMute.Payload, contracts.EventVoiceMuteChanged)
	}

	_ = clientA.emitCommand(contracts.CmdVoiceSpeaking, map[string]any{
		"requestId":  "req-speak",
		"roomId":     roomID,
		"isSpeaking": true,
		"level":      0.75,
	})
	if ack, ok := clientA.waitForAck("req-speak", time.Second); !ok || !ack.Ok {
		t.Fatalf("voice.speaking ack failed: ok=%v ack=%+v", ok, ack)
	}
	if evSpeak, ok := clientB.waitForEvent(contracts.EventVoiceSpeakingChanged, time.Second); !ok {
		t.Fatalf("expected voice.speaking_changed on clientB")
	} else {
		assertValidEnvelope(t, evSpeak.Payload, contracts.EventVoiceSpeakingChanged)
	}

	// 4. screen_share.start & screen_share.stop
	_ = clientA.emitCommand(contracts.CmdScreenShareStart, map[string]any{"requestId": "req-ss-start", "roomId": roomID})
	if ack, ok := clientA.waitForAck("req-ss-start", time.Second); !ok || !ack.Ok {
		t.Fatalf("screen_share.start ack failed: ok=%v ack=%+v", ok, ack)
	}
	if evSS, ok := clientB.waitForEvent(contracts.EventScreenShareStarted, time.Second); !ok {
		t.Fatalf("expected screen_share.started on clientB")
	} else {
		assertValidEnvelope(t, evSS.Payload, contracts.EventScreenShareStarted)
	}

	_ = clientA.emitCommand(contracts.CmdScreenShareStop, map[string]any{"requestId": "req-ss-stop", "roomId": roomID})
	if ack, ok := clientA.waitForAck("req-ss-stop", time.Second); !ok || !ack.Ok {
		t.Fatalf("screen_share.stop ack failed: ok=%v ack=%+v", ok, ack)
	}
	if evSSStop, ok := clientB.waitForEvent(contracts.EventScreenShareStopped, time.Second); !ok {
		t.Fatalf("expected screen_share.stopped on clientB")
	} else {
		assertValidEnvelope(t, evSSStop.Payload, contracts.EventScreenShareStopped)
	}

	// 5. chat.react
	_ = clientA.emitCommand(contracts.CmdChatReact, map[string]any{
		"requestId": "req-react",
		"roomId":    roomID,
		"messageId": "msg-123",
		"emoji":     "🔥",
	})
	if ack, ok := clientA.waitForAck("req-react", time.Second); !ok || !ack.Ok {
		t.Fatalf("chat.react ack failed: ok=%v ack=%+v", ok, ack)
	}
	if evReact, ok := clientB.waitForEvent(contracts.EventChatReactionUpdated, time.Second); !ok {
		t.Fatalf("expected chat.reaction_updated on clientB")
	} else {
		envReact := assertValidEnvelope(t, evReact.Payload, contracts.EventChatReactionUpdated)
		var reactDTO contracts.ChatReactionUpdatedDTO
		_ = json.Unmarshal(envReact.Data, &reactDTO)
		if reactDTO.UserID != "user-a" {
			t.Fatalf("chat.reaction_updated userId = %q, want user-a", reactDTO.UserID)
		}
	}

	// 6. mesh.ping
	_ = clientA.emitCommand(contracts.CmdMeshPing, map[string]any{"requestId": "req-ping"})
	if ack, ok := clientA.waitForAck("req-ping", time.Second); !ok || !ack.Ok {
		t.Fatalf("mesh.ping ack failed: ok=%v ack=%+v", ok, ack)
	} else {
		var pingAck contracts.MeshPingAckDTO
		if err := json.Unmarshal(*ack.Data, &pingAck); err != nil || pingAck.ServerTime <= 0 {
			t.Fatalf("invalid MeshPingAckDTO: %+v err=%v", pingAck, err)
		}
	}

	// 7. room.leave + lobby.leave
	_ = clientA.emitCommand(contracts.CmdRoomLeave, map[string]any{"requestId": "req-leave-a", "roomId": roomID})
	if ack, ok := clientA.waitForAck("req-leave-a", time.Second); !ok || !ack.Ok {
		t.Fatalf("room.leave ack failed: ok=%v ack=%+v", ok, ack)
	}
	if evLeft, ok := clientB.waitForEvent(contracts.EventRoomMemberLeft, time.Second); !ok {
		t.Fatalf("expected room.member_left on clientB")
	} else {
		assertValidEnvelope(t, evLeft.Payload, contracts.EventRoomMemberLeft)
	}

	_ = clientA.emitCommand(contracts.CmdLobbyLeave, map[string]string{"requestId": "req-lobby-leave"})
	if ack, ok := clientA.waitForAck("req-lobby-leave", time.Second); !ok || !ack.Ok {
		t.Fatalf("lobby.leave ack failed: ok=%v ack=%+v", ok, ack)
	}
}

func TestSocketServer_AuthorizationCrossRoomAndModeration(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	tokHost := makeTestJWT(t, "host-1", "host@example.com", "USER")
	tokPeer := makeTestJWT(t, "peer-2", "peer@example.com", "USER")
	tokOutsider := makeTestJWT(t, "outsider-3", "out@example.com", "USER")

	hostClient, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, tokHost))
	defer hostClient.close()
	peerClient, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, tokPeer))
	defer peerClient.close()
	outsiderClient, _ := dialWSSocketClient(t, ts.URL, fmt.Sprintf(`{"token":"Bearer %s"}`, tokOutsider))
	defer outsiderClient.close()

	roomA := "auth-room-a"
	roomB := "auth-room-b"

	// Host and Peer join roomA; Outsider joins roomB
	_ = hostClient.emitCommand(contracts.CmdRoomJoin, map[string]any{"requestId": "h-join", "roomId": roomA})
	hAck, ok := hostClient.waitForAck("h-join", time.Second)
	if !ok || !hAck.Ok {
		t.Fatalf("host join failed")
	}
	var hJoinData contracts.RoomJoinAckDTO
	_ = json.Unmarshal(*hAck.Data, &hJoinData)

	_ = peerClient.emitCommand(contracts.CmdRoomJoin, map[string]any{"requestId": "p-join", "roomId": roomA})
	pAck, ok := peerClient.waitForAck("p-join", time.Second)
	if !ok || !pAck.Ok {
		t.Fatalf("peer join failed")
	}
	var pJoinData contracts.RoomJoinAckDTO
	_ = json.Unmarshal(*pAck.Data, &pJoinData)

	_ = outsiderClient.emitCommand(contracts.CmdRoomJoin, map[string]any{"requestId": "o-join", "roomId": roomB})
	oAck, ok := outsiderClient.waitForAck("o-join", time.Second)
	if !ok || !oAck.Ok {
		t.Fatalf("outsider join failed")
	}
	var oJoinData contracts.RoomJoinAckDTO
	_ = json.Unmarshal(*oAck.Data, &oJoinData)

	// 1. Unjoined socket receives FORBIDDEN on in-room commands targeting roomA
	forbiddenCmds := []struct {
		cmd     string
		payload map[string]any
	}{
		{contracts.CmdVoiceMute, map[string]any{"requestId": "f-mute", "roomId": roomA, "isMuted": true}},
		{contracts.CmdScreenShareStart, map[string]any{"requestId": "f-ss1", "roomId": roomA}},
		{contracts.CmdScreenShareStop, map[string]any{"requestId": "f-ss2", "roomId": roomA}},
		{contracts.CmdChatReact, map[string]any{"requestId": "f-react", "roomId": roomA, "messageId": "m1", "emoji": "👍"}},
		{contracts.CmdRoomLeave, map[string]any{"requestId": "f-leave", "roomId": roomA}},
		{contracts.CmdWebRTCOffer, map[string]any{"requestId": "f-offer", "roomId": roomA, "targetSocketId": hJoinData.SocketID, "sdp": map[string]string{"type": "offer"}}},
		{contracts.CmdWebRTCAnswer, map[string]any{"requestId": "f-ans", "roomId": roomA, "targetSocketId": hJoinData.SocketID, "sdp": map[string]string{"type": "answer"}}},
		{contracts.CmdWebRTCIceCandidate, map[string]any{"requestId": "f-ice", "roomId": roomA, "targetSocketId": hJoinData.SocketID, "candidate": map[string]string{"candidate": "cand"}}},
	}
	for _, tc := range forbiddenCmds {
		reqID := tc.payload["requestId"].(string)
		_ = outsiderClient.emitCommand(tc.cmd, tc.payload)
		ack, ok := outsiderClient.waitForAck(reqID, time.Second)
		if !ok || ack.Ok || ack.Error == nil || ack.Error.Code != contracts.ErrCodeForbidden {
			t.Fatalf("expected FORBIDDEN ack for unjoined %s, got ok=%v ack=%+v", tc.cmd, ok, ack)
		}
	}

	// 2. WebRTC cross-room isolation & self-target validation
	_ = hostClient.emitCommand(contracts.CmdWebRTCOffer, map[string]any{
		"requestId":      "cross-room-offer",
		"roomId":         roomA,
		"targetSocketId": oJoinData.SocketID, // outsider is in roomB, not roomA
		"sdp":            map[string]string{"type": "offer", "sdp": "v=0"},
	})
	if ack, ok := hostClient.waitForAck("cross-room-offer", time.Second); !ok || ack.Ok || ack.Error == nil || ack.Error.Code != contracts.ErrCodeForbidden {
		t.Fatalf("expected FORBIDDEN on cross-room webrtc.offer, got ok=%v ack=%+v", ok, ack)
	}

	_ = hostClient.emitCommand(contracts.CmdWebRTCIceCandidate, map[string]any{
		"requestId":      "self-ice",
		"roomId":         roomA,
		"targetSocketId": hJoinData.SocketID, // self
		"candidate":      map[string]string{"candidate": "cand"},
	})
	if ack, ok := hostClient.waitForAck("self-ice", time.Second); !ok || ack.Ok || ack.Error == nil || ack.Error.Code != contracts.ErrCodeValidation {
		t.Fatalf("expected VALIDATION_ERROR on self-target webrtc.ice_candidate, got ok=%v ack=%+v", ok, ack)
	}

	// Valid same-room WebRTC offer stamps senderSocketId + senderUserId
	_ = hostClient.emitCommand(contracts.CmdWebRTCOffer, map[string]any{
		"requestId":      "valid-offer",
		"roomId":         roomA,
		"targetSocketId": pJoinData.SocketID,
		"sdp":            map[string]string{"type": "offer", "sdp": "v=0"},
	})
	if ack, ok := hostClient.waitForAck("valid-offer", time.Second); !ok || !ack.Ok {
		t.Fatalf("expected ok=true on valid same-room webrtc.offer, got ok=%v ack=%+v", ok, ack)
	}
	if evOffer, ok := peerClient.waitForEvent(contracts.EventWebRTCOffer, time.Second); !ok {
		t.Fatalf("expected webrtc.offer on peerClient")
	} else {
		envOffer := assertValidEnvelope(t, evOffer.Payload, contracts.EventWebRTCOffer)
		var offerDTO contracts.WebRTCOfferDTO
		_ = json.Unmarshal(envOffer.Data, &offerDTO)
		if offerDTO.SenderSocketID != hJoinData.SocketID || offerDTO.SenderUserID != "host-1" {
			t.Fatalf("unexpected stamped offer sender: %+v", offerDTO)
		}
	}

	// 3. Moderation fail-closed when RoomAuthorizer is nil
	_ = hostClient.emitCommand(contracts.CmdModerationMuteParticipant, map[string]any{
		"requestId":    "nil-auth-mute",
		"roomId":       roomA,
		"targetUserId": "peer-2",
	})
	if ack, ok := hostClient.waitForAck("nil-auth-mute", time.Second); !ok || ack.Ok || ack.Error == nil || ack.Error.Code != contracts.ErrCodeForbidden {
		t.Fatalf("expected FORBIDDEN when RoomAuthorizer is nil, got ok=%v ack=%+v", ok, ack)
	}

	// 4. Wire mock RoomAuthorizer: peer-2 (non-host) fails closed; host-1 succeeds
	sockServer.SetRoomAuthorizer(&mockRoomAuthorizer{
		hosts: map[string]string{roomA: "host-1"},
	})

	_ = peerClient.emitCommand(contracts.CmdModerationLockRoom, map[string]any{
		"requestId": "nonhost-lock",
		"roomId":    roomA,
		"isLocked":  true,
	})
	if ack, ok := peerClient.waitForAck("nonhost-lock", time.Second); !ok || ack.Ok || ack.Error == nil || ack.Error.Code != contracts.ErrCodeForbidden {
		t.Fatalf("expected FORBIDDEN for non-host moderation.lock_room, got ok=%v ack=%+v", ok, ack)
	}

	// Host mutes peer-2
	_ = hostClient.emitCommand(contracts.CmdModerationMuteParticipant, map[string]any{
		"requestId":    "host-mute-peer",
		"roomId":       roomA,
		"targetUserId": "peer-2",
	})
	if ack, ok := hostClient.waitForAck("host-mute-peer", time.Second); !ok || !ack.Ok {
		t.Fatalf("expected host moderation.mute_participant ack ok=true, got ok=%v ack=%+v", ok, ack)
	}
	if evMuted, ok := peerClient.waitForEvent(contracts.EventVoiceMuteChanged, time.Second); !ok {
		t.Fatalf("expected forced voice.mute_changed on peerClient")
	} else {
		envMuted := assertValidEnvelope(t, evMuted.Payload, contracts.EventVoiceMuteChanged)
		var mDTO contracts.VoiceMuteChangedDTO
		_ = json.Unmarshal(envMuted.Data, &mDTO)
		if !mDTO.ForcedByHost || !mDTO.IsMuted || mDTO.UserID != "peer-2" {
			t.Fatalf("unexpected forced mute DTO: %+v", mDTO)
		}
	}

	// Host locks room
	_ = hostClient.emitCommand(contracts.CmdModerationLockRoom, map[string]any{
		"requestId": "host-lock",
		"roomId":    roomA,
		"isLocked":  true,
	})
	if ack, ok := hostClient.waitForAck("host-lock", time.Second); !ok || !ack.Ok {
		t.Fatalf("expected host moderation.lock_room ack ok=true, got ok=%v ack=%+v", ok, ack)
	}
	if evLock, ok := peerClient.waitForEvent(contracts.EventModerationRoomLocked, time.Second); !ok {
		t.Fatalf("expected moderation.room_locked on peerClient")
	} else {
		assertValidEnvelope(t, evLock.Payload, contracts.EventModerationRoomLocked)
	}

	// Host kicks peer-2
	_ = hostClient.emitCommand(contracts.CmdModerationKickParticipant, map[string]any{
		"requestId":    "host-kick",
		"roomId":       roomA,
		"targetUserId": "peer-2",
		"reason":       "disruptive",
	})
	if ack, ok := hostClient.waitForAck("host-kick", time.Second); !ok || !ack.Ok {
		t.Fatalf("expected host moderation.kick_participant ack ok=true, got ok=%v ack=%+v", ok, ack)
	}
	if evKicked, ok := peerClient.waitForEvent(contracts.EventModerationParticipantKicked, time.Second); !ok {
		t.Fatalf("expected moderation.participant_kicked on peerClient")
	} else {
		envKick := assertValidEnvelope(t, evKicked.Payload, contracts.EventModerationParticipantKicked)
		var kDTO contracts.ModerationParticipantKickedDTO
		_ = json.Unmarshal(envKick.Data, &kDTO)
		if kDTO.TargetUserID != "peer-2" || kDTO.KickedBy != "host-1" || kDTO.Reason != "disruptive" {
			t.Fatalf("unexpected kick DTO: %+v", kDTO)
		}
	}
	if evLeft, ok := hostClient.waitForEvent(contracts.EventRoomMemberLeft, time.Second); !ok {
		t.Fatalf("expected room.member_left on hostClient after kick")
	} else {
		envLeft := assertValidEnvelope(t, evLeft.Payload, contracts.EventRoomMemberLeft)
		var lDTO contracts.RoomMemberLeftDTO
		_ = json.Unmarshal(envLeft.Data, &lDTO)
		if lDTO.Reason != "kicked" || lDTO.UserID != "peer-2" {
			t.Fatalf("unexpected room.member_left DTO after kick: %+v", lDTO)
		}
	}
	if sockServer.isSocketMember(roomA, pJoinData.SocketID) {
		t.Fatalf("kicked peer socket should be removed from socketRoom")
	}
}

func TestSocketServer_ConcurrentAccess(t *testing.T) {
	sockServer, ts := setupTestServer(t)
	defer ts.Close()
	defer sockServer.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	var wg sync.WaitGroup
	numClients := 10

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tok, err := jwt.GenerateToken(fmt.Sprintf("user-%d", idx), fmt.Sprintf("u%d@example.com", idx), "USER", "", "")
			if err != nil {
				return
			}
			resp, err := client.Get(ts.URL + "/socket.io/?EIO=4&transport=polling")
			if err != nil {
				return
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			sid := extractSID(string(body))
			if sid == "" {
				return
			}

			connBody := bytes.NewBufferString(fmt.Sprintf(`40{"token":"Bearer %s"}`, tok))
			r, err := client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sid, "text/plain", connBody)
			if err == nil {
				_ = r.Body.Close()
			}

			joinBody := bytes.NewBufferString(fmt.Sprintf(`42["room.join",{"requestId":"c-%d","roomId":"concurrency-room"}]`, idx))
			r, err = client.Post(ts.URL+"/socket.io/?EIO=4&transport=polling&sid="+sid, "text/plain", joinBody)
			if err == nil {
				_ = r.Body.Close()
			}
		}(i)
	}

	wg.Wait()
	if sockServer.Server.RoomLen("/", "concurrency-room") != numClients {
		t.Fatalf("expected %d members in concurrency-room, got %d", numClients, sockServer.Server.RoomLen("/", "concurrency-room"))
	}
}

func TestSocketServer_IsSocketMember(t *testing.T) {
	server := &SocketServer{
		socketRoom: map[string]map[string]bool{
			"room-a": {
				"socket-a": true,
				"socket-b": true,
			},
		},
	}

	tests := []struct {
		name     string
		roomID   string
		socketID string
		want     bool
	}{
		{name: "member is accepted", roomID: "room-a", socketID: "socket-a", want: true},
		{name: "other room is rejected", roomID: "room-b", socketID: "socket-a", want: false},
		{name: "non-member is rejected", roomID: "room-a", socketID: "socket-c", want: false},
		{name: "missing room is rejected", roomID: "", socketID: "socket-a", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := server.isSocketMember(test.roomID, test.socketID); got != test.want {
				t.Fatalf("isSocketMember(%q, %q) = %v, want %v", test.roomID, test.socketID, got, test.want)
			}
		})
	}
}

func TestSocketServer_GinHandlerCORSAndWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opts := &gsocketio.Options{
		PingInterval: 5 * time.Second,
		PingTimeout:  3 * time.Second,
		MaxPayload:   512 * 1024,
	}
	sockServer, err := NewSocketServer(opts)
	if err != nil {
		t.Fatalf("NewSocketServer failed: %v", err)
	}
	defer sockServer.Close()

	go func() {
		_ = sockServer.Serve()
	}()

	r := gin.New()
	handler := sockServer.GinHandler()
	r.GET("/socket.io/*any", handler)
	r.POST("/socket.io/*any", handler)
	r.OPTIONS("/socket.io/*any", handler)

	ts := httptest.NewServer(r)
	defer ts.Close()

	client := &http.Client{Timeout: 5 * time.Second}

	// Allowlisted GET polling must not leak "*"
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/socket.io/?EIO=4&transport=polling", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GinHandler GET polling failed: %v", err)
	}
	_ = resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("GinHandler GET ACAO = %q, want %q", got, "http://localhost:3000")
	}

	// Unlisted GET polling must return 403
	evilReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/socket.io/?EIO=4&transport=polling", nil)
	evilReq.Header.Set("Origin", "https://evil.example")
	evilResp, err := client.Do(evilReq)
	if err != nil {
		t.Fatalf("GinHandler unlisted GET failed: %v", err)
	}
	_ = evilResp.Body.Close()
	if evilResp.StatusCode != http.StatusForbidden {
		t.Fatalf("GinHandler unlisted status = %d, want 403", evilResp.StatusCode)
	}

	// Allowlisted WebSocket upgrade through GinHandler must succeed via Hijacker
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/socket.io/?EIO=4&transport=websocket"
	allowHdr := http.Header{}
	allowHdr.Set("Origin", "http://localhost:3000")
	wsConn, wsResp, err := websocket.DefaultDialer.Dial(wsURL, allowHdr)
	if err != nil {
		status := 0
		if wsResp != nil {
			status = wsResp.StatusCode
		}
		t.Fatalf("GinHandler WebSocket upgrade failed: status=%d err=%v", status, err)
	}
	_ = wsConn.Close()
}

func TestSocketServer_NewSocketServerWithOriginsAndSetAllowedOrigins(t *testing.T) {
	opts := &gsocketio.Options{
		PingInterval: 5 * time.Second,
		PingTimeout:  3 * time.Second,
		MaxPayload:   512 * 1024,
	}
	sockServer, err := NewSocketServerWithOrigins([]string{"https://app.example.com/"}, opts)
	if err != nil {
		t.Fatalf("NewSocketServerWithOrigins failed: %v", err)
	}
	defer sockServer.Close()

	go func() {
		_ = sockServer.Serve()
	}()

	ts := httptest.NewServer(sockServer)
	defer ts.Close()
	client := &http.Client{Timeout: 5 * time.Second}

	// Default localhost:3000 is rejected when only https://app.example.com is configured
	reqLocal, _ := http.NewRequest(http.MethodGet, ts.URL+"/socket.io/?EIO=4&transport=polling", nil)
	reqLocal.Header.Set("Origin", "http://localhost:3000")
	respLocal, err := client.Do(reqLocal)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = respLocal.Body.Close()
	if respLocal.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for unconfigured localhost origin, got %d", respLocal.StatusCode)
	}

	// Configured origin (with trailing slash normalized) is accepted without "*"
	reqApp, _ := http.NewRequest(http.MethodGet, ts.URL+"/socket.io/?EIO=4&transport=polling", nil)
	reqApp.Header.Set("Origin", "https://app.example.com")
	respApp, err := client.Do(reqApp)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = respApp.Body.Close()
	if respApp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for allowlisted app origin, got %d", respApp.StatusCode)
	}
	if got := respApp.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("expected ACAO https://app.example.com, got %q", got)
	}

	// Updating allowlist via SetAllowedOrigins switches permitted origins
	sockServer.SetAllowedOrigins([]string{"https://studio.example.com"})
	reqOld, _ := http.NewRequest(http.MethodGet, ts.URL+"/socket.io/?EIO=4&transport=polling", nil)
	reqOld.Header.Set("Origin", "https://app.example.com")
	respOld, err := client.Do(reqOld)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	_ = respOld.Body.Close()
	if respOld.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 after SetAllowedOrigins removed old origin, got %d", respOld.StatusCode)
	}
}
