# Swagger / OpenAPI Setup & Architecture Guide

This document explains **how Swagger documentation is structured, generated, and served** in **Nimble Voice Backend** (`nimble-voice-backend`), the exact mechanics behind the scenes, and how to maintain and expand this setup.

---

## 1. Architectural Overview: How It All Works

```
┌────────────────────────────────────────────────────────┐
│ 1. API General Info                                    │
│    (cmd/server/main.go)                                │
│    - @title, @version, @host, @BasePath, @securityDef  │
└──────────────────────────┬─────────────────────────────┘
                           │
┌──────────────────────────┴─────────────────────────────┐
│ 2. Route Annotations & DTOs                            │
│    (module/v1/*/*.swagger.go & *.dto.go)               │
│    - @Summary, @Tags, @Param, @Router                  │
│    - Struct tags: `example:"..."`, `swaggerignore:"true"`
└──────────────────────────┬─────────────────────────────┘
                           │
                           │  `swag init -g cmd/server/main.go -o docs --parseDependency --parseInternal`
                           ▼
┌────────────────────────────────────────────────────────┐
│ 3. Generated Artifacts (docs/)                         │
│    - docs/docs.go        (Go code with init() hook)    │
│    - docs/swagger.json   (OpenAPI 2.0 JSON)            │
│    - docs/swagger.yaml   (OpenAPI 2.0 YAML)            │
└──────────────────────────┬─────────────────────────────┘
                           │
                           │  Blank import: `_ "nimble-voice-backend/docs"`
                           ▼
┌────────────────────────────────────────────────────────┐
│ 4. Serving the Documentation (module/router.go)        │
│    - ginSwagger.CustomWrapHandler                      │
│    - swaggerFiles.Handler                              │
│    - PersistAuthorization: true                        │
│    Accessible at: http://localhost:8080/swagger/index.html
└────────────────────────────────────────────────────────┘
```

### The Three Core Libraries
1. **`github.com/swaggo/swag/cmd/swag`**: Command-line tool that parses Go code and AST comments into Swagger (OpenAPI 2.0) format.
2. **`github.com/swaggo/gin-swagger`**: Gin middleware that serves the interactive Swagger UI website and handles `/swagger/doc.json`.
3. **`github.com/swaggo/files`**: Embedded static assets (HTML, CSS, JavaScript) for Swagger UI.

---

## 2. Step-by-Step Breakdown of the Workflow

### Step 1: Global API Configuration (`cmd/server/main.go`)

In `cmd/server/main.go`, declarative comments are added directly above the `main()` function defining general metadata, base path, and JWT authentication:

```go
// @title                      Nimble Voice Backend API
// @version                    1.0.0
// @description                Real-time audio rooms, voice signaling, and language practice platform backend.
// @contact.name               Nimble Voice Engineering Team
// @host                       localhost:8080
// @BasePath                   /api/v1
// @securityDefinitions.apikey BearerAuth
// @in                         header
// @name                       Authorization
// @description                Type "Bearer" followed by a space and your JWT token.
func main() { ... }
```

* `@securityDefinitions.apikey BearerAuth`: Registers Bearer token authentication in Swagger UI so protected endpoints can be tested directly from the browser.
* `@BasePath /api/v1`: Base path prefixed to all `@Router` endpoints.

---

### Step 2: Route Annotations via the `*.swagger.go` Pattern

Rather than cluttering controller functions or route definitions, this repository isolates Swagger definitions into dedicated `*.swagger.go` files:
- `module/v1/auth/auth.swagger.go`
- `module/v1/room/room.swagger.go`
- `module/v1/chat/chat.swagger.go`
- `module/v1/user/user.swagger.go`
- `module/v1/topic/topic.swagger.go`
- `module/v1/stats/stats.swagger.go`

Because route handlers in Gin are often registered as closures:
```go
router.POST("/create", func(ctx *gin.Context) { ... })
```
The codebase uses **blank anonymous functions** (`func _() {}`) to attach Swag declarative comments:

