package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                       string
	Environment                string
	AppURL                     string
	CORSAllowedOrigins         []string
	FrontendOrigins            []string
	TrustedProxies             []string
	SwaggerEnabled             bool
	PresencePingInterval       time.Duration
	PresencePongTimeout        time.Duration
	DBHost                     string
	DBPort                     string
	DBUser                     string
	DBPassword                 string
	DBName                     string
	DBSSLMode                  string
	DBTimezone                 string
	DBMaxOpenConns             int
	DBMaxIdleConns             int
	JWTSecret                  string
	UploadDir                  string
	MaxFileSizeMB              int64
	WebRTCSTUNURLs             []string
	TURNEnabled                bool
	TURNURLs                   []string
	TURNSharedSecret           string
	TURNCredentialTTLSeconds   int
	RedisEnabled               bool
	RedisURL                   string
	RoomDefaultMaxParticipants int
	RoomIdleGraceSeconds       int
}

var (
	disallowedDBPasswords = map[string]struct{}{
		"password":         {},
		"postgres":         {},
		"changeme":         {},
		"change_me":        {},
		"your_db_password": {},
	}
	disallowedJWTSecrets = map[string]struct{}{
		"default_jwt_secret":                   {},
		"enterprise_super_secret_jwt_key_2026": {},
		"changeme":                             {},
		"change_me":                            {},
		"secret":                               {},
		"your_jwt_secret":                      {},
	}
	disallowedTURNSecrets = map[string]struct{}{
		"changeme":                {},
		"change_me":               {},
		"secret":                  {},
		"turn_secret":             {},
		"your_turn_shared_secret": {},
	}
)

func LoadConfig() (*Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("failed to load .env file: %w", err)
	}
	return LoadConfigFromLookup(os.LookupEnv)
}

