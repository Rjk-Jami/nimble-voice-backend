package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"nimble-voice-backend/utils/jwt"

	"github.com/gin-gonic/gin"
	"github.com/shishir1290/gsocketio"
)

// AuthContext holds user credentials extracted during connection handshake
type AuthContext struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Token  string `json:"token"`
}

// Event payload structures for real-time voice room interactions
type RoomEventPayload struct {
	RoomID         string      `json:"room_id"`
	RoomId         string      `json:"roomId"`
	UserID         string      `json:"user_id"`
	UserId         string      `json:"userId"`
	Role           string      `json:"role,omitempty"`
	IsMuted        *bool       `json:"is_muted,omitempty"`
	IsMute         *bool       `json:"isMute,omitempty"`
	IsDeafened     *bool       `json:"is_deafened,omitempty"`
	HandRaised     *bool       `json:"hand_raised,omitempty"`
	Speaking       *bool       `json:"speaking,omitempty"`
	IsSpeaking     *bool       `json:"is_speaking,omitempty"`
	Level          float64     `json:"level,omitempty"`
	Signal         interface{} `json:"signal,omitempty"`
	SDP            interface{} `json:"sdp,omitempty"`
	Candidate      interface{} `json:"candidate,omitempty"`
	TargetID       string      `json:"target_id,omitempty"`
	TargetId       string      `json:"targetId,omitempty"`
	TargetSocketID string      `json:"targetSocketId,omitempty"`
	TargetUserID   string      `json:"targetUserId,omitempty"`
	Content        string      `json:"content,omitempty"`
	Type           string      `json:"type,omitempty"`
	MessageID      string      `json:"messageId,omitempty"`
	Emoji          string      `json:"emoji,omitempty"`
	User           interface{} `json:"user,omitempty"`
}

func (p *RoomEventPayload) ResolvedRoomID() string {
	if p.RoomID != "" {
		return p.RoomID
	}
	return p.RoomId
}

func (p *RoomEventPayload) ResolvedUserID() string {
	if p.UserID != "" {
		return p.UserID
	}
	return p.UserId
}

func (p *RoomEventPayload) ResolvedTargetSocketID() string {
	if p.TargetSocketID != "" {
		return p.TargetSocketID
	}
	if p.TargetID != "" {
		return p.TargetID
	}
	return p.TargetId
}

// SocketServer wraps gsocketio.Server with voice room routing and lifecycle handlers
type SocketServer struct {
	Server       *gsocketio.Server
	userSocketMu sync.RWMutex
	userToSocket map[string]string            // UserID -> SocketID
	socketToUser map[string]string            // SocketID -> UserID
	socketRoom   map[string]map[string]bool   // RoomID -> Set of SocketIDs
	connsMu      sync.RWMutex
	conns        map[string]gsocketio.Conn    // SocketID -> Conn
}

// NewSocketServer initializes a pure Go Socket.IO v4 server with resilient ping/pong timeouts
func NewSocketServer(customOpts ...*gsocketio.Options) (*SocketServer, error) {
	opts := &gsocketio.Options{
		PingInterval: 25 * time.Second,
		PingTimeout:  20 * time.Second,
		MaxPayload:   1024 * 1024, // 1MB
	}

	if len(customOpts) > 0 && customOpts[0] != nil {
		opts = customOpts[0]
	}

	srv := gsocketio.New(opts)
	s := &SocketServer{
		Server:       srv,
		userToSocket: make(map[string]string),
		socketToUser: make(map[string]string),
		socketRoom:   make(map[string]map[string]bool),
		conns:        make(map[string]gsocketio.Conn),
	}
	s.registerHandlers()

	return s, nil
}

// BroadcastToRoom sends an event to all clients in a specific room
func (s *SocketServer) BroadcastToRoom(roomID, event string, data interface{}) {
	s.Server.ToRoom("/", roomID, event, nil, data)
}

// OnlineUserCount returns the count of currently connected unique authenticated/guest users
func (s *SocketServer) OnlineUserCount() int {
	s.userSocketMu.RLock()
	defer s.userSocketMu.RUnlock()
	return len(s.userToSocket)
}

