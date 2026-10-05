# AGENTS.md - Developer & AI Agent Guide for Nimble Voice Backend

Welcome, AI Agent or Engineer! This document defines the engineering standards, architecture, real-time audio conventions, and implementation recipes for **Nimble Voice Backend** (`nimble-voice-backend`).

Whenever you are tasked with creating a new module, modifying an existing one, writing database models, implementing real-time socket events, executing concurrent room transactions, or building APIs, **you MUST follow the conventions described in this guide**.

---

## 1. Project Overview & Architecture

- **Project Module:** `nimble-voice-backend`
- **Language & Runtime:** Go (1.26+)
- **HTTP Web Framework:** [Gin Web Framework](https://github.com/gin-gonic/gin) (`github.com/gin-gonic/gin`)
- **Primary Database ORM:** [GORM](https://gorm.io/) (`gorm.io/gorm` with `gorm.io/driver/postgres`)
- **Primary Key Strategy:** **CUID** (`github.com/lucsky/cuid`) strings across all domain models
- **Real-Time Communication:** [go-socket.io](https://github.com/googollee/go-socket.io) (`github.com/googollee/go-socket.io`) for audio room events, signaling, and participant presence
- **Rate Limiting:** [Tollbooth](https://github.com/didip/tollbooth/v7) (`github.com/didip/tollbooth/v7`)
- **Authentication & Security:** JWT (`github.com/golang-jwt/jwt/v5`) + Bcrypt (`golang.org/x/crypto/bcrypt`)
- **Structured Logging:** Uber Zap (`go.uber.org/zap`) with Lumberjack log rotation (`gopkg.in/natefinch/lumberjack.v2`)
- **Media & Avatar Uploads:** Multipart file validation and static file hosting via `uploads/` directory

### Architecture Pattern: Modular Layered Architecture

Every feature resides in its own isolated module under `module/v1/<feature_name>/` and follows strict Separation of Concerns:

```
HTTP Request / WebSocket Event
     │
     ▼
[Gin Engine & Middlewares] (Logger, Recovery, RateLimiter, CORS, Auth)
     │
     ▼
[Controller] (module/v1/<name>/<name>.controller.go)
  ├── 1. Request DTO binding & validation (ctx.ShouldBindJSON / httpx.BindAndValidate)
  ├── 2. Extract Auth claims from gin.Context (user_id, role, email)
  └── 3. Call Service method
     │
     ▼
[Service] (module/v1/<name>/<name>.service.go)
  ├── 1. Business rules & validation (capacity checks, user state, host permissions)
  ├── 2. ACID Transactions & GORM operations (s.db with row locks)
  ├── 3. Socket event emission & room state broadcasting
  └── 4. Return httpx.APIResponse using httpx.SendData(code, message, data)
     │
     ▼
[Database / Entities] (domain/<name>.go)
  └── PostgreSQL (Users, Rooms, Participants, AudioSessions, Messages)
     │
     ▼
HTTP Response
[httpx.SendResponse(ctx, response)] -> Standardized JSON envelope
```

---

## 2. Directory Layout & Module Structure

```
nimble-voice-backend/
├── cmd/
│   └── server/
│       └── main.go               # Application entry point (Logger, DB, HTTP Server, Graceful Shutdown)
├── config/
│   └── config.go                 # Environment configuration loader (.env)
├── db/
│   ├── db.go                     # PostgreSQL connection pool & auto-migrations
│   └── sqlserver.go              # Optional secondary SQL Server connection
├── domain/
│   ├── base_model.go             # CUID & timestamp base model
│   ├── user.go                   # User entity with bcrypt & CUID hook
│   ├── room.go                   # Voice room entity (title, status, max_participants, host_id)
│   └── participant.go            # Room participant entity (role, mute_status, joined_at)
├── middleware/
│   ├── auth.go                   # JWT validation & role-based access control
│   ├── logger.go                 # Zap request logging middleware
│   └── rate_limiter.go           # Tollbooth rate limiter middleware
├── module/
│   ├── router.go                 # Global router, CORS, static uploads server
│   └── v1/
│       ├── routes.go             # API v1 sub-module registration
│       ├── socket/
│       │   └── socket.go         # Socket.IO initialization & room event handlers
│       ├── user/                 # User module
│       │   ├── user.dto.go       # DTOs
│       │   ├── user.service.go   # Business logic (register, login, avatar)
│       │   ├── user.controller.go# HTTP endpoints
│       │   └── user.module.go    # Dependency injection wiring
│       └── room/                 # Voice Room module
│           ├── room.dto.go
│           ├── room.service.go
│           ├── room.controller.go
│           └── room.module.go
├── pkg/
│   ├── cron/
│   │   └── cron.go               # Background scheduled tasks
│   └── logger/
│       └── logger.go             # Zap logger initialization
├── utils/
│   ├── fileutil/
│   │   └── uploader.go           # File & avatar validation, unique naming, and storage
│   ├── httpx/
│   │   ├── response.go           # Standardized API response envelope (APIResponse, SendData, SendResponse)
│   │   └── validator.go          # Request validation helpers
│   └── jwt/
│       └── jwt.go                # Token generation & verification
├── uploads/                      # Uploaded files stored here
│   └── avatars/                  # Avatar image storage
├── .env
├── .env.example
├── go.mod
├── go.sum
└── README.md
```

When implementing a new feature `<name>` (e.g., `room`, `participant`, `category`), the module directory structure MUST be:

```
module/v1/<name>/
├── <name>.dto.go         # Request and Response DTO structs with validation tags
├── <name>.service.go     # Service struct, interface, and business logic
├── <name>.controller.go  # Controller struct, constructor, and Gin route bindings
└── <name>.module.go      # Dependency injection wiring (DB -> Service -> Controller -> Router)
```

---

## 3. Defining Data Tables (Domain Entities)

Database entities live in the `domain/` package. All tables in this codebase follow strict standards:

### Standard Entity Template

```go
package domain

import (
	"time"

	"github.com/lucsky/cuid"
	"gorm.io/gorm"
)

type Room struct {
	ID              string     `gorm:"column:id;primaryKey" json:"id"`
	Title           string     `gorm:"column:title;not null;type:varchar(255)" json:"title"`
	Description     *string    `gorm:"column:description;type:text" json:"description"`
	Topic           string     `gorm:"column:topic;default:general;type:varchar(100)" json:"topic"`
	HostID          string     `gorm:"column:host_id;not null;index" json:"host_id"`
	MaxParticipants int        `gorm:"column:max_participants;default:50" json:"max_participants"`
	IsPrivate       bool       `gorm:"column:is_private;default:false" json:"is_private"`
	RoomKey         *string    `gorm:"column:room_key;type:varchar(100)" json:"-"`
	Status          string     `gorm:"column:status;default:active;type:varchar(20)" json:"status"` // active, ended
	CoverImageURL   *string    `gorm:"column:cover_image_url" json:"cover_image_url"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	// Relationships
	Host         User              `gorm:"foreignKey:HostID;onUpdate:CASCADE;onDelete:RESTRICT" json:"host,omitempty"`
	Participants []RoomParticipant `gorm:"foreignKey:RoomID;constraint:OnDelete:CASCADE" json:"participants,omitempty"`
}

func (Room) TableName() string {
	return "rooms"
}

func (r *Room) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == "" {
		r.ID = cuid.New()
	}
	return nil
}
```

### Essential Entity Rules:
1. **Primary Key:** Always `ID string` with `cuid.New()`. Do **not** use auto-increment integers.
2. **TableName:** Always implement `func (Model) TableName() string` returning lowercase snake_case plural table names.
3. **Foreign Keys:** Always specify explicit foreign keys with `onUpdate` and `onDelete` clauses.
4. **Auto-Migration:** Whenever a new entity is created, register it in `db/db.go`:
   ```go
   // db/db.go
   _ = db.AutoMigrate(
       &domain.User{},
       &domain.Room{},
       &domain.RoomParticipant{},
   )
   ```

---

## 4. Defining DTOs & Validation

DTOs are defined in `module/v1/<name>/<name>.dto.go`. 

Use `binding:"..."` tags for body payloads (JSON) and `form:"..."` tags for query params:

```go
package room

// Create DTO: required fields must be validated
type CreateRoomDto struct {
	Title           string  `json:"title" binding:"required,min=3,max=100"`
	Description     *string `json:"description" binding:"omitempty,max=500"`
	Topic           string  `json:"topic" binding:"omitempty,max=50"`
	MaxParticipants int     `json:"max_participants" binding:"omitempty,min=2,max=500"`
	IsPrivate       bool    `json:"is_private"`
	RoomKey         *string `json:"room_key" binding:"omitempty,min=4,max=32"`

	// Injected by Controller from JWT Context
	HostID string `json:"-"`
}

// Update DTO: all fields MUST be pointers to support partial updates
type UpdateRoomDto struct {
	Title           *string `json:"title" binding:"omitempty,min=3,max=100"`
	Description     *string `json:"description" binding:"omitempty,max=500"`
	Topic           *string `json:"topic" binding:"omitempty,max=50"`
	MaxParticipants *int    `json:"max_participants" binding:"omitempty,min=2,max=500"`
	Status          *string `json:"status" binding:"omitempty,oneof=active ended"`
}

// Query / Filter DTO
type RoomQuery struct {
	Search    *string `form:"search"`
	Topic     *string `form:"topic"`
	Status    *string `form:"status"` // default: "active"
	SortBy    *string `form:"sort_by"`
	SortOrder *string `form:"sort_order"` // "asc" or "desc"
	Skip      *int    `form:"skip"`
	Limit     *int    `form:"limit"`
}

// Join Room Request
type JoinRoomDto struct {
	RoomKey *string `json:"room_key" binding:"omitempty"`
	UserID  string  `json:"-"`
}
```

### Key DTO Rules:
- For `Update` requests: Use pointers (`*string`, `*int`, `*bool`) to distinguish between `nil` (omitted) and zero values (`""`, `0`, `false`).
- For pagination: Default `Limit` to `20` (max `100`), default `Skip` to `0`.
- Mark context-injected fields with `json:"-"` or `form:"-"`.

---

## 5. Defining the Service Layer & Business Logic

The service layer contains **all** database interactions, room state checks, and business rules. It lives in `module/v1/<name>/<name>.service.go`.

### Standards:
- Must define an Interface: `<Name>ServiceInterface`.
- Must return `httpx.APIResponse`.
- Use `httpx.SendData(httpStatusCode, message, data)` to build standardized responses.
- Construct queries safely using parameterized bindings (`?`).

```go
package room

import (
	"errors"
	"fmt"
	"net/http"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/utils/httpx"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RoomServiceInterface interface {
	CreateRoom(data *CreateRoomDto) httpx.APIResponse
	GetRoomByID(id string) httpx.APIResponse
	GetRoomList(query *RoomQuery) httpx.APIResponse
	UpdateRoom(id, hostID string, data *UpdateRoomDto) httpx.APIResponse
	JoinRoom(roomID string, data *JoinRoomDto) httpx.APIResponse
	LeaveRoom(roomID, userID string) httpx.APIResponse
	EndRoom(roomID, hostID string) httpx.APIResponse
}

type RoomService struct {
	db *gorm.DB
}

func NewRoomService(db *gorm.DB) RoomServiceInterface {
	return &RoomService{db: db}
}

func (s *RoomService) CreateRoom(data *CreateRoomDto) httpx.APIResponse {
	maxParticipants := 50
	if data.MaxParticipants > 0 {
		maxParticipants = data.MaxParticipants
	}

	topic := "general"
	if data.Topic != "" {
		topic = data.Topic
	}

	room := domain.Room{
		Title:           data.Title,
		Description:     data.Description,
		Topic:           topic,
		HostID:          data.HostID,
		MaxParticipants: maxParticipants,
		IsPrivate:       data.IsPrivate,
		RoomKey:         data.RoomKey,
		Status:          "active",
	}

	// ACID Transaction: Create room and enroll host as participant
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&room).Error; err != nil {
			return err
		}

		hostParticipant := domain.RoomParticipant{
			RoomID:    room.ID,
			UserID:    data.HostID,
			Role:      "host", // host, speaker, listener
			IsMuted:   false,
			JoinedAt:  room.CreatedAt,
		}
		return tx.Create(&hostParticipant).Error
	})

	if err != nil {
		return httpx.SendData(http.StatusInternalServerError, "Failed to create room", err.Error())
	}

	return httpx.SendData(http.StatusCreated, "Voice room created successfully", room)
}

