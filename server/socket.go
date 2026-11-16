package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nimble-voice-backend/internal/contracts"
	"nimble-voice-backend/utils/jwt"

	"github.com/gin-gonic/gin"
	"github.com/shishir1290/gsocketio"
)

// AuthContext holds verified user identity extracted from the HS256 JWT handshake.
type AuthContext struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Token  string `json:"token"`
}

type peerRoomState struct {
	UserID          string
	Email           string
	Role            string
	IsHost          bool
	IsMuted         bool
	IsDeafened      bool
	HandRaised      bool
	IsSpeaking      bool
	IsScreenSharing bool
}

// SocketServer wraps gsocketio.Server with JWT-authenticated voice room routing and canonical event emission.
type SocketServer struct {
	Server         *gsocketio.Server
	presenceStore  PresenceStore
	userSocketMu   sync.RWMutex
	socketRoom     map[string]map[string]bool          // RoomID -> Set of SocketIDs
	peerStates     map[string]map[string]*peerRoomState // RoomID -> SocketID -> state
	roomVersions   map[string]int64                    // RoomID -> monotonic version
	lockedRooms    map[string]bool                     // RoomID -> lock state
	presenceSeq    atomic.Int64
	connsMu        sync.RWMutex
	conns          map[string]gsocketio.Conn // SocketID -> Conn
	originsMu      sync.RWMutex
	allowedOrigins map[string]struct{}
	authorizerMu   sync.RWMutex
	roomAuthorizer contracts.RoomAuthorizer
}

var _ contracts.RoomBroadcaster = (*SocketServer)(nil)

// NewSocketServer initializes a pure Go Socket.IO v4 server with resilient ping/pong timeouts
// and defaults to allowing http://localhost:3000 for local/test usage.
func NewSocketServer(customOpts ...*gsocketio.Options) (*SocketServer, error) {
	return NewSocketServerWithOrigins([]string{"http://localhost:3000"}, customOpts...)
}

// NewSocketServerWithOrigins initializes a pure Go Socket.IO v4 server enforcing an explicit origin allowlist.
func NewSocketServerWithOrigins(allowedOrigins []string, customOpts ...*gsocketio.Options) (*SocketServer, error) {
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
		Server:        srv,
		presenceStore: NewMemoryPresenceStore(),
		socketRoom:    make(map[string]map[string]bool),
		peerStates:    make(map[string]map[string]*peerRoomState),
		roomVersions:  make(map[string]int64),
		lockedRooms:   make(map[string]bool),
		conns:         make(map[string]gsocketio.Conn),
	}
	s.SetAllowedOrigins(allowedOrigins)
	s.registerHandlers()

	return s, nil
}

// SetRoomAuthorizer configures the authoritative room host/member authorizer.
func (s *SocketServer) SetRoomAuthorizer(auth contracts.RoomAuthorizer) {
	s.authorizerMu.Lock()
	s.roomAuthorizer = auth
	s.authorizerMu.Unlock()
}

func (s *SocketServer) getRoomAuthorizer() contracts.RoomAuthorizer {
	s.authorizerMu.RLock()
	auth := s.roomAuthorizer
	s.authorizerMu.RUnlock()
	return auth
}

// SetAllowedOrigins updates the allowlisted CORS and WebSocket origins in a concurrency-safe manner.
func (s *SocketServer) SetAllowedOrigins(origins []string) {
	normalized := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		clean := normalizeSocketOrigin(origin)
		if clean != "" {
			normalized[clean] = struct{}{}
		}
	}
	s.originsMu.Lock()
	s.allowedOrigins = normalized
	s.originsMu.Unlock()
}

func normalizeSocketOrigin(origin string) string {
	return strings.TrimRight(strings.TrimSpace(origin), "/")
}

func (s *SocketServer) isOriginAllowed(origin string) bool {
	clean := normalizeSocketOrigin(origin)
	if clean == "" {
		return false
	}
	s.originsMu.RLock()
	_, ok := s.allowedOrigins[clean]
	s.originsMu.RUnlock()
	return ok
}

// PresenceStore returns the server's presence store.
func (s *SocketServer) PresenceStore() PresenceStore {
	return s.presenceStore
}

// OnlineUserCount returns the count of currently connected unique authenticated users.
func (s *SocketServer) OnlineUserCount() int {
	if s.presenceStore == nil {
		return 0
	}
	return s.presenceStore.OnlineCount()
}

// TotalConnectionsCount returns the count of currently active raw socket connections.
func (s *SocketServer) TotalConnectionsCount() int {
	s.connsMu.RLock()
	defer s.connsMu.RUnlock()
	return len(s.conns)
}

func (s *SocketServer) nextRoomVersionLocked(roomID string) int64 {
	if s.roomVersions == nil {
		s.roomVersions = make(map[string]int64)
	}
	next := s.roomVersions[roomID] + 1
	s.roomVersions[roomID] = next
	return next
}

func (s *SocketServer) currentRoomVersion(roomID string) int64 {
	s.userSocketMu.RLock()
	defer s.userSocketMu.RUnlock()
	return s.roomVersions[roomID]
}

// BroadcastRoomEvent wraps data in a canonical ServerEventEnvelope and emits it to all sockets in roomID.
func (s *SocketServer) BroadcastRoomEvent(roomID, event string, version int64, data any) {
	if roomID == "" || event == "" {
		return
	}
	env := contracts.NewServerEventEnvelope(event, roomID, version, data)
	s.Server.ToRoom("/", roomID, event, nil, env)
}

// BroadcastLobbyEvent wraps data in a canonical ServerEventEnvelope and emits it to all lobby subscribers.
func (s *SocketServer) BroadcastLobbyEvent(event, roomID string, version int64, data any) {
	if event == "" {
		return
	}
	env := contracts.NewServerEventEnvelope(event, roomID, version, data)
	s.Server.ToRoom("/", "lobby", event, nil, env)
}

