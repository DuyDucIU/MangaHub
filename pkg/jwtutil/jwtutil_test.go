package jwtutil_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"mangahub/pkg/jwtutil"
)

const testSecret = "test-secret"

func signedToken(userID, username, secret string, ttl time.Duration) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(ttl).Unix(),
	})
	s, _ := tok.SignedString([]byte(secret))
	return s
}

func TestValidateToken_Valid(t *testing.T) {
	tok := signedToken("usr_abc", "alice", testSecret, time.Hour)
	claims, err := jwtutil.ValidateToken(tok, testSecret)
	assert.NoError(t, err)
	assert.Equal(t, "usr_abc", claims.UserID)
	assert.Equal(t, "alice", claims.Username)
}

func TestValidateToken_WrongSecret(t *testing.T) {
	tok := signedToken("usr_abc", "alice", "wrong-secret", time.Hour)
	_, err := jwtutil.ValidateToken(tok, testSecret)
	assert.Error(t, err)
}

func TestValidateToken_Expired(t *testing.T) {
	tok := signedToken("usr_abc", "alice", testSecret, -time.Hour)
	_, err := jwtutil.ValidateToken(tok, testSecret)
	assert.Error(t, err)
}

func TestValidateToken_Malformed(t *testing.T) {
	_, err := jwtutil.ValidateToken("not.a.jwt", testSecret)
	assert.Error(t, err)
}

func TestValidateToken_MissingUserID(t *testing.T) {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, _ := tok.SignedString([]byte(testSecret))
	_, err := jwtutil.ValidateToken(s, testSecret)
	assert.Error(t, err)
}

func TestDefaultSecret_FallsBackToDevDefault(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	assert.Equal(t, "mangahub-dev-secret", jwtutil.DefaultSecret())
}

func TestDefaultSecret_ReadsEnvVar(t *testing.T) {
	t.Setenv("JWT_SECRET", "my-prod-secret")
	assert.Equal(t, "my-prod-secret", jwtutil.DefaultSecret())
}
