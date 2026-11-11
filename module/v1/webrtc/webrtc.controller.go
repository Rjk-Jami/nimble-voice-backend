package webrtc

import (
	"net/http"
	"strings"

	"nimble-voice-backend/middleware"
	"nimble-voice-backend/utils/httpx"

	"github.com/gin-gonic/gin"
)

type WebRTCController struct {
	service WebRTCServiceInterface
}

func NewWebRTCController(service WebRTCServiceInterface) WebRTCController {
	return WebRTCController{service: service}
}

func WebRTCRouter(router *gin.RouterGroup, controller WebRTCController) {
	// GET /ice-servers (Protected)
	router.GET("/ice-servers", middleware.Auth(), func(ctx *gin.Context) {
		ctx.Header("Cache-Control", "no-store")
		ctx.Header("Pragma", "no-cache")

		userIDRaw, ok := ctx.Get("user_id")
		userID, validStr := userIDRaw.(string)
		if !ok || !validStr || strings.TrimSpace(userID) == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		response := controller.service.GetIceServers(userID)
		httpx.SendResponse(ctx, response)
	})
}
