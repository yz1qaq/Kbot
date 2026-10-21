package iam

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"my_kbot_final/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// 嵌入接口补齐未使用的方法；本轮只允许调用 CreateUser。
type createUserStoreStub struct {
	Store
	create func(context.Context, *domain.User) error
}

func (s *createUserStoreStub) CreateUser(ctx context.Context, user *domain.User) error {
	return s.create(ctx, user)
}

func TestServiceCreateUserRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name, email, password, displayName string
		want                               error
	}{
		{"invalid email", "not-an-email", "password123", "小明", ErrInvalidEmail},
		{"display address", "Alice <alice@example.com>", "password123", "小明", ErrInvalidEmail},
		{"short password", "alice@example.com", "1234567", "小明", ErrWeakPassword},
		{"blank name", "alice@example.com", "password123", " \t\n", ErrNameRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &createUserStoreStub{create: func(context.Context, *domain.User) error {
				t.Fatal("invalid input must not reach storage")
				return nil
			}}
			user, err := NewService(store, nil).CreateUser(context.Background(), tc.email, tc.password, tc.displayName)
			if user != nil || !errors.Is(err, tc.want) {
				t.Fatalf("expected nil user and %v, got userPresent=%t err=%v", tc.want, user != nil, err)
			}
		})
	}
}

func TestServiceCreateUserNormalizesAndHashes(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-context")
	var saved *domain.User
	store := &createUserStoreStub{create: func(gotCtx context.Context, user *domain.User) error {
		if gotCtx.Value(contextKey{}) != "request-context" {
			t.Fatal("request context was not forwarded")
		}
		saved = user
		return nil
	}}
	service := NewService(store, nil)
	first, err := service.CreateUser(ctx, "  ALICE@Example.COM \t", "password123", " 小明 ")
	if err != nil || first == nil {
		t.Fatalf("create user: %v", err)
	}
	if first != saved || first.Email != "alice@example.com" || first.Name != "小明" || first.Role != GlobalRoleMember || first.Status != "active" {
		t.Fatal("created user has incorrect normalized fields, defaults, or storage result")
	}
	if _, err := uuid.Parse(first.ID); err != nil || first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatal("created user needs a valid ID and timestamps")
	}
	if first.PasswordHash == "password123" || bcrypt.CompareHashAndPassword([]byte(first.PasswordHash), []byte("password123")) != nil {
		t.Fatal("stored password must be a verifiable bcrypt hash")
	}
	if bcrypt.CompareHashAndPassword([]byte(first.PasswordHash), []byte("wrong-password")) == nil {
		t.Fatal("wrong password must not verify")
	}
	cost, err := bcrypt.Cost([]byte(first.PasswordHash))
	if err != nil || cost != bcrypt.DefaultCost {
		t.Fatal("unexpected bcrypt work factor")
	}
	second, err := service.CreateUser(ctx, "bob@example.com", "password123", "小红")
	if err != nil || second == nil {
		t.Fatalf("create second user: %v", err)
	}
	if first.ID == second.ID || first.PasswordHash == second.PasswordHash {
		t.Fatal("different users need distinct IDs and independently salted password hashes")
	}
	encoded, err := json.Marshal(first)
	if err != nil || strings.Contains(string(encoded), "password123") || strings.Contains(string(encoded), first.PasswordHash) {
		t.Fatal("user JSON must not expose the password or hash")
	}
}

func TestServiceCreateUserPreservesErrors(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	calls := 0
	store := &createUserStoreStub{create: func(context.Context, *domain.User) error {
		calls++
		return storageErr
	}}
	service := NewService(store, nil)
	user, err := service.CreateUser(context.Background(), "alice@example.com", "password123", "小明")
	if user != nil || !errors.Is(err, storageErr) || calls != 1 {
		t.Fatal("storage failure should return no user and retain the original error")
	}
	user, err = service.CreateUser(context.Background(), "alice@example.com", strings.Repeat("a", 73), "小明")
	if user != nil || !errors.Is(err, bcrypt.ErrPasswordTooLong) || calls != 1 {
		t.Fatal("bcrypt length failure must be preserved and must not reach storage")
	}
}

func TestServiceCreateUserPostgres(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	service := NewService(store, nil)
	user, err := service.CreateUser(ctx, "  ALICE@Example.COM ", "password123", " 小明 ")
	if err != nil || user == nil {
		t.Fatalf("create user in PostgreSQL: %v", err)
	}
	got, err := store.GetUserByEmail(ctx, "alice@example.com")
	if err != nil || got == nil {
		t.Fatalf("read user from PostgreSQL: %v", err)
	}
	if got.ID != user.ID || got.Name != "小明" || got.Role != GlobalRoleMember || got.Status != "active" || !got.CreatedAt.Equal(user.CreatedAt) || !got.UpdatedAt.Equal(user.UpdatedAt) {
		t.Fatal("database user differs from the created user")
	}
	if bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte("password123")) != nil {
		t.Fatal("database password hash does not verify")
	}
	if got.CreatedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("database creation timestamp is unexpectedly old")
	}
	duplicate, err := service.CreateUser(ctx, "ALICE@example.com", "password456", "另一个人")
	var pgErr *pgconn.PgError
	if duplicate != nil || !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatal("normalized duplicate email must retain the PostgreSQL unique violation")
	}
}
