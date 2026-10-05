# Nimble Voice Backend

Enterprise Modular Go Backend built with Gin, GORM, PostgreSQL, Zap Logger, Tollbooth Rate Limiter, JWT Authentication, and Avatar/File Upload capabilities.

## Architecture & Directory Structure

```
nimble-voice-backend/
├── cmd/
│   └── server/
│       └── main.go               # Application entry point
├── config/
│   └── config.go                 # Environment configuration loader
├── db/
│   ├── db.go                     # PostgreSQL connection & auto-migration
│   └── sqlserver.go              # Optional MS SQL Server connection
├── domain/
│   ├── base_model.go             # CUID & timestamp base model
│   └── user.go                   # User entity with bcrypt & CUID hook
├── middleware/
│   ├── auth.go                   # JWT validation & role-based access control
│   ├── logger.go                 # Zap request logging middleware
│   └── rate_limiter.go           # Tollbooth rate limiter
├── module/
│   ├── router.go                 # Global router, CORS, static uploads
│   └── v1/
│       ├── routes.go             # API v1 sub-module registration
│       ├── socket/
│       │   └── socket.go         # Socket.IO initialization
│       └── user/
│           ├── user.dto.go       # User request & response DTOs
│           ├── user.service.go   # User business logic
│           ├── user.controller.go# User HTTP endpoints
│           └── user.module.go    # Dependency injection wiring
├── pkg/
│   ├── cron/
│   │   └── cron.go               # Background scheduler
│   └── logger/
│       └── logger.go             # Uber Zap + Lumberjack setup
├── utils/
│   ├── fileutil/
│   │   └── uploader.go           # Image validation & storage utility
│   ├── httpx/
│   │   ├── response.go           # Standardized API response builder
│   │   └── validator.go          # Request validation helper
│   └── jwt/
│       └── jwt.go                # Token generation & verification
├── uploads/                      # Uploaded files stored here
│   └── avatars/
├── .env
├── .env.example
├── go.mod
└── README.md
```

## Getting Started

### 1. Configure Environment
Update `.env` with your database credentials:
```env
PORT=8080
APP_ENV=development
APP_URL=http://localhost:8080

# PostgreSQL Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=enterprise_db
DB_SSLMODE=disable
DB_TIMEZONE=Asia/Dhaka

JWT_SECRET=enterprise_super_secret_jwt_key_2026
UPLOAD_DIR=./uploads
```

### 2. Run Database Migrations & Start Server
```bash
go run cmd/server/main.go
```

### 3. API Endpoints
- `GET /health` - Health check
- `POST /api/v1/user/register` - Register a new user
- `POST /api/v1/user/login` - Authenticate user & get JWT token
- `GET /api/v1/user/profile` - Get user profile (Bearer token required)
- `POST /api/v1/user/upload-avatar` - Upload user avatar (multipart/form-data with key `avatar`)
- `GET /uploads/avatars/:filename` - Static image file access
