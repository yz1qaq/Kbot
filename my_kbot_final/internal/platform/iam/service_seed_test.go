package iam

import (
	"context"
	"errors"
	"testing"

	"my_kbot_final/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

type seedAdminStoreStub struct {
	Store
	get     func(context.Context, string) (*domain.User, error)
	create  func(context.Context, *domain.User) error
	setRole func(context.Context, string, string) error
}

func (s *seedAdminStoreStub) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.get(ctx, email)
}

func (s *seedAdminStoreStub) CreateUser(ctx context.Context, user *domain.User) error {
	return s.create(ctx, user)
}

func (s *seedAdminStoreStub) SetUserRole(ctx context.Context, id, role string) error {
	return s.setRole(ctx, id, role)
}

func TestServiceEnsureSeedAdminPostgres(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	service := NewService(store, []byte("learning-test-key-with-at-least-32-bytes"))
	if err := service.EnsureSeedAdmin(ctx, " ADMIN@Example.COM ", "password123"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	first, err := store.GetUserByEmail(ctx, "admin@example.com")
	if err != nil || first == nil {
		t.Fatalf("read seed admin: %v", err)
	}
	if first.Role != GlobalRoleAdmin || first.Status != "active" || first.Name != "Admin" {
		t.Fatal("new seed user must be an active global admin")
	}
	if bcrypt.CompareHashAndPassword([]byte(first.PasswordHash), []byte("password123")) != nil {
		t.Fatal("seed admin password must be stored as a valid bcrypt hash")
	}
	if err := service.EnsureSeedAdmin(ctx, "admin@example.com", "changed-password456"); err != nil {
		t.Fatalf("repeat bootstrap: %v", err)
	}
	second, err := store.GetUserByEmail(ctx, "admin@example.com")
	if err != nil || second == nil {
		t.Fatalf("read repeated seed admin: %v", err)
	}
	if second.ID != first.ID || second.PasswordHash != first.PasswordHash || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("repeat bootstrap must retain the existing account and password")
	}
	users, err := store.ListUsers(ctx, 10, 0)
	if err != nil || len(users) != 1 {
		t.Fatal("repeat bootstrap must not create duplicate accounts")
	}
	if err := store.SetUserRole(ctx, first.ID, GlobalRoleMember); err != nil {
		t.Fatalf("prepare existing member: %v", err)
	}
	if err := service.EnsureSeedAdmin(ctx, "admin@example.com", "unused-password789"); err != nil {
		t.Fatalf("promote existing seed account: %v", err)
	}
	promoted, err := store.GetUserByID(ctx, first.ID)
	if err != nil || promoted == nil || promoted.Role != GlobalRoleAdmin || promoted.PasswordHash != first.PasswordHash {
		t.Fatal("bootstrap must ensure the admin role without replacing the password")
	}
	if _, err := service.Login(ctx, LoginRequest{Email: "admin@example.com", Password: "password123"}); err != nil {
		t.Fatalf("original password must still work: %v", err)
	}
	if result, err := service.Login(ctx, LoginRequest{Email: "admin@example.com", Password: "changed-password456"}); err == nil || result != nil {
		t.Fatal("repeat bootstrap must not activate the newly supplied password")
	}
}

func TestServiceEnsureSeedAdminPreservesErrors(t *testing.T) {
	t.Run("create failure", func(t *testing.T) {
		want := errors.New("insert failed")
		store := &seedAdminStoreStub{
			get: func(context.Context, string) (*domain.User, error) {
				return nil, errors.New("user not found")
			},
			create: func(context.Context, *domain.User) error { return want },
			setRole: func(context.Context, string, string) error {
				t.Fatal("must not set role after failed creation")
				return nil
			},
		}
		if err := NewService(store, nil).EnsureSeedAdmin(context.Background(), "admin@example.com", "password123"); !errors.Is(err, want) {
			t.Fatalf("creation error must retain its cause: %v", err)
		}
	})
	t.Run("role update failure", func(t *testing.T) {
		want := errors.New("update role failed")
		store := &seedAdminStoreStub{
			get: func(_ context.Context, email string) (*domain.User, error) {
				if email != "admin@example.com" {
					t.Fatal("seed lookup must normalize email")
				}
				return &domain.User{ID: "existing-user"}, nil
			},
			create: func(context.Context, *domain.User) error {
				t.Fatal("existing seed account must not be recreated")
				return nil
			},
			setRole: func(_ context.Context, id, role string) error {
				if id != "existing-user" || role != GlobalRoleAdmin {
					t.Fatal("role update must target the seed account and global admin role")
				}
				return want
			},
		}
		if err := NewService(store, nil).EnsureSeedAdmin(context.Background(), " ADMIN@EXAMPLE.COM ", "unused-password"); !errors.Is(err, want) {
			t.Fatalf("role error must retain its cause: %v", err)
		}
	})
}