func (s *RoomService) JoinRoom(roomID string, data *JoinRoomDto) httpx.APIResponse {
	var room domain.Room

	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Row lock room record to prevent race conditions exceeding capacity
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = 'active'", roomID).First(&room).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("room not found or is no longer active")
			}
			return err
		}

		// Check private room key
		if room.IsPrivate && (room.RoomKey != nil && *room.RoomKey != "") {
			if data.RoomKey == nil || *data.RoomKey != *room.RoomKey {
				return errors.New("invalid room key")
			}
		}

		// Count active participants
		var currentCount int64
		if err := tx.Model(&domain.RoomParticipant{}).Where("room_id = ? AND left_at IS NULL", roomID).Count(&currentCount).Error; err != nil {
			return err
		}

		if currentCount >= int64(room.MaxParticipants) {
			return errors.New("room is already at full capacity")
		}

		// Enroll or reactivate participant
		var participant domain.RoomParticipant
		err := tx.Where("room_id = ? AND user_id = ?", roomID, data.UserID).First(&participant).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			newParticipant := domain.RoomParticipant{
				RoomID:   roomID,
				UserID:   data.UserID,
				Role:     "listener",
				IsMuted:  true,
			}
			return tx.Create(&newParticipant).Error
		} else if err == nil {
			return tx.Model(&participant).Updates(map[string]interface{}{
				"left_at":  nil,
				"is_muted": true,
			}).Error
		}
		return err
	})

	if err != nil {
		return httpx.SendData(http.StatusBadRequest, err.Error())
	}

	return httpx.SendData(http.StatusOK, "Joined voice room successfully", room)
}
```

---

## 6. Defining the Controller & Router

Controllers live in `module/v1/<name>/<name>.controller.go`.

### Responsibilities:
1. Bind & Validate request inputs:
   - Body: `if err := ctx.ShouldBindJSON(&dto); err != nil { ... }` or `httpx.BindAndValidate(ctx, &dto)`
   - Query: `if err := ctx.ShouldBindQuery(&query); err != nil { ... }`
2. Extract authenticated claims from `ctx` (`user_id`, `role`, `email`).
3. Pass data to service.
4. Send response using `httpx.SendResponse(ctx, response)`.

```go
package room