func (s *SocketServer) emitEnvelopeToConn(c gsocketio.Conn, event, roomID string, version int64, data any) {
	if c == nil || event == "" {
		return
	}
	env := contracts.NewServerEventEnvelope(event, roomID, version, data)
	_ = c.Emit(event, env)
}

func (s *SocketServer) emitEnvelopeToRoomExcept(roomID, event string, version int64, except gsocketio.Conn, data any) {
	if roomID == "" || event == "" {
		return
	}
	env := contracts.NewServerEventEnvelope(event, roomID, version, data)
	s.Server.ToRoom("/", roomID, event, except, env)
}

func (s *SocketServer) emitAckOK(c gsocketio.Conn, requestID string, data any) {
	if c == nil {
		return
	}
	ack := contracts.NewAckOK(requestID, data)
	_ = c.Emit(contracts.EventCommandAck, ack)
}

func (s *SocketServer) emitAckError(c gsocketio.Conn, requestID, code, message string) {
	if c == nil {
		return
	}
	ack := contracts.NewAckError(requestID, code, message)
	_ = c.Emit(contracts.EventCommandAck, ack)
}

func (s *SocketServer) requireAuth(c gsocketio.Conn) (*AuthContext, bool) {
	if c == nil {
		return nil, false
	}
	authCtx, ok := c.Context().(*AuthContext)
	if !ok || authCtx == nil || strings.TrimSpace(authCtx.UserID) == "" {
		return nil, false
	}
	return authCtx, true
}

func decodeCommandPayload[T any](args []json.RawMessage, out *T) error {
	if len(args) == 0 {
		return errors.New("payload is required")
	}
	trimmed := strings.TrimSpace(string(args[0]))
	if trimmed == "" || trimmed == "null" {
		return errors.New("payload is required")
	}
	if err := json.Unmarshal(args[0], out); err != nil {
		return err
	}
	return nil
}

func decodeOptionalCommandPayload[T any](args []json.RawMessage, out *T) error {
	if len(args) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(string(args[0]))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	return json.Unmarshal(args[0], out)
}

func (s *SocketServer) authenticateHandshake(raw interface{}) (*AuthContext, error) {
	if raw == nil {
		return nil, errors.New("unauthorized: missing handshake auth")
	}
	if ac, ok := raw.(*AuthContext); ok && ac != nil && strings.TrimSpace(ac.UserID) != "" {
		return ac, nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, errors.New("unauthorized: invalid handshake auth")
	}
	rawToken, _ := m["token"].(string)
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rawToken), "Bearer "))
	if token == "" {
		return nil, errors.New("unauthorized: token is required")
	}
	claims, err := jwt.ParseToken(token)
	if err != nil || claims == nil || strings.TrimSpace(claims.UserID) == "" {
		return nil, errors.New("unauthorized: invalid or expired token")
	}
	return &AuthContext{
		UserID: claims.UserID,
		Email:  claims.Email,
		Role:   claims.Role,
		Token:  token,
	}, nil
}

func displayNameFromEmail(email, userID string) string {
	if idx := strings.Index(email, "@"); idx > 0 {
		return email[:idx]
	}
	if strings.HasPrefix(userID, "user") {
		digits := strings.TrimPrefix(userID, "user")
		if len(digits) >= 8 {
			suffix := digits[8:]
			if suffix != "" {
				trimmed := strings.TrimLeft(suffix, "0")
				if trimmed != "" {
					return "Guest #" + trimmed
				}
			}
		}
		return "Guest #" + userID
	}
	if userID != "" {
		return userID
	}
	return "Learner"
}

func buildUserSummaryDTO(userID, email string) contracts.UserSummaryDTO {
	isGuest := strings.HasPrefix(userID, "user") || email == ""
	return contracts.NewUserSummaryDTO(contracts.UserSummaryDTO{
		ID:      userID,
		Name:    displayNameFromEmail(email, userID),
		IsGuest: isGuest,
	})
}

func buildParticipantDTO(socketID string, ps *peerRoomState) contracts.ParticipantDTO {
	return contracts.NewParticipantDTO(contracts.ParticipantDTO{
		UserSummaryDTO:  buildUserSummaryDTO(ps.UserID, ps.Email),
		IsHost:          ps.IsHost,
		IsSpeaking:      ps.IsSpeaking,
		IsMuted:         ps.IsMuted,
		IsDeafened:      ps.IsDeafened,
		HandRaised:      ps.HandRaised,
		IsScreenSharing: ps.IsScreenSharing,
		SocketID:        socketID,
	})
}

