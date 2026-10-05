package route_v1

import (
	"nimble-voice-backend/module/v1/auth"
	"nimble-voice-backend/module/v1/chat"
	"nimble-voice-backend/module/v1/room"
	"nimble-voice-backend/module/v1/stats"
	"nimble-voice-backend/module/v1/topic"
	"nimble-voice-backend/module/v1/user"
	"nimble-voice-backend/server"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterV1Routes(router *gin.RouterGroup, db *gorm.DB, sockServer ...*server.SocketServer) {
	// Auth: /auth/guest, /auth/session, /auth/profile, /auth/register, /auth/login
	auth.AuthModule(router.Group("/auth"), db)

	// Rooms: /rooms (CRUD, join, leave)
	room.RoomModule(router.Group("/rooms"), db)
	room.RoomModule(router.Group("/room"), db) // Singular alias

	// Chat backchannel: /rooms/:roomId/messages
	chat.ChatModule(router.Group("/rooms"), db, sockServer...)
	chat.ChatModule(router.Group("/room"), db, sockServer...)

	// Users: /users/:userId, /users/portfolio, /users/stats
	user.UserModule(router.Group("/users"), db)
	user.UserModule(router.Group("/user"), db) // Singular alias

	// Topics: /topics/prompts
	topic.TopicModule(router.Group("/topics"), db)
	topic.TopicModule(router.Group("/topic"), db)

	// Stats: /stats/network
	stats.StatsModule(router.Group("/stats"), db, sockServer...)
}
