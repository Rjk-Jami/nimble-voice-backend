# Go Modular Enterprise Backend - Complete Starter Template & Architecture

This document provides a **complete, production-ready starter template (boilerplate)** incorporating all the architectural patterns, libraries, and best practices used in the **Pakiza POS Backend** project.

It features the entire technology stack (PostgreSQL, MS SQL Server, Uber Zap Logger, Tollbooth Rate Limiting, Cron Jobs, Socket.IO, Swagger, JWT Authentication) along with a **complete image/file upload system for user avatars with frontend integration examples**.

---

## 1. Technology Stack Overview

| Technology / Library | Package | Purpose in Project |
|---|---|---|
| **Gin Gonic** | `github.com/gin-gonic/gin` | High-performance HTTP web framework and routing engine |
| **GORM** | `gorm.io/gorm` | Most popular Golang ORM for database querying and relations |
| **PostgreSQL Driver** | `gorm.io/driver/postgres` | Primary relational database for transactions, products, users |
| **MS SQL Server Driver**| `gorm.io/driver/sqlserver`| Central ERP integration and data synchronization |
| **Uber Zap Logger** | `go.uber.org/zap` | Blazing-fast, enterprise-grade structured JSON logging |
| **Lumberjack** | `gopkg.in/natefinch/lumberjack.v2`| Automatic log file rotation (size, backups, retention days) |
| **JWT (v5)** | `github.com/golang-jwt/jwt/v5` | Secure stateless token-based authentication |
| **Bcrypt** | `golang.org/x/crypto/bcrypt` | Secure one-way password hashing |
| **Tollbooth** | `github.com/didip/tollbooth/v7` | IP-based request rate limiting (DDoS & brute-force protection) |
| **Robfig Cron** | `github.com/robfig/cron/v3` | Scheduled background cron jobs |
| **Socket.IO** | `github.com/googollee/go-socket.io`| Real-time bi-directional client-server communication |
| **Atlas Migration** | `ariga.io/atlas-provider-gorm` | Declarative schema migrations and version control |
| **CUID / UUID** | `github.com/lucsky/cuid`, `uuid` | Collision-resistant unique primary key generation |
| **Validator v10** | `github.com/go-playground/validator/v10` | Request DTO & form validation |
| **CORS** | `github.com/gin-contrib/cors` | Cross-Origin Resource Sharing middleware |
| **Godotenv** | `github.com/joho/godotenv` | Environment variable loader from `.env` file |
| **Swagger / OpenAPI**| `github.com/swaggo/gin-swagger` | Interactive API documentation in browser |

---

## 2. Project Directory Structure

```
my-enterprise-backend/
├── cmd/
│   └── server/
│       └── main.go               # Application entry point (Logger, DB, Cron, Server)
├── config/
│   └── config.go                 # Environment configuration loader
├── db/
│   ├── db.go                     # PostgreSQL connection & retry logic
│   └── sqlserver.go              # MS SQL Server (ERP) connection
├── domain/
│   ├── base_model.go             # CUID & timestamp base model
│   └── user.go                   # User entity (with Avatar URL & bcrypt hook)
├── middleware/
│   ├── auth.go                   # JWT validation & role-based access control
│   ├── rate_limiter.go           # Tollbooth rate limiter middleware
│   └── logger.go                 # Zap request logging middleware
├── module/
│   ├── router.go                 # Global router, CORS, and static file server
│   └── v1/
│       ├── routes.go             # API v1 sub-module registration
│       ├── socket/
│       │   └── socket.go         # Socket.IO initialization
│       └── user/
│           ├── user.dto.go       # Request & response DTOs
│           ├── user.service.go   # Business logic (registration, login, avatar)
│           ├── user.controller.go# HTTP endpoints (including image upload)
│           └── user.module.go    # Dependency Injection (DI) wiring
├── pkg/
│   ├── cron/
│   │   └── cron.go               # Background scheduler
│   └── logger/
│       └── logger.go             # Uber Zap + Lumberjack setup
├── utils/
│   ├── fileutil/
│   │   └── uploader.go           # Image validation, unique naming, and storage
│   ├── httpx/
│   │   ├── response.go           # Standardized API response builder
│   │   └── validator.go          # Request body & query validator
│   └── jwt/
│       └── jwt.go                # Token generation & verification
├── uploads/                      # Uploaded files stored here
│   └── avatars/
├── .env
├── .env.example
├── go.mod
└── README.md
```

