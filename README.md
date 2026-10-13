# Nimble Voice Backend

An enterprise-grade, modular real-time backend in Go for audio rooms, WebRTC voice signaling, and language practice platforms. Built with **Gin**, **GORM**, **PostgreSQL**, **Socket.IO v4**, **Uber Zap**, and **JWT Authentication**.

---

## 🚀 Features

- **Modular Architecture**: Domain-driven design with modular packaging (`auth`, `room`, `chat`, `user`, `topic`, `stats`).
- **Real-Time Signaling**: Pure Go Socket.IO v4 (`gsocketio`) mounted at `/socket.io/` for WebRTC mesh signaling, voice room events, and live backchannel chat.
- **Authentication & Security**:
  - Guest one-click access and JWT user registration/login.
  - Role-based route protection middleware.
  - Password hashing with Bcrypt.
  - In-memory rate limiting via **Tollbooth**.
  - Configurable CORS policies.
- **Persistence & ORM**:
  - **GORM** with PostgreSQL driver.
  - Automatic database schema migrations.
  - Collision-resistant **CUID** primary keys.
  - Optional MS SQL Server connector for enterprise ERP integrations.
- **Media & Avatar Uploads**: Secure multipart file uploader with MIME type verification and static delivery under `/uploads`.
- **API Documentation**: Automated Swagger (OpenAPI 2.0) UI with persisted Bearer authentication at `/swagger/index.html`.
- **Structured Logging**: High-performance Uber **Zap** logger with **Lumberjack** log file rotation.
- **Graceful Shutdown**: Zero-downtime connection draining on SIGINT/SIGTERM.

---

## 📁 Directory Structure

```text
nimble-voice-backend/
├── cmd/
│   └── server/
│       └── main.go               # Application entry point & lifecycle
├── config/
│   └── config.go                 # Environment configuration loader
├── db/
│   ├── db.go                     # PostgreSQL connection & auto-migrations
│   └── sqlserver.go              # Optional MS SQL Server connector
├── docs/                         # Generated Swagger / OpenAPI documentation
│   ├── docs.go
│   ├── swagger.json
│   └── swagger.yaml
├── domain/                       # Core database entities & GORM models
│   ├── base_model.go             # CUID primary key & timestamp base
│   ├── chat_message.go           # Room chat message entity
│   ├── participant.go            # Room participant tracking
│   ├── room.go                   # Audio room entity
│   ├── topic_prompt.go           # Conversation practice topics
│   └── user.go                   # User entity with bcrypt hashing
├── middleware/
│   ├── auth.go                   # JWT verification & claims injector
│   ├── logger.go                 # Zap HTTP request logging
│   └── rate_limiter.go           # Tollbooth rate-limiting middleware
├── module/
│   ├── router.go                 # Central router, CORS, Swagger & Socket.IO mount
│   └── v1/
│       ├── routes.go             # API v1 sub-module aggregator
│       ├── auth/                 # Guest & user authentication endpoints
│       ├── chat/                 # Room backchannel messaging
│       ├── room/                 # Voice room lifecycle (create, join, leave)
│       ├── socket/               # Socket.IO event handlers & signaling
│       ├── stats/                # Real-time network & active user metrics
│       ├── topic/                # Conversation starters & practice topics
│       └── user/                 # Profile, portfolio, and avatar upload
├── pkg/
│   ├── cron/                     # Background job scheduler
│   └── logger/                   # Zap logger initialization & rotation
├── server/
│   └── socket.go                 # Socket.IO v4 engine & Gin bridge
├── utils/
│   ├── fileutil/                 # Upload validation & file handling
│   ├── httpx/                    # Standardized JSON response helpers
│   └── jwt/                      # JWT generation & token parsing
├── uploads/                      # Uploaded files (avatars, attachments)
├── tmp/                          # Temporary build files (ignored by Git)
├── .air.toml                     # Air live-reload configuration
├── .env.example                  # Environment variable template
├── go.mod                        # Go module dependencies
└── README.md
```

---

## 🛠️ Prerequisites

- **Go**: Version 1.23 or newer (tested on Go 1.26+)
- **PostgreSQL**: Version 14 or newer
- **Git**

---

## ⚙️ Environment Configuration

1. Copy the example environment file:

   **Linux / macOS / Git Bash:**
   ```bash
   cp .env.example .env
   ```

   **Windows PowerShell:**
   ```powershell
   Copy-Item .env.example .env
   ```

2. Open `.env` and configure your credentials:

```env
PORT=8080
APP_ENV=development
APP_URL=http://localhost:8080

# PostgreSQL Configuration
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_postgres_password
DB_NAME=enterprise_db
DB_SSLMODE=disable
DB_TIMEZONE=Asia/Dhaka

# JWT Authentication
JWT_SECRET=enterprise_super_secret_jwt_key_2026

# Uploads Configuration
UPLOAD_DIR=./uploads
MAX_FILE_SIZE_MB=5
```

---

## ⚡ Development with Air (Live-Reload)

