package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func baseValidEnv() map[string]string {
	return map[string]string{
		"DB_PASSWORD": "strong-local-db-password-2026",
		"JWT_SECRET":  "strong-local-jwt-secret-at-least-32-chars",
	}
}

func mapLookup(m map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestLoadConfigFromLookup_ValidDefaultsAndZeroInfra(t *testing.T) {
	env := baseValidEnv()
	cfg, err := LoadConfigFromLookup(mapLookup(env))
	if err != nil {
		t.Fatalf("expected valid minimal env to succeed, got error: %v", err)
	}

	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want 8080", cfg.Port)
	}
	if cfg.Environment != "development" {
		t.Errorf("Environment = %q, want development", cfg.Environment)
	}
	if !cfg.SwaggerEnabled {
		t.Errorf("SwaggerEnabled = false, want true in development")
	}
	if cfg.DBMaxOpenConns != 25 || cfg.DBMaxIdleConns != 10 {
		t.Errorf("DB pool = (%d, %d), want (25, 10)", cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)
	}
	if cfg.MaxFileSizeMB != 5 {
		t.Errorf("MaxFileSizeMB = %d, want 5", cfg.MaxFileSizeMB)
	}
	if cfg.RoomDefaultMaxParticipants != 6 {
		t.Errorf("RoomDefaultMaxParticipants = %d, want 6", cfg.RoomDefaultMaxParticipants)
	}
	if cfg.RoomIdleGraceSeconds != 30 {
		t.Errorf("RoomIdleGraceSeconds = %d, want 30", cfg.RoomIdleGraceSeconds)
	}
	if cfg.TURNEnabled {
		t.Errorf("TURNEnabled = true, want false by default")
	}
	if cfg.TURNCredentialTTLSeconds != 3600 {
		t.Errorf("TURNCredentialTTLSeconds = %d, want 3600", cfg.TURNCredentialTTLSeconds)
	}
	if cfg.RedisEnabled {
		t.Errorf("RedisEnabled = true, want false by default")
	}
	if cfg.PresencePingInterval != 25*time.Second || cfg.PresencePongTimeout != 60*time.Second {
		t.Errorf("Presence durations = (%v, %v), want (25s, 60s)", cfg.PresencePingInterval, cfg.PresencePongTimeout)
	}
	if len(cfg.WebRTCSTUNURLs) != 2 {
		t.Errorf("WebRTCSTUNURLs = %v, want 2 default STUN URLs", cfg.WebRTCSTUNURLs)
	}
	if !cfg.IsOriginAllowed("http://localhost:3000/") {
		t.Errorf("expected default http://localhost:3000 origin to be allowed in development")
	}
}

func TestLoadConfigFromLookup_MissingAndPlaceholderSecretsAggregated(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantSubs []string
	}{
		{
			name:     "missing both DB_PASSWORD and JWT_SECRET reports both via errors.Join",
			env:      map[string]string{},
			wantSubs: []string{"DB_PASSWORD", "JWT_SECRET"},
		},
		{
			name: "whitespace DB_PASSWORD and placeholder JWT_SECRET rejected",
			env: map[string]string{
				"DB_PASSWORD": "   ",
				"JWT_SECRET":  "default_jwt_secret",
			},
			wantSubs: []string{"DB_PASSWORD", "JWT_SECRET"},
		},
		{
			name: "known committed DB_PASSWORD and JWT_SECRET literals rejected",
			env: map[string]string{
				"DB_PASSWORD": "rjk.207235",
				"JWT_SECRET":  "enterprise_super_secret_jwt_key_2026",
			},
			wantSubs: []string{"DB_PASSWORD", "JWT_SECRET"},
		},
		{
			name: "production requires JWT_SECRET >= 32 chars and explicit CORS_ALLOWED_ORIGINS",
			env: map[string]string{
				"APP_ENV":     "production",
				"DB_PASSWORD": "strong-prod-db-password",
				"JWT_SECRET":  "short-secret-under-32-chars",
			},
			wantSubs: []string{"JWT_SECRET", "CORS_ALLOWED_ORIGINS"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadConfigFromLookup(mapLookup(tc.env))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			msg := err.Error()
			for _, sub := range tc.wantSubs {
				if !strings.Contains(msg, sub) {
					t.Errorf("expected aggregated error %q to mention %q", msg, sub)
				}
			}
		})
	}
}