---


### 2. `config/config.go`
```go
package config

import (
	"os"
	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	Environment    string
	AppURL         string
	DBHost         string
	DBPort         string
	DBUser         string
	DBPassword     string
	DBName         string
	DBSSLMode      string
	DBTimezone     string
	SQLServerHost  string
	SQLServerPort  string
	SQLServerUser  string
	SQLServerPass  string
	SQLServerDB    string
	JWTSecret      string
	UploadDir      string
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
		DBPassword:    getEnv("DB_PASSWORD", "secret"),
		DBName:        getEnv("DB_NAME", "enterprise_db"),
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
```

---

### 3. `pkg/logger/logger.go` (Uber Zap with Lumberjack File Rotation)
```go
package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *zap.Logger

func InitLogger(env string) (*zap.Logger, error) {
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   "./logs/app.log",
		MaxSize:    20, // Megabytes
		MaxBackups: 5,
		MaxAge:     30, // Days
		Compress:   true,
	})

	consoleWriter := zapcore.AddSync(os.Stdout)

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	core := zapcore.NewTee(
		zapcore.NewCore(zapcore.NewConsoleEncoder(encoderConfig), consoleWriter, zap.DebugLevel),
		zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), fileWriter, zap.InfoLevel),
	)

	Log = zap.New(core, zap.AddCaller())
	zap.ReplaceGlobals(Log)
	return Log, nil
}
```

---

### 4. `db/db.go` (PostgreSQL Connection & Automatic Retry)
```go
package db

import (
	"fmt"
	"log"
	"my-enterprise-backend/config"
	"my-enterprise-backend/domain"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func ConnectPostgres(cfg *config.Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=%s TimeZone=%s",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort, cfg.DBSSLMode, cfg.DBTimezone,
	)

	var db *gorm.DB
	var err error

	for i := 1; i <= 5; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			log.Println("PostgreSQL database connection established successfully")
			break
		}
		log.Printf("Attempt %d: Retrying PostgreSQL connection in 2 seconds... error: %v", i, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		return nil, err
	}

	// Auto migration
	_ = db.AutoMigrate(&domain.User{})
	return db, nil
}
```

---

### 5. `domain/user.go` (CUID Primary Key, Avatar URL, and Password Hashing)
```go
package domain

import (
	"time"

	"github.com/lucsky/cuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	ID        string    `gorm:"column:id;primaryKey" json:"id"`
	Name      string    `gorm:"column:name;not null" json:"name"`
	Email     string    `gorm:"column:email;uniqueIndex;not null" json:"email"`
	Password  string    `gorm:"column:password;not null" json:"-"`
	AvatarURL string    `gorm:"column:avatar_url" json:"avatar_url"`
	Role      string    `gorm:"column:role;default:user" json:"role"`
	StoreID   string    `gorm:"column:store_id" json:"store_id"`
	BranchID  string    `gorm:"column:branch_id" json:"branch_id"`
	Status    string    `gorm:"column:status;default:active" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}

// BeforeCreate hook generates CUID and hashes password if not already hashed
func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == "" {
		u.ID = cuid.New()
	}
	if u.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Password = string(hash)
	}
	return nil
}

// ComparePassword verifies raw password against the stored bcrypt hash
func (u *User) ComparePassword(raw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(raw)) == nil
}
```

---

### 6. `utils/fileutil/uploader.go` (Image Validation & Storage Utility)
```go
package fileutil

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var AllowedImageExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".webp": true,
	".gif":  true,
}

const MaxFileSize = 5 * 1024 * 1024 // 5 MB

