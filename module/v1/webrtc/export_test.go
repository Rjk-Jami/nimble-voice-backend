package webrtc

import (
	"time"

	"nimble-voice-backend/config"
)

// NewWebRTCServiceWithClock exports newWebRTCServiceWithClock for external package tests.
func NewWebRTCServiceWithClock(cfg *config.Config, nowFn func() time.Time) *WebRTCService {
	return newWebRTCServiceWithClock(cfg, nowFn)
}
