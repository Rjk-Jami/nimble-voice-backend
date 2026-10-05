package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port          string
	Environment   string
	AppURL        string
	DBHost        string
	DBPort        string
	DBUser        string
	DBPassword    string
	DBName        string
	DBSSLMode     string
	DBTimezone    string
	SQLServerHost string
	SQLServerPort string
	SQLServerUser string
	SQLServerPass string
	SQLServerDB   string
	JWTSecret     string
	UploadDir     string
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	return &Config{
		Port:          getEnv("PORT", "8080"),
		Environment:   getEnv("APP_ENV", "development"),
		AppURL:        getEnv("APP_URL", "http://localhost:8080"),
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnv("DB_PORT", "5432"),
		DBUser:        getEnv("DB_USER", "postgres"),
		DBPassword:    getEnv("DB_PASSWORD", "rjk.207235"),
		DBName:        getEnv("DB_NAME", "nimbleVoice"),
		DBSSLMode:     getEnv("DB_SSLMODE", "disable"),
		DBTimezone:    getEnv("DB_TIMEZONE", "Asia/Dhaka"),
		SQLServerHost: getEnv("SQLSERVER_HOST", "localhost"),
		SQLServerPort: getEnv("SQLSERVER_PORT", "1433"),
		SQLServerUser: getEnv("SQLSERVER_USER", "sa"),
		SQLServerPass: getEnv("SQLSERVER_PASSWORD", ""),
		SQLServerDB:   getEnv("SQLSERVER_DB", "erp_pos"),
		JWTSecret:     getEnv("JWT_SECRET", "default_jwt_secret"),
		UploadDir:     getEnv("UPLOAD_DIR", "./uploads"),
	}, nil
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}
