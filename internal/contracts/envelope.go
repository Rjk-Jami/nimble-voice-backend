package contracts

import (
	"time"

	"github.com/lucsky/cuid"
)

// SchemaVersion is the current wire contract schema version for ServerEventEnvelope.
const SchemaVersion = 1

// Standardized command.ack error codes.
const (
	ErrCodeUnauthorized = "UNAUTHORIZED"
	ErrCodeForbidden    = "FORBIDDEN"
	ErrCodeNotFound     = "NOT_FOUND"
	ErrCodeValidation   = "VALIDATION_ERROR"
	ErrCodeConflict     = "CONFLICT"
	ErrCodeRoomFull     = "ROOM_FULL"
	ErrCodeRoomLocked   = "ROOM_LOCKED"
	ErrCodeInternal     = "INTERNAL_ERROR"
)

// ServerEventEnvelope wraps every canonical server-to-client Socket.IO event.
type ServerEventEnvelope[T any] struct {
	EventID       string `json:"eventId"`
	Event         string `json:"event"`
	SchemaVersion int    `json:"schemaVersion"`
	OccurredAt    string `json:"occurredAt"`
	RoomID        string `json:"roomId,omitempty"`
	Version       int64  `json:"version"`
	Data          T      `json:"data"`
}

// NewServerEventEnvelope builds a versioned envelope with a unique cuid EventID and UTC RFC3339Nano timestamp.
func NewServerEventEnvelope[T any](event, roomID string, version int64, data T) ServerEventEnvelope[T] {
	return ServerEventEnvelope[T]{
		EventID:       cuid.New(),
		Event:         event,
		SchemaVersion: SchemaVersion,
		OccurredAt:    time.Now().UTC().Format(time.RFC3339Nano),
		RoomID:        roomID,
		Version:       version,
		Data:          data,
	}
}

// AckError represents a structured command failure returned in command.ack.
type AckError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CommandAck correlates a client-issued requestId with an authoritative server outcome.
type CommandAck[T any] struct {
	Ok        bool      `json:"ok"`
	RequestID string    `json:"requestId"`
	Data      *T        `json:"data,omitempty"`
	Error     *AckError `json:"error,omitempty"`
}

// NewAckOK creates a successful CommandAck for requestID with payload data.
func NewAckOK[T any](requestID string, data T) CommandAck[T] {
	return CommandAck[T]{
		Ok:        true,
		RequestID: requestID,
		Data:      &data,
	}
}

// NewAckError creates a failed CommandAck for requestID with a standardized error code and public message.
func NewAckError(requestID, code, message string) CommandAck[struct{}] {
	return CommandAck[struct{}]{
		Ok:        false,
		RequestID: requestID,
		Error: &AckError{
			Code:    code,
			Message: message,
		},
	}
}
