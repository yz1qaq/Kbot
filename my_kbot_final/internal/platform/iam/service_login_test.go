package iam

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"my_kbot_final/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type loginStoreStub struct {
	Store
	lookup func(context.Context, string) (*domain.User, error)
}

func (s *loginStoreStub) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.lookup(ctx, email)
}

func assertLoginToken(t *testing.T, result *LoginResponse, key []byte, user *domain.User, started time.Time) {
	t.Helper()
	if result == nil || result.User == nil || result.User.ID != user.ID || result.Token == "" {
		t.Fatal("login must return the user and a nonempty token")
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(result.Token, claims, func(token *jwt.Token) (any, error) {
		return key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("kbot"), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		t.Fatalf("login token must have a valid signature and registered claims: %v", err)
	}
	if claims.UserID != user.ID || claims.Subject != user.ID || claims.Email != user.Email || claims.IssuedAt == nil || claims.NotBefore == nil || claims.ExpiresAt == nil {
		t.Fatal("login token is missing the expected identity or timestamps")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode login response: %v", err)
	}
	var response struct {
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if response.ExpiresAt.Before(started.Add(24*time.Hour)) || response.ExpiresAt.After(time.Now().Add(24*time.Hour)) {
		t.Fatal("login token must expire approximately 24 hours after signing")
	}
	if d := response.ExpiresAt.Sub(claims.ExpiresAt.Time); d < 0 || d >= time.Second {
		t.Fatal("response expiry and token expiry must agree within JWT timestamp precision")
	}
	if _, err := jwt.Parse(result.Token, func(*jwt.Token) (any, error) {
		return []byte("a-different-test-signing-key"), nil
	}, jwt.WithValidMethods([]string{"HS256"})); err == nil {
		t.Fatal("the token must not verify with a different signing key")
	}
	if strings.Contains(string(encoded), user.PasswordHash) || strings.Contains(string(encoded), "password123") {
		t.Fatal("login JSON must not expose password material")
	}
}

func TestServiceLoginSignsToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{ID: "user-1", Email: "alice@example.com", PasswordHash: string(hash), Status: "active"}
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "login-context")
	store := &loginStoreStub{lookup: func(gotCtx context.Context, email string) (*domain.User, error) {
		if gotCtx.Value(ctxKey{}) != "login-context" || email != "alice@example.com" {
			t.Fatal("login must normalize the email and forward the context")
		}
		return user, nil
	}}
	key := []byte("learning-test-key-with-at-least-32-bytes")
	started := time.Now()
	result, err := NewService(store, key).Login(ctx, LoginRequest{Email: " ALICE@Example.COM ", Password: "password123"})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	assertLoginToken(t, result, key, user, started)
}

func TestServiceLoginRejectsInvalidCredentials(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, status, password, passwordHash string
		lookupErr                            error
	}{
		{name: "missing user", lookupErr: errors.New("user not found")},
		{name: "cancelled request", lookupErr: context.Canceled},
		{name: "wrong password", status: "active", password: "wrong-password", passwordHash: string(hash)},
		{name: "inactive account", status: "inactive", password: "password123", passwordHash: string(hash)},
		{name: "invalid stored hash", status: "active", password: "password123", passwordHash: "not-a-bcrypt-hash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &loginStoreStub{lookup: func(context.Context, string) (*domain.User, error) {
				if tc.lookupErr != nil {
					return nil, tc.lookupErr
				}
				return &domain.User{ID: "user-1", Status: tc.status, PasswordHash: tc.passwordHash}, nil
			}}
			result, err := NewService(store, []byte("test-key")).Login(context.Background(), LoginRequest{Email: "alice@example.com", Password: tc.password})
			if result != nil || err == nil {
				t.Fatal("invalid credentials must not produce a login response")
			}
			if tc.lookupErr != nil && !errors.Is(err, tc.lookupErr) {
				t.Fatal("lookup errors should retain their cause")
			}
		})
	}
}

func TestServiceLoginPostgres(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	key := []byte("learning-test-key-with-at-least-32-bytes")
	service := NewService(store, key)
	user, err := service.CreateUser(ctx, "alice@example.com", "password123", "小明")
	if err != nil {
		t.Fatalf("create login fixture: %v", err)
	}
	started := time.Now()
	result, err := service.Login(ctx, LoginRequest{Email: " ALICE@EXAMPLE.COM ", Password: "password123"})
	if err != nil {
		t.Fatalf("login using PostgreSQL: %v", err)
	}
	assertLoginToken(t, result, key, user, started)
}
