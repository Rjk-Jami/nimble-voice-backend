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
	"nimble-voice-backend/server"
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

	// 5. Initialize Pure Go Socket.IO v4 Server (gsocketio)
	sockServer, err := server.NewSocketServer()
	if err != nil {
		log.Fatalf("Socket server init error: %v", err)
	}
	defer sockServer.Close()

	go func() {
		if err := sockServer.Serve(); err != nil {
			log.Printf("Socket.IO server loop ended: %v", err)
		}
	}()

	// 6. Setup Router with CORS and Socket.IO handler mounted
	router := module.SetupRouter(cfg, database, sockServer)

	// 7. Initialize HTTP Server
	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	go func() {
		fmt.Printf("🚀 Server running on port :%s (Mode: %s)\n", cfg.Port, cfg.Environment)
		fmt.Println("⚡ Real-Time Socket.IO v4 mounted on /socket.io/")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server Listen error: %v", err)
		}
	}()

	// 8. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server gracefully stopped")
}
