package config

import (
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                 string
	Environment          string
	AppURL               string
	FrontendOrigins      []string
	PresencePingInterval time.Duration
	PresencePongTimeout  time.Duration
	DBHost               string
	DBPort               string
	DBUser               string
	DBPassword           string
	DBName               string
	DBSSLMode            string
	DBTimezone           string
	SQLServerHost        string
	SQLServerPort        string
	SQLServerUser        string
	SQLServerPass        string
	SQLServerDB          string
	JWTSecret            string
	UploadDir            string
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	frontendOrigins := strings.Split(getEnv("FRONTEND_ORIGINS", "http://localhost:3000"), ",")
	for i := range frontendOrigins {
		frontendOrigins[i] = strings.TrimSpace(frontendOrigins[i])
	}

	return &Config{
		Port:                 getEnv("PORT", "8080"),
		Environment:          getEnv("APP_ENV", "development"),
		AppURL:               getEnv("APP_URL", "http://localhost:8080"),
		FrontendOrigins:      frontendOrigins,
		PresencePingInterval: getDuration("PRESENCE_PING_INTERVAL", "25s"),
		PresencePongTimeout:  getDuration("PRESENCE_PONG_TIMEOUT", "60s"),
		DBHost:               getEnv("DB_HOST", "localhost"),
		DBPort:               getEnv("DB_PORT", "5432"),
		DBUser:               getEnv("DB_USER", "postgres"),
		DBPassword:           getEnv("DB_PASSWORD", "rjk.207235"),
		DBName:               getEnv("DB_NAME", "nimbleVoice"),
		DBSSLMode:            getEnv("DB_SSLMODE", "disable"),
		DBTimezone:           getEnv("DB_TIMEZONE", "Asia/Dhaka"),
		SQLServerHost:        getEnv("SQLSERVER_HOST", "localhost"),
		SQLServerPort:        getEnv("SQLSERVER_PORT", "1433"),
		SQLServerUser:        getEnv("SQLSERVER_USER", "sa"),
		SQLServerPass:        getEnv("SQLSERVER_PASSWORD", ""),
		SQLServerDB:          getEnv("SQLSERVER_DB", "erp_pos"),
		JWTSecret:            getEnv("JWT_SECRET", "default_jwt_secret"),
		UploadDir:            getEnv("UPLOAD_DIR", "./uploads"),
	}, nil
}

func getDuration(key, fallback string) time.Duration {
	if value, err := time.ParseDuration(getEnv(key, fallback)); err == nil && value > 0 {
		return value
	}
	value, _ := time.ParseDuration(fallback)
	return value
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