// registerHandlers registers connection lifecycle and voice room event dispatchers
func (s *SocketServer) registerHandlers() {
	// 1. Connection Lifecycle & Handshake Auth
	s.Server.OnConnect("/", func(c gsocketio.Conn) error {
		authCtx := s.extractAuthContext(c.Context())
		c.SetContext(authCtx)

		s.connsMu.Lock()
		s.conns[c.ID()] = c
		s.connsMu.Unlock()

		if authCtx.UserID != "" {
			s.userSocketMu.Lock()
			s.userToSocket[authCtx.UserID] = c.ID()
			s.socketToUser[c.ID()] = authCtx.UserID
			s.userSocketMu.Unlock()
		}

		log.Printf("[Socket.IO] Client connected: %s (UserID: %s, Role: %s)", c.ID(), authCtx.UserID, authCtx.Role)

		_ = c.Emit("connected", map[string]interface{}{
			"sid":     c.ID(),
			"user_id": authCtx.UserID,
			"time":    time.Now().UTC(),
		})
		return nil
	})

	// 2. Disconnect Handler & Room Cleanup
	s.Server.OnDisconnect("/", func(c gsocketio.Conn, reason string) {
		sid := c.ID()
		s.connsMu.Lock()
		delete(s.conns, sid)
		s.connsMu.Unlock()

		s.userSocketMu.Lock()
		userID := s.socketToUser[sid]
		delete(s.socketToUser, sid)
		if userID != "" && s.userToSocket[userID] == sid {
			delete(s.userToSocket, userID)
		}

		// Find rooms where this socket was present and emit room:user-left
		for roomID, set := range s.socketRoom {
			if set[sid] {
				delete(set, sid)
				s.Server.ToRoom("/", roomID, "room:user-left", c, map[string]interface{}{
					"userId":   userID,
					"user_id":  userID,
					"socketId": sid,
					"left_at":  time.Now().UTC(),
				})
				s.Server.ToRoom("/", roomID, "room:user_left", c, map[string]interface{}{
					"userId":   userID,
					"user_id":  userID,
					"socketId": sid,
				})
			}
		}
		s.userSocketMu.Unlock()

		log.Printf("[Socket.IO] Client disconnected: %s (UserID: %s, Reason: %s)", sid, userID, reason)
	})

	// 3. Error Handler
	s.Server.OnError("/", func(c gsocketio.Conn, err error) {
		log.Printf("[Socket.IO] Connection error (%s): %v", c.ID(), err)
	})

	// 4. Voice Room: Join
	s.Server.OnEvent("/", "room:join", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			_ = c.Emit("error", map[string]string{"message": "room_id is required"})
			return
		}

		userID := payload.ResolvedUserID()
		if userID == "" {
			authCtx := s.extractAuthContext(c.Context())
			userID = authCtx.UserID
		}

		s.userSocketMu.Lock()
		if s.socketRoom[roomID] == nil {
			s.socketRoom[roomID] = make(map[string]bool)
		}
		s.socketRoom[roomID][c.ID()] = true
		if userID != "" {
			s.userToSocket[userID] = c.ID()
			s.socketToUser[c.ID()] = userID
		}
		s.userSocketMu.Unlock()

		// Join room in gsocketio
		c.Join(roomID)

		joinData := map[string]interface{}{
			"userId":   userID,
			"user_id":  userID,
			"socketId": c.ID(),
			"role":     payload.Role,
			"user":     payload.User,
			"joined":   time.Now().UTC(),
		}

		// Broadcast presence to all other peers in the room (both kebab and snake case for compatibility)
		s.Server.ToRoom("/", roomID, "room:user-joined", c, joinData)
		s.Server.ToRoom("/", roomID, "room:user_joined", c, joinData)

		// Acknowledge join success to the caller
		_ = c.Emit("room:joined", map[string]interface{}{
			"roomId":  roomID,
			"room_id": roomID,
			"userId":  userID,
			"user_id": userID,
			"status":  "ok",
		})
	})

	// 5. Voice Room: Leave
	s.Server.OnEvent("/", "room:leave", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			return
		}

		userID := payload.ResolvedUserID()
		if userID == "" {
			authCtx := s.extractAuthContext(c.Context())
			userID = authCtx.UserID
		}

		s.userSocketMu.Lock()
		if s.socketRoom[roomID] != nil {
			delete(s.socketRoom[roomID], c.ID())
		}
		s.userSocketMu.Unlock()

		c.Leave(roomID)

		leaveData := map[string]interface{}{
			"roomId":   roomID,
			"room_id":  roomID,
			"userId":   userID,
			"user_id":  userID,
			"socketId": c.ID(),
			"left_at":  time.Now().UTC(),
		}

		s.Server.ToRoom("/", roomID, "room:user-left", c, leaveData)
		s.Server.ToRoom("/", roomID, "room:user_left", c, leaveData)

		_ = c.Emit("room:left", map[string]interface{}{
			"roomId":  roomID,
			"room_id": roomID,
			"status":  "ok",
		})
	})

	// 6. WebRTC: Offer Relay
	s.Server.OnEvent("/", "webrtc:offer", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		targetSocketID := payload.ResolvedTargetSocketID()
		senderID := payload.ResolvedUserID()
		if senderID == "" {
			authCtx := s.extractAuthContext(c.Context())
			senderID = authCtx.UserID
		}

		offerData := map[string]interface{}{
			"targetSocketId": targetSocketID,
			"senderSocketId": c.ID(),
			"senderId":       senderID,
			"sdp":            payload.SDP,
			"signal":         payload.Signal,
		}

		if targetSocketID != "" {
			s.connsMu.RLock()
			targetConn := s.conns[targetSocketID]
			s.connsMu.RUnlock()
			if targetConn != nil {
				_ = targetConn.Emit("webrtc:offer", offerData)
				return
			}
		}

		roomID := payload.ResolvedRoomID()
		if roomID != "" {
			s.Server.ToRoom("/", roomID, "webrtc:offer", c, offerData)
		}
	})

	// 7. WebRTC: Answer Relay
	s.Server.OnEvent("/", "webrtc:answer", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		targetSocketID := payload.ResolvedTargetSocketID()
		senderID := payload.ResolvedUserID()
		if senderID == "" {
			authCtx := s.extractAuthContext(c.Context())
			senderID = authCtx.UserID
		}

		answerData := map[string]interface{}{
			"targetSocketId": targetSocketID,
			"senderSocketId": c.ID(),
			"senderId":       senderID,
			"sdp":            payload.SDP,
			"signal":         payload.Signal,
		}

		if targetSocketID != "" {
			s.connsMu.RLock()
			targetConn := s.conns[targetSocketID]
			s.connsMu.RUnlock()
			if targetConn != nil {
				_ = targetConn.Emit("webrtc:answer", answerData)
				return
			}
		}

		roomID := payload.ResolvedRoomID()
		if roomID != "" {
			s.Server.ToRoom("/", roomID, "webrtc:answer", c, answerData)
		}
	})

	// 8. WebRTC: ICE Candidate Relay
	s.Server.OnEvent("/", "webrtc:ice-candidate", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		targetSocketID := payload.ResolvedTargetSocketID()
		senderID := payload.ResolvedUserID()
		if senderID == "" {
			authCtx := s.extractAuthContext(c.Context())
			senderID = authCtx.UserID
		}

		candData := map[string]interface{}{
			"targetSocketId": targetSocketID,
			"senderSocketId": c.ID(),
			"senderId":       senderID,
			"candidate":      payload.Candidate,
			"signal":         payload.Signal,
		}

		if targetSocketID != "" {
			s.connsMu.RLock()
			targetConn := s.conns[targetSocketID]
			s.connsMu.RUnlock()
			if targetConn != nil {
				_ = targetConn.Emit("webrtc:ice-candidate", candData)
				return
			}
		}

		roomID := payload.ResolvedRoomID()
		if roomID != "" {
			s.Server.ToRoom("/", roomID, "webrtc:ice-candidate", c, candData)
		}
	})

	// 9. Voice: Speaking Indicator & Waveform (Voice Activity Detection 40ms ticks)
	s.Server.OnEvent("/", "voice:speaking", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			return
		}

		userID := payload.ResolvedUserID()
		if userID == "" {
			authCtx := s.extractAuthContext(c.Context())
			userID = authCtx.UserID
		}

		isSpeaking := false
		if payload.IsSpeaking != nil {
			isSpeaking = *payload.IsSpeaking
		} else if payload.Speaking != nil {
			isSpeaking = *payload.Speaking
		}

		speakingData := map[string]interface{}{
			"roomId":     roomID,
			"room_id":    roomID,
			"userId":     userID,
			"user_id":    userID,
			"isSpeaking": isSpeaking,
			"speaking":   isSpeaking,
			"level":      payload.Level,
		}

		s.Server.ToRoom("/", roomID, "voice:speaking-changed", c, speakingData)
		s.Server.ToRoom("/", roomID, "room:speaking_state", c, speakingData)
	})

	// 10. Voice: State Toggle (Mute / Deafen / Raise Hand)
	s.Server.OnEvent("/", "voice:state-toggle", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			return
		}

		userID := payload.ResolvedUserID()
		if userID == "" {
			authCtx := s.extractAuthContext(c.Context())
			userID = authCtx.UserID
		}

		isMuted := false
		if payload.IsMuted != nil {
			isMuted = *payload.IsMuted
		} else if payload.IsMute != nil {
			isMuted = *payload.IsMute
		}

		isDeafened := false
		if payload.IsDeafened != nil {
			isDeafened = *payload.IsDeafened
		}

		handRaised := false
		if payload.HandRaised != nil {
			handRaised = *payload.HandRaised
		}

		stateData := map[string]interface{}{
			"roomId":     roomID,
			"room_id":    roomID,
			"userId":     userID,
			"user_id":    userID,
			"isMuted":    isMuted,
			"is_muted":   isMuted,
			"isDeafened": isDeafened,
			"handRaised": handRaised,
		}

		s.Server.ToRoom("/", roomID, "voice:state-updated", c, stateData)
		s.Server.ToRoom("/", roomID, "room:mute", c, stateData)
	})

	// 11. In-Room Chat: Send
	s.Server.OnEvent("/", "chat:send", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			return
		}

		userID := payload.ResolvedUserID()
		if userID == "" {
			authCtx := s.extractAuthContext(c.Context())
			userID = authCtx.UserID
		}

		msgType := payload.Type
		if msgType == "" {
			msgType = "TEXT"
		}

		msgData := map[string]interface{}{
			"id":            "msg_" + time.Now().Format("20060102150405"),
			"roomId":        roomID,
			"senderId":      userID,
			"content":       payload.Content,
			"type":          msgType,
			"isHighlighted": msgType == "IDIOM",
			"reactions":     []string{},
			"timestamp":     time.Now().UTC(),
		}

		s.Server.ToRoom("/", roomID, "chat:new-message", nil, msgData)
	})

	// 12. In-Room Chat: Reaction
	s.Server.OnEvent("/", "chat:reaction-add", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			return
		}

		s.Server.ToRoom("/", roomID, "chat:reaction-add", c, map[string]interface{}{
			"roomId":    roomID,
			"messageId": payload.MessageID,
			"emoji":     payload.Emoji,
			"userId":    payload.ResolvedUserID(),
		})
	})

	// 13. Host Moderation: Kick User
	s.Server.OnEvent("/", "host:kick-user", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}

		roomID := payload.ResolvedRoomID()
		targetUserID := payload.TargetUserID
		if targetUserID == "" {
			targetUserID = payload.ResolvedTargetSocketID()
		}

		s.userSocketMu.RLock()
		targetSocketID := s.userToSocket[targetUserID]
		if targetSocketID == "" {
			targetSocketID = targetUserID
		}
		s.userSocketMu.RUnlock()

		if targetSocketID != "" {
			s.connsMu.RLock()
			targetConn := s.conns[targetSocketID]
			s.connsMu.RUnlock()
			if targetConn != nil {
				_ = targetConn.Emit("room:kicked", map[string]interface{}{
					"roomId": roomID,
					"reason": "Host removed you from the voice room",
				})
				targetConn.Leave(roomID)
			}
		}

		s.Server.ToRoom("/", roomID, "room:user-left", c, map[string]interface{}{
			"userId":   targetUserID,
			"socketId": targetSocketID,
			"reason":   "kicked",
		})
	})

	// Backwards compatibility event listeners
	s.Server.OnEvent("/", "room:mute", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}
		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			return
		}
		userID := payload.ResolvedUserID()
		if userID == "" {
			authCtx := s.extractAuthContext(c.Context())
			userID = authCtx.UserID
		}
		isMuted := (payload.IsMuted != nil && *payload.IsMuted) || (payload.IsMute != nil && *payload.IsMute)
		s.Server.ToRoom("/", roomID, "room:mute", c, map[string]interface{}{
			"room_id":  roomID,
			"user_id":  userID,
			"is_muted": isMuted,
		})
	})

	s.Server.OnEvent("/", "room:speaking_state", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}
		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			return
		}
		userID := payload.ResolvedUserID()
		if userID == "" {
			authCtx := s.extractAuthContext(c.Context())
			userID = authCtx.UserID
		}
		speaking := payload.Speaking != nil && *payload.Speaking
		s.Server.ToRoom("/", roomID, "room:speaking_state", c, map[string]interface{}{
			"room_id":  roomID,
			"user_id":  userID,
			"speaking": speaking,
		})
	})

	s.Server.OnEvent("/", "room:raise_hand", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}
		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			return
		}
		userID := payload.ResolvedUserID()
		if userID == "" {
			authCtx := s.extractAuthContext(c.Context())
			userID = authCtx.UserID
		}
		s.Server.ToRoom("/", roomID, "room:raise_hand", c, map[string]interface{}{
			"room_id": roomID,
			"user_id": userID,
			"time":    time.Now().UTC(),
		})
	})

	s.Server.OnEvent("/", "webrtc:signal", func(c gsocketio.Conn, args []json.RawMessage) {
		var payload RoomEventPayload
		if len(args) > 0 {
			_ = json.Unmarshal(args[0], &payload)
		}
		roomID := payload.ResolvedRoomID()
		if roomID == "" {
			return
		}
		senderID := payload.ResolvedUserID()
		if senderID == "" {
			authCtx := s.extractAuthContext(c.Context())
			senderID = authCtx.UserID
		}
		targetID := payload.ResolvedTargetSocketID()
		s.Server.ToRoom("/", roomID, "webrtc:signal", c, map[string]interface{}{
			"room_id":   roomID,
			"sender_id": senderID,
			"target_id": targetID,
			"signal":    payload.Signal,
		})
	})
}