func LoadConfigFromLookup(lookup func(string) (string, bool)) (*Config, error) {
	if lookup == nil {
		return nil, errors.New("config lookup function is nil")
	}

	var errs []error
	cfg := &Config{}

	env, err := parseStringWithDefault(lookup, "APP_ENV", "development")
	if err != nil {
		errs = append(errs, err)
		env = "development"
	}
	cfg.Environment = env
	isProd := strings.EqualFold(env, "production")

	portInt, err := parseIntRange(lookup, "PORT", 8080, 1, 65535)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.Port = strconv.Itoa(portInt)
	}

	appURL, err := parseHTTPURL(lookup, "APP_URL", "http://localhost:8080")
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.AppURL = appURL
	}

	origins, err := parseAllowedOrigins(lookup, isProd)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.CORSAllowedOrigins = origins
		cfg.FrontendOrigins = append([]string(nil), origins...)
	}

	trustedProxies, err := parseTrustedProxies(lookup)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.TrustedProxies = trustedProxies
	}

	swaggerEnabled, err := parseBoolWithDefault(lookup, "SWAGGER_ENABLED", !isProd)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.SwaggerEnabled = swaggerEnabled
	}

	pingInterval, err := parsePositiveDuration(lookup, "PRESENCE_PING_INTERVAL", 25*time.Second)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.PresencePingInterval = pingInterval
	}

	pongTimeout, err := parsePositiveDuration(lookup, "PRESENCE_PONG_TIMEOUT", 60*time.Second)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.PresencePongTimeout = pongTimeout
	}

	dbHost, err := parseStringWithDefault(lookup, "DB_HOST", "localhost")
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.DBHost = dbHost
	}

	dbPortInt, err := parseIntRange(lookup, "DB_PORT", 5432, 1, 65535)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.DBPort = strconv.Itoa(dbPortInt)
	}

	dbUser, err := parseStringWithDefault(lookup, "DB_USER", "postgres")
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.DBUser = dbUser
	}

	dbPassword, err := parseRequiredSecret(lookup, "DB_PASSWORD", disallowedDBPasswords, 1)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.DBPassword = dbPassword
	}

	dbName, err := parseStringWithDefault(lookup, "DB_NAME", "nimbleVoice")
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.DBName = dbName
	}

	dbSSLMode, err := parseStringWithDefault(lookup, "DB_SSLMODE", "disable")
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.DBSSLMode = dbSSLMode
	}

	dbTimezone, err := parseStringWithDefault(lookup, "DB_TIMEZONE", "Asia/Dhaka")
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.DBTimezone = dbTimezone
	}

	dbMaxOpen, err := parseIntMin(lookup, "DB_MAX_OPEN_CONNS", 25, 1)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.DBMaxOpenConns = dbMaxOpen
	}

	dbMaxIdle, err := parseIntMin(lookup, "DB_MAX_IDLE_CONNS", 10, 0)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.DBMaxIdleConns = dbMaxIdle
	}
	if cfg.DBMaxOpenConns >= 1 && cfg.DBMaxIdleConns > cfg.DBMaxOpenConns {
		errs = append(errs, fmt.Errorf("DB_MAX_IDLE_CONNS (%d) must be <= DB_MAX_OPEN_CONNS (%d)", cfg.DBMaxIdleConns, cfg.DBMaxOpenConns))
	}

	minJWTLen := 1
	if isProd {
		minJWTLen = 32
	}
	jwtSecret, err := parseRequiredSecret(lookup, "JWT_SECRET", disallowedJWTSecrets, minJWTLen)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.JWTSecret = jwtSecret
	}

	uploadDir, err := parseStringWithDefault(lookup, "UPLOAD_DIR", "./uploads")
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.UploadDir = uploadDir
	}

	maxFileSizeMB, err := parseIntRange(lookup, "MAX_FILE_SIZE_MB", 5, 1, 100)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.MaxFileSizeMB = int64(maxFileSizeMB)
	}

	stunURLs, err := parseICEURLList(lookup, "WEBRTC_STUN_URLS", "stun:stun.l.google.com:19302,stun:stun.cloudflare.com:3478", true, "stun", "stuns")
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.WebRTCSTUNURLs = stunURLs
	}

	turnEnabled, err := parseBoolWithDefault(lookup, "TURN_ENABLED", false)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.TURNEnabled = turnEnabled
	}

	turnTTL, err := parseIntRange(lookup, "TURN_CREDENTIAL_TTL_SECONDS", 3600, 60, 86400)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.TURNCredentialTTLSeconds = turnTTL
	}

	turnURLs, err := parseICEURLList(lookup, "TURN_URLS", "", turnEnabled, "turn", "turns")
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.TURNURLs = turnURLs
	}

	if turnEnabled {
		turnSecret, err := parseRequiredSecret(lookup, "TURN_SHARED_SECRET", disallowedTURNSecrets, 1)
		if err != nil {
			errs = append(errs, err)
		} else {
			cfg.TURNSharedSecret = turnSecret
		}
	} else if rawSecret, ok := lookup("TURN_SHARED_SECRET"); ok {
		cfg.TURNSharedSecret = strings.TrimSpace(rawSecret)
	}

	redisEnabled, err := parseBoolWithDefault(lookup, "REDIS_ENABLED", false)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.RedisEnabled = redisEnabled
	}

	redisURL, err := parseRedisURL(lookup, redisEnabled)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.RedisURL = redisURL
	}

	maxParticipants, err := parseIntRange(lookup, "ROOM_DEFAULT_MAX_PARTICIPANTS", 6, 2, 20)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.RoomDefaultMaxParticipants = maxParticipants
	}

	idleGrace, err := parseIntMin(lookup, "ROOM_IDLE_GRACE_SECONDS", 30, 0)
	if err != nil {
		errs = append(errs, err)
	} else {
		cfg.RoomIdleGraceSeconds = idleGrace
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return cfg, nil
}

