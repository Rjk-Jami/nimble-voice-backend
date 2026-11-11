package webrtc_test

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nimble-voice-backend/config"
	"nimble-voice-backend/module"
	"nimble-voice-backend/module/v1/webrtc"
	"nimble-voice-backend/utils/jwt"

	"github.com/gin-gonic/gin"
)

type apiEnvelope struct {
	Status  int                       `json:"status"`
	Message string                    `json:"message"`
	Data    webrtc.IceServersResponse `json:"data"`
}

func expectedCoturnHMAC(secret, username string) string {
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write([]byte(username))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func decodeIceResponse(t *testing.T, body []byte) apiEnvelope {
	t.Helper()
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("failed to unmarshal ICE response JSON: %v (body: %s)", err, string(body))
	}
	return env
}

func TestGetIceServers_AuthAndSTUNOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwt.SetSecretKeyForTest("test-webrtc-jwt-secret-key-at-least-32-chars")

	cfg := &config.Config{
		Port:               "8080",
		Environment:        "test",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		WebRTCSTUNURLs:     []string{"stun:stun.l.google.com:19302", "stun:stun.cloudflare.com:3478"},
		TURNEnabled:        false,
	}

	router := module.SetupRouter(cfg, nil, nil)

	t.Run("unauthenticated request returns 401", func(t *testing.T) {
		for _, path := range []string{"/api/v1/webrtc/ice-servers", "/api/webrtc/ice-servers"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Fatalf("GET %s status = %d, want 401", path, w.Code)
			}
		}
	})

	t.Run("authenticated request with TURN_ENABLED=false returns STUN-only and no-store headers", func(t *testing.T) {
		token, err := jwt.GenerateToken("user-123", "user@example.com", "USER", "", "")
		if err != nil {
			t.Fatalf("failed to generate test token: %v", err)
		}

		for _, path := range []string{"/api/v1/webrtc/ice-servers", "/api/webrtc/ice-servers"} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200 (body: %s)", path, w.Code, w.Body.String())
			}
			if got := w.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("GET %s Cache-Control = %q, want %q", path, got, "no-store")
			}
			if got := w.Header().Get("Pragma"); got != "no-cache" {
				t.Fatalf("GET %s Pragma = %q, want %q", path, got, "no-cache")
			}

			env := decodeIceResponse(t, w.Body.Bytes())
			if env.Status != http.StatusOK {
				t.Fatalf("GET %s envelope status = %d, want 200", path, env.Status)
			}
			if env.Data.TurnEnabled {
				t.Fatalf("GET %s turnEnabled = true, want false", path)
			}
			if env.Data.TTLSeconds != 0 {
				t.Fatalf("GET %s ttlSeconds = %d, want 0", path, env.Data.TTLSeconds)
			}
			if env.Data.ExpiresAt != "" {
				t.Fatalf("GET %s expiresAt = %q, want empty", path, env.Data.ExpiresAt)
			}
			if len(env.Data.IceServers) != 1 || len(env.Data.IceServers[0].URLs) != 2 {
				t.Fatalf("GET %s iceServers = %+v, want 1 STUN entry with 2 URLs", path, env.Data.IceServers)
			}
			if env.Data.IceServers[0].Username != "" || env.Data.IceServers[0].Credential != "" {
				t.Fatalf("GET %s STUN-only entry unexpectedly contains credentials: %+v", path, env.Data.IceServers[0])
			}
		}
	})
}

