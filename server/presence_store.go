package server

import (
	"sync"
	"time"
)

// PresenceStore defines the contract for cluster-ready unique user presence tracking.
type PresenceStore interface {
	// AddConnection records an active connection for a user.
	// Returns firstConnection=true if this user had 0 prior active connections (unique user joined),
	// and returns the updated count of unique online users.
	AddConnection(userID, connectionID string) (firstConnection bool, onlineUsers int)

	// RemoveConnection removes a specific connection for a user.
	// Returns lastConnection=true if the user has 0 remaining connections (unique user left),
	// and returns the updated count of unique online users.
	RemoveConnection(userID, connectionID string) (lastConnection bool, onlineUsers int)

	// OnlineCount returns the current count of unique online users.
	OnlineCount() int

	// TotalConnections returns the total number of open sockets across all users.
	TotalConnections() int

	// IsUserOnline returns true if the user has at least one active connection.
	IsUserOnline(userID string) bool

	// UserConnections returns all active socket IDs for a given user.
	UserConnections(userID string) []string

	// UserByConnection returns the userID associated with a given connection ID, or empty string.
	UserByConnection(connectionID string) string

	// Touch records activity timestamp for heartbeat tracking.
	Touch(connectionID string)

	// PruneStale evicts connections exceeding maxAge and returns any users who went completely offline.
	PruneStale(maxAge time.Duration) (prunedSockets int, offlineUsers []string, newOnlineCount int)
}

// memoryPresenceStore tracks unique users and all active connections per user.
type memoryPresenceStore struct {
	mu          sync.RWMutex
	users       map[string]map[string]time.Time // userID -> connectionID -> lastSeen
	connections map[string]string               // connectionID -> userID (reverse index)
}

// NewMemoryPresenceStore returns a new thread-safe in-memory PresenceStore.
func NewMemoryPresenceStore() PresenceStore {
	return &memoryPresenceStore{
		users:       make(map[string]map[string]time.Time),
		connections: make(map[string]string),
	}
}

// newMemoryPresenceStore is an internal alias for backwards compatibility.
func newMemoryPresenceStore() PresenceStore {
	return NewMemoryPresenceStore()
}

func (s *memoryPresenceStore) AddConnection(userID, connectionID string) (bool, int) {
	if userID == "" || connectionID == "" {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return false, len(s.users)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// If this connection ID was previously associated with another user, clean up
	if prevUID, exists := s.connections[connectionID]; exists && prevUID != userID {
		if prevConns := s.users[prevUID]; prevConns != nil {
			delete(prevConns, connectionID)
			if len(prevConns) == 0 {
				delete(s.users, prevUID)
			}
		}
	}

	userConns := s.users[userID]
	firstConnection := len(userConns) == 0
	if firstConnection {
		userConns = make(map[string]time.Time)
		s.users[userID] = userConns
	}

	userConns[connectionID] = time.Now()
	s.connections[connectionID] = userID

	return firstConnection, len(s.users)
}

func (s *memoryPresenceStore) RemoveConnection(userID, connectionID string) (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Look up reverse index if userID was omitted
	if userID == "" {
		userID = s.connections[connectionID]
	}
	delete(s.connections, connectionID)

	if userID == "" {
		return false, len(s.users)
	}

	userConns := s.users[userID]
	if userConns == nil {
		return false, len(s.users)
	}

	delete(userConns, connectionID)
	lastConnection := len(userConns) == 0
	if lastConnection {
		delete(s.users, userID)
	}

	return lastConnection, len(s.users)
}

func (s *memoryPresenceStore) OnlineCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users)
}

func (s *memoryPresenceStore) TotalConnections() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.connections)
}

func (s *memoryPresenceStore) IsUserOnline(userID string) bool {
	if userID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.users[userID]) > 0
}

func (s *memoryPresenceStore) UserConnections(userID string) []string {
	if userID == "" {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	userConns := s.users[userID]
	if len(userConns) == 0 {
		return nil
	}

	conns := make([]string, 0, len(userConns))
	for cid := range userConns {
		conns = append(conns, cid)
	}
	return conns
}

func (s *memoryPresenceStore) UserByConnection(connectionID string) string {
	if connectionID == "" {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connections[connectionID]
}

func (s *memoryPresenceStore) Touch(connectionID string) {
	if connectionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	userID, exists := s.connections[connectionID]
	if !exists {
		return
	}

	userConns := s.users[userID]
	if userConns != nil {
		userConns[connectionID] = time.Now()
	}
}

func (s *memoryPresenceStore) PruneStale(maxAge time.Duration) (int, []string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-maxAge)
	prunedSockets := 0
	var offlineUsers []string

	for userID, conns := range s.users {
		for connID, lastSeen := range conns {
			if lastSeen.Before(cutoff) {
				delete(conns, connID)
				delete(s.connections, connID)
				prunedSockets++
			}
		}
		if len(conns) == 0 {
			delete(s.users, userID)
			offlineUsers = append(offlineUsers, userID)
		}
	}

	return prunedSockets, offlineUsers, len(s.users)
}
