package stats

import (
	"nimble-voice-backend/server"
	"nimble-voice-backend/utils/httpx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type StatsController struct {
	service StatsServiceInterface
}

func NewStatsController(service StatsServiceInterface) StatsController {
	return StatsController{service: service}
}

func StatsRouter(router *gin.RouterGroup, controller StatsController) {
	// GET /network
	router.GET("/network", func(ctx *gin.Context) {
		response := controller.service.GetNetworkStats()
		httpx.SendResponse(ctx, response)
	})
}

func StatsModule(router *gin.RouterGroup, db *gorm.DB, sockServer ...*server.SocketServer) {
	service := NewStatsService(db, sockServer...)
	controller := NewStatsController(service)
	StatsRouter(router, controller)
}
