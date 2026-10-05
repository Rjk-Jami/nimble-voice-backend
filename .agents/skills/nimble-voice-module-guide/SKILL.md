---
name: nimble-voice-module-guide
description: Step-by-step cheatsheet and execution guide for creating and modifying modules, controllers, services, database entities, real-time voice room events, concurrency locks, and Socket.IO signaling in Nimble Voice Backend (Go/Gin/GORM).
---

# Nimble Voice Backend - Agent Module & Real-Time API Skill

Use this skill whenever asked to build a new module, create an API, write GORM database entities, implement real-time voice room signaling events, execute concurrent room transactions, or add background routines in **Nimble Voice Backend** (`nimble-voice-backend`).

---

## ⚡ Quick Architecture Cheatsheet

| Component | File Path Pattern | Primary Responsibility |
|---|---|---|
| **Domain Model** | `domain/<name>.go` | GORM struct, CUID PK (`cuid.New()`), `TableName()`, relationships |
| **Database Migration** | `db/db.go` | AutoMigrate registration in `ConnectPostgres` |
| **DTOs** | `module/v1/<name>/<name>.dto.go` | Create/Update/Query request and response structs with validation tags |
| **Service** | `module/v1/<name>/<name>.service.go` | Interface, business logic, ACID transactions, returns `httpx.APIResponse` |
| **Controller** | `module/v1/<name>/<name>.controller.go` | HTTP routes, `middleware.Auth()`, DTO binding, claims extraction |
| **Module Wiring** | `module/v1/<name>/<name>.module.go` | Initializes service, controller, and mounts endpoints onto Gin RouterGroup |
| **Route Registration**| `module/v1/routes.go` | Registers module group under `/api/v1/<name>` |
| **Real-time Signaling**| `module/v1/socket/socket.go` | Real-time audio room event listeners and socket broadcasting |

---

## 🛠️ Step-by-Step Module Creation Workflow

When a user asks: *"Create a new `<name>` module/feature"* (e.g. `room`, `channel`, `recording`), execute these steps:

1. **Domain Entity (`domain/<name>.go`):** Define struct with CUID PK, `TableName()`, and relations.
2. **AutoMigrate (`db/db.go`):** Register domain model in `db.AutoMigrate(...)`.
3. **DTOs (`module/v1/<name>/<name>.dto.go`):** Create/Update/Query DTOs with `binding:"required"` and pointer types for partial updates.
4. **Service Layer (`module/v1/<name>/<name>.service.go`):** Define interface `<Name>ServiceInterface`, business validations, ACID transactions, and return `httpx.APIResponse`.
5. **Controller (`module/v1/<name>/<name>.controller.go`):** Bind DTO, extract JWT claims (`user_id`), invoke service, and reply with `httpx.SendResponse(ctx, response)`.
6. **Module Wiring (`module/v1/<name>/<name>.module.go`):** Wire DB -> Service -> Controller -> Router.
7. **Mount in Global Routes (`module/v1/routes.go`):** Register module group under `RegisterV1Routes`.