func (s *SocketServer) registerHandlers() {
	// 1. Connection Lifecycle & Strict HS256 JWT Handshake Auth
	s.Server.OnConnect("/", func(c gsocketio.Conn) error {
		authCtx, err := s.authenticateHandshake(c.Context())
		if err != nil {
			c.SetContext(nil)
			return err
		}
		c.SetContext(authCtx)

		s.connsMu.Lock()
		s.conns[c.ID()] = c
		s.connsMu.Unlock()

		firstUserSocket, onlineCount := s.presenceStore.AddConnection(authCtx.UserID, c.ID())

		var presenceVer int64
		if firstUserSocket {
			presenceVer = s.presenceSeq.Add(1)
			s.BroadcastLobbyEvent(contracts.EventPresenceUpdated, "", presenceVer, contracts.PresenceDTO{
				OnlineCount: onlineCount,
			})
		} else {
			presenceVer = s.presenceSeq.Load()
		}

		s.emitEnvelopeToConn(c, contracts.EventPresenceSnapshot, "", presenceVer, contracts.PresenceDTO{
			OnlineCount: onlineCount,
		})
		return nil
	})

	// 2. Disconnect Handler & Lock-Free Room Cleanup
	s.Server.OnDisconnect("/", func(c gsocketio.Conn, reason string) {
		authCtx, ok := s.requireAuth(c)
		if !ok {
			return
		}

		sid := c.ID()
		userID := authCtx.UserID

		s.connsMu.Lock()
		delete(s.conns, sid)
		s.connsMu.Unlock()

		userWentOffline, onlineCount := s.presenceStore.RemoveConnection(userID, sid)

		type leftRoomSnapshot struct {
			roomID    string
			remaining int
			isLocked  bool
			leaveVer  int64
			lobbyVer  int64
		}
		var leftRooms []leftRoomSnapshot

		s.userSocketMu.Lock()
		for roomID, set := range s.socketRoom {
			if set[sid] {
				delete(set, sid)
				if s.peerStates[roomID] != nil {
					delete(s.peerStates[roomID], sid)
				}
				remaining := len(set)
				isLocked := s.lockedRooms[roomID]
				leaveVer := s.nextRoomVersionLocked(roomID)
				lobbyVer := s.nextRoomVersionLocked(roomID)
				if remaining == 0 {
					delete(s.socketRoom, roomID)
					delete(s.peerStates, roomID)
					delete(s.lockedRooms, roomID)
				}
				leftRooms = append(leftRooms, leftRoomSnapshot{
					roomID:    roomID,
					remaining: remaining,
					isLocked:  isLocked,
					leaveVer:  leaveVer,
					lobbyVer:  lobbyVer,
				})
			}
		}
		s.userSocketMu.Unlock()

		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, lr := range leftRooms {
			s.BroadcastRoomEvent(lr.roomID, contracts.EventRoomMemberLeft, lr.leaveVer, contracts.RoomMemberLeftDTO{
				RoomID:   lr.roomID,
				UserID:   userID,
				SocketID: sid,
				Reason:   "disconnect",
				LeftAt:   now,
			})
			if lr.remaining == 0 {
				s.BroadcastLobbyEvent(contracts.EventLobbyRoomDeleted, lr.roomID, lr.lobbyVer, contracts.RoomDeletedDTO{
					RoomID: lr.roomID,
					Reason: "empty",
				})
			} else {
				s.BroadcastLobbyEvent(contracts.EventLobbyRoomUpdated, lr.roomID, lr.lobbyVer, contracts.RoomLobbyUpdateDTO{
					RoomID:       lr.roomID,
					CurrentSlots: lr.remaining,
					HasFreeSeats: true,
					Status:       "LIVE",
					IsLocked:     lr.isLocked,
				})
			}
		}

		if userWentOffline {
			ver := s.presenceSeq.Add(1)
			s.BroadcastLobbyEvent(contracts.EventPresenceUpdated, "", ver, contracts.PresenceDTO{
				OnlineCount: onlineCount,
			})
		}
		_ = reason
	})

	// 3. Error Handler
	s.Server.OnError("/", func(c gsocketio.Conn, err error) {
		if c != nil {
			log.Printf("[Socket.IO] Connection error (%s): %v", c.ID(), err)
		}
	})

	// 4. Canonical Lobby & Presence Commands
	s.Server.OnEvent("/", contracts.CmdLobbyJoin, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID string `json:"requestId"`
		}
		if err := decodeOptionalCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid lobby.join payload")
			return
		}
		if _, ok := s.requireAuth(c); !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}

		c.Join("lobby")
		onlineCount := s.OnlineUserCount()
		ver := s.presenceSeq.Load()
		s.emitEnvelopeToConn(c, contracts.EventPresenceSnapshot, "", ver, contracts.PresenceDTO{
			OnlineCount: onlineCount,
		})
		s.emitAckOK(c, req.RequestID, contracts.LobbySubscriptionAckDTO{Subscribed: true})
	})

	s.Server.OnEvent("/", contracts.CmdLobbyLeave, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID string `json:"requestId"`
		}
		if err := decodeOptionalCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid lobby.leave payload")
			return
		}
		if _, ok := s.requireAuth(c); !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}

		c.Leave("lobby")
		s.emitAckOK(c, req.RequestID, contracts.LobbySubscriptionAckDTO{Subscribed: false})
	})

	s.Server.OnEvent("/", contracts.CmdPresenceRequest, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID string `json:"requestId"`
		}
		if err := decodeOptionalCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid presence.request payload")
			return
		}
		if _, ok := s.requireAuth(c); !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}

		onlineCount := s.OnlineUserCount()
		ver := s.presenceSeq.Load()
		dto := contracts.PresenceDTO{OnlineCount: onlineCount}
		s.emitEnvelopeToConn(c, contracts.EventPresenceSnapshot, "", ver, dto)
		s.emitAckOK(c, req.RequestID, dto)
	})

	// 5. Canonical Room Lifecycle Commands
	s.Server.OnEvent("/", contracts.CmdRoomJoin, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID  string `json:"requestId"`
			RoomID     string `json:"roomId"`
			IsMuted    *bool  `json:"isMuted,omitempty"`
			IsDeafened *bool  `json:"isDeafened,omitempty"`
			HandRaised *bool  `json:"handRaised,omitempty"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid room.join payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		if roomID == "" {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId is required")
			return
		}

		isHost := false
		if authorizer := s.getRoomAuthorizer(); authorizer != nil {
			if host, err := authorizer.IsRoomHost(roomID, authCtx.UserID); err == nil && host {
				isHost = true
			}
		}

		isMuted := false
		if req.IsMuted != nil {
			isMuted = *req.IsMuted
		}
		isDeafened := false
		if req.IsDeafened != nil {
			isDeafened = *req.IsDeafened
		}
		handRaised := false
		if req.HandRaised != nil {
			handRaised = *req.HandRaised
		}

		var (
			memberCount    int
			isLocked       bool
			joinVersion    int64
			createdVer     int64
			lobbyUpdateVer int64
			snapshot       contracts.RoomSnapshotDTO
			selfDTO        contracts.ParticipantDTO
		)

		s.userSocketMu.Lock()
		if s.socketRoom[roomID] == nil {
			s.socketRoom[roomID] = make(map[string]bool)
		}
		if s.peerStates[roomID] == nil {
			s.peerStates[roomID] = make(map[string]*peerRoomState)
		}

		// Evict any existing socket for the same user in this room to eliminate ghost peers on reload/rejoin
		var staleSIDs []string
		for existingSID := range s.socketRoom[roomID] {
			if existingSID == c.ID() {
				continue
			}
			if existingUID := s.presenceStore.UserByConnection(existingSID); existingUID == authCtx.UserID {
				staleSIDs = append(staleSIDs, existingSID)
			}
		}
		for _, staleSID := range staleSIDs {
			delete(s.socketRoom[roomID], staleSID)
			if s.peerStates[roomID] != nil {
				delete(s.peerStates[roomID], staleSID)
			}
		}

		ps := &peerRoomState{
			UserID:     authCtx.UserID,
			Email:      authCtx.Email,
			Role:       authCtx.Role,
			IsHost:     isHost,
			IsMuted:    isMuted,
			IsDeafened: isDeafened,
			HandRaised: handRaised,
		}
		s.socketRoom[roomID][c.ID()] = true
		s.peerStates[roomID][c.ID()] = ps
		memberCount = len(s.socketRoom[roomID])
		isLocked = s.lockedRooms[roomID]

		joinVersion = s.nextRoomVersionLocked(roomID)
		if memberCount == 1 {
			createdVer = s.nextRoomVersionLocked(roomID)
		}
		lobbyUpdateVer = s.nextRoomVersionLocked(roomID)

		peers := make([]contracts.PeerSnapshotDTO, 0, memberCount)
		for peerSID := range s.socketRoom[roomID] {
			if peerSID == c.ID() {
				continue
			}
			peerState := s.peerStates[roomID][peerSID]
			if peerState == nil {
				peerUID := s.presenceStore.UserByConnection(peerSID)
				peerState = &peerRoomState{UserID: peerUID}
			}
			peers = append(peers, contracts.PeerSnapshotDTO{
				SocketID:    peerSID,
				UserID:      peerState.UserID,
				Participant: buildParticipantDTO(peerSID, peerState),
				VoiceState: contracts.VoiceStateDTO{
					IsMuted:         peerState.IsMuted,
					IsDeafened:      peerState.IsDeafened,
					HandRaised:      peerState.HandRaised,
					IsScreenSharing: peerState.IsScreenSharing,
				},
			})
		}
		selfDTO = buildParticipantDTO(c.ID(), ps)
		snapshot = contracts.NewRoomSnapshotDTO(roomID, nil, peers, isLocked)
		s.userSocketMu.Unlock()

		for _, staleSID := range staleSIDs {
			s.connsMu.RLock()
			staleConn := s.conns[staleSID]
			s.connsMu.RUnlock()
			if staleConn != nil {
				staleConn.Leave(roomID)
			}
		}

		c.Join(roomID)

		now := time.Now().UTC().Format(time.RFC3339Nano)
		s.emitEnvelopeToConn(c, contracts.EventRoomSnapshot, roomID, joinVersion, snapshot)
		s.emitEnvelopeToRoomExcept(roomID, contracts.EventRoomMemberJoined, joinVersion, c, contracts.RoomMemberJoinedDTO{
			RoomID:      roomID,
			SocketID:    c.ID(),
			UserID:      authCtx.UserID,
			Participant: selfDTO,
			JoinedAt:    now,
		})

		if memberCount == 1 {
			s.BroadcastLobbyEvent(contracts.EventLobbyRoomCreated, roomID, createdVer, contracts.NewRoomSummaryDTO(contracts.RoomSummaryDTO{
				ID:           roomID,
				CurrentSlots: memberCount,
				Status:       "LIVE",
				StartedAt:    now,
				HasFreeSeats: true,
				IsLive:       true,
				IsLocked:     isLocked,
				Version:      createdVer,
				Host:         buildUserSummaryDTO(authCtx.UserID, authCtx.Email),
			}))
		}
		s.BroadcastLobbyEvent(contracts.EventLobbyRoomUpdated, roomID, lobbyUpdateVer, contracts.RoomLobbyUpdateDTO{
			RoomID:       roomID,
			CurrentSlots: memberCount,
			HasFreeSeats: true,
			Status:       "LIVE",
			IsLocked:     isLocked,
		})

		s.emitAckOK(c, req.RequestID, contracts.RoomJoinAckDTO{
			RoomID:   roomID,
			SocketID: c.ID(),
			Version:  joinVersion,
			Snapshot: snapshot,
		})
	})

	s.Server.OnEvent("/", contracts.CmdRoomLeave, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID string `json:"requestId"`
			RoomID    string `json:"roomId"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid room.leave payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		if roomID == "" {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId is required")
			return
		}

		s.userSocketMu.Lock()
		isMember := s.socketRoom[roomID] != nil && s.socketRoom[roomID][c.ID()]
		if !isMember {
			s.userSocketMu.Unlock()
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "not a member of this room")
			return
		}
		delete(s.socketRoom[roomID], c.ID())
		if s.peerStates[roomID] != nil {
			delete(s.peerStates[roomID], c.ID())
		}
		remaining := len(s.socketRoom[roomID])
		isLocked := s.lockedRooms[roomID]
		leaveVer := s.nextRoomVersionLocked(roomID)
		lobbyVer := s.nextRoomVersionLocked(roomID)
		if remaining == 0 {
			delete(s.socketRoom, roomID)
			delete(s.peerStates, roomID)
			delete(s.lockedRooms, roomID)
		}
		s.userSocketMu.Unlock()

		c.Leave(roomID)

		now := time.Now().UTC().Format(time.RFC3339Nano)
		s.emitEnvelopeToRoomExcept(roomID, contracts.EventRoomMemberLeft, leaveVer, c, contracts.RoomMemberLeftDTO{
			RoomID:   roomID,
			UserID:   authCtx.UserID,
			SocketID: c.ID(),
			Reason:   "left",
			LeftAt:   now,
		})
		if remaining == 0 {
			s.BroadcastLobbyEvent(contracts.EventLobbyRoomDeleted, roomID, lobbyVer, contracts.RoomDeletedDTO{
				RoomID: roomID,
				Reason: "empty",
			})
		} else {
			s.BroadcastLobbyEvent(contracts.EventLobbyRoomUpdated, roomID, lobbyVer, contracts.RoomLobbyUpdateDTO{
				RoomID:       roomID,
				CurrentSlots: remaining,
				HasFreeSeats: true,
				Status:       "LIVE",
				IsLocked:     isLocked,
			})
		}

		s.emitAckOK(c, req.RequestID, contracts.RoomLeaveAckDTO{
			RoomID: roomID,
			Left:   true,
		})
	})

	// 6. WebRTC Signaling Relays (Member-Gated + Same-Room Target Verification)
	s.Server.OnEvent("/", contracts.CmdWebRTCOffer, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID      string `json:"requestId"`
			RoomID         string `json:"roomId"`
			TargetSocketID string `json:"targetSocketId"`
			SDP            any    `json:"sdp"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid webrtc.offer payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		targetSID := strings.TrimSpace(req.TargetSocketID)
		if roomID == "" || targetSID == "" || req.SDP == nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId, targetSocketId, and sdp are required")
			return
		}
		if targetSID == c.ID() {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "targetSocketId cannot equal sender socket")
			return
		}
		if !s.isSocketMember(roomID, c.ID()) || !s.isSocketMember(roomID, targetSID) {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "signaling peers must share an active room")
			return
		}

		s.connsMu.RLock()
		targetConn := s.conns[targetSID]
		s.connsMu.RUnlock()
		if targetConn == nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeNotFound, "target socket is not connected")
			return
		}

		s.emitEnvelopeToConn(targetConn, contracts.EventWebRTCOffer, roomID, 0, contracts.WebRTCOfferDTO{
			RoomID:         roomID,
			TargetSocketID: targetSID,
			SenderSocketID: c.ID(),
			SenderUserID:   authCtx.UserID,
			SDP:            req.SDP,
		})
		s.emitAckOK(c, req.RequestID, contracts.DeliveryAckDTO{Delivered: true})
	})

	s.Server.OnEvent("/", contracts.CmdWebRTCAnswer, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID      string `json:"requestId"`
			RoomID         string `json:"roomId"`
			TargetSocketID string `json:"targetSocketId"`
			SDP            any    `json:"sdp"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid webrtc.answer payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		targetSID := strings.TrimSpace(req.TargetSocketID)
		if roomID == "" || targetSID == "" || req.SDP == nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId, targetSocketId, and sdp are required")
			return
		}
		if targetSID == c.ID() {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "targetSocketId cannot equal sender socket")
			return
		}
		if !s.isSocketMember(roomID, c.ID()) || !s.isSocketMember(roomID, targetSID) {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "signaling peers must share an active room")
			return
		}

		s.connsMu.RLock()
		targetConn := s.conns[targetSID]
		s.connsMu.RUnlock()
		if targetConn == nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeNotFound, "target socket is not connected")
			return
		}

		s.emitEnvelopeToConn(targetConn, contracts.EventWebRTCAnswer, roomID, 0, contracts.WebRTCAnswerDTO{
			RoomID:         roomID,
			TargetSocketID: targetSID,
			SenderSocketID: c.ID(),
			SenderUserID:   authCtx.UserID,
			SDP:            req.SDP,
		})
		s.emitAckOK(c, req.RequestID, contracts.DeliveryAckDTO{Delivered: true})
	})

	s.Server.OnEvent("/", contracts.CmdWebRTCIceCandidate, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID      string `json:"requestId"`
			RoomID         string `json:"roomId"`
			TargetSocketID string `json:"targetSocketId"`
			Candidate      any    `json:"candidate"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid webrtc.ice_candidate payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		targetSID := strings.TrimSpace(req.TargetSocketID)
		if roomID == "" || targetSID == "" || req.Candidate == nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId, targetSocketId, and candidate are required")
			return
		}
		if targetSID == c.ID() {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "targetSocketId cannot equal sender socket")
			return
		}
		if !s.isSocketMember(roomID, c.ID()) || !s.isSocketMember(roomID, targetSID) {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "signaling peers must share an active room")
			return
		}

		s.connsMu.RLock()
		targetConn := s.conns[targetSID]
		s.connsMu.RUnlock()
		if targetConn == nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeNotFound, "target socket is not connected")
			return
		}

		s.emitEnvelopeToConn(targetConn, contracts.EventWebRTCIceCandidate, roomID, 0, contracts.WebRTCIceCandidateDTO{
			RoomID:         roomID,
			TargetSocketID: targetSID,
			SenderSocketID: c.ID(),
			SenderUserID:   authCtx.UserID,
			Candidate:      req.Candidate,
		})
		s.emitAckOK(c, req.RequestID, contracts.DeliveryAckDTO{Delivered: true})
	})

	// 7. Voice State & VAD Commands
	s.Server.OnEvent("/", contracts.CmdVoiceMute, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID  string `json:"requestId"`
			RoomID     string `json:"roomId"`
			IsMuted    bool   `json:"isMuted"`
			IsDeafened *bool  `json:"isDeafened,omitempty"`
			HandRaised *bool  `json:"handRaised,omitempty"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid voice.mute payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		if roomID == "" {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId is required")
			return
		}

		s.userSocketMu.Lock()
		isMember := s.socketRoom[roomID] != nil && s.socketRoom[roomID][c.ID()]
		if !isMember {
			s.userSocketMu.Unlock()
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "not a member of this room")
			return
		}
		if s.peerStates[roomID] != nil && s.peerStates[roomID][c.ID()] != nil {
			ps := s.peerStates[roomID][c.ID()]
			ps.IsMuted = req.IsMuted
			if req.IsDeafened != nil {
				ps.IsDeafened = *req.IsDeafened
			}
			if req.HandRaised != nil {
				ps.HandRaised = *req.HandRaised
			}
		}
		s.userSocketMu.Unlock()

		dto := contracts.VoiceMuteChangedDTO{
			RoomID:       roomID,
			UserID:       authCtx.UserID,
			IsMuted:      req.IsMuted,
			IsDeafened:   req.IsDeafened,
			HandRaised:   req.HandRaised,
			ForcedByHost: false,
		}
		s.BroadcastRoomEvent(roomID, contracts.EventVoiceMuteChanged, 0, dto)
		s.emitAckOK(c, req.RequestID, dto)
	})

	s.Server.OnEvent("/", contracts.CmdVoiceSpeaking, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID  string  `json:"requestId"`
			RoomID     string  `json:"roomId"`
			IsSpeaking bool    `json:"isSpeaking"`
			Level      float64 `json:"level"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			if req.RequestID != "" {
				s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid voice.speaking payload")
			}
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			if req.RequestID != "" {
				s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			}
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		if roomID == "" {
			if req.RequestID != "" {
				s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId is required")
			}
			return
		}

		s.userSocketMu.Lock()
		isMember := s.socketRoom[roomID] != nil && s.socketRoom[roomID][c.ID()]
		if !isMember {
			s.userSocketMu.Unlock()
			if req.RequestID != "" {
				s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "not a member of this room")
			}
			return
		}
		if s.peerStates[roomID] != nil && s.peerStates[roomID][c.ID()] != nil {
			s.peerStates[roomID][c.ID()].IsSpeaking = req.IsSpeaking
		}
		s.userSocketMu.Unlock()

		dto := contracts.VoiceSpeakingChangedDTO{
			RoomID:     roomID,
			UserID:     authCtx.UserID,
			IsSpeaking: req.IsSpeaking,
			Level:      req.Level,
		}
		s.emitEnvelopeToRoomExcept(roomID, contracts.EventVoiceSpeakingChanged, 0, c, dto)
		if req.RequestID != "" {
			s.emitAckOK(c, req.RequestID, dto)
		}
	})

	// 8. Screen Share Commands
	handleScreenShare := func(sharing bool) func(c gsocketio.Conn, args []json.RawMessage) {
		return func(c gsocketio.Conn, args []json.RawMessage) {
			var req struct {
				RequestID string `json:"requestId"`
				RoomID    string `json:"roomId"`
			}
			if err := decodeCommandPayload(args, &req); err != nil {
				s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid screen_share payload")
				return
			}
			authCtx, ok := s.requireAuth(c)
			if !ok {
				s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
				return
			}
			roomID := strings.TrimSpace(req.RoomID)
			if roomID == "" {
				s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId is required")
				return
			}

			s.userSocketMu.Lock()
			isMember := s.socketRoom[roomID] != nil && s.socketRoom[roomID][c.ID()]
			if !isMember {
				s.userSocketMu.Unlock()
				s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "not a member of this room")
				return
			}
			if s.peerStates[roomID] != nil && s.peerStates[roomID][c.ID()] != nil {
				s.peerStates[roomID][c.ID()].IsScreenSharing = sharing
			}
			s.userSocketMu.Unlock()

			dto := contracts.ScreenShareDTO{
				RoomID:    roomID,
				UserID:    authCtx.UserID,
				SocketID:  c.ID(),
				IsSharing: sharing,
			}
			evName := contracts.EventScreenShareStopped
			if sharing {
				evName = contracts.EventScreenShareStarted
			}
			s.BroadcastRoomEvent(roomID, evName, 0, dto)
			s.emitAckOK(c, req.RequestID, dto)
		}
	}
	s.Server.OnEvent("/", contracts.CmdScreenShareStart, handleScreenShare(true))
	s.Server.OnEvent("/", contracts.CmdScreenShareStop, handleScreenShare(false))

	// 9. Chat Reaction Command (Chat message creation is strictly REST POST /rooms/:id/messages)
	s.Server.OnEvent("/", contracts.CmdChatReact, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID string `json:"requestId"`
			RoomID    string `json:"roomId"`
			MessageID string `json:"messageId"`
			Emoji     string `json:"emoji"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid chat.react payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		messageID := strings.TrimSpace(req.MessageID)
		emoji := strings.TrimSpace(req.Emoji)
		if roomID == "" || messageID == "" || emoji == "" {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId, messageId, and emoji are required")
			return
		}
		if !s.isSocketMember(roomID, c.ID()) {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "not a member of this room")
			return
		}

		s.BroadcastRoomEvent(roomID, contracts.EventChatReactionUpdated, 0, contracts.ChatReactionUpdatedDTO{
			RoomID:    roomID,
			MessageID: messageID,
			Emoji:     emoji,
			UserID:    authCtx.UserID,
		})
		s.emitAckOK(c, req.RequestID, contracts.ChatReactAckDTO{
			RoomID:    roomID,
			MessageID: messageID,
			Emoji:     emoji,
		})
	})

	// 10. Host Moderation Commands (Fail Closed via RoomAuthorizer.IsRoomHost)
	s.Server.OnEvent("/", contracts.CmdModerationMuteParticipant, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID    string `json:"requestId"`
			RoomID       string `json:"roomId"`
			TargetUserID string `json:"targetUserId"`
			IsMuted      *bool  `json:"isMuted,omitempty"`
			Reason       string `json:"reason,omitempty"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid moderation.mute_participant payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		targetUserID := strings.TrimSpace(req.TargetUserID)
		if roomID == "" || targetUserID == "" {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId and targetUserId are required")
			return
		}
		if !s.isSocketMember(roomID, c.ID()) {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "not a member of this room")
			return
		}
		authorizer := s.getRoomAuthorizer()
		if authorizer == nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "host authorization unavailable")
			return
		}
		isHost, err := authorizer.IsRoomHost(roomID, authCtx.UserID)
		if err != nil || !isHost {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "only the room host can mute participants")
			return
		}

		muted := true
		if req.IsMuted != nil {
			muted = *req.IsMuted
		}

		s.userSocketMu.Lock()
		if s.peerStates[roomID] != nil {
			for _, ps := range s.peerStates[roomID] {
				if ps != nil && ps.UserID == targetUserID {
					ps.IsMuted = muted
				}
			}
		}
		s.userSocketMu.Unlock()

		reason := strings.TrimSpace(req.Reason)
		if reason == "" && muted {
			reason = "Host muted your microphone"
		}

		s.BroadcastRoomEvent(roomID, contracts.EventVoiceMuteChanged, 0, contracts.VoiceMuteChangedDTO{
			RoomID:       roomID,
			UserID:       targetUserID,
			IsMuted:      muted,
			ForcedByHost: true,
			Reason:       reason,
		})
		s.emitAckOK(c, req.RequestID, contracts.ModerationMuteAckDTO{
			RoomID:       roomID,
			TargetUserID: targetUserID,
			IsMuted:      muted,
		})
	})

	s.Server.OnEvent("/", contracts.CmdModerationKickParticipant, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID    string `json:"requestId"`
			RoomID       string `json:"roomId"`
			TargetUserID string `json:"targetUserId"`
			Reason       string `json:"reason,omitempty"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid moderation.kick_participant payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		targetUserID := strings.TrimSpace(req.TargetUserID)
		if roomID == "" || targetUserID == "" {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId and targetUserId are required")
			return
		}
		if !s.isSocketMember(roomID, c.ID()) {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "not a member of this room")
			return
		}
		authorizer := s.getRoomAuthorizer()
		if authorizer == nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "host authorization unavailable")
			return
		}
		isHost, err := authorizer.IsRoomHost(roomID, authCtx.UserID)
		if err != nil || !isHost {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "only the room host can kick participants")
			return
		}

		var (
			targetSIDs []string
			remaining  int
			isLocked   bool
			leaveVer   int64
			lobbyVer   int64
		)

		s.userSocketMu.Lock()
		for sid := range s.socketRoom[roomID] {
			ps := s.peerStates[roomID][sid]
			uid := s.presenceStore.UserByConnection(sid)
			if (ps != nil && ps.UserID == targetUserID) || uid == targetUserID {
				targetSIDs = append(targetSIDs, sid)
			}
		}
		for _, sid := range targetSIDs {
			delete(s.socketRoom[roomID], sid)
			if s.peerStates[roomID] != nil {
				delete(s.peerStates[roomID], sid)
			}
		}
		remaining = len(s.socketRoom[roomID])
		isLocked = s.lockedRooms[roomID]
		leaveVer = s.nextRoomVersionLocked(roomID)
		lobbyVer = s.nextRoomVersionLocked(roomID)
		if remaining == 0 {
			delete(s.socketRoom, roomID)
			delete(s.peerStates, roomID)
			delete(s.lockedRooms, roomID)
		}
		s.userSocketMu.Unlock()

		targetConns := make([]gsocketio.Conn, 0, len(targetSIDs))
		s.connsMu.RLock()
		for _, sid := range targetSIDs {
			if tc := s.conns[sid]; tc != nil {
				targetConns = append(targetConns, tc)
			}
		}
		s.connsMu.RUnlock()

		kickReason := strings.TrimSpace(req.Reason)
		if kickReason == "" {
			kickReason = "Host removed you from the voice room"
		}

		kickDTO := contracts.ModerationParticipantKickedDTO{
			RoomID:       roomID,
			TargetUserID: targetUserID,
			KickedBy:     authCtx.UserID,
			Reason:       kickReason,
		}
		for _, tc := range targetConns {
			tc.Leave(roomID)
			s.emitEnvelopeToConn(tc, contracts.EventModerationParticipantKicked, roomID, 0, kickDTO)
		}

		now := time.Now().UTC().Format(time.RFC3339Nano)
		if len(targetSIDs) == 0 {
			s.BroadcastRoomEvent(roomID, contracts.EventRoomMemberLeft, leaveVer, contracts.RoomMemberLeftDTO{
				RoomID:   roomID,
				UserID:   targetUserID,
				SocketID: "",
				Reason:   "kicked",
				LeftAt:   now,
			})
		} else {
			for _, sid := range targetSIDs {
				s.BroadcastRoomEvent(roomID, contracts.EventRoomMemberLeft, leaveVer, contracts.RoomMemberLeftDTO{
					RoomID:   roomID,
					UserID:   targetUserID,
					SocketID: sid,
					Reason:   "kicked",
					LeftAt:   now,
				})
			}
		}

		if remaining == 0 {
			s.BroadcastLobbyEvent(contracts.EventLobbyRoomDeleted, roomID, lobbyVer, contracts.RoomDeletedDTO{
				RoomID: roomID,
				Reason: "empty",
			})
		} else {
			s.BroadcastLobbyEvent(contracts.EventLobbyRoomUpdated, roomID, lobbyVer, contracts.RoomLobbyUpdateDTO{
				RoomID:       roomID,
				CurrentSlots: remaining,
				HasFreeSeats: true,
				Status:       "LIVE",
				IsLocked:     isLocked,
			})
		}

		s.emitAckOK(c, req.RequestID, contracts.ModerationKickAckDTO{
			RoomID:       roomID,
			TargetUserID: targetUserID,
		})
	})

	s.Server.OnEvent("/", contracts.CmdModerationLockRoom, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID string `json:"requestId"`
			RoomID    string `json:"roomId"`
			IsLocked  bool   `json:"isLocked"`
		}
		if err := decodeCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid moderation.lock_room payload")
			return
		}
		authCtx, ok := s.requireAuth(c)
		if !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		roomID := strings.TrimSpace(req.RoomID)
		if roomID == "" {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "roomId is required")
			return
		}
		if !s.isSocketMember(roomID, c.ID()) {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "not a member of this room")
			return
		}
		authorizer := s.getRoomAuthorizer()
		if authorizer == nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "host authorization unavailable")
			return
		}
		isHost, err := authorizer.IsRoomHost(roomID, authCtx.UserID)
		if err != nil || !isHost {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeForbidden, "only the room host can lock the room")
			return
		}

		s.userSocketMu.Lock()
		if s.lockedRooms == nil {
			s.lockedRooms = make(map[string]bool)
		}
		s.lockedRooms[roomID] = req.IsLocked
		remaining := len(s.socketRoom[roomID])
		lockVer := s.nextRoomVersionLocked(roomID)
		lobbyVer := s.nextRoomVersionLocked(roomID)
		s.userSocketMu.Unlock()

		s.BroadcastRoomEvent(roomID, contracts.EventModerationRoomLocked, lockVer, contracts.ModerationRoomLockedDTO{
			RoomID:    roomID,
			IsLocked:  req.IsLocked,
			UpdatedBy: authCtx.UserID,
		})
		s.BroadcastLobbyEvent(contracts.EventLobbyRoomUpdated, roomID, lobbyVer, contracts.RoomLobbyUpdateDTO{
			RoomID:       roomID,
			CurrentSlots: remaining,
			HasFreeSeats: !req.IsLocked,
			Status:       "LIVE",
			IsLocked:     req.IsLocked,
		})
		s.emitAckOK(c, req.RequestID, contracts.ModerationLockAckDTO{
			RoomID:   roomID,
			IsLocked: req.IsLocked,
		})
	})

	// 11. Network Mesh Latency Ping (responds via correlated command.ack)
	s.Server.OnEvent("/", contracts.CmdMeshPing, func(c gsocketio.Conn, args []json.RawMessage) {
		var req struct {
			RequestID string `json:"requestId"`
		}
		if err := decodeOptionalCommandPayload(args, &req); err != nil {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeValidation, "invalid mesh.ping payload")
			return
		}
		if _, ok := s.requireAuth(c); !ok {
			s.emitAckError(c, req.RequestID, contracts.ErrCodeUnauthorized, "unauthorized")
			return
		}
		s.emitAckOK(c, req.RequestID, contracts.MeshPingAckDTO{
			ServerTime: time.Now().UTC().UnixMilli(),
		})
	})
}

