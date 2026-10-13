package module

import (
	"net/http"
	"time"

	"nimble-voice-backend/config"
	_ "nimble-voice-backend/docs"
	"nimble-voice-backend/middleware"
	route_v1 "nimble-voice-backend/module/v1"
	"nimble-voice-backend/server"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)


func SetupRouter(cfg *config.Config, db *gorm.DB, socketServer ...*server.SocketServer) *gin.Engine {
	router := gin.New()

	// Global Middlewares
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(middleware.RateLimiter())

	// CORS Policy: allow incoming Next.js frontend origins with credentials and standard headers
	router.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Serve uploaded media statically over HTTP
	// Example accessible URL: http://localhost:8080/uploads/avatars/xxxx.png
	router.Static("/uploads", "./uploads")

	// Health Check
	router.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"status": "healthy", "time": time.Now().UTC()})
	})

	// Swagger API Documentation UI
	router.GET("/swagger", func(ctx *gin.Context) {
		ctx.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})
	router.GET("/swagger/*any", ginSwagger.CustomWrapHandler(&ginSwagger.Config{
		URL:                      "/swagger/doc.json",
		DocExpansion:             "none", // Keeps module tags collapsed initially
		DefaultModelsExpandDepth: -1,     // Hides default model schemas at bottom
		DeepLinking:              true,   // Deep link to specific endpoints
		PersistAuthorization:     true,   // Retains Bearer token across refreshes
		Title:                    "Nimble Voice API Docs",
	}, swaggerFiles.Handler))


	// Mount Socket.IO v4 handler if provided
	if len(socketServer) > 0 && socketServer[0] != nil {
		sockHandler := socketServer[0].GinHandler()
		router.GET("/socket.io/*any", sockHandler)
		router.POST("/socket.io/*any", sockHandler)
		router.OPTIONS("/socket.io/*any", sockHandler)
		router.GET("/socket.io", sockHandler)
		router.POST("/socket.io", sockHandler)
		router.OPTIONS("/socket.io", sockHandler)
	}

	// Mount API routes under /api/v1 and /api for seamless frontend compatibility
	v1 := router.Group("/api/v1")
	{
		route_v1.RegisterV1Routes(v1, db, socketServer...)
	}

	api := router.Group("/api")
	{
		route_v1.RegisterV1Routes(api, db, socketServer...)
	}

	return router
}
