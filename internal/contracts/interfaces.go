package contracts

// RoomBroadcaster decouples domain/HTTP services from the Socket.IO server transport.
type RoomBroadcaster interface {
	BroadcastRoomEvent(roomID, event string, version int64, data any)
	BroadcastLobbyEvent(event, roomID string, version int64, data any)
}

// RoomAuthorizer provides authoritative room membership and host ownership checks.
type RoomAuthorizer interface {
	IsRoomHost(roomID, userID string) (bool, error)
	IsRoomMember(roomID, userID string) (bool, error)
}