// isSocketMember verifies that a signaling sender or recipient is in the requested room.
func (s *SocketServer) isSocketMember(roomID, socketID string) bool {
	s.userSocketMu.RLock()
	defer s.userSocketMu.RUnlock()
	return s.socketRoom[roomID] != nil && s.socketRoom[roomID][socketID]
}

// Serve starts the socket transport acceptance loop.
func (s *SocketServer) Serve() error {
	return s.Server.Serve()
}

// Close gracefully closes the socket server.
func (s *SocketServer) Close() error {
	return s.Server.Close()
}

type corsGuardedWriter struct {
	http.ResponseWriter
	allowedOrigin string
	wroteHeader   bool
}

func (w *corsGuardedWriter) applyCORS() {
	if w.allowedOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", w.allowedOrigin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Accept, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	} else {
		w.Header().Del("Access-Control-Allow-Origin")
		w.Header().Del("Access-Control-Allow-Credentials")
	}
}

func (w *corsGuardedWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.applyCORS()
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *corsGuardedWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *corsGuardedWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijack not supported")
	}
	return hj.Hijack()
}

func (w *corsGuardedWriter) Flush() {
	if fl, ok := w.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

func (w *corsGuardedWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (s *SocketServer) handleCORS(w http.ResponseWriter, r *http.Request) (normalizedOrigin string, allowed bool, preflightDone bool) {
	origin := normalizeSocketOrigin(r.Header.Get("Origin"))
	if origin != "" {
		if !s.isOriginAllowed(origin) {
			http.Error(w, "origin not allowed", http.StatusForbidden)
			return "", false, false
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, Accept, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	}

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return origin, true, true
	}

	return origin, true, false
}

// ServeHTTP implements net/http.Handler with allowlisted CORS headers and Hijacker-safe guard for Socket.IO.
func (s *SocketServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin, allowed, preflightDone := s.handleCORS(w, r)
	if !allowed || preflightDone {
		return
	}

	guarded := &corsGuardedWriter{
		ResponseWriter: w,
		allowedOrigin:  origin,
	}
	s.Server.ServeHTTP(guarded, r)
}

// GinHandler wraps the socket server into a Gin-compatible handler with allowlisted CORS and Hijacker support.
func (s *SocketServer) GinHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin, allowed, preflightDone := s.handleCORS(c.Writer, c.Request)
		if !allowed || preflightDone {
			c.Abort()
			return
		}

		guarded := &corsGuardedWriter{
			ResponseWriter: c.Writer,
			allowedOrigin:  origin,
		}
		s.Server.ServeHTTP(guarded, c.Request)
	}
}
