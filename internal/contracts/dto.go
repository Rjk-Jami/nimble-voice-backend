package contracts

import (
	"bytes"
	"encoding/json"
)

// PresenceDTO carries global online learner presence counts.
type PresenceDTO struct {
	OnlineCount int `json:"onlineCount"`
}

// UserSummaryDTO is the sanitized camelCase user projection sent over Socket.IO.
type UserSummaryDTO struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	AvatarURL        string            `json:"avatarUrl,omitempty"`
	Location         string            `json:"location,omitempty"`
	NativeLanguage   string            `json:"nativeLanguage,omitempty"`
	LearningLanguage string            `json:"learningLanguage,omitempty"`
	IsVerified       bool              `json:"isVerified"`
	IsGuest          bool              `json:"isGuest"`
	CEFRPortfolio    map[string]string `json:"cefrPortfolio"`
	Karma            int               `json:"karma"`
	HoursSpoken      float64           `json:"hoursSpoken"`
	Streak           int               `json:"streak"`
}

// NewUserSummaryDTO returns a UserSummaryDTO with non-nil CEFRPortfolio map.
func NewUserSummaryDTO(u UserSummaryDTO) UserSummaryDTO {
	if u.CEFRPortfolio == nil {
		u.CEFRPortfolio = make(map[string]string)
	}
	return u
}

// VoiceStateDTO captures mutable per-peer voice/presence flags in a room.
type VoiceStateDTO struct {
	IsMuted         bool `json:"isMuted"`
	IsDeafened      bool `json:"isDeafened"`
	HandRaised      bool `json:"handRaised"`
	IsScreenSharing bool `json:"isScreenSharing"`
}

// ParticipantDTO projects an active room participant with flat camelCase fields.
type ParticipantDTO struct {
	UserSummaryDTO
	IsHost          bool   `json:"isHost"`
	IsSpeaking      bool   `json:"isSpeaking"`
	IsMuted         bool   `json:"isMuted"`
	IsDeafened      bool   `json:"isDeafened"`
	HandRaised      bool   `json:"handRaised"`
	IsScreenSharing bool   `json:"isScreenSharing"`
	SocketID        string `json:"socketId"`
}

// NewParticipantDTO initializes a ParticipantDTO with non-nil collection fields.
func NewParticipantDTO(p ParticipantDTO) ParticipantDTO {
	p.UserSummaryDTO = NewUserSummaryDTO(p.UserSummaryDTO)
	return p
}

// PeerSnapshotDTO describes a single connected peer inside RoomSnapshotDTO.
type PeerSnapshotDTO struct {
	SocketID    string         `json:"socketId"`
	UserID      string         `json:"userId"`
	Participant ParticipantDTO `json:"participant"`
	VoiceState  VoiceStateDTO  `json:"voiceState"`
}

// RoomSummaryDTO is the explicit camelCase room projection used in snapshots and lobby creation.
type RoomSummaryDTO struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Topic              string         `json:"topic"`
	Language           string         `json:"language"`
	Flag               string         `json:"flag"`
	CEFRLevel          string         `json:"cefrLevel"`
	LevelLabel         string         `json:"levelLabel"`
	MaxSlots           int            `json:"maxSlots"`
	CurrentSlots       int            `json:"currentSlots"`
	Tags               []string       `json:"tags"`
	Status             string         `json:"status"`
	StartedAt          string         `json:"startedAt"`
	HasFreeSeats       bool           `json:"hasFreeSeats"`
	IsBeginnerFriendly bool           `json:"isBeginnerFriendly"`
	HasNativeSpeaker   bool           `json:"hasNativeSpeaker"`
	IsLive             bool           `json:"isLive"`
	IsLocked           bool           `json:"isLocked"`
	Version            int64          `json:"version"`
	Host               UserSummaryDTO `json:"host"`
}

// NewRoomSummaryDTO ensures slice and map fields on RoomSummaryDTO marshal as [] and {}.
func NewRoomSummaryDTO(r RoomSummaryDTO) RoomSummaryDTO {
	if r.Tags == nil {
		r.Tags = make([]string, 0)
	}
	r.Host = NewUserSummaryDTO(r.Host)
	return r
}

// RoomLobbyUpdateDTO carries lightweight room slot/status updates for lobby subscribers.
type RoomLobbyUpdateDTO struct {
	RoomID       string `json:"roomId"`
	CurrentSlots int    `json:"currentSlots"`
	MaxSlots     int    `json:"maxSlots"`
	HasFreeSeats bool   `json:"hasFreeSeats"`
	Status       string `json:"status"`
	IsLocked     bool   `json:"isLocked"`
	HostID       string `json:"hostId,omitempty"`
}

// RoomSnapshotDTO delivers authoritative room metadata and active peer roster to a joining client.
type RoomSnapshotDTO struct {
	RoomID   string            `json:"roomId"`
	Room     *RoomSummaryDTO   `json:"room,omitempty"`
	Peers    []PeerSnapshotDTO `json:"peers"`
	IsLocked bool              `json:"isLocked"`
}