```go
package room

import _ "nimble-voice-backend/utils/httpx"

// CreateRoom godoc
// @Summary      Create a new voice room
// @Description  Creates a new voice room and enrolls the authenticated host
// @Tags         Rooms
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        payload body CreateRoomDto true "Room configuration details"
// @Success      201     {object} httpx.APIResponse
// @Failure      400     {object} httpx.APIResponse
// @Failure      401     {object} httpx.APIResponse
// @Router       /rooms [post]
func _() {}
```

#### Why `func _() {}`?
In Go, `_` is a blank identifier. You can have multiple `func _() {}` declarations in the same file without name conflicts. Swag parses each AST function node and extracts the comment block above it.

---

### Step 3: DTO Documentation Tags (`*.dto.go`)

In DTO structs (e.g., `room.dto.go`, `auth.dto.go`), struct tags instruct `swag` on how to render request and response bodies:

```go
type CreateRoomDto struct {
    Title              string   `json:"title" binding:"required,min=3,max=100" example:"Daily English Practice"`
    Language           string   `json:"language" binding:"required" example:"en"`
    CEFRLevel          string   `json:"cefrLevel" example:"B1"`
    LevelLabel         string   `json:"levelLabel" example:"Intermediate"`
    MaxSlots           int      `json:"maxSlots" example:"6"`
    TopicTag           string   `json:"topicTag" example:"Casual Chat"`
    Description        *string  `json:"description" example:"Practice casual conversation and fluency"`
    Topic              string   `json:"topic" example:"Weekend plans and hobbies"`
    IsBeginnerFriendly *bool    `json:"isBeginnerFriendly" example:"true"`
    Tags               []string `json:"tags" example:"english,speaking,fluency"`
    RoomKey            *string  `json:"room_key" example:""`
}
```

* `example:"..."`: Pre-fills the Swagger UI interactive request payload with realistic mock data.
* `swaggerignore:"true"`: Hides internal fields from the API documentation (e.g., fields populated automatically by auth middleware).

---

### Step 4: Generating the Swagger Specification (`swag init`)

To compile all comments into OpenAPI documentation, run:

```powershell
swag init -g cmd/server/main.go -o ./docs --parseDependency --parseInternal
```

#### What happens during generation:
1. `swag` scans `cmd/server/main.go` for the root API metadata.
2. It parses imports recursively, finding all packages under `module/v1/...`.
3. It extracts every comment matching `@Summary`, `@Router`, `@Param`, `@Tags`, etc.
4. It inspects all referenced Go structs and parses their tags (`json`, `example`, `swaggerignore`).
5. It generates three files inside `docs/`:
   - `docs/docs.go`: Registers the spec with `swag.Register()` inside an `init()` function.
   - `docs/swagger.json`: JSON format OpenAPI 2.0 spec.
   - `docs/swagger.yaml`: YAML format OpenAPI 2.0 spec.

---

### Step 5: Registering & Serving Swagger in Gin (`module/router.go`)

Swagger UI is mounted directly into the Gin engine:

#### 1. Imports in `module/router.go`
```go
import (
    "net/http"
    swaggerFiles "github.com/swaggo/files"
    ginSwagger "github.com/swaggo/gin-swagger"
    _ "nimble-voice-backend/docs" // Registers SwaggerInfo
)
```

#### 2. Route Registration in `SetupRouter`
```go
// Redirect /swagger to /swagger/index.html
router.GET("/swagger", func(ctx *gin.Context) {
    ctx.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
})

// Serve Swagger UI with custom configuration
router.GET("/swagger/*any", ginSwagger.CustomWrapHandler(&ginSwagger.Config{
    URL:                      "/swagger/doc.json",
    DocExpansion:             "none", // Keeps module tags collapsed initially
    DefaultModelsExpandDepth: -1,     // Hides default model schemas at bottom
    DeepLinking:              true,   // Deep link to specific endpoints
    PersistAuthorization:     true,   // Retains Bearer token across refreshes
    Title:                    "Nimble Voice API Docs",
}, swaggerFiles.Handler))
```

---

## 3. Quick Reference: How to Add Swagger for a New Module

When creating a new resource (e.g. `recording`):

