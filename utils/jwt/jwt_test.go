package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
)

func restoreSecretKey(t *testing.T) {
	t.Helper()
	prev := append([]byte(nil), SecretKey...)
	t.Cleanup(func() {
		SecretKey = prev
	})
}

func TestGenerateAndParseToken_RejectsEmptySecretKey(t *testing.T) {
	restoreSecretKey(t)
	SecretKey = nil

	if _, err := GenerateToken("user-1", "u@example.com", "guest", "", ""); err == nil {
		t.Fatal("expected GenerateToken to fail when SecretKey is empty")
	}

	// Sign an HS256 token using an empty byte slice directly to ensure ParseToken rejects it.
	tok := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, Claims{
		UserID: "user-1",
		Role:   "guest",
		RegisteredClaims: jwtv5.RegisteredClaims{
			ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwtv5.NewNumericDate(time.Now()),
		},
	})
	signed, err := tok.SignedString([]byte{})
	if err != nil {
		t.Fatalf("failed to sign test token with empty key: %v", err)
	}

	if _, err := ParseToken(signed); err == nil {
		t.Fatal("expected ParseToken to fail when SecretKey is empty")
	}
}

func TestGenerateAndParseToken_HS256RoundTrip(t *testing.T) {
	restoreSecretKey(t)
	SetSecretKeyForTest("test-only-ephemeral-jwt-secret-key-32chars")

	tokenStr, err := GenerateToken("user-123", "alice@example.com", "user", "store-1", "branch-1")
	if err != nil {
		t.Fatalf("GenerateToken returned unexpected error: %v", err)
	}

	claims, err := ParseToken(tokenStr)
	if err != nil {
		t.Fatalf("ParseToken returned unexpected error: %v", err)
	}
	if claims.UserID != "user-123" || claims.Email != "alice@example.com" || claims.Role != "user" {
		t.Fatalf("unexpected claims parsed: %+v", claims)
	}
}

func TestParseToken_RejectsNonHS256Algorithms(t *testing.T) {
	restoreSecretKey(t)
	SecretKey = []byte("test-only-ephemeral-jwt-secret-key-32chars")

	t.Run("rejects alg=none", func(t *testing.T) {
		tok := jwtv5.NewWithClaims(jwtv5.SigningMethodNone, Claims{
			UserID: "user-1",
			Role:   "admin",
			RegisteredClaims: jwtv5.RegisteredClaims{
				ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(time.Hour)),
			},
		})
		unsigned, err := tok.SignedString(jwtv5.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatalf("failed to build alg=none token: %v", err)
		}
		if _, err := ParseToken(unsigned); err == nil {
			t.Fatal("expected ParseToken to reject alg=none token")
		}
	})

	t.Run("rejects HS384", func(t *testing.T) {
		tok := jwtv5.NewWithClaims(jwtv5.SigningMethodHS384, Claims{
			UserID: "user-1",
			Role:   "user",
			RegisteredClaims: jwtv5.RegisteredClaims{
				ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(time.Hour)),
			},
		})
		signed, err := tok.SignedString(SecretKey)
		if err != nil {
			t.Fatalf("failed to sign HS384 token: %v", err)
		}
		if _, err := ParseToken(signed); err == nil {
			t.Fatal("expected ParseToken to reject HS384 token when only HS256 is permitted")
		}
	})

	t.Run("rejects RS256", func(t *testing.T) {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("failed to generate RSA key: %v", err)
		}
		tok := jwtv5.NewWithClaims(jwtv5.SigningMethodRS256, Claims{
			UserID: "user-1",
			Role:   "user",
			RegisteredClaims: jwtv5.RegisteredClaims{
				ExpiresAt: jwtv5.NewNumericDate(time.Now().Add(time.Hour)),
			},
		})
		signed, err := tok.SignedString(priv)
		if err != nil {
			t.Fatalf("failed to sign RS256 token: %v", err)
		}
		if _, err := ParseToken(signed); err == nil || !strings.Contains(err.Error(), "signing method") && !strings.Contains(err.Error(), "invalid") {
			if err == nil {
				t.Fatal("expected ParseToken to reject RS256 token")
			}
		}
	})
}
