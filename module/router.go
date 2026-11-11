package module

import (
	"net/http"
	"strings"
	"time"

	"nimble-voice-backend/config"
	_ "nimble-voice-backend/docs"
	"nimble-voice-backend/middleware"
	route_v1 "nimble-voice-backend/module/v1"
	"nimble-voice-backend/pkg/logger"
	"nimble-voice-backend/server"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func SetupRouter(cfg *config.Config, db *gorm.DB, socketServer *server.SocketServer, presenceServers ...*server.PresenceServer) *gin.Engine {
	router := gin.New()

	var trustedProxies []string
	if cfg != nil {
		trustedProxies = cfg.TrustedProxies
	}
	if err := router.SetTrustedProxies(trustedProxies); err != nil {
		log := logger.Log
		if log == nil {
			log = zap.L()
		}
		log.Warn("Failed to configure trusted proxies", zap.Error(err))
	}

	// Global Middlewares
	if logger.Log != nil {
		router.Use(middleware.ZapLogger(logger.Log))
	}
	router.Use(gin.CustomRecovery(func(ctx *gin.Context, recovered interface{}) {
		log := logger.Log
		if log == nil {
			log = zap.L()
		}
		log.Error("HTTP panic recovered",
			zap.Any("error", recovered),
			zap.String("method", ctx.Request.Method),
			zap.String("path", ctx.Request.URL.Path),
		)
		ctx.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"status":  http.StatusInternalServerError,
			"message": "Internal server error",
		})
	}))
	router.Use(middleware.RateLimiter())

	// CORS Policy: allow only configured frontend origins with credentials and standard headers
	router.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			return cfg != nil && cfg.IsOriginAllowed(origin)
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Serve uploaded media statically over HTTP with MIME-sniffing disabled
	uploadDir := "./uploads"
	if cfg != nil && strings.TrimSpace(cfg.UploadDir) != "" {
		uploadDir = strings.TrimSpace(cfg.UploadDir)
	}
	uploads := router.Group("/uploads")
	uploads.Use(func(ctx *gin.Context) {
		ctx.Header("X-Content-Type-Options", "nosniff")
		ctx.Next()
	})
	uploads.Static("/", uploadDir)

	// Health Check
	router.GET("/health", func(ctx *gin.Context) {
		ctx.JSON(200, gin.H{"status": "healthy", "time": time.Now().UTC()})
	})

	// Native WebSocket presence is separate from Socket.IO voice signaling.
	if len(presenceServers) > 0 && presenceServers[0] != nil {
		presenceServer := presenceServers[0]
		router.GET("/ws/presence", func(ctx *gin.Context) {
			presenceServer.ServeHTTP(ctx.Writer, ctx.Request)
		})
	}

	// Swagger API Documentation UI (explicitly gated by config)
	if cfg != nil && cfg.SwaggerEnabled {
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
	}

	// Mount Socket.IO v4 handler if provided
	if socketServer != nil {
		sockHandler := socketServer.GinHandler()
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
		route_v1.RegisterV1Routes(v1, cfg, db, socketServer)
	}

	api := router.Group("/api")
	{
		route_v1.RegisterV1Routes(api, cfg, db, socketServer)
	}

	return router
}