// SaveImage validates the uploaded file, generates a safe unique filename, saves it, and returns the accessible public path
func SaveImage(file *multipart.FileHeader, subFolder string) (string, error) {
	// 1. File size validation
	if file.Size > MaxFileSize {
		return "", errors.New("file size exceeds the 5MB limit")
	}

	// 2. Extension validation
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !AllowedImageExtensions[ext] {
		return "", fmt.Errorf("invalid file format: %s. Only JPG, PNG, WEBP, and GIF are allowed", ext)
	}

	// 3. Ensure target directory exists
	targetDir := filepath.Join("./uploads", subFolder)
	if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("failed to create upload directory: %v", err)
	}

	// 4. Generate unique filename (timestamp + uuid + original extension)
	uniqueFileName := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), uuid.New().String()[:8], ext)
	destinationPath := filepath.Join(targetDir, uniqueFileName)

	// 5. Open and copy file to destination
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.Create(destinationPath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}

	// Relative public URL path for clients (e.g., /uploads/avatars/172810000_a1b2c3d4.png)
	publicURLPath := fmt.Sprintf("/uploads/%s/%s", subFolder, uniqueFileName)
	return publicURLPath, nil
}
```

---

### 7. `utils/httpx/response.go`
```go
package httpx

import (
	"time"
	"github.com/gin-gonic/gin"
)

type APIResponse struct {
	StatusCode int         `json:"status_code"`
	Message    string      `json:"message"`
	Data       interface{} `json:"data,omitempty"`
}

func SendData(statusCode int, message string, data ...interface{}) APIResponse {
	var d interface{}
	if len(data) > 0 {
		d = data[0]
	}
	return APIResponse{StatusCode: statusCode, Message: message, Data: d}
}

func SendResponse(ctx *gin.Context, res APIResponse) {
	ctx.JSON(res.StatusCode, gin.H{
		"status":    res.StatusCode,
		"message":   res.Message,
		"data":      res.Data,
		"timestamp": time.Now().UTC(),
	})
}
```

---

### 8. `utils/jwt/jwt.go`
```go
package jwt

import (
	"errors"
	"time"
	"github.com/golang-jwt/jwt/v5"
)

var SecretKey = []byte("enterprise_super_secret_jwt_key_2026")

type Claims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	StoreID  string `json:"store_id"`
	BranchID string `json:"branch_id"`
	jwt.RegisteredClaims
}

func GenerateToken(userID, email, role, storeID, branchID string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Email:    email,
		Role:     role,
		StoreID:  storeID,
		BranchID: branchID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * 7 * time.Hour)), // 7 days expiration
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(SecretKey)
}

func ParseToken(tokenStr string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return SecretKey, nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := t.Claims.(*Claims); ok && t.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid or expired token")
}
```

---

### 9. `middleware/auth.go`
```go
package middleware

import (
	"my-enterprise-backend/utils/jwt"
	"strings"
	"github.com/gin-gonic/gin"
)

func Auth(allowedRoles ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			ctx.AbortWithStatusJSON(401, gin.H{"status": 401, "message": "Authorization header is required"})
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			ctx.AbortWithStatusJSON(401, gin.H{"status": 401, "message": "Invalid token header format, expected: Bearer <token>"})
			return
		}

		claims, err := jwt.ParseToken(parts[1])
		if err != nil {
			ctx.AbortWithStatusJSON(401, gin.H{"status": 401, "message": "Unauthorized: " + err.Error()})
			return
		}

		// Role-based authorization check
		if len(allowedRoles) > 0 {
			permitted := false
			for _, r := range allowedRoles {
				if strings.EqualFold(r, claims.Role) {
					permitted = true
					break
				}
			}
			if !permitted {
				ctx.AbortWithStatusJSON(403, gin.H{"status": 403, "message": "Forbidden: insufficient permissions"})
				return
			}
		}

		// Store user claims in context
		ctx.Set("user_id", claims.UserID)
		ctx.Set("email", claims.Email)
		ctx.Set("role", claims.Role)
		ctx.Set("store_id", claims.StoreID)
		ctx.Set("branch_id", claims.BranchID)

		ctx.Next()
	}
}
```

---

### 10. `middleware/rate_limiter.go` (Tollbooth)
```go
package middleware

import (
	"time"
	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth/v7/limiter"
	"github.com/gin-gonic/gin"
)

