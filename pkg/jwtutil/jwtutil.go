package jwtutil

import (
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v4"
)

// Claims holds the fields MangaHub embeds in every JWT.
type Claims struct {
	UserID   string
	Username string
}

// DefaultSecret returns the HMAC key from JWT_SECRET env var, or the dev default.
func DefaultSecret() string {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return s
	}
	return "mangahub-dev-secret"
}

// ValidateToken parses tokenStr using secret and returns the embedded claims.
func ValidateToken(tokenStr, secret string) (Claims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return Claims{}, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return Claims{}, fmt.Errorf("invalid token")
	}
	m, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, fmt.Errorf("invalid claims")
	}
	userID, _ := m["user_id"].(string)
	if userID == "" {
		return Claims{}, fmt.Errorf("missing user_id claim")
	}
	username, _ := m["username"].(string) // optional; callers may fall back to UserID
	return Claims{UserID: userID, Username: username}, nil
}
