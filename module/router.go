package module

import (
	"time"

	"nimble-voice-backend/config"
	"nimble-voice-backend/middleware"
	route_v1 "nimble-voice-backend/module/v1"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(cfg *config.Config, db *gorm.DB) *gin.Engine {
	router := gin.New()

	// Global Middlewares
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(middleware.RateLimiter())

	// CORS Policy
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// CRITICAL: Serve uploaded media statically over HTTP
	// Example accessible URL: http://localhost:8080/uploads/avatars/xxxx.png
	router.Static("/uploads", "./uploads")

	// Health Check
	router.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"status": "healthy", "time": time.Now().UTC()})
	})

	// API v1 Routing
	v1 := router.Group("/api/v1")
	{
		route_v1.RegisterV1Routes(v1, db)
	}

	return router
}