func TestGetIceServers_TURNEnabledAndCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwt.SetSecretKeyForTest("test-webrtc-jwt-secret-key-at-least-32-chars")

	const turnSecret = "super-sensitive-coturn-hmac-secret-998877"
	cfg := &config.Config{
		Port:                     "8080",
		Environment:              "test",
		CORSAllowedOrigins:       []string{"http://localhost:3000"},
		WebRTCSTUNURLs:           []string{"stun:stun.l.google.com:19302"},
		TURNEnabled:              true,
		TURNURLs:                 []string{"turn:turn.example.com:3478?transport=udp", "turns:turn.example.com:5349?transport=tcp"},
		TURNSharedSecret:         turnSecret,
		TURNCredentialTTLSeconds: 3600,
	}

	router := module.SetupRouter(cfg, nil, nil)

	token, err := jwt.GenerateToken("user:with:colons", "user@example.com", "USER", "", "")
	if err != nil {
		t.Fatalf("failed to generate test token: %v", err)
	}

	doReq := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/webrtc/ice-servers", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	w1 := doReq()
	if w1.Code != http.StatusOK {
		t.Fatalf("first TURN request status = %d, want 200 (body: %s)", w1.Code, w1.Body.String())
	}
	if got := w1.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
	}
	if got := w1.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want %q", got, "no-cache")
	}
	if strings.Contains(w1.Body.String(), turnSecret) {
		t.Fatalf("response body leaked TURN_SHARED_SECRET")
	}

	env1 := decodeIceResponse(t, w1.Body.Bytes())
	if !env1.Data.TurnEnabled {
		t.Fatalf("turnEnabled = false, want true")
	}
	if env1.Data.TTLSeconds != 3600 {
		t.Fatalf("ttlSeconds = %d, want 3600", env1.Data.TTLSeconds)
	}
	if _, err := time.Parse(time.RFC3339, env1.Data.ExpiresAt); err != nil {
		t.Fatalf("expiresAt %q is not valid RFC3339: %v", env1.Data.ExpiresAt, err)
	}
	if len(env1.Data.IceServers) != 2 {
		t.Fatalf("len(iceServers) = %d, want 2 (STUN + TURN)", len(env1.Data.IceServers))
	}
	turnEntry := env1.Data.IceServers[1]
	if len(turnEntry.URLs) != 2 || turnEntry.URLs[0] != cfg.TURNURLs[0] || turnEntry.URLs[1] != cfg.TURNURLs[1] {
		t.Fatalf("TURN URLs = %v, want %v", turnEntry.URLs, cfg.TURNURLs)
	}

	parts := strings.SplitN(turnEntry.Username, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("username %q does not match <unix>:<user>", turnEntry.Username)
	}
	if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
		t.Fatalf("username unix prefix %q is not an integer: %v", parts[0], err)
	}
	if parts[1] != "user_with_colons" {
		t.Fatalf("sanitized user = %q, want %q", parts[1], "user_with_colons")
	}
	wantCred := expectedCoturnHMAC(turnSecret, turnEntry.Username)
	if turnEntry.Credential != wantCred {
		t.Fatalf("credential = %q, want HMAC-SHA1 %q", turnEntry.Credential, wantCred)
	}

	// Second call within TTL window must hit RAM cache and return identical TURN credentials.
	w2 := doReq()
	if w2.Code != http.StatusOK {
		t.Fatalf("second TURN request status = %d, want 200", w2.Code)
	}
	env2 := decodeIceResponse(t, w2.Body.Bytes())
	if env2.Data.IceServers[1].Username != turnEntry.Username ||
		env2.Data.IceServers[1].Credential != turnEntry.Credential ||
		env2.Data.ExpiresAt != env1.Data.ExpiresAt {
		t.Fatalf("expected RAM cache hit, got username=%q/%q cred=%q/%q exp=%q/%q",
			env2.Data.IceServers[1].Username, turnEntry.Username,
			env2.Data.IceServers[1].Credential, turnEntry.Credential,
			env2.Data.ExpiresAt, env1.Data.ExpiresAt)
	}
}

