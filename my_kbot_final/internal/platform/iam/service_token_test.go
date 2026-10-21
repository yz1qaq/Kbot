package iam

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestServiceValidateToken(t *testing.T) {
	key := []byte("learning-test-key-with-at-least-32-bytes")
	service := NewService(nil, key)
	newClaims := func() *Claims {
		return &Claims{
			UserID: "user-1", Email: "alice@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "kbot", Subject: "user-1",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
				NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			},
		}
	}
	sign := func(method jwt.SigningMethod, claims *Claims, signingKey []byte) string {
		t.Helper()
		raw, err := jwt.NewWithClaims(method, claims).SignedString(signingKey)
		if err != nil {
			t.Fatalf("sign fixture token: %v", err)
		}
		return raw
	}
	valid := sign(jwt.SigningMethodHS256, newClaims(), key)
	t.Run("valid", func(t *testing.T) {
		claims, err := service.ValidateToken(valid)
		if err != nil || claims == nil {
			t.Fatalf("valid token was rejected: %v", err)
		}
		if claims.UserID != "user-1" || claims.Email != "alice@example.com" || claims.Subject != "user-1" || claims.Issuer != "kbot" {
			t.Fatal("validated claims lost user identity or registered fields")
		}
	})
	expired := newClaims()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	future := newClaims()
	future.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour))
	wrongIssuer := newClaims()
	wrongIssuer.Issuer = "another-service"
	parts := strings.Split(valid, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	payload = []byte(strings.Replace(string(payload), "user-1", "user-2", 1))
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	tests := []struct{ name, token string }{
		{"empty", ""},
		{"malformed", "not-a-jwt"},
		{"wrong signing key", sign(jwt.SigningMethodHS256, newClaims(), []byte("wrong-key"))},
		{"tampered identity", strings.Join(parts, ".")},
		{"wrong algorithm", sign(jwt.SigningMethodHS384, newClaims(), key)},
		{"expired", sign(jwt.SigningMethodHS256, expired, key)},
		{"not yet valid", sign(jwt.SigningMethodHS256, future, key)},
		{"wrong issuer", sign(jwt.SigningMethodHS256, wrongIssuer, key)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := service.ValidateToken(tc.token)
			if err == nil || claims != nil {
				t.Fatal("invalid token must return an error and no claims")
			}
		})
	}
}

func TestServiceValidateTokenPostgresLogin(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	service := NewService(store, []byte("learning-test-key-with-at-least-32-bytes"))
	user, err := service.CreateUser(ctx, "alice@example.com", "password123", "小明")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	result, err := service.Login(ctx, LoginRequest{Email: "alice@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	claims, err := service.ValidateToken(result.Token)
	if err != nil || claims == nil {
		t.Fatalf("validate token returned by database-backed login: %v", err)
	}
	if claims.UserID != user.ID || claims.Subject != user.ID || claims.Email != user.Email {
		t.Fatal("validated token must refer to the database user who logged in")
	}
}