func TestLoadConfigFromLookup_NumericBoundsAndTypedParsing(t *testing.T) {
	validTests := []struct {
		name      string
		overrides map[string]string
	}{
		{
			name: "minimum inclusive bounds",
			overrides: map[string]string{
				"PORT":                         "1",
				"DB_PORT":                      "1",
				"DB_MAX_OPEN_CONNS":            "1",
				"DB_MAX_IDLE_CONNS":            "0",
				"MAX_FILE_SIZE_MB":             "1",
				"ROOM_DEFAULT_MAX_PARTICIPANTS": "2",
				"TURN_CREDENTIAL_TTL_SECONDS":  "60",
				"ROOM_IDLE_GRACE_SECONDS":      "0",
			},
		},
		{
			name: "maximum inclusive bounds",
			overrides: map[string]string{
				"PORT":                         "65535",
				"DB_PORT":                      "65535",
				"DB_MAX_OPEN_CONNS":            "100",
				"DB_MAX_IDLE_CONNS":            "100",
				"MAX_FILE_SIZE_MB":             "100",
				"ROOM_DEFAULT_MAX_PARTICIPANTS": "20",
				"TURN_CREDENTIAL_TTL_SECONDS":  "86400",
				"ROOM_IDLE_GRACE_SECONDS":      "300",
			},
		},
	}

	for _, tc := range validTests {
		t.Run(tc.name, func(t *testing.T) {
			env := baseValidEnv()
			for k, v := range tc.overrides {
				env[k] = v
			}
			if _, err := LoadConfigFromLookup(mapLookup(env)); err != nil {
				t.Fatalf("expected inclusive bounds to succeed, got: %v", err)
			}
		})
	}

	invalidTests := []struct {
		name      string
		overrides map[string]string
		wantSub   string
	}{
		{name: "PORT 0", overrides: map[string]string{"PORT": "0"}, wantSub: "PORT"},
		{name: "PORT 65536", overrides: map[string]string{"PORT": "65536"}, wantSub: "PORT"},
		{name: "DB_PORT non-int", overrides: map[string]string{"DB_PORT": "abc"}, wantSub: "DB_PORT"},
		{name: "MAX_FILE_SIZE_MB 0", overrides: map[string]string{"MAX_FILE_SIZE_MB": "0"}, wantSub: "MAX_FILE_SIZE_MB"},
		{name: "MAX_FILE_SIZE_MB 101", overrides: map[string]string{"MAX_FILE_SIZE_MB": "101"}, wantSub: "MAX_FILE_SIZE_MB"},
		{name: "ROOM_DEFAULT_MAX_PARTICIPANTS 1", overrides: map[string]string{"ROOM_DEFAULT_MAX_PARTICIPANTS": "1"}, wantSub: "ROOM_DEFAULT_MAX_PARTICIPANTS"},
		{name: "ROOM_DEFAULT_MAX_PARTICIPANTS 21", overrides: map[string]string{"ROOM_DEFAULT_MAX_PARTICIPANTS": "21"}, wantSub: "ROOM_DEFAULT_MAX_PARTICIPANTS"},
		{name: "TURN_CREDENTIAL_TTL_SECONDS 59", overrides: map[string]string{"TURN_CREDENTIAL_TTL_SECONDS": "59"}, wantSub: "TURN_CREDENTIAL_TTL_SECONDS"},
		{name: "TURN_CREDENTIAL_TTL_SECONDS 86401", overrides: map[string]string{"TURN_CREDENTIAL_TTL_SECONDS": "86401"}, wantSub: "TURN_CREDENTIAL_TTL_SECONDS"},
		{name: "ROOM_IDLE_GRACE_SECONDS -1", overrides: map[string]string{"ROOM_IDLE_GRACE_SECONDS": "-1"}, wantSub: "ROOM_IDLE_GRACE_SECONDS"},
		{name: "DB_MAX_OPEN_CONNS 0", overrides: map[string]string{"DB_MAX_OPEN_CONNS": "0"}, wantSub: "DB_MAX_OPEN_CONNS"},
		{name: "DB_MAX_IDLE_CONNS > DB_MAX_OPEN_CONNS", overrides: map[string]string{"DB_MAX_OPEN_CONNS": "5", "DB_MAX_IDLE_CONNS": "6"}, wantSub: "DB_MAX_IDLE_CONNS"},
		{name: "SWAGGER_ENABLED invalid bool", overrides: map[string]string{"SWAGGER_ENABLED": "maybe"}, wantSub: "SWAGGER_ENABLED"},
		{name: "TURN_ENABLED invalid bool", overrides: map[string]string{"TURN_ENABLED": "yesplease"}, wantSub: "TURN_ENABLED"},
		{name: "REDIS_ENABLED invalid bool", overrides: map[string]string{"REDIS_ENABLED": "nah"}, wantSub: "REDIS_ENABLED"},
		{name: "PRESENCE_PING_INTERVAL invalid duration", overrides: map[string]string{"PRESENCE_PING_INTERVAL": "fast"}, wantSub: "PRESENCE_PING_INTERVAL"},
		{name: "PRESENCE_PONG_TIMEOUT non-positive duration", overrides: map[string]string{"PRESENCE_PONG_TIMEOUT": "0s"}, wantSub: "PRESENCE_PONG_TIMEOUT"},
	}

	for _, tc := range invalidTests {
		t.Run(tc.name, func(t *testing.T) {
			env := baseValidEnv()
			for k, v := range tc.overrides {
				env[k] = v
			}
			_, err := LoadConfigFromLookup(mapLookup(env))
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("expected error %q to contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestLoadConfigFromLookup_ConditionalTURNAndRedis(t *testing.T) {
	t.Run("TURN_ENABLED=true requires valid TURN_URLS and non-placeholder TURN_SHARED_SECRET", func(t *testing.T) {
		env := baseValidEnv()
		env["TURN_ENABLED"] = "true"
		_, err := LoadConfigFromLookup(mapLookup(env))
		if err == nil || !strings.Contains(err.Error(), "TURN_URLS") || !strings.Contains(err.Error(), "TURN_SHARED_SECRET") {
			t.Fatalf("expected TURN_URLS and TURN_SHARED_SECRET errors, got: %v", err)
		}

		env["TURN_URLS"] = "http://not-turn.example.com"
		env["TURN_SHARED_SECRET"] = "change_me"
		_, err = LoadConfigFromLookup(mapLookup(env))
		if err == nil || !strings.Contains(err.Error(), "TURN_URLS") || !strings.Contains(err.Error(), "TURN_SHARED_SECRET") {
			t.Fatalf("expected invalid TURN_URLS and placeholder TURN_SHARED_SECRET errors, got: %v", err)
		}

		env["TURN_URLS"] = "turn:turn.example.com:3478?transport=udp,turns:turn.example.com:5349"
		env["TURN_SHARED_SECRET"] = "real-coturn-hmac-secret-value"
		cfg, err := LoadConfigFromLookup(mapLookup(env))
		if err != nil {
			t.Fatalf("expected valid TURN config to succeed, got: %v", err)
		}
		if !cfg.TURNEnabled || len(cfg.TURNURLs) != 2 {
			t.Fatalf("unexpected TURN config: %+v", cfg)
		}
	})

	t.Run("REDIS_ENABLED=true requires valid redis/rediss URL", func(t *testing.T) {
		env := baseValidEnv()
		env["REDIS_ENABLED"] = "true"
		if _, err := LoadConfigFromLookup(mapLookup(env)); err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
			t.Fatalf("expected REDIS_URL error when REDIS_ENABLED=true, got: %v", err)
		}

		env["REDIS_URL"] = "http://localhost:6379"
		if _, err := LoadConfigFromLookup(mapLookup(env)); err == nil || !strings.Contains(err.Error(), "REDIS_URL") {
			t.Fatalf("expected invalid scheme REDIS_URL error, got: %v", err)
		}

		env["REDIS_URL"] = "rediss://cache.example.com:6380/0"
		cfg, err := LoadConfigFromLookup(mapLookup(env))
		if err != nil {
			t.Fatalf("expected valid rediss URL to succeed, got: %v", err)
		}
		if !cfg.RedisEnabled || cfg.RedisURL != "rediss://cache.example.com:6380/0" {
			t.Fatalf("unexpected Redis config: %+v", cfg)
		}
	})
}

func TestNormalizeOriginAndAllowlistPrecedence(t *testing.T) {
	if got, err := NormalizeOrigin("  HTTPS://App.Example.com:3000/  "); err != nil || got != "https://app.example.com:3000" {
		t.Fatalf("NormalizeOrigin got (%q, %v), want (https://app.example.com:3000, nil)", got, err)
	}

	for _, bad := range []string{"", "*", "https://*.example.com", "ftp://example.com", "https://example.com/path", "https://example.com?a=1", "http://user:pass@example.com"} {
		if _, err := NormalizeOrigin(bad); err == nil {
			t.Errorf("expected NormalizeOrigin(%q) to fail", bad)
		}
	}

	env := baseValidEnv()
	env["FRONTEND_URL"] = "https://fallback-url.example.com"
	env["FRONTEND_ORIGINS"] = "https://legacy-origins.example.com/"
	env["CORS_ALLOWED_ORIGINS"] = "https://primary.example.com/,http://localhost:3000"
	cfg, err := LoadConfigFromLookup(mapLookup(env))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.IsOriginAllowed("https://primary.example.com") || !cfg.IsOriginAllowed("http://localhost:3000/") {
		t.Errorf("expected primary origins to be allowed: %v", cfg.CORSAllowedOrigins)
	}
	if cfg.IsOriginAllowed("https://legacy-origins.example.com") || cfg.IsOriginAllowed("https://evil.example.com") {
		t.Errorf("expected overridden/unlisted origins to be denied")
	}

	// Deterministic repeated calls produce identical Config without global mutation
	cfg2, err2 := LoadConfigFromLookup(mapLookup(env))
	if err2 != nil || !reflect.DeepEqual(cfg, cfg2) {
		t.Fatalf("expected identical Config on repeated calls, got err=%v", err2)
	}
}

func TestLoadConfigFromLookup_STUNTrustedProxiesAndAppURL(t *testing.T) {
	env := baseValidEnv()
	env["TRUSTED_PROXIES"] = "127.0.0.1, 10.0.0.0/8"
	env["APP_URL"] = "https://api.example.com/"
	env["WEBRTC_STUN_URLS"] = "stun:stun.example.com:3478,stuns:stun.example.com:5349"

	cfg, err := LoadConfigFromLookup(mapLookup(env))
	if err != nil {
		t.Fatalf("expected valid proxies/STUN/APP_URL to pass, got: %v", err)
	}
	if cfg.AppURL != "https://api.example.com" {
		t.Errorf("AppURL = %q, want https://api.example.com", cfg.AppURL)
	}
	if len(cfg.TrustedProxies) != 2 {
		t.Errorf("TrustedProxies = %v, want 2 items", cfg.TrustedProxies)
	}
	if len(cfg.WebRTCSTUNURLs) != 2 {
		t.Errorf("WebRTCSTUNURLs = %v, want 2 items", cfg.WebRTCSTUNURLs)
	}

	badEnv := baseValidEnv()
	badEnv["TRUSTED_PROXIES"] = "not-an-ip"
	badEnv["APP_URL"] = "ftp://invalid"
	badEnv["WEBRTC_STUN_URLS"] = "http://not-stun"
	_, err = LoadConfigFromLookup(mapLookup(badEnv))
	if err == nil {
		t.Fatal("expected errors for invalid TRUSTED_PROXIES, APP_URL, WEBRTC_STUN_URLS")
	}
	for _, sub := range []string{"TRUSTED_PROXIES", "APP_URL", "WEBRTC_STUN_URLS"} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("expected error %q to contain %q", err.Error(), sub)
		}
	}
}
