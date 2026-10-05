package topic

import (
	"nimble-voice-backend/utils/httpx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type TopicController struct {
	service TopicServiceInterface
}

func NewTopicController(service TopicServiceInterface) TopicController {
	return TopicController{service: service}
}

func TopicRouter(router *gin.RouterGroup, controller TopicController) {
	// GET /prompts
	router.GET("/prompts", func(ctx *gin.Context) {
		var query TopicQuery
		_ = ctx.ShouldBindQuery(&query)
		response := controller.service.GetPrompts(&query)
		httpx.SendResponse(ctx, response)
	})
}

func TopicModule(router *gin.RouterGroup, db *gorm.DB) {
	service := NewTopicService(db)
	controller := NewTopicController(service)
	TopicRouter(router, controller)
}
