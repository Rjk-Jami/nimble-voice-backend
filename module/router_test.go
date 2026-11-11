package module_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestRouterCORSAndHardening(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uploadDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(uploadDir, "sample.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("failed to create temp upload file: %v", err)
	}

	cfg := &config.Config{
		Port:               "8080",
		Environment:        "production",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		FrontendOrigins:    []string{"http://localhost:3000"},
		TrustedProxies:     nil,
		SwaggerEnabled:     false,
		UploadDir:          uploadDir,
	}

	router := module.SetupRouter(cfg, nil, nil)
	router.GET("/panic-test", func(ctx *gin.Context) {
		panic("sensitive-internal-panic-detail")
	})

	t.Run("allowlisted origin receives ACAO and credentials on OPTIONS and GET", func(t *testing.T) {
		for _, method := range []string{http.MethodOptions, http.MethodGet} {
			req := httptest.NewRequest(method, "/health", nil)
			req.Header.Set("Origin", "http://localhost:3000")
			if method == http.MethodOptions {
				req.Header.Set("Access-Control-Request-Method", "GET")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
				t.Fatalf("%s Access-Control-Allow-Origin = %q, want %q", method, got, "http://localhost:3000")
			}
			if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
				t.Fatalf("%s Access-Control-Allow-Credentials = %q, want %q", method, got, "true")
			}
		}
	})

	t.Run("unlisted origin receives no ACAO header and never wildcard", func(t *testing.T) {
		for _, method := range []string{http.MethodOptions, http.MethodGet} {
			req := httptest.NewRequest(method, "/health", nil)
			req.Header.Set("Origin", "https://evil.example")
			if method == http.MethodOptions {
				req.Header.Set("Access-Control-Request-Method", "GET")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("%s Access-Control-Allow-Origin = %q, want empty (never reflected or *)", method, got)
			}
		}
	})

	t.Run("panic recovery returns sanitized 500 JSON without leaking panic details", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/panic-test", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", w.Code)
		}
		body := w.Body.String()
		if strings.Contains(body, "sensitive-internal-panic-detail") {
			t.Fatalf("response body leaked panic string: %s", body)
		}

		var payload map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("expected sanitized JSON body, got %q (err: %v)", body, err)
		}
		if payload["status"] != float64(500) {
			t.Fatalf("payload.status = %v, want 500", payload["status"])
		}
		if payload["message"] != "Internal server error" {
			t.Fatalf("payload.message = %v, want %q", payload["message"], "Internal server error")
		}
	})

	t.Run("uploads route sets X-Content-Type-Options nosniff and serves UploadDir", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/uploads/sample.txt", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
		}
	})

	t.Run("swagger returns 404 when SwaggerEnabled is false and mounts when true", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("SwaggerEnabled=false status = %d, want 404", w.Code)
		}

		reqRoot := httptest.NewRequest(http.MethodGet, "/swagger", nil)
		wRoot := httptest.NewRecorder()
		router.ServeHTTP(wRoot, reqRoot)
		if wRoot.Code != http.StatusNotFound {
			t.Fatalf("SwaggerEnabled=false /swagger status = %d, want 404", wRoot.Code)
		}

		enabledCfg := &config.Config{
			Port:               "8080",
			Environment:        "development",
			CORSAllowedOrigins: []string{"http://localhost:3000"},
			SwaggerEnabled:     true,
		}
		enabledRouter := module.SetupRouter(enabledCfg, nil, nil)
		reqEnabled := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
		wEnabled := httptest.NewRecorder()
		enabledRouter.ServeHTTP(wEnabled, reqEnabled)
		if wEnabled.Code == http.StatusNotFound {
			t.Fatalf("SwaggerEnabled=true status = %d, want non-404", wEnabled.Code)
		}
	})
}