// NormalizeOrigin validates and canonicalizes an HTTP(S) origin without trailing slashes, paths, queries, or wildcards.
func NormalizeOrigin(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("origin cannot be empty")
	}
	if strings.Contains(trimmed, "*") {
		return "", fmt.Errorf("origin %q must not contain wildcard '*'", raw)
	}
	trimmed = strings.TrimRight(trimmed, "/")

	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid origin %q: %w", raw, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("origin %q must use http or https scheme", raw)
	}
	if u.User != nil {
		return "", fmt.Errorf("origin %q must not contain userinfo", raw)
	}
	if u.Host == "" || u.Hostname() == "" {
		return "", fmt.Errorf("origin %q must include a valid host", raw)
	}
	if u.Path != "" || u.RawPath != "" {
		return "", fmt.Errorf("origin %q must not include a path", raw)
	}
	if u.RawQuery != "" || strings.Contains(trimmed, "?") {
		return "", fmt.Errorf("origin %q must not include a query string", raw)
	}
	if u.Fragment != "" || strings.Contains(trimmed, "#") {
		return "", fmt.Errorf("origin %q must not include a fragment", raw)
	}

	return scheme + "://" + strings.ToLower(u.Host), nil
}

// IsOriginAllowed checks whether an incoming request Origin matches the configured allowlist.
func (c *Config) IsOriginAllowed(origin string) bool {
	if c == nil {
		return false
	}
	normalized, err := NormalizeOrigin(origin)
	if err != nil {
		return false
	}
	list := c.CORSAllowedOrigins
	if len(list) == 0 {
		list = c.FrontendOrigins
	}
	for _, allowed := range list {
		normAllowed, err := NormalizeOrigin(allowed)
		if err != nil {
			continue
		}
		if normAllowed == normalized {
			return true
		}
	}
	return false
}

func parseStringWithDefault(lookup func(string) (string, bool), key, fallback string) (string, error) {
	val, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return "", fmt.Errorf("%s cannot be empty", key)
	}
	return trimmed, nil
}

func parseRequiredSecret(lookup func(string) (string, bool), key string, disallowed map[string]struct{}, minLen int) (string, error) {
	val, ok := lookup(key)
	if !ok {
		return "", fmt.Errorf("%s is required", key)
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return "", fmt.Errorf("%s cannot be empty", key)
	}
	if _, banned := disallowed[strings.ToLower(trimmed)]; banned {
		return "", fmt.Errorf("%s uses a known insecure placeholder value", key)
	}
	if len(trimmed) < minLen {
		return "", fmt.Errorf("%s must be at least %d characters", key, minLen)
	}
	return trimmed, nil
}

func parseBoolWithDefault(lookup func(string) (string, bool), key string, fallback bool) (bool, error) {
	val, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return false, fmt.Errorf("%s cannot be empty when set (expected true or false)", key)
	}
	parsed, err := strconv.ParseBool(trimmed)
	if err != nil {
		return false, fmt.Errorf("%s has invalid bool value %q: %w", key, val, err)
	}
	return parsed, nil
}

func parseIntRange(lookup func(string) (string, bool), key string, fallback, minVal, maxVal int) (int, error) {
	val, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return 0, fmt.Errorf("%s cannot be empty when set", key)
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%s has invalid integer value %q: %w", key, val, err)
	}
	if parsed < minVal || parsed > maxVal {
		return 0, fmt.Errorf("%s (%d) must be between %d and %d", key, parsed, minVal, maxVal)
	}
	return parsed, nil
}

func parseIntMin(lookup func(string) (string, bool), key string, fallback, minVal int) (int, error) {
	val, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return 0, fmt.Errorf("%s cannot be empty when set", key)
	}
	parsed, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%s has invalid integer value %q: %w", key, val, err)
	}
	if parsed < minVal {
		return 0, fmt.Errorf("%s (%d) must be >= %d", key, parsed, minVal)
	}
	return parsed, nil
}

func parsePositiveDuration(lookup func(string) (string, bool), key string, fallback time.Duration) (time.Duration, error) {
	val, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return 0, fmt.Errorf("%s cannot be empty when set", key)
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%s has invalid duration value %q: %w", key, val, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s (%v) must be > 0", key, parsed)
	}
	return parsed, nil
}