// extractAuthContext parses handshake payload or query params into typed AuthContext
func (s *SocketServer) extractAuthContext(raw interface{}) *AuthContext {
	ctx := &AuthContext{}
	if raw == nil {
		return ctx
	}

	if ac, ok := raw.(*AuthContext); ok {
		return ac
	}

	if m, ok := raw.(map[string]interface{}); ok {
		if t, ok := m["token"].(string); ok {
			ctx.Token = strings.TrimPrefix(t, "Bearer ")
		}
		if u, ok := m["user_id"].(string); ok {
			ctx.UserID = u
		} else if u, ok := m["userId"].(string); ok {
			ctx.UserID = u
		}
		if e, ok := m["email"].(string); ok {
			ctx.Email = e
		}
		if r, ok := m["role"].(string); ok {
			ctx.Role = r
		}
	}

	// Validate JWT token if provided
	if ctx.Token != "" && len(jwt.SecretKey) > 0 {
		if claims, err := jwt.ParseToken(ctx.Token); err == nil && claims != nil {
			if ctx.UserID == "" {
				ctx.UserID = claims.UserID
			}
			if ctx.Email == "" {
				ctx.Email = claims.Email
			}
			if ctx.Role == "" {
				ctx.Role = claims.Role
			}
		}
	}

	return ctx
}

// Serve starts the socket transport acceptance loop
func (s *SocketServer) Serve() error {
	return s.Server.Serve()
}

// Close gracefully closes the socket server
func (s *SocketServer) Close() error {
	return s.Server.Close()
}

// ServeHTTP implements net/http.Handler with standard CORS headers for Socket.IO
func (s *SocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Accept, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	s.Server.ServeHTTP(w, r)
}

// GinHandler wraps the socket server into a Gin-compatible handler with Next.js CORS support
func (s *SocketServer) GinHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Accept, X-Requested-With")
			c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		s.Server.ServeHTTP(c.Writer, c.Request)
	}
}