[Air](https://github.com/air-verse/air) provides fast live-reloading for Go applications during development.

### 1. Install Air

Install the latest version of Air using Go:

```bash
go install github.com/air-verse/air@latest
```

> **Note**: Ensure that your Go binary path (`$GOPATH/bin` or `%USERPROFILE%\go\bin`) is added to your system `PATH` environment variable.
>
> You can verify the installation with:
> ```bash
> air -v
> ```

### 2. Run with Air

From the project root directory, run:

```bash
air
```

Air will automatically read [.air.toml](file:///d:/comeBack/2026/voicerooms/nimble-voice-backend/.air.toml), watch for changes in `.go`, `.env`, and template files, rebuild the binary into `./tmp`, and restart the server automatically.

#### Platform-Specific Notes for `.air.toml`:
The project includes a ready-to-use `.air.toml`.
- **Windows**: The default `.air.toml` uses `cmd = "go build -o .\\tmp\\main.exe .\\cmd\\server"` and `bin = ".\\tmp\\main.exe"`.
- **Linux / macOS**: If running on Unix-like environments, ensure the build section uses forward slashes:
  ```toml
  [build]
    cmd = "go build -o ./tmp/main ./cmd/server"
    bin = "./tmp/main"
    entrypoint = "./tmp/main"
  ```

---

## 🏗️ Build & Production Deployment

### 1. Standard Local Run (Without Air)

To run the backend directly without hot-reloading:

```bash
go run cmd/server/main.go
```

### 2. Build Executable Binary

#### For Windows:
```powershell
# Standard build
go build -o bin/server.exe ./cmd/server

# Optimized production build (smaller binary, stripped debug symbols)
go build -ldflags="-s -w" -o bin/server.exe ./cmd/server
```

Run the built binary:
```powershell
.\bin\server.exe
```

#### For Linux / macOS:
```bash
# Standard build
go build -o bin/server ./cmd/server

# Optimized production build
go build -ldflags="-s -w" -o bin/server ./cmd/server
```

Run the built binary:
```bash
./bin/server
```

---

## 📖 API Documentation (Swagger)

Interactive Swagger API docs are embedded and accessible once the server is running:

- **Swagger UI**: [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)
- **JSON Specification**: [http://localhost:8080/swagger/doc.json](http://localhost:8080/swagger/doc.json)

### Regenerating Swagger Documentation
If you modify route annotations or DTOs:

1. Install Swag CLI:
   ```bash
   go install github.com/swaggo/swag/cmd/swag@latest
   ```

2. Regenerate docs:
   ```bash
   swag init -g cmd/server/main.go -o ./docs
   ```

---

## 🌐 API Endpoints Reference

Base Path: `/api/v1` (also aliased under `/api`)

### Health
| Method | Endpoint | Description | Auth |
|---|---|---|---|
| `GET` | `/health` | Health check endpoint | Public |

### Authentication (`/auth`)
| Method | Endpoint | Description | Auth |
|---|---|---|---|
| `POST` | `/api/v1/auth/guest` | Create instant guest session | Public |
| `POST` | `/api/v1/auth/register` | Register new user account | Public |
| `POST` | `/api/v1/auth/login` | Authenticate and obtain JWT | Public |
| `GET` | `/api/v1/auth/session` | Get active session info | Bearer JWT |
| `GET` | `/api/v1/auth/profile` | Get authenticated user profile | Bearer JWT |

### Voice Rooms (`/rooms`)
| Method | Endpoint | Description | Auth |
|---|---|---|---|
| `GET` | `/api/v1/rooms` | List active voice rooms | Public |
| `POST` | `/api/v1/rooms` | Create a new voice room | Bearer JWT |
| `GET` | `/api/v1/rooms/:roomId` | Get voice room details | Public |
| `POST` | `/api/v1/rooms/:roomId/join` | Join a voice room | Bearer JWT |
| `POST` | `/api/v1/rooms/:roomId/leave` | Leave a voice room | Bearer JWT |

### Room Chat Backchannel (`/rooms/:roomId/messages`)
| Method | Endpoint | Description | Auth |
|---|---|---|---|
| `GET` | `/api/v1/rooms/:roomId/messages` | Get room message history | Bearer JWT |
| `POST` | `/api/v1/rooms/:roomId/messages` | Send a chat message | Bearer JWT |

### Users & Portfolio (`/users`)
| Method | Endpoint | Description | Auth |
|---|---|---|---|
| `GET` | `/api/v1/users/:userId` | Get user profile by ID | Public |
| `GET` | `/api/v1/users/portfolio` | Get user speech portfolio | Bearer JWT |
| `GET` | `/api/v1/users/stats` | Get user learning stats | Bearer JWT |
| `POST` | `/api/v1/users/upload-avatar` | Upload avatar image (`multipart/form-data`) | Bearer JWT |

### Practice Topics & Prompts (`/topics`)
| Method | Endpoint | Description | Auth |
|---|---|---|---|
| `GET` | `/api/v1/topics/prompts` | Get practice conversation prompts | Public |

### Network & Real-Time Stats (`/stats`)
| Method | Endpoint | Description | Auth |
|---|---|---|---|
| `GET` | `/api/v1/stats/network` | Get active room & peer network metrics | Public |

### Static File Access
| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/uploads/avatars/:filename` | Access uploaded user avatars |

---

## 🔌 Socket.IO Real-Time Voice & Signaling

Socket.IO v4 is mounted at `/socket.io/`.

### Core Real-Time Events:
- **`join-room`**: Join a voice room with participant metadata.
- **`leave-room`**: Disconnect from the current voice room.
- **`offer` / `answer`**: WebRTC SDP signaling exchange between room peers.
- **`ice-candidate`**: WebRTC ICE candidates exchange.
- **`mute-state-changed`**: Broadcast microphone mute/unmute status.
- **`chat-message`**: Real-time backchannel chat message broadcast.

---

## 📜 License

This project is proprietary and confidential. All rights reserved.
