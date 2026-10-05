package auth

import (
	"net/http"
	"strings"

	"nimble-voice-backend/middleware"
	"nimble-voice-backend/utils/httpx"
	"nimble-voice-backend/utils/jwt"

	"github.com/gin-gonic/gin"
)

type AuthController struct {
	service AuthServiceInterface
}

func NewAuthController(service AuthServiceInterface) AuthController {
	return AuthController{service: service}
}

func AuthRouter(router *gin.RouterGroup, controller AuthController) {
	// POST /guest
	router.POST("/guest", func(ctx *gin.Context) {
		var dto GuestAuthDto
		_ = ctx.ShouldBindJSON(&dto)
		response := controller.service.CreateGuest(&dto)
		httpx.SendResponse(ctx, response)
	})

	// GET /session (supports Bearer token header or cookie)
	router.GET("/session", func(ctx *gin.Context) {
		token := ""
		authHeader := ctx.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}
		if token == "" {
			cookie, _ := ctx.Cookie("next-auth.session-token")
			if cookie == "" {
				cookie, _ = ctx.Cookie("token")
			}
			token = cookie
		}

		userID := ""
		if token != "" {
			if claims, err := jwt.ParseToken(token); err == nil && claims != nil {
				userID = claims.UserID
			}
		}

		response := controller.service.GetSession(userID)
		httpx.SendResponse(ctx, response)
	})

	// GET /profile (Protected)
	router.GET("/profile", middleware.Auth(), func(ctx *gin.Context) {
		userID, ok := ctx.Get("user_id")
		if !ok || userID == "" {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusUnauthorized, "User session not found"))
			return
		}

		response := controller.service.GetProfile(userID.(string))
		httpx.SendResponse(ctx, response)
	})

	// POST /register
	router.POST("/register", func(ctx *gin.Context) {
		var dto RegisterDto
		if err := ctx.ShouldBindJSON(&dto); err != nil {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusBadRequest, "Invalid registration payload", err.Error()))
			return
		}

		response := controller.service.Register(&dto)
		httpx.SendResponse(ctx, response)
	})

	// POST /login
	router.POST("/login", func(ctx *gin.Context) {
		var dto LoginDto
		if err := ctx.ShouldBindJSON(&dto); err != nil {
			httpx.SendResponse(ctx, httpx.SendData(http.StatusBadRequest, "Invalid login credentials", err.Error()))
			return
		}

		response := controller.service.Login(&dto)
		httpx.SendResponse(ctx, response)
	})
}
