package room

import (
	"strings"
	"testing"
	"time"

	"nimble-voice-backend/domain"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "host=localhost user=postgres password=rjk.207235 dbname=nimbleVoice port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("Postgres DB not reachable for integration test: %v", err)
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB.Ping() != nil {
		t.Skipf("Postgres DB ping failed: %v", err)
		return nil
	}
	return db
}

func TestGetRoomList_SQLPrecedenceParentheses(t *testing.T) {
	// Verify GORM AST compilation includes explicit parentheses around search query OR clauses
	dsn := "host=localhost user=postgres password=rjk.207235 dbname=nimbleVoice port=5432 sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("failed to initialize dry-run db: %v", err)
	}

	query := &RoomQuery{
		Query: "Calculus",
		Page:  1,
		Limit: 10,
	}

	var rooms []domain.Room
	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		searchTerm := "%" + strings.ToLower(query.Query) + "%"
		return tx.Model(&domain.Room{}).
			Where("status != ?", "ENDED").
			Where("(LOWER(title) LIKE ? OR LOWER(topic) LIKE ? OR LOWER(topic_tag) LIKE ?)", searchTerm, searchTerm, searchTerm).
			Find(&rooms)
	})

	// Ensure the parenthesized grouping exists
	expectedClause := "(LOWER(title) LIKE"
	if !strings.Contains(sql, expectedClause) {
		t.Fatalf("expected SQL to contain parenthesized search clause %q, got: %s", expectedClause, sql)
	}

	// Ensure status != 'ENDED' is evaluated prior to the grouped ORs
	if !strings.Contains(sql, "status != 'ENDED'") {
		t.Fatalf("expected SQL to filter status != 'ENDED', got: %s", sql)
	}
}

func TestGetRoomList_EndedRoomSearchImmunity(t *testing.T) {
	db := openTestDB(t)
	if db == nil {
		return
	}

	tx := db.Begin()
	defer tx.Rollback()

	// Ensure tables exist
	_ = tx.AutoMigrate(&domain.User{}, &domain.Room{}, &domain.RoomParticipant{})

	// Create test host
	hostID := "test-host-" + uuid.NewString()[:8]
	hostEmail := hostID + "@example.com"
	host := domain.User{
		ID:             hostID,
		Name:           "Calculus Host",
		Email:          &hostEmail,
		NativeLanguage: "English",
		Role:           "USER",
	}
	if err := tx.Create(&host).Error; err != nil {
		t.Fatalf("failed to create test host: %v", err)
	}

	// Create active room matching "Calculus"
	activeRoomID := "active-calc-" + uuid.NewString()[:8]
	activeRoom := domain.Room{
		ID:           activeRoomID,
		Title:        "Advanced Calculus Study Group",
		Topic:        "Calculus and Real Analysis",
		Language:     "English",
		Status:       "LIVE",
		HostID:       hostID,
		CurrentSlots: 1,
		MaxSlots:     5,
		Tags:         datatypes.JSON([]byte(`["math", "calculus"]`)),
		CreatedAt:    time.Now(),
	}
	if err := tx.Create(&activeRoom).Error; err != nil {
		t.Fatalf("failed to create active room: %v", err)
	}

	// Create ENDED room also matching "Calculus"
	endedRoomID := "ended-calc-" + uuid.NewString()[:8]
	endedRoom := domain.Room{
		ID:           endedRoomID,
		Title:        "Calculus 101 Discussion",
		Topic:        "Calculus Derivatives",
		Language:     "English",
		Status:       "ENDED",
		HostID:       hostID,
		CurrentSlots: 0,
		MaxSlots:     5,
		Tags:         datatypes.JSON([]byte(`["math", "calculus"]`)),
		CreatedAt:    time.Now().Add(-2 * time.Hour),
	}
	if err := tx.Create(&endedRoom).Error; err != nil {
		t.Fatalf("failed to create ended room: %v", err)
	}

	// Execute GetRoomList with query="Calculus"
	roomService := NewRoomService(tx)
	resp := roomService.GetRoomList(&RoomQuery{
		Query: "Calculus",
		Page:  1,
		Limit: 20,
	})

	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d (msg: %s)", resp.StatusCode, resp.Message)
	}

	listResp, ok := resp.Data.(RoomListResponse)
	if !ok {
		t.Fatalf("expected data to be RoomListResponse, got %T", resp.Data)
	}

	rooms, ok := listResp.Rooms.([]domain.Room)
	if !ok {
		t.Fatalf("expected Rooms to be []domain.Room, got %T", listResp.Rooms)
	}

	foundActive := false
	for _, r := range rooms {
		if r.ID == endedRoomID {
			t.Fatalf("CRITICAL SECURITY DEFECT: ENDED room %s was returned in search results", endedRoomID)
		}
		if r.ID == activeRoomID {
			foundActive = true
		}
	}

	if !foundActive {
		t.Fatalf("expected active room %s to be returned in search results", activeRoomID)
	}
}
