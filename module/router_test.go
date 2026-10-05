package module_test

import (
	"testing"

	"nimble-voice-backend/config"
	"nimble-voice-backend/module"
	"nimble-voice-backend/server"

	"github.com/gin-gonic/gin"
)

func TestRouterInitialization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Port:        "8080",
		Environment: "test",
	}

	sockServer, err := server.NewSocketServer()
	if err != nil {
		t.Fatalf("Failed to initialize socket server: %v", err)
	}
	defer sockServer.Close()

	// SetupRouter should not panic
	router := module.SetupRouter(cfg, nil, sockServer)
	if router == nil {
		t.Fatal("Expected router to be non-nil")
	}
}
