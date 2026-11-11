package webrtc

import (
	"nimble-voice-backend/config"

	"github.com/gin-gonic/gin"
)

func WebRTCModule(router *gin.RouterGroup, cfg *config.Config) {
	service := NewWebRTCService(cfg)
	controller := NewWebRTCController(service)
	WebRTCRouter(router, controller)
}