func RateLimiter() gin.HandlerFunc {
	// Maximum 20 requests per second per IP
	lmt := tollbooth.NewLimiter(20, &limiter.ExpirableOptions{
		DefaultExpirationTTL: time.Minute,
	})

	return func(c *gin.Context) {
		httpError := tollbooth.LimitByRequest(lmt, c.Writer, c.Request)
		if httpError != nil {
			c.AbortWithStatusJSON(httpError.StatusCode, gin.H{
				"status":  httpError.StatusCode,
				"message": "Too many requests. Please slow down.",
			})
			return
		}
		c.Next()
	}
}
```

---

### 11. `module/v1/user/user.dto.go`
```go
package user

type RegisterUserDto struct {
	Name     string `json:"name" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
	Role     string `json:"role"`
	StoreID  string `json:"store_id"`
	BranchID string `json:"branch_id"`
}

type LoginUserDto struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type LoginResponseDto struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}
```

---

### 12. `module/v1/user/user.service.go`
```go
package user

import (
	"my-enterprise-backend/domain"
	"my-enterprise-backend/utils/httpx"
	"my-enterprise-backend/utils/jwt"
	"gorm.io/gorm"
)

type UserServiceInterface interface {
	Register(dto *RegisterUserDto) httpx.APIResponse
	Login(dto *LoginUserDto) httpx.APIResponse
	GetProfile(userID string) httpx.APIResponse
	UpdateAvatar(userID, avatarURL string) httpx.APIResponse
}

type UserService struct {
	db *gorm.DB
}

func NewUserService(db *gorm.DB) UserServiceInterface {
	return &UserService{db: db}
}

func (s *UserService) Register(dto *RegisterUserDto) httpx.APIResponse {
	var count int64
	s.db.Model(&domain.User{}).Where("email = ?", dto.Email).Count(&count)
	if count > 0 {
		return httpx.SendData(400, "User already exists with this email")
	}

	role := dto.Role
	if role == "" {
		role = "user"
	}

	user := domain.User{
		Name:     dto.Name,
		Email:    dto.Email,
		Password: dto.Password,
		Role:     role,
		StoreID:  dto.StoreID,
		BranchID: dto.BranchID,
	}

	if err := s.db.Create(&user).Error; err != nil {
		return httpx.SendData(500, "Failed to create user", err.Error())
	}
	return httpx.SendData(201, "Registration successful", user)
}

func (s *UserService) Login(dto *LoginUserDto) httpx.APIResponse {
	var user domain.User
	if err := s.db.Where("email = ?", dto.Email).First(&user).Error; err != nil {
		return httpx.SendData(401, "Invalid email or password")
	}

	if !user.ComparePassword(dto.Password) {
		return httpx.SendData(401, "Invalid email or password")
	}

	token, err := jwt.GenerateToken(user.ID, user.Email, user.Role, user.StoreID, user.BranchID)
	if err != nil {
		return httpx.SendData(500, "Failed to generate token")
	}

	return httpx.SendData(200, "Login successful", LoginResponseDto{Token: token, User: user})
}

func (s *UserService) GetProfile(userID string) httpx.APIResponse {
	var user domain.User
	if err := s.db.Where("id = ?", userID).First(&user).Error; err != nil {
		return httpx.SendData(404, "User not found")
	}
	return httpx.SendData(200, "Profile fetched", user)
}

func (s *UserService) UpdateAvatar(userID, avatarURL string) httpx.APIResponse {
	if err := s.db.Model(&domain.User{}).Where("id = ?", userID).Update("avatar_url", avatarURL).Error; err != nil {
		return httpx.SendData(500, "Failed to update user avatar", err.Error())
	}

	return httpx.SendData(200, "Avatar updated successfully", gin.H{
		"avatar_url": avatarURL,
	})
}
```

---

### 13. `module/v1/user/user.controller.go` (With Image Upload API)
```go
package user

import (
	"my-enterprise-backend/middleware"
	"my-enterprise-backend/utils/fileutil"
	"my-enterprise-backend/utils/httpx"

	"github.com/gin-gonic/gin"
)

type UserController struct {
	service UserServiceInterface
}

func NewUserController(service UserServiceInterface) UserController {
	return UserController{service: service}
}

func UserRouter(router *gin.RouterGroup, controller UserController) {
	// 1. User Registration
	router.POST("/register", func(ctx *gin.Context) {
		var data RegisterUserDto
		if err := ctx.ShouldBindJSON(&data); err != nil {
			ctx.JSON(400, gin.H{"status": 400, "error": err.Error()})
			return
		}
		httpx.SendResponse(ctx, controller.service.Register(&data))
	})

	// 2. User Login
	router.POST("/login", func(ctx *gin.Context) {
		var data LoginUserDto
		if err := ctx.ShouldBindJSON(&data); err != nil {
			ctx.JSON(400, gin.H{"status": 400, "error": err.Error()})
			return
		}
		httpx.SendResponse(ctx, controller.service.Login(&data))
	})

	// 3. User Profile (JWT Protected)
	router.GET("/profile", middleware.Auth(), func(ctx *gin.Context) {
		userID, _ := ctx.Get("user_id")
		httpx.SendResponse(ctx, controller.service.GetProfile(userID.(string)))
	})

	// 4. Avatar Image Upload (multipart/form-data)
	router.POST("/upload-avatar", middleware.Auth(), func(ctx *gin.Context) {
		// Read file from the 'avatar' multipart form key
		file, err := ctx.FormFile("avatar")
		if err != nil {
			ctx.JSON(400, gin.H{
				"status":  400,
				"message": "Avatar image file is required in 'avatar' field",
				"error":   err.Error(),
			})
			return
		}

		// Validate & save image into ./uploads/avatars directory
		imageURL, err := fileutil.SaveImage(file, "avatars")
		if err != nil {
			ctx.JSON(400, gin.H{
				"status":  400,
				"message": "Image upload failed",
				"error":   err.Error(),
			})
			return
		}

		// Update avatar URL in the database
		userID, _ := ctx.Get("user_id")
		res := controller.service.UpdateAvatar(userID.(string), imageURL)
		httpx.SendResponse(ctx, res)
	})
}
```

---

### 14. `module/v1/user/user.module.go`
```go
package user

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func UserModule(router *gin.RouterGroup, db *gorm.DB) {
	service := NewUserService(db)
	controller := NewUserController(service)
	UserRouter(router, controller)
}
```

---

### 15. `module/v1/routes.go`
```go
package route_v1

import (
	"my-enterprise-backend/module/v1/user"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterV1Routes(router *gin.RouterGroup, db *gorm.DB) {
	user.UserModule(router.Group("/user"), db)
}
```

---

### 16. `module/router.go` (With `/uploads` Static File Handler)
```go
package module

import (
	"my-enterprise-backend/config"
	"my-enterprise-backend/middleware"
	route_v1 "my-enterprise-backend/module/v1"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(cfg *config.Config, db *gorm.DB) *gin.Engine {
	router := gin.New()

	// Global Middlewares
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(middleware.RateLimiter())

	// CORS Policy
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// CRITICAL: Serve uploaded media statically over HTTP
	// Example accessible URL: http://localhost:8080/uploads/avatars/xxxx.png
	router.Static("/uploads", "./uploads")

	// Health Check
	router.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"status": "healthy", "time": time.Now().UTC()})
	})

	// API v1 Routing
	v1 := router.Group("/api/v1")
	{
		route_v1.RegisterV1Routes(v1, db)
	}

	return router
}
```

---

### 17. `cmd/server/main.go`
```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"my-enterprise-backend/config"
	"my-enterprise-backend/db"
	"my-enterprise-backend/module"
	"my-enterprise-backend/pkg/logger"
	"my-enterprise-backend/utils/jwt"
)

func main() {
	// 1. Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Config load error: %v", err)
	}

	// 2. Initialize Zap Logger
	appLogger, err := logger.InitLogger(cfg.Environment)
	if err != nil {
		log.Fatalf("Logger init error: %v", err)
	}
	defer appLogger.Sync()

	// 3. Initialize JWT Secret
	jwt.SecretKey = []byte(cfg.JWTSecret)

	// 4. Connect to database
	database, err := db.ConnectPostgres(cfg)
	if err != nil {
		log.Fatalf("Postgres connection error: %v", err)
	}

	// 5. Setup Router
	router := module.SetupRouter(cfg, database)

	// 6. Initialize HTTP Server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		fmt.Printf("🚀 Server running on port :%s (Mode: %s)\n", cfg.Port, cfg.Environment)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server Listen error: %v", err)
		}
	}()

	// 7. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server gracefully stopped")
}
```

---
