package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalError_SanitizesErrorDetails(t *testing.T) {
	rawErr := errors.New("pq: password authentication failed for user postgres on 10.0.0.5:5432")
	res := InternalError("room.create", rawErr, "Failed to create voice room")

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode = %d, want 500", res.StatusCode)
	}
	if res.Message != "Failed to create voice room" {
		t.Fatalf("Message = %q, want %q", res.Message, "Failed to create voice room")
	}
	if res.Data != nil {
		t.Fatalf("Data = %v, want nil", res.Data)
	}

	rawJSON, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("json.Marshal(res) failed: %v", err)
	}
	for _, sensitive := range []string{"pq:", "postgres", "10.0.0.5", "5432", "password"} {
		if strings.Contains(string(rawJSON), sensitive) {
			t.Fatalf("serialized APIResponse leaked sensitive substring %q: %s", sensitive, string(rawJSON))
		}
	}

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	SendResponse(ctx, res)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("HTTP status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	for _, sensitive := range []string{"pq:", "postgres", "10.0.0.5", "5432", "password"} {
		if strings.Contains(body, sensitive) {
			t.Fatalf("HTTP JSON response leaked sensitive substring %q: %s", sensitive, body)
		}
	}
}
