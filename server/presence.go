package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"nimble-voice-backend/utils/jwt"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	presenceCookieName = "nimble_presence"
	presenceQueueSize  = 16
	writeWait          = 10 * time.Second
	maxMessageSize     = 1024
)

type PresenceStore interface {
	AddConnection(userID, connectionID string) (firstConnection bool, onlineLearners int)
	RemoveConnection(userID, connectionID string) (lastConnection bool, onlineLearners int)
	OnlineCount() int
}

// memoryPresenceStore tracks unique learners and all of their active connections.
// It is local to one Go process; multi-instance deployments need a shared store and pub/sub.
type memoryPresenceStore struct {
	mu    sync.RWMutex
	users map[string]map[string]struct{}
}

func newMemoryPresenceStore() *memoryPresenceStore {
	return &memoryPresenceStore{users: make(map[string]map[string]struct{})}
}

func (s *memoryPresenceStore) AddConnection(userID, connectionID string) (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	connections := s.users[userID]
	first := connections == nil
	if first {
		connections = make(map[string]struct{})
		s.users[userID] = connections
	}
	connections[connectionID] = struct{}{}
	return first, len(s.users)
}

func (s *memoryPresenceStore) RemoveConnection(userID, connectionID string) (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	connections := s.users[userID]
	if connections == nil {
		return false, len(s.users)
	}
	delete(connections, connectionID)
	last := len(connections) == 0
	if last {
		delete(s.users, userID)
	}
	return last, len(s.users)
}

func (s *memoryPresenceStore) OnlineCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

type presenceEnvelope struct {
	Version int    `json:"version"`
	Type    string `json:"type"`
	Data    struct {
		OnlineLearners int `json:"onlineLearners"`
	} `json:"data"`
}

func presenceMessage(eventType string, count int) []byte {
	message := presenceEnvelope{Version: 1, Type: eventType}
	message.Data.OnlineLearners = count
	encoded, _ := json.Marshal(message)
	return encoded
}

type presenceConn struct {
	id        string
	userID    string
	expiresAt time.Time
	ws        *websocket.Conn
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

func (c *presenceConn) enqueue(message []byte) bool {
	select {
	case <-c.done:
		return false
	case c.send <- message:
		return true
	default:
		return false
	}
}

func (c *presenceConn) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.ws.Close()
	})
}

func (c *presenceConn) expire() {
	c.closeOnce.Do(func() {
		_ = c.ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(4001, "authentication expired"), time.Now().Add(writeWait))
		close(c.done)
		_ = c.ws.Close()
	})
}

type PresenceServer struct {
	mu          sync.Mutex // serializes store transitions and bounded queue ordering only
	store       PresenceStore
	connections map[string]*presenceConn
	origins     map[string]struct{}
	upgrader    websocket.Upgrader
	pingEvery   time.Duration
	pongTimeout time.Duration
	closed      bool
}

func NewPresenceServer(origins []string, pingEvery, pongTimeout time.Duration) *PresenceServer {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		origin = strings.TrimRight(strings.TrimSpace(origin), "/")
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	if pingEvery <= 0 {
		pingEvery = 25 * time.Second
	}
	if pongTimeout <= pingEvery {
		pongTimeout = 60 * time.Second
	}

	s := &PresenceServer{
		store:       newMemoryPresenceStore(),
		connections: make(map[string]*presenceConn),
		origins:     allowed,
		pingEvery:   pingEvery,
		pongTimeout: pongTimeout,
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     s.checkOrigin,
	}
	return s
}

func (s *PresenceServer) checkOrigin(r *http.Request) bool {
	origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
	if origin == "" {
		return false
	}
	_, ok := s.origins[origin]
	return ok
}

func (s *PresenceServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.checkOrigin(r) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if s.isClosed() {
		http.Error(w, "presence service unavailable", http.StatusServiceUnavailable)
		return
	}
	cookie, err := r.Cookie(presenceCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	claims, err := jwt.ParseToken(cookie.Value)
	if err != nil || claims == nil || claims.UserID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	connectionID := uuid.NewString()
	c := &presenceConn{
		id:     connectionID,
		userID: claims.UserID,
		expiresAt: func() time.Time {
			if claims.ExpiresAt == nil {
				return time.Time{}
			}
			return claims.ExpiresAt.Time
		}(),
		ws:   ws,
		send: make(chan []byte, presenceQueueSize),
		done: make(chan struct{}),
	}
	s.register(c)
	go c.writePump(s)
	s.readPump(c)
}

func (s *PresenceServer) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *PresenceServer) OnlineCount() int {
	return s.store.OnlineCount()
}

func (s *PresenceServer) register(c *presenceConn) {
	var slow []*presenceConn
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		c.close()
		return
	}
	first, count := s.store.AddConnection(c.userID, c.id)
	s.connections[c.id] = c
	_ = c.enqueue(presenceMessage("presence:sync", count))
	if first {
		message := presenceMessage("presence:count", count)
		for _, other := range s.connections {
			if other != c && !other.enqueue(message) {
				slow = append(slow, other)
			}
		}
	}
	s.mu.Unlock()
	for _, stalled := range slow {
		s.disconnect(stalled)
	}
}

func (s *PresenceServer) unregister(c *presenceConn) {
	var slow []*presenceConn
	s.mu.Lock()
	delete(s.connections, c.id)
	last, count := s.store.RemoveConnection(c.userID, c.id)
	if last && !s.closed {
		message := presenceMessage("presence:count", count)
		for _, other := range s.connections {
			if !other.enqueue(message) {
				slow = append(slow, other)
			}
		}
	}
	s.mu.Unlock()
	for _, stalled := range slow {
		s.disconnect(stalled)
	}
}

func (s *PresenceServer) disconnect(c *presenceConn) {
	c.close()
	s.unregister(c)
}

func (s *PresenceServer) sendSnapshot(c *presenceConn) {
	s.mu.Lock()
	queued := c.enqueue(presenceMessage("presence:sync", s.store.OnlineCount()))
	s.mu.Unlock()
	if !queued {
		s.disconnect(c)
	}
}

func (s *PresenceServer) readPump(c *presenceConn) {
	defer s.disconnect(c)
	if !c.expiresAt.IsZero() {
		expiryTimer := time.AfterFunc(time.Until(c.expiresAt), c.expire)
		defer expiryTimer.Stop()
	}
	c.ws.SetReadLimit(maxMessageSize)
	_ = c.ws.SetReadDeadline(time.Now().Add(s.pongTimeout))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(s.pongTimeout))
	})
	for {
		messageType, payload, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.TextMessage {
			continue
		}
		var request struct {
			Version int    `json:"version"`
			Type    string `json:"type"`
		}
		if json.Unmarshal(payload, &request) == nil && request.Version == 1 && request.Type == "presence:sync" {
			s.sendSnapshot(c)
		}
	}
}

func (c *presenceConn) writePump(s *PresenceServer) {
	ticker := time.NewTicker(s.pingEvery)
	defer ticker.Stop()
	defer s.disconnect(c)
	for {
		select {
		case <-c.done:
			return
		case message := <-c.send:
			_ = c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.ws.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				return
			}
		}
	}
}

func (s *PresenceServer) Close() {
	s.mu.Lock()
	s.closed = true
	connections := make([]*presenceConn, 0, len(s.connections))
	for _, c := range s.connections {
		connections = append(connections, c)
	}
	s.mu.Unlock()
	for _, c := range connections {
		s.disconnect(c)
	}
}