### 1. Create `module/v1/recording/recording.swagger.go`
```go
package recording

import _ "nimble-voice-backend/utils/httpx"

// GetRecordings godoc
// @Summary      Get room recordings
// @Description  Returns list of audio recordings for a session
// @Tags         Recordings
// @Produce      json
// @Security     BearerAuth
// @Param        roomId query string true "Room ID"
// @Success      200    {object} httpx.APIResponse
// @Router       /recordings [get]
func _() {}
```

### 2. Add `example` tags in `module/v1/recording/recording.dto.go`
```go
type CreateRecordingDto struct {
    RoomID   string `json:"roomId" binding:"required" example:"ckz1234567890"`
    Duration int    `json:"duration" example:"120"`
}
```

### 3. Regenerate Swagger Docs
```powershell
swag init -g cmd/server/main.go -o ./docs --parseDependency --parseInternal
```

---

## 4. Endpoints Currently Documented

| Tag | Method | Endpoint | Description | Security |
|---|---|---|---|---|
| **Auth** | `POST` | `/api/v1/auth/guest` | Create guest session | Public |
| **Auth** | `GET` | `/api/v1/auth/session` | Get current session details | Public/Token |
| **Auth** | `GET` | `/api/v1/auth/profile` | Get authenticated user profile | BearerAuth |
| **Auth** | `POST` | `/api/v1/auth/register` | Register new user account | Public |
| **Auth** | `POST` | `/api/v1/auth/login` | Authenticate user & get token | Public |
| **Rooms** | `GET` | `/api/v1/rooms` | List and filter active voice rooms | Public |
| **Rooms** | `POST` | `/api/v1/rooms` | Create new voice room (Host) | BearerAuth |
| **Rooms** | `GET` | `/api/v1/rooms/{id}` | Get room details & participants | Public |
| **Rooms** | `PATCH` | `/api/v1/rooms/{id}` | Update room properties | BearerAuth |
| **Rooms** | `DELETE` | `/api/v1/rooms/{id}` | Delete / end voice room | BearerAuth |
| **Rooms** | `POST` | `/api/v1/rooms/{id}/join` | Join room & allocate audio slot | BearerAuth |
| **Rooms** | `POST` | `/api/v1/rooms/{id}/leave` | Leave room & free slot | BearerAuth |
| **Chat** | `GET` | `/api/v1/rooms/{id}/messages` | Get room chat history | Public |
| **Chat** | `POST` | `/api/v1/rooms/{id}/messages` | Send message/reaction to backchannel | BearerAuth |
| **Users** | `PATCH` | `/api/v1/users/portfolio` | Update learner portfolio | BearerAuth |
| **Users** | `GET` | `/api/v1/users/stats` | Get user audio speaking stats | BearerAuth |
| **Users** | `GET` | `/api/v1/users/{userId}` | Get public learner card | Public |
| **Topics** | `GET` | `/api/v1/topics/prompts` | Get conversation starters & decks | Public |
| **Stats** | `GET` | `/api/v1/stats/network` | Get live platform stats | Public |

---

## 5. Troubleshooting & FAQ

| Issue | Cause | Fix |
|---|---|---|
| **`404 Not Found` at `/swagger/index.html`** | Swagger route not registered or trailing slash redirection | Visit `http://localhost:8080/swagger/index.html` directly or ensure `/swagger` redirects to `/swagger/index.html`. |
| **`Swagger doc.json not found` or panic** | Blank import missing | Ensure `_ "nimble-voice-backend/docs"` is imported in `cmd/server/main.go` and `module/router.go`. |
| **Changes not reflected in Swagger UI** | `swag init` was not run after editing comments | Run `swag init -g cmd/server/main.go -o ./docs --parseDependency --parseInternal` and recompile. |
| **Path parameters not showing input box in UI** | Path in comment uses `:id` instead of `{id}` | `@Router` syntax requires curly braces: `/rooms/{id} [get]`. |
| **Unused import compile error** | Import in `*.swagger.go` without code usage | Use blank import: `import _ "nimble-voice-backend/utils/httpx"`. |
