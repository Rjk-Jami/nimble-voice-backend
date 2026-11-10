package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nimble-voice-backend/utils/jwt"

	golangjwt "github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
)

func TestMemoryPresenceStoreCountsUniqueLearners(t *testing.T) {
	store := newMemoryPresenceStore()
	first, count := store.AddConnection("learner-a", "tab-a")
	if !first || count != 1 {
		t.Fatalf("first connection = (%v, %d), want (true, 1)", first, count)
	}
	first, count = store.AddConnection("learner-a", "tab-b")
	if first || count != 1 {
		t.Fatalf("second tab = (%v, %d), want (false, 1)", first, count)
	}
	first, count = store.AddConnection("learner-b", "tab-c")
	if !first || count != 2 {
		t.Fatalf("second learner = (%v, %d), want (true, 2)", first, count)
	}
	last, count := store.RemoveConnection("learner-a", "tab-a")
	if last || count != 2 {
		t.Fatalf("first tab removed = (%v, %d), want (false, 2)", last, count)
	}
	last, count = store.RemoveConnection("learner-a", "tab-b")
	if !last || count != 1 {
		t.Fatalf("last tab removed = (%v, %d), want (true, 1)", last, count)
	}
	last, count = store.RemoveConnection("learner-a", "tab-b")
	if last || count != 1 {
		t.Fatalf("duplicate removal = (%v, %d), want (false, 1)", last, count)
	}
}

func TestMemoryPresenceStoreConcurrentConnections(t *testing.T) {
	store := newMemoryPresenceStore()
	const connections = 100
	var wg sync.WaitGroup
	for i := 0; i < connections; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.AddConnection("learner", strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	if got := store.OnlineCount(); got != 1 {
		t.Fatalf("online count after concurrent add = %d, want 1", got)
	}
	for i := 0; i < connections; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.RemoveConnection("learner", strconv.Itoa(i))
		}(i)
	}
	wg.Wait()
	if got := store.OnlineCount(); got != 0 {
		t.Fatalf("online count after concurrent remove = %d, want 0", got)
	}
}

func newPresenceTestServer(t *testing.T, origins []string) (*PresenceServer, *httptest.Server) {
	t.Helper()
	presence := NewPresenceServer(origins, time.Hour, 2*time.Hour)
	httpServer := httptest.NewServer(presence)
	t.Cleanup(func() {
		presence.Close()
		httpServer.Close()
	})
	return presence, httpServer
}

func dialPresence(t *testing.T, httpServer *httptest.Server, origin, userID string) (*websocket.Conn, *http.Response) {
	t.Helper()
	token, err := jwt.GenerateToken(userID, "", "user", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return dialPresenceToken(t, httpServer, origin, token)
}

func dialPresenceToken(t *testing.T, httpServer *httptest.Server, origin, token string) (*websocket.Conn, *http.Response) {
	t.Helper()
	dialer := websocket.Dialer{HandshakeTimeout: time.Second}
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, response, err := dialer.Dial(wsURL, http.Header{
		"Origin": []string{origin},
		"Cookie": []string{presenceCookieName + "=" + token},
	})
	if err != nil {
		return nil, response
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn, response
}

func readPresenceCount(t *testing.T, conn *websocket.Conn, wantType string, wantCount int) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read presence message: %v", err)
	}
	var message presenceEnvelope
	if err := json.Unmarshal(payload, &message); err != nil {
		t.Fatalf("decode presence message: %v", err)
	}
	if message.Version != 1 || message.Type != wantType || message.Data.OnlineLearners != wantCount {
		t.Fatalf("message = %+v, want version=1 type=%s count=%d", message, wantType, wantCount)
	}
}

func TestPresenceBroadcastsUniqueCountAndKeepsMultipleTabsOnline(t *testing.T) {
	_, httpServer := newPresenceTestServer(t, []string{"http://localhost:3000"})
	a1, _ := dialPresence(t, httpServer, "http://localhost:3000", "learner-a")
	if a1 == nil {
		t.Fatal("first learner connection failed")
	}
	readPresenceCount(t, a1, "presence:sync", 1)

	a2, _ := dialPresence(t, httpServer, "http://localhost:3000", "learner-a")
	if a2 == nil {
		t.Fatal("second tab connection failed")
	}
	readPresenceCount(t, a2, "presence:sync", 1)

	b, _ := dialPresence(t, httpServer, "http://localhost:3000", "learner-b")
	if b == nil {
		t.Fatal("second learner connection failed")
	}
	readPresenceCount(t, b, "presence:sync", 2)
	readPresenceCount(t, a1, "presence:count", 2)
	readPresenceCount(t, a2, "presence:count", 2)

	_ = a1.Close()
	_ = a2.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err := a2.ReadMessage(); err == nil {
		t.Fatal("closing one of two tabs should not broadcast a count change")
	}
	_ = a2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_ = a2.Close()
	readPresenceCount(t, b, "presence:count", 1)
}

func TestPresenceRejectsBadOriginAndMissingAuthentication(t *testing.T) {
	_, httpServer := newPresenceTestServer(t, []string{"http://localhost:3000"})
	_, response := dialPresence(t, httpServer, "https://attacker.example", "learner-a")
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("bad origin response = %v, want HTTP 403", response)
	}

	dialer := websocket.Dialer{HandshakeTimeout: time.Second}
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	_, response, err := dialer.Dial(wsURL, http.Header{"Origin": []string{"http://localhost:3000"}})
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing auth response = (%v, %v), want HTTP 401", response, err)
	}
}

func TestPresenceRejectsInvalidAndExpiredTokens(t *testing.T) {
	_, httpServer := newPresenceTestServer(t, []string{"http://localhost:3000"})
	_, response := dialPresenceToken(t, httpServer, "http://localhost:3000", "not-a-jwt")
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid token response = %v, want HTTP 401", response)
	}

	expired, err := golangjwt.NewWithClaims(golangjwt.SigningMethodHS256, golangjwt.MapClaims{
		"user_id": "expired-learner",
		"exp":     time.Now().Add(-time.Minute).Unix(),
	}).SignedString(jwt.SecretKey)
	if err != nil {
		t.Fatal(err)
	}
	_, response = dialPresenceToken(t, httpServer, "http://localhost:3000", expired)
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expired token response = %v, want HTTP 401", response)
	}
}

func TestPresenceRemovesStaleConnectionsAfterPongTimeout(t *testing.T) {
	presence := NewPresenceServer([]string{"http://localhost:3000"}, 20*time.Millisecond, 100*time.Millisecond)
	httpServer := httptest.NewServer(presence)
	defer httpServer.Close()
	defer presence.Close()

	conn, _ := dialPresence(t, httpServer, "http://localhost:3000", "stale-learner")
	if conn == nil {
		t.Fatal("learner connection failed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for presence.OnlineCount() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := presence.OnlineCount(); got != 0 {
		t.Fatalf("online count after missed pong = %d, want 0", got)
	}
}

func TestPresenceConnectionQueueIsBounded(t *testing.T) {
	conn := &presenceConn{send: make(chan []byte, 1), done: make(chan struct{})}
	if !conn.enqueue([]byte("first")) {
		t.Fatal("first enqueue should fit")
	}
	if conn.enqueue([]byte("second")) {
		t.Fatal("full queue should reject another message")
	}
}
