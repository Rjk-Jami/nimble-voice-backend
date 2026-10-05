package user

import (
	"nimble-voice-backend/middleware"
	"nimble-voice-backend/utils/fileutil"
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
	// 1. User Registration
	router.POST("/register", func(ctx *gin.Context) {
		var data RegisterUserDto
		if err := ctx.ShouldBindJSON(&data); err != nil {
			ctx.JSON(400, gin.H{"status": 400, "error": err.Error()})
			return
		}
		httpx.SendResponse(ctx, controller.service.Register(&data))
	})

	// 2. User Login
	router.POST("/login", func(ctx *gin.Context) {
		var data LoginUserDto
		if err := ctx.ShouldBindJSON(&data); err != nil {
			ctx.JSON(400, gin.H{"status": 400, "error": err.Error()})
			return
		}
		httpx.SendResponse(ctx, controller.service.Login(&data))
	})

	// 3. User Profile (JWT Protected)
	router.GET("/profile", middleware.Auth(), func(ctx *gin.Context) {
		userID, _ := ctx.Get("user_id")
		httpx.SendResponse(ctx, controller.service.GetProfile(userID.(string)))
	})

	// 4. Avatar Image Upload (multipart/form-data)
	router.POST("/upload-avatar", middleware.Auth(), func(ctx *gin.Context) {
		// Read file from the 'avatar' multipart form key
		file, err := ctx.FormFile("avatar")
		if err != nil {
			ctx.JSON(400, gin.H{
				"status":  400,
				"message": "Avatar image file is required in 'avatar' field",
				"error":   err.Error(),
			})
			return
		}

		// Validate & save image into ./uploads/avatars directory
		imageURL, err := fileutil.SaveImage(file, "avatars")
		if err != nil {
			ctx.JSON(400, gin.H{
				"status":  400,
				"message": "Image upload failed",
				"error":   err.Error(),
			})
			return
		}

		// Update avatar URL in the database
		userID, _ := ctx.Get("user_id")
		res := controller.service.UpdateAvatar(userID.(string), imageURL)
		httpx.SendResponse(ctx, res)
	})
}
