package user

import (
	"net/http"

	"nimble-voice-backend/middleware"
	"nimble-voice-backend/utils/httpx"

	"github.com/gin-gonic/gin"
)

type UserController struct {
	service UserServiceInterface
}

func NewUserController(service UserServiceInterface) UserController {
	return UserController{service: service}
}

func UserRouter(router *gin.RouterGroup, controller UserController) {
	// PATCH /portfolio (Protected)
	router.PATCH("/portfolio", middleware.Auth(), func(ctx *gin.Context) {
		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		var dto UpdatePortfolioDto
		if err := ctx.ShouldBindJSON(&dto); err != nil {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusBadRequest, "Invalid request payload", err.Error()))
			return
		}

		response := controller.service.UpdatePortfolio(userID.(string), &dto)
		httpx.SendResponse(ctx, response)
	})

	// GET /stats (Protected)
	router.GET("/stats", middleware.Auth(), func(ctx *gin.Context) {
		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		response := controller.service.GetUserStats(userID.(string))
		httpx.SendResponse(ctx, response)
	})

	// GET /:userId (Public learner card)
	router.GET("/:userId", func(ctx *gin.Context) {
		targetUserID := ctx.Param("userId")
		response := controller.service.GetPublicProfile(targetUserID)
		httpx.SendResponse(ctx, response)
	})
}
