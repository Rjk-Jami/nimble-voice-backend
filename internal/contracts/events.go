package contracts

// Canonical server-to-client Socket.IO event names (SOCK-02).
const (
	EventPresenceSnapshot           = "presence.snapshot"
	EventPresenceUpdated            = "presence.updated"
	EventLobbyRoomCreated           = "lobby.room_created"
	EventLobbyRoomUpdated           = "lobby.room_updated"
	EventLobbyRoomDeleted           = "lobby.room_deleted"
	EventRoomMemberJoined           = "room.member_joined"
	EventRoomMemberLeft             = "room.member_left"
	EventRoomSnapshot               = "room.snapshot"
	EventRoomSettingsUpdated        = "room.settings_updated"
	EventRoomDeleted                = "room.deleted"
	EventRoomHostChanged            = "room.host_changed"
	EventVoiceMuteChanged           = "voice.mute_changed"
	EventVoiceSpeakingChanged       = "voice.speaking_changed"
	EventWebRTCOffer                = "webrtc.offer"
	EventWebRTCAnswer               = "webrtc.answer"
	EventWebRTCIceCandidate         = "webrtc.ice_candidate"
	EventScreenShareStarted         = "screen_share.started"
	EventScreenShareStopped         = "screen_share.stopped"
	EventChatMessageCreated         = "chat.message_created"
	EventChatReactionUpdated        = "chat.reaction_updated"
	EventModerationParticipantKicked = "moderation.participant_kicked"
	EventModerationRoomLocked       = "moderation.room_locked"
)

// EventCommandAck is the correlated server-to-client command acknowledgement event.
const EventCommandAck = "command.ack"

// Canonical client-to-server Socket.IO command names.
const (
	CmdLobbyJoin                = "lobby.join"
	CmdLobbyLeave               = "lobby.leave"
	CmdPresenceRequest          = "presence.request"
	CmdRoomJoin                 = "room.join"
	CmdRoomLeave                = "room.leave"
	CmdWebRTCOffer              = "webrtc.offer"
	CmdWebRTCAnswer             = "webrtc.answer"
	CmdWebRTCIceCandidate       = "webrtc.ice_candidate"
	CmdVoiceMute                = "voice.mute"
	CmdVoiceSpeaking            = "voice.speaking"
	CmdScreenShareStart         = "screen_share.start"
	CmdScreenShareStop          = "screen_share.stop"
	CmdChatReact                = "chat.react"
	CmdModerationMuteParticipant = "moderation.mute_participant"
	CmdModerationKickParticipant = "moderation.kick_participant"
	CmdModerationLockRoom       = "moderation.lock_room"
	CmdMeshPing                 = "mesh.ping"
)

var canonicalServerEvents = []string{
	EventPresenceSnapshot,
	EventPresenceUpdated,
	EventLobbyRoomCreated,
	EventLobbyRoomUpdated,
	EventLobbyRoomDeleted,
	EventRoomMemberJoined,
	EventRoomMemberLeft,
	EventRoomSnapshot,
	EventRoomSettingsUpdated,
	EventRoomDeleted,
	EventRoomHostChanged,
	EventVoiceMuteChanged,
	EventVoiceSpeakingChanged,
	EventWebRTCOffer,
	EventWebRTCAnswer,
	EventWebRTCIceCandidate,
	EventScreenShareStarted,
	EventScreenShareStopped,
	EventChatMessageCreated,
	EventChatReactionUpdated,
	EventModerationParticipantKicked,
	EventModerationRoomLocked,
}

var canonicalServerEventSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(canonicalServerEvents))
	for _, ev := range canonicalServerEvents {
		m[ev] = struct{}{}
	}
	return m
}()

var canonicalClientCommands = []string{
	CmdLobbyJoin,
	CmdLobbyLeave,
	CmdPresenceRequest,
	CmdRoomJoin,
	CmdRoomLeave,
	CmdWebRTCOffer,
	CmdWebRTCAnswer,
	CmdWebRTCIceCandidate,
	CmdVoiceMute,
	CmdVoiceSpeaking,
	CmdScreenShareStart,
	CmdScreenShareStop,
	CmdChatReact,
	CmdModerationMuteParticipant,
	CmdModerationKickParticipant,
	CmdModerationLockRoom,
	CmdMeshPing,
}

var canonicalClientCommandSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(canonicalClientCommands))
	for _, cmd := range canonicalClientCommands {
		m[cmd] = struct{}{}
	}
	return m
}()

// CanonicalServerEvents returns a copy of all 22 canonical server event names.
func CanonicalServerEvents() []string {
	out := make([]string, len(canonicalServerEvents))
	copy(out, canonicalServerEvents)
	return out
}

// IsCanonicalServerEvent reports whether name is one of the 22 canonical server events.
func IsCanonicalServerEvent(name string) bool {
	_, ok := canonicalServerEventSet[name]
	return ok
}

// CanonicalClientCommands returns a copy of all canonical client command names.
func CanonicalClientCommands() []string {
	out := make([]string, len(canonicalClientCommands))
	copy(out, canonicalClientCommands)
	return out
}

// IsCanonicalClientCommand reports whether name is a canonical client command.
func IsCanonicalClientCommand(name string) bool {
	_, ok := canonicalClientCommandSet[name]
	return ok
}