func parseHTTPURL(lookup func(string) (string, bool), key, fallback string) (string, error) {
	val, ok := lookup(key)
	if !ok {
		return fallback, nil
	}
	trimmed := strings.TrimSpace(val)
	if trimmed == "" {
		return "", fmt.Errorf("%s cannot be empty when set", key)
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%s has invalid URL %q: %w", key, val, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "http" && scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%s (%q) must be a valid http or https URL with host", key, val)
	}
	return strings.TrimRight(trimmed, "/"), nil
}

func parseAllowedOrigins(lookup func(string) (string, bool), isProd bool) ([]string, error) {
	var raw string
	var sourceKey string
	for _, candidate := range []string{"CORS_ALLOWED_ORIGINS", "FRONTEND_ORIGINS", "FRONTEND_URL"} {
		if val, ok := lookup(candidate); ok {
			raw = val
			sourceKey = candidate
			break
		}
	}

	if sourceKey == "" {
		if isProd {
			return nil, errors.New("CORS_ALLOWED_ORIGINS is required in production")
		}
		return []string{"http://localhost:3000"}, nil
	}

	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%s cannot be empty", sourceKey)
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		norm, err := NormalizeOrigin(part)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", sourceKey, err)
		}
		if _, exists := seen[norm]; !exists {
			seen[norm] = struct{}{}
			out = append(out, norm)
		}
	}
	return out, nil
}

func parseTrustedProxies(lookup func(string) (string, bool)) ([]string, error) {
	val, ok := lookup("TRUSTED_PROXIES")
	if !ok || strings.TrimSpace(val) == "" {
		return nil, nil
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, errors.New("TRUSTED_PROXIES contains an empty entry")
		}
		if net.ParseIP(trimmed) == nil {
			if _, _, err := net.ParseCIDR(trimmed); err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES has invalid IP or CIDR %q", trimmed)
			}
		}
		out = append(out, trimmed)
	}
	return out, nil
}

func parseICEURLList(lookup func(string) (string, bool), key, fallback string, required bool, schemes ...string) ([]string, error) {
	val, ok := lookup(key)
	raw := fallback
	if ok {
		raw = val
	}

	trimmedRaw := strings.TrimSpace(raw)
	if trimmedRaw == "" {
		if required {
			return nil, fmt.Errorf("%s is required", key)
		}
		return nil, nil
	}

	schemeSet := make(map[string]struct{}, len(schemes))
	for _, s := range schemes {
		schemeSet[strings.ToLower(s)] = struct{}{}
	}

	parts := strings.Split(trimmedRaw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			return nil, fmt.Errorf("%s contains an empty URL entry", key)
		}
		if strings.ContainsAny(item, " \t\n\r") {
			return nil, fmt.Errorf("%s entry %q contains whitespace", key, item)
		}
		u, err := url.Parse(item)
		if err != nil {
			return nil, fmt.Errorf("%s has invalid URL %q: %w", key, item, err)
		}
		if _, allowed := schemeSet[strings.ToLower(u.Scheme)]; !allowed {
			return nil, fmt.Errorf("%s entry %q must use scheme %v", key, item, schemes)
		}
		target := u.Host
		if target == "" {
			target = u.Opaque
		}
		if strings.TrimSpace(target) == "" {
			return nil, fmt.Errorf("%s entry %q must include a host", key, item)
		}
		out = append(out, item)
	}
	return out, nil
}

func parseRedisURL(lookup func(string) (string, bool), required bool) (string, error) {
	val, ok := lookup("REDIS_URL")
	trimmed := strings.TrimSpace(val)
	if !ok || trimmed == "" {
		if required {
			return "", errors.New("REDIS_URL is required when REDIS_ENABLED=true")
		}
		return "", nil
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("REDIS_URL has invalid URL %q: %w", trimmed, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme != "redis" && scheme != "rediss") || u.Host == "" {
		return "", fmt.Errorf("REDIS_URL (%q) must use redis:// or rediss:// with a host", trimmed)
	}
	return trimmed, nil
}
