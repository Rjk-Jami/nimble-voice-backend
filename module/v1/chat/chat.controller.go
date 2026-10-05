package chat

import (
	"net/http"

	"nimble-voice-backend/middleware"
	"nimble-voice-backend/server"
	"nimble-voice-backend/utils/httpx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ChatController struct {
	service ChatServiceInterface
}

func NewChatController(service ChatServiceInterface) ChatController {
	return ChatController{service: service}
}

func ChatRouter(router *gin.RouterGroup, controller ChatController) {
	// GET /:id/messages (matches /api/rooms/:id/messages)
	router.GET("/:id/messages", func(ctx *gin.Context) {
		roomID := ctx.Param("id")
		var query ChatQuery
		_ = ctx.ShouldBindQuery(&query)

		response := controller.service.GetMessages(roomID, &query)
		httpx.SendResponse(ctx, response)
	})

	// POST /:id/messages (Protected - matches /api/rooms/:id/messages)
	router.POST("/:id/messages", middleware.Auth(), func(ctx *gin.Context) {
		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		roomID := ctx.Param("id")
		var dto SendMessageDto
		if err := ctx.ShouldBindJSON(&dto); err != nil {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusBadRequest, "Invalid message payload", err.Error()))
			return
		}

		response := controller.service.SendMessage(roomID, userID.(string), &dto)
		httpx.SendResponse(ctx, response)
	})
}

func ChatModule(router *gin.RouterGroup, db *gorm.DB, sockServer ...*server.SocketServer) {
	service := NewChatService(db, sockServer...)
	controller := NewChatController(service)
	ChatRouter(router, controller)
}
