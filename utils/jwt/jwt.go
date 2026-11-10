package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SecretKey must be configured at startup from validated config.JWTSecret.
var SecretKey []byte

type Claims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	StoreID  string `json:"store_id"`
	BranchID string `json:"branch_id"`
	jwt.RegisteredClaims
}

// SetSecretKeyForTest sets SecretKey to an explicit ephemeral test secret.
func SetSecretKeyForTest(secret string) {
	SecretKey = []byte(secret)
}

func GenerateToken(userID, email, role, storeID, branchID string) (string, error) {
	if len(SecretKey) == 0 {
		return "", errors.New("jwt signing key is not configured")
	}

	claims := Claims{
		UserID:   userID,
		Email:    email,
		Role:     role,
		StoreID:  storeID,
		BranchID: branchID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * 7 * time.Hour)), // 7 days expiration
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(SecretKey)
}

func ParseToken(tokenStr string) (*Claims, error) {
	if len(SecretKey) == 0 {
		return nil, errors.New("jwt signing key is not configured")
	}

	t, err := jwt.ParseWithClaims(
		tokenStr,
		&Claims{},
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return SecretKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		return nil, err
	}
	if claims, ok := t.Claims.(*Claims); ok && t.Valid {
		return claims, nil
	}
	return nil, errors.New("invalid or expired token")
}