// NewRoomSnapshotDTO ensures Peers is always a non-nil slice so JSON marshals [].
func NewRoomSnapshotDTO(roomID string, room *RoomSummaryDTO, peers []PeerSnapshotDTO, isLocked bool) RoomSnapshotDTO {
	if peers == nil {
		peers = make([]PeerSnapshotDTO, 0)
	} else {
		for i := range peers {
			peers[i].Participant = NewParticipantDTO(peers[i].Participant)
		}
	}
	if room != nil {
		normalized := NewRoomSummaryDTO(*room)
		room = &normalized
	}
	return RoomSnapshotDTO{
		RoomID:   roomID,
		Room:     room,
		Peers:    peers,
		IsLocked: isLocked,
	}
}

// RoomMemberJoinedDTO notifies room peers that a participant joined.
type RoomMemberJoinedDTO struct {
	RoomID      string         `json:"roomId"`
	SocketID    string         `json:"socketId"`
	UserID      string         `json:"userId"`
	Participant ParticipantDTO `json:"participant"`
	JoinedAt    string         `json:"joinedAt"`
}

// RoomMemberLeftDTO notifies room peers that a participant disconnected or left.
type RoomMemberLeftDTO struct {
	RoomID   string `json:"roomId"`
	UserID   string `json:"userId"`
	SocketID string `json:"socketId"`
	Reason   string `json:"reason"`
	LeftAt   string `json:"leftAt"`
}

// RoomSettingsUpdatedDTO represents partial room metadata updates.
type RoomSettingsUpdatedDTO struct {
	RoomID       string  `json:"roomId"`
	Title        *string `json:"title,omitempty"`
	Topic        *string `json:"topic,omitempty"`
	Status       *string `json:"status,omitempty"`
	MaxSlots     *int    `json:"maxSlots,omitempty"`
	HasFreeSeats *bool   `json:"hasFreeSeats,omitempty"`
}

// RoomDeletedDTO notifies room and lobby subscribers that a room was deleted/ended.
type RoomDeletedDTO struct {
	RoomID string `json:"roomId"`
	Reason string `json:"reason,omitempty"`
}

// RoomHostChangedDTO notifies subscribers when host ownership transfers.
type RoomHostChangedDTO struct {
	RoomID         string `json:"roomId"`
	PreviousHostID string `json:"previousHostId"`
	NewHostID      string `json:"newHostId"`
}

// VoiceMuteChangedDTO notifies peers of mute/deafen/hand-raise updates (including host force-mute).
type VoiceMuteChangedDTO struct {
	RoomID       string `json:"roomId"`
	UserID       string `json:"userId"`
	IsMuted      bool   `json:"isMuted"`
	IsDeafened   *bool  `json:"isDeafened,omitempty"`
	HandRaised   *bool  `json:"handRaised,omitempty"`
	ForcedByHost bool   `json:"forcedByHost"`
	Reason       string `json:"reason,omitempty"`
}

// VoiceSpeakingChangedDTO notifies peers of active voice-activity level changes.
type VoiceSpeakingChangedDTO struct {
	RoomID     string  `json:"roomId"`
	UserID     string  `json:"userId"`
	IsSpeaking bool    `json:"isSpeaking"`
	Level      float64 `json:"level"`
}

// WebRTCOfferDTO relays an SDP offer between authenticated room peers.
type WebRTCOfferDTO struct {
	RoomID         string `json:"roomId"`
	TargetSocketID string `json:"targetSocketId"`
	SenderSocketID string `json:"senderSocketId"`
	SenderUserID   string `json:"senderUserId"`
	SDP            any    `json:"sdp"`
}

// WebRTCAnswerDTO relays an SDP answer between authenticated room peers.
type WebRTCAnswerDTO struct {
	RoomID         string `json:"roomId"`
	TargetSocketID string `json:"targetSocketId"`
	SenderSocketID string `json:"senderSocketId"`
	SenderUserID   string `json:"senderUserId"`
	SDP            any    `json:"sdp"`
}

// WebRTCIceCandidateDTO relays an ICE candidate between authenticated room peers.
type WebRTCIceCandidateDTO struct {
	RoomID         string `json:"roomId"`
	TargetSocketID string `json:"targetSocketId"`
	SenderSocketID string `json:"senderSocketId"`
	SenderUserID   string `json:"senderUserId"`
	Candidate      any    `json:"candidate"`
}

// ScreenShareDTO notifies room peers of screen share start/stop transitions.
type ScreenShareDTO struct {
	RoomID    string `json:"roomId"`
	UserID    string `json:"userId"`
	SocketID  string `json:"socketId"`
	IsSharing bool   `json:"isSharing"`
}

// ChatSenderDTO identifies the author of a chat message.
type ChatSenderDTO struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl,omitempty"`
}

