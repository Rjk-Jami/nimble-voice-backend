package stats

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"nimble-voice-backend/domain"
	"nimble-voice-backend/server"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "host=localhost user=postgres password=rjk.207235 dbname=nimbleVoice port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("Postgres DB not reachable for stats test: %v", err)
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		t.Skipf("Postgres DB ping failed: %v", err)
		return nil
	}
	return db
}

func TestStatsService_GetNetworkStats_AuthoritativePresence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openTestDB(t)
	if db == nil {
		return
	}

	tx := db.Begin()
	defer tx.Rollback()
	_ = tx.AutoMigrate(&domain.Room{})

	sockServer, err := server.NewSocketServerWithOrigins([]string{"http://localhost:3000"})
	if err != nil {
		t.Fatalf("failed to initialize socket server: %v", err)
	}
	defer sockServer.Close()

	// Initial count must be exactly 0
	if sockServer.OnlineUserCount() != 0 {
		t.Fatalf("initial online user count = %d, want 0", sockServer.OnlineUserCount())
	}

	svc := NewStatsService(tx, sockServer)

	// Case 1: Zero users connected
	resp := svc.GetNetworkStats()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	data, ok := resp.Data.(NetworkStatsResponse)
	if !ok {
		t.Fatalf("expected data to be NetworkStatsResponse, got %T", resp.Data)
	}
	if data.OnlineCount != 0 {
		t.Fatalf("CRITICAL: expected authoritative onlineCount 0 without dummy offsets, got %d", data.OnlineCount)
	}

	// Case 2: Multi-tab user connects 3 tabs -> unique count should be 1
	store := sockServer.PresenceStore()
	store.AddConnection("learner-1", "conn-1-tab-a")
	store.AddConnection("learner-1", "conn-1-tab-b")
	store.AddConnection("learner-1", "conn-1-tab-c")

	resp = svc.GetNetworkStats()
	data = resp.Data.(NetworkStatsResponse)
	if data.OnlineCount != 1 {
		t.Fatalf("expected onlineCount 1 for 1 multi-tab user, got %d", data.OnlineCount)
	}

	// Case 3: Second user connects
	store.AddConnection("learner-2", "conn-2-tab-a")
	resp = svc.GetNetworkStats()
	data = resp.Data.(NetworkStatsResponse)
	if data.OnlineCount != 2 {
		t.Fatalf("expected onlineCount 2 for 2 unique users, got %d", data.OnlineCount)
	}

	// Case 4: Verify via Gin HTTP router (/api/v1/stats/network)
	engine := gin.New()
	v1 := engine.Group("/api/v1")
	StatsModule(v1.Group("/stats"), tx, sockServer)

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/stats/network", nil)
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP GET /api/v1/stats/network returned %d, want 200", rec.Code)
	}

	var httpResp struct {
		Status  int                  `json:"status"`
		Message string               `json:"message"`
		Data    NetworkStatsResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &httpResp); err != nil {
		t.Fatalf("failed to parse HTTP response: %v", err)
	}

	if httpResp.Data.OnlineCount != 2 {
		t.Fatalf("HTTP response onlineCount = %d, want 2", httpResp.Data.OnlineCount)
	}

	// Case 5: Disconnect user-2 -> drops to 1
	store.RemoveConnection("learner-2", "conn-2-tab-a")
	rec2 := httptest.NewRecorder()
	engine.ServeHTTP(rec2, req)
	_ = json.Unmarshal(rec2.Body.Bytes(), &httpResp)
	if httpResp.Data.OnlineCount != 1 {
		t.Fatalf("HTTP response onlineCount after disconnect = %d, want 1", httpResp.Data.OnlineCount)
	}
}