func TestWebRTCService_ClockDrivenCacheHitAndPreExpiryRefresh(t *testing.T) {
	const turnSecret = "clock-test-turn-secret"

	t.Run("3600s TTL uses 900s refresh margin and refreshes near expiry", func(t *testing.T) {
		currentTime := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
		cfg := &config.Config{
			WebRTCSTUNURLs:           []string{"stun:stun.l.google.com:19302"},
			TURNEnabled:              true,
			TURNURLs:                 []string{"turn:turn.example.com:3478"},
			TURNSharedSecret:         turnSecret,
			TURNCredentialTTLSeconds: 3600,
		}
		svc := webrtc.NewWebRTCServiceWithClock(cfg, func() time.Time { return currentTime })

		res1 := svc.GetIceServers("user-alpha").Data.(webrtc.IceServersResponse)
		turn1 := res1.IceServers[1]

		// Advance 2699s (remaining = 901s > 900s margin) -> cache hit.
		currentTime = currentTime.Add(2699 * time.Second)
		res2 := svc.GetIceServers("user-alpha").Data.(webrtc.IceServersResponse)
		turn2 := res2.IceServers[1]
		if turn2.Username != turn1.Username || turn2.Credential != turn1.Credential || res2.ExpiresAt != res1.ExpiresAt {
			t.Fatalf("expected cache hit at +2699s, got %+v vs %+v", res2, res1)
		}
		if res2.TTLSeconds != 901 {
			t.Fatalf("cached remaining ttlSeconds = %d, want 901", res2.TTLSeconds)
		}

		// Advance 1s more (remaining = 900s <= 900s margin) -> pre-expiry refresh.
		currentTime = currentTime.Add(1 * time.Second)
		res3 := svc.GetIceServers("user-alpha").Data.(webrtc.IceServersResponse)
		turn3 := res3.IceServers[1]
		if turn3.Username == turn1.Username || turn3.Credential == turn1.Credential || res3.ExpiresAt == res1.ExpiresAt {
			t.Fatalf("expected pre-expiry refresh at +2700s, got identical creds %+v", res3)
		}
		wantExp3 := currentTime.Add(3600 * time.Second).Format(time.RFC3339)
		if res3.ExpiresAt != wantExp3 {
			t.Fatalf("refreshed expiresAt = %q, want %q", res3.ExpiresAt, wantExp3)
		}
		if turn3.Credential != expectedCoturnHMAC(turnSecret, turn3.Username) {
			t.Fatalf("refreshed credential failed HMAC verification")
		}
	})

	t.Run("small TTL (40s) caps refresh margin at ttl/2 (20s) and sanitizes empty userID to guest", func(t *testing.T) {
		currentTime := time.Date(2026, 11, 11, 13, 0, 0, 0, time.UTC)
		cfg := &config.Config{
			TURNEnabled:              true,
			TURNURLs:                 []string{"turn:turn.example.com:3478"},
			TURNSharedSecret:         turnSecret,
			TURNCredentialTTLSeconds: 40,
		}
		svc := webrtc.NewWebRTCServiceWithClock(cfg, func() time.Time { return currentTime })

		res1 := svc.GetIceServers("   ").Data.(webrtc.IceServersResponse)
		if len(res1.IceServers) != 1 {
			t.Fatalf("len(iceServers) = %d, want 1 TURN-only entry", len(res1.IceServers))
		}
		wantUser1 := fmt.Sprintf("%d:guest", currentTime.Add(40*time.Second).Unix())
		if res1.IceServers[0].Username != wantUser1 {
			t.Fatalf("username = %q, want %q", res1.IceServers[0].Username, wantUser1)
		}

		// +19s -> remaining 21s > 20s -> cache hit.
		currentTime = currentTime.Add(19 * time.Second)
		res2 := svc.GetIceServers(" ").Data.(webrtc.IceServersResponse)
		if res2.IceServers[0].Username != wantUser1 {
			t.Fatalf("expected cache hit at +19s, got %q", res2.IceServers[0].Username)
		}

		// +1s -> remaining 20s <= 20s -> refresh.
		currentTime = currentTime.Add(1 * time.Second)
		res3 := svc.GetIceServers("").Data.(webrtc.IceServersResponse)
		wantUser3 := fmt.Sprintf("%d:guest", currentTime.Add(40*time.Second).Unix())
		if res3.IceServers[0].Username != wantUser3 {
			t.Fatalf("expected refresh at +20s with username %q, got %q", wantUser3, res3.IceServers[0].Username)
		}
	})
}

func TestGetIceServers_InvalidEnabledTURNConfigReturnsSanitized500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwt.SetSecretKeyForTest("test-webrtc-jwt-secret-key-at-least-32-chars")

	token, err := jwt.GenerateToken("user-500", "u@example.com", "USER", "", "")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	cases := []struct {
		name string
		cfg  *config.Config
	}{
		{
			name: "missing TURN_SHARED_SECRET",
			cfg: &config.Config{
				TURNEnabled:              true,
				TURNURLs:                 []string{"turn:turn.example.com:3478"},
				TURNSharedSecret:         "",
				TURNCredentialTTLSeconds: 3600,
			},
		},
		{
			name: "missing TURN_URLS",
			cfg: &config.Config{
				TURNEnabled:              true,
				TURNURLs:                 nil,
				TURNSharedSecret:         "must-never-leak-secret-value",
				TURNCredentialTTLSeconds: 3600,
			},
		},
		{
			name: "non-positive TTL",
			cfg: &config.Config{
				TURNEnabled:              true,
				TURNURLs:                 []string{"turn:turn.example.com:3478"},
				TURNSharedSecret:         "must-never-leak-secret-value",
				TURNCredentialTTLSeconds: 0,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := module.SetupRouter(tc.cfg, nil, nil)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/webrtc/ice-servers", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", w.Code)
			}
			if got := w.Header().Get("Cache-Control"); got != "no-store" {
				t.Fatalf("Cache-Control = %q, want %q", got, "no-store")
			}
			body := w.Body.String()
			if strings.Contains(body, "must-never-leak-secret-value") || strings.Contains(body, "incomplete") {
				t.Fatalf("500 response leaked internal details: %s", body)
			}

			var payload map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
				t.Fatalf("invalid JSON 500 body: %v", err)
			}
			if payload["data"] != nil {
				t.Fatalf("expected nil data on sanitized 500, got %v", payload["data"])
			}
		})
	}
}