// ChatMessageDTO is the canonical camelCase chat message wire representation.
type ChatMessageDTO struct {
	ID              string              `json:"id"`
	ClientMessageID string              `json:"clientMessageId,omitempty"`
	RoomID          string              `json:"roomId"`
	Sender          ChatSenderDTO       `json:"sender"`
	Content         string              `json:"content"`
	Type            string              `json:"type"`
	MediaURL        string              `json:"mediaUrl,omitempty"`
	MediaType       string              `json:"mediaType,omitempty"`
	IsHighlighted   bool                `json:"isHighlighted"`
	Reactions       map[string][]string `json:"reactions"`
	CreatedAt       string              `json:"createdAt"`
}

// NewChatMessageDTO returns a ChatMessageDTO with a guaranteed non-nil Reactions map.
func NewChatMessageDTO(msg ChatMessageDTO) ChatMessageDTO {
	if msg.Reactions == nil {
		msg.Reactions = make(map[string][]string)
	} else {
		for emoji, users := range msg.Reactions {
			if users == nil {
				msg.Reactions[emoji] = make([]string, 0)
			}
		}
	}
	return msg
}

// NormalizeReactionsMap converts stored JSONB reactions (including legacy [] arrays or null)
// into a non-nil map[string][]string where each entry also has a non-nil slice.
func NormalizeReactionsMap(raw []byte) map[string][]string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("[]")) || bytes.Equal(trimmed, []byte("{}")) {
		return make(map[string][]string)
	}

	var mapped map[string][]string
	if err := json.Unmarshal(trimmed, &mapped); err == nil {
		if mapped == nil {
			return make(map[string][]string)
		}
		for emoji, users := range mapped {
			if users == nil {
				mapped[emoji] = make([]string, 0)
			}
		}
		return mapped
	}

	var legacyList []string
	if err := json.Unmarshal(trimmed, &legacyList); err == nil {
		out := make(map[string][]string, len(legacyList))
		for _, emoji := range legacyList {
			if emoji != "" {
				out[emoji] = make([]string, 0)
			}
		}
		return out
	}

	return make(map[string][]string)
}

// ChatReactionUpdatedDTO notifies room peers when a reaction is toggled/added on a message.
type ChatReactionUpdatedDTO struct {
	RoomID    string              `json:"roomId"`
	MessageID string              `json:"messageId"`
	Emoji     string              `json:"emoji"`
	UserID    string              `json:"userId"`
	Reactions map[string][]string `json:"reactions,omitempty"`
}

// ModerationParticipantKickedDTO notifies target/peers when a host removes a participant.
type ModerationParticipantKickedDTO struct {
	RoomID       string `json:"roomId"`
	TargetUserID string `json:"targetUserId"`
	KickedBy     string `json:"kickedBy"`
	Reason       string `json:"reason"`
}

// ModerationRoomLockedDTO notifies room subscribers when a host locks or unlocks a room.
type ModerationRoomLockedDTO struct {
	RoomID    string `json:"roomId"`
	IsLocked  bool   `json:"isLocked"`
	UpdatedBy string `json:"updatedBy"`
}

// LobbySubscriptionAckDTO is returned in command.ack for lobby.join and lobby.leave.
type LobbySubscriptionAckDTO struct {
	Subscribed bool `json:"subscribed"`
}

// RoomJoinAckDTO is returned in command.ack for room.join.
type RoomJoinAckDTO struct {
	RoomID   string          `json:"roomId"`
	SocketID string          `json:"socketId"`
	Version  int64           `json:"version"`
	Snapshot RoomSnapshotDTO `json:"snapshot"`
}

// RoomLeaveAckDTO is returned in command.ack for room.leave.
type RoomLeaveAckDTO struct {
	RoomID string `json:"roomId"`
	Left   bool   `json:"left"`
}

// DeliveryAckDTO is returned in command.ack for relayed WebRTC signaling commands.
type DeliveryAckDTO struct {
	Delivered bool `json:"delivered"`
}

// MeshPingAckDTO is returned in command.ack for mesh.ping.
type MeshPingAckDTO struct {
	ServerTime int64 `json:"serverTime"`
}

// ModerationMuteAckDTO is returned in command.ack for moderation.mute_participant.
type ModerationMuteAckDTO struct {
	RoomID       string `json:"roomId"`
	TargetUserID string `json:"targetUserId"`
	IsMuted      bool   `json:"isMuted"`
}

// ModerationKickAckDTO is returned in command.ack for moderation.kick_participant.
type ModerationKickAckDTO struct {
	RoomID       string `json:"roomId"`
	TargetUserID string `json:"targetUserId"`
}

// ModerationLockAckDTO is returned in command.ack for moderation.lock_room.
type ModerationLockAckDTO struct {
	RoomID   string `json:"roomId"`
	IsLocked bool   `json:"isLocked"`
}

// ScreenShareAckDTO is returned in command.ack for screen_share.start and screen_share.stop.
type ScreenShareAckDTO struct {
	RoomID    string `json:"roomId"`
	IsSharing bool   `json:"isSharing"`
}

// ChatReactAckDTO is returned in command.ack for chat.react.
type ChatReactAckDTO struct {
	RoomID    string `json:"roomId"`
	MessageID string `json:"messageId"`
	Emoji     string `json:"emoji"`
}