import (
	"net/http"

	"nimble-voice-backend/middleware"
	"nimble-voice-backend/utils/httpx"

	"github.com/gin-gonic/gin"
)

type RoomController struct {
	service RoomServiceInterface
}

func NewRoomController(service RoomServiceInterface) RoomController {
	return RoomController{service: service}
}

func RoomRouter(router *gin.RouterGroup, controller RoomController) {
	// POST /api/v1/room/create (Authenticated)
	router.POST("/create", middleware.Auth(), func(ctx *gin.Context) {
		var data CreateRoomDto
		if err := ctx.ShouldBindJSON(&data); err != nil {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusBadRequest, "Invalid request payload", err.Error()))
			return
		}

		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}
		data.HostID = userID.(string)

		response := controller.service.CreateRoom(&data)
		httpx.SendResponse(ctx, response)
	})

	// POST /api/v1/room/:id/join
	router.POST("/:id/join", middleware.Auth(), func(ctx *gin.Context) {
		roomID := ctx.Param("id")
		var data JoinRoomDto
		_ = ctx.ShouldBindJSON(&data)

		userID, _ := ctx.Get("user_id")
		data.UserID = userID.(string)

		response := controller.service.JoinRoom(roomID, &data)
		httpx.SendResponse(ctx, response)
	})

	// GET /api/v1/room/list
	router.GET("/list", func(ctx *gin.Context) {
		var query RoomQuery
		if err := ctx.ShouldBindQuery(&query); err != nil {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusBadRequest, "Invalid query parameters", err.Error()))
			return
		}

		response := controller.service.GetRoomList(&query)
		httpx.SendResponse(ctx, response)
	})

	// GET /api/v1/room/:id
	router.GET("/:id", func(ctx *gin.Context) {
		roomID := ctx.Param("id")
		response := controller.service.GetRoomByID(roomID)
		httpx.SendResponse(ctx, response)
	})
}
```

---

## 7. Real-Time Audio Signaling & Socket.IO Events

Real-time audio room presence, signaling, and controls are handled via `module/v1/socket/socket.go`.

### Core Socket Events:
| Event Name | Direction | Payload | Purpose |
|---|---|---|---|
| `room:join` | Client -> Server | `{ "room_id": "...", "user_id": "..." }` | Join voice room channel |
| `room:leave` | Client -> Server | `{ "room_id": "...", "user_id": "..." }` | Leave voice room channel |
| `room:mute` | Client -> Server | `{ "room_id": "...", "is_muted": true }` | Toggle participant microphone mute |
| `room:raise_hand` | Client -> Server | `{ "room_id": "..." }` | Request permission to speak |
| `room:user_joined` | Server -> Room | `{ "user": {...}, "role": "listener" }` | Broadcast new member joined |
| `room:user_left` | Server -> Room | `{ "user_id": "..." }` | Broadcast member left |
| `room:speaking_state` | Server -> Room | `{ "user_id": "...", "speaking": true }`| Real-time audio waveform/speaker glow |

---

## 8. Critical Voice Room Patterns

### Pattern 1: ACID Room Concurrency & Capacity Guard
Always lock room capacity checks using `tx.Clauses(clause.Locking{Strength: "UPDATE"})` inside a database transaction to prevent exceeding `max_participants` during simultaneous join bursts.

### Pattern 2: Multi-Role Authorization
- **Host:** Can edit room details, end room, mute any participant, promote listeners to speakers, and kick users.
- **Speaker:** Can broadcast audio stream, mute/unmute own mic, and lower hand.
- **Listener:** Default state upon joining. Receives audio stream, cannot speak unless invited or promoted by host.

### Pattern 3: Clean Separation of Media & Signaling
- Signaling & Room state: Handled by this backend (`nimble-voice-backend`) via REST APIs and WebSockets / Socket.IO.
- Audio Media Stream: WebRTC SFU/Mesh (e.g. LiveKit, mediasoup, Pion) handles RTP streams while this backend coordinates permissions and tokens.

---

## 9. Quality Checklist Before Completing Any Task

1. [ ] **Domain Entity:** Primary key uses `cuid.New()` in `BeforeCreate`. Explicit `TableName()` method returning plural snake_case.
2. [ ] **Auto-Migration:** Registered in `db/db.go`.
3. [ ] **DTOs:** Create DTO has `binding:"required"`, Update DTO fields are pointers (`*string`, `*int`).
4. [ ] **Service Interface:** Exported methods return `httpx.APIResponse`.
5. [ ] **Safe SQL:** Queries parameterized with `?` (never string concatenation).
6. [ ] **Auth & Context:** Protected routes use `middleware.Auth()`. User identity strictly verified from context.
7. [ ] **Standardized Envelope:** Responses returned via `httpx.SendResponse(ctx, response)` using `httpx.SendData(...)`.
8. [ ] **Compilation:** Project builds clean with `go vet ./...` or `go build ./cmd/server/main.go`.
