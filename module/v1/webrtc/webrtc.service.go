package webrtc

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
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
	if stunURLs := filterNonEmptyStrings(s.cfg.WebRTCSTUNURLs); len(stunURLs) > 0 {
		iceServers = append(iceServers, RTCIceServerDTO{
			URLs: stunURLs,
		})
	}

	if !s.cfg.TURNEnabled {
		return httpx.SendData(http.StatusOK, "ICE servers retrieved successfully", IceServersResponse{
			IceServers:  iceServers,
			TurnEnabled: false,
			TTLSeconds:  0,
		})
	}

	turnURLs := filterNonEmptyStrings(s.cfg.TURNURLs)
	if len(turnURLs) == 0 || strings.TrimSpace(s.cfg.TURNSharedSecret) == "" || s.cfg.TURNCredentialTTLSeconds <= 0 {
		return httpx.InternalError("webrtc.ice_servers", errors.New("turn configuration is incomplete"), "TURN configuration is invalid")
	}

	now := s.now().UTC()
	entry := s.getOrRefreshTurnCredential(userID, now)

	remainingSeconds := int(math.Ceil(entry.expiresAt.Sub(now).Seconds()))
	if remainingSeconds < 1 {
		remainingSeconds = 1
	}

	iceServers = append(iceServers, RTCIceServerDTO{
		URLs:       turnURLs,
		Username:   entry.username,
		Credential: entry.credential,
	})

	return httpx.SendData(http.StatusOK, "ICE servers retrieved successfully", IceServersResponse{
		IceServers:  iceServers,
		TurnEnabled: true,
		TTLSeconds:  remainingSeconds,
		ExpiresAt:   entry.expiresAt.UTC().Format(time.RFC3339),
	})
}

func (s *WebRTCService) getOrRefreshTurnCredential(userID string, now time.Time) cachedTurnCredential {
	cacheKey := sanitizeCoturnUserID(userID)
	margin := computeRefreshMargin(s.cfg.TURNCredentialTTLSeconds)

	s.mu.RLock()
	entry, ok := s.cache[cacheKey]
	s.mu.RUnlock()

	if ok && entry.expiresAt.Sub(now) > margin {
		return entry
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok = s.cache[cacheKey]
	if ok && entry.expiresAt.Sub(now) > margin {
		return entry
	}

	username, credential, expiresAt := generateCoturnCredentials(
		userID,
		s.cfg.TURNSharedSecret,
		s.cfg.TURNCredentialTTLSeconds,
		now,
	)
	entry = cachedTurnCredential{
		username:   username,
		credential: credential,
		expiresAt:  expiresAt,
	}
	s.cache[cacheKey] = entry
	return entry
}

// generateCoturnCredentials builds ephemeral RFC draft-uberti-behave-turn-rest-00 credentials.
func generateCoturnCredentials(userID, sharedSecret string, ttlSeconds int, now time.Time) (username, credential string, expiresAt time.Time) {
	sanitizedUser := sanitizeCoturnUserID(userID)
	expiresAt = now.UTC().Add(time.Duration(ttlSeconds) * time.Second)
	username = fmt.Sprintf("%d:%s", expiresAt.Unix(), sanitizedUser)

	mac := hmac.New(sha1.New, []byte(sharedSecret))
	_, _ = mac.Write([]byte(username))
	credential = base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return username, credential, expiresAt
}

func sanitizeCoturnUserID(userID string) string {
	sanitized := strings.ReplaceAll(strings.TrimSpace(userID), ":", "_")
	if sanitized == "" {
		return "guest"
	}
	return sanitized
}

func computeRefreshMargin(ttlSeconds int) time.Duration {
	ttl := time.Duration(ttlSeconds) * time.Second
	margin := ttl / 4
	if margin < 30*time.Second {
		margin = 30 * time.Second
	}
	if ttl <= 60*time.Second {
		maxMargin := ttl / 2
		if margin > maxMargin {
			margin = maxMargin
		}
	}
	return margin
}

func filterNonEmptyStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	dst := make([]string, 0, len(src))
	for _, item := range src {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			dst = append(dst, trimmed)
		}
	}
	return dst
}
