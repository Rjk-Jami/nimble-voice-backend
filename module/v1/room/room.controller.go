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
	// GET / (List & filter rooms)
	router.GET("", func(ctx *gin.Context) {
		var query RoomQuery
		_ = ctx.ShouldBindQuery(&query)
		response := controller.service.GetRoomList(&query)
		httpx.SendResponse(ctx, response)
	})

	// POST / (Create Room - Authenticated)
	router.POST("", middleware.Auth(), func(ctx *gin.Context) {
		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		var dto CreateRoomDto
		if err := ctx.ShouldBindJSON(&dto); err != nil {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusBadRequest, "Invalid room creation payload", err.Error()))
			return
		}

		response := controller.service.CreateRoom(userID.(string), &dto)
		httpx.SendResponse(ctx, response)
	})

	// GET /:id (Get single room details)
	router.GET("/:id", func(ctx *gin.Context) {
		roomID := ctx.Param("id")
		response := controller.service.GetRoomByID(roomID)
		httpx.SendResponse(ctx, response)
	})

	// PATCH /:id (Update room - Host only)
	router.PATCH("/:id", middleware.Auth(), func(ctx *gin.Context) {
		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		roomID := ctx.Param("id")
		var dto UpdateRoomDto
		if err := ctx.ShouldBindJSON(&dto); err != nil {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusBadRequest, "Invalid update payload", err.Error()))
			return
		}

		response := controller.service.UpdateRoom(roomID, userID.(string), &dto)
		httpx.SendResponse(ctx, response)
	})

	// DELETE /:id (Delete / close room - Host only)
	router.DELETE("/:id", middleware.Auth(), func(ctx *gin.Context) {
		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		roomID := ctx.Param("id")
		response := controller.service.DeleteRoom(roomID, userID.(string))
		httpx.SendResponse(ctx, response)
	})

	// POST /:id/join (Join room & allocate slot)
	router.POST("/:id/join", middleware.Auth(), func(ctx *gin.Context) {
		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		roomID := ctx.Param("id")
		var dto JoinRoomDto
		_ = ctx.ShouldBindJSON(&dto)

		response := controller.service.JoinRoom(roomID, userID.(string), &dto)
		httpx.SendResponse(ctx, response)
	})

	// POST /:id/leave (Leave room & release slot)
	router.POST("/:id/leave", middleware.Auth(), func(ctx *gin.Context) {
		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		roomID := ctx.Param("id")
		response := controller.service.LeaveRoom(roomID, userID.(string))
		httpx.SendResponse(ctx, response)
	})
}
