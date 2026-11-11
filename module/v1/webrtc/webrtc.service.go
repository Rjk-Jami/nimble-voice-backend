package webrtc

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"nimble-voice-backend/config"
	"nimble-voice-backend/utils/httpx"
)

type WebRTCServiceInterface interface {
	GetIceServers(userID string) httpx.APIResponse
}

type cachedTurnCredential struct {
	username   string
	credential string
	expiresAt  time.Time
}

type WebRTCService struct {
	cfg   *config.Config
	now   func() time.Time
	mu    sync.RWMutex
	cache map[string]cachedTurnCredential
}

func NewWebRTCService(cfg *config.Config) WebRTCServiceInterface {
	return newWebRTCServiceWithClock(cfg, time.Now)
}

func newWebRTCServiceWithClock(cfg *config.Config, nowFn func() time.Time) *WebRTCService {
	if nowFn == nil {
		nowFn = time.Now
	}
	return &WebRTCService{
		cfg:   cfg,
		now:   nowFn,
		cache: make(map[string]cachedTurnCredential),
	}
}

func (s *WebRTCService) GetIceServers(userID string) httpx.APIResponse {
	if s.cfg == nil {
		return httpx.InternalError("webrtc.ice_servers", errors.New("webrtc config is nil"), "WebRTC configuration is unavailable")
	}

	iceServers := make([]RTCIceServerDTO, 0, 2)
	if len(s.cfg.WebRTCSTUNURLs) > 0 {
		iceServers = append(iceServers, RTCIceServerDTO{
			URLs: copyStringSlice(s.cfg.WebRTCSTUNURLs),
		})
	}

	if !s.cfg.TURNEnabled {
		return httpx.SendData(http.StatusOK, "ICE servers retrieved successfully", IceServersResponse{
			IceServers:  iceServers,
			TurnEnabled: false,
			TTLSeconds:  0,
		})
	}

	return httpx.InternalError("webrtc.ice_servers", errors.New("TURN provisioning not yet implemented"), "TURN provisioning is unavailable")
}

func copyStringSlice(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}
