package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nimble-voice-backend/config"
	"nimble-voice-backend/db"
	"nimble-voice-backend/module"
	"nimble-voice-backend/pkg/logger"
	"nimble-voice-backend/utils/jwt"
)

func main() {
	// 1. Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Config load error: %v", err)
	}

	// 2. Initialize Zap Logger
	appLogger, err := logger.InitLogger(cfg.Environment)
	if err != nil {
		log.Fatalf("Logger init error: %v", err)
	}
	defer appLogger.Sync()

	// 3. Initialize JWT Secret
	jwt.SecretKey = []byte(cfg.JWTSecret)

	// 4. Connect to database
	database, err := db.ConnectPostgres(cfg)
	if err != nil {
		log.Fatalf("Postgres connection error: %v", err)
	}

	// 5. Setup Router
	router := module.SetupRouter(cfg, database)

	// 6. Initialize HTTP Server
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		fmt.Printf("🚀 Server running on port :%s (Mode: %s)\n", cfg.Port, cfg.Environment)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server Listen error: %v", err)
		}
	}()

	// 7. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server gracefully stopped")
}
