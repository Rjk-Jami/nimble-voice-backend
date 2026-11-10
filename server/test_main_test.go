package server

import (
	"os"
	"testing"

	"nimble-voice-backend/utils/jwt"
)

func TestMain(m *testing.M) {
	jwt.SetSecretKeyForTest("test-only-ephemeral-jwt-secret-key-32chars")
	os.Exit(m.Run())
}