func TestGetIceServers_ConcurrentAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwt.SetSecretKeyForTest("test-webrtc-jwt-secret-key-at-least-32-chars")

	const turnSecret = "concurrent-turn-secret-value"
	cfg := &config.Config{
		Port:                     "8080",
		Environment:              "test",
		CORSAllowedOrigins:       []string{"http://localhost:3000"},
		WebRTCSTUNURLs:           []string{"stun:stun.l.google.com:19302"},
		TURNEnabled:              true,
		TURNURLs:                 []string{"turn:turn.example.com:3478"},
		TURNSharedSecret:         turnSecret,
		TURNCredentialTTLSeconds: 3600,
	}
	router := module.SetupRouter(cfg, nil, nil)

	tokens := make([]string, 4)
	for i := range tokens {
		tok, err := jwt.GenerateToken(fmt.Sprintf("concurrent-user-%d", i), "u@example.com", "USER", "", "")
		if err != nil {
			t.Fatalf("failed to generate token %d: %v", i, err)
		}
		tokens[i] = tok
	}

	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/webrtc/ice-servers", nil)
			req.RemoteAddr = fmt.Sprintf("10.0.0.%d:1234", idx+1)
			req.Header.Set("Authorization", "Bearer "+tokens[idx%len(tokens)])
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				errs <- fmt.Errorf("worker %d status = %d, want 200", idx, w.Code)
				return
			}
			if strings.Contains(w.Body.String(), turnSecret) {
				errs <- fmt.Errorf("worker %d leaked TURN secret", idx)
				return
			}
			var env apiEnvelope
			if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
				errs <- fmt.Errorf("worker %d JSON error: %v", idx, err)
				return
			}
			if len(env.Data.IceServers) != 2 {
				errs <- fmt.Errorf("worker %d len(iceServers) = %d, want 2", idx, len(env.Data.IceServers))
				return
			}
			turnEntry := env.Data.IceServers[1]
			if turnEntry.Credential != expectedCoturnHMAC(turnSecret, turnEntry.Username) {
				errs <- fmt.Errorf("worker %d invalid HMAC credential", idx)
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Fatal(err)
	}

	// Direct service-level concurrency stress on a single cache key.
	svc := webrtc.NewWebRTCService(cfg)
	const svcWorkers = 64
	var svcWg sync.WaitGroup
	svcErrs := make(chan error, svcWorkers)
	for i := 0; i < svcWorkers; i++ {
		svcWg.Add(1)
		go func(idx int) {
			defer svcWg.Done()
			res := svc.GetIceServers("shared-concurrent-user")
			if res.StatusCode != http.StatusOK {
				svcErrs <- fmt.Errorf("svc worker %d status = %d, want 200", idx, res.StatusCode)
				return
			}
			payload, ok := res.Data.(webrtc.IceServersResponse)
			if !ok || len(payload.IceServers) != 2 {
				svcErrs <- fmt.Errorf("svc worker %d unexpected payload: %+v", idx, res.Data)
				return
			}
			turnEntry := payload.IceServers[1]
			if turnEntry.Credential != expectedCoturnHMAC(turnSecret, turnEntry.Username) {
				svcErrs <- fmt.Errorf("svc worker %d invalid HMAC credential", idx)
			}
		}(i)
	}
	svcWg.Wait()
	close(svcErrs)
	for err := range svcErrs {
		t.Fatal(err)
	}
}
