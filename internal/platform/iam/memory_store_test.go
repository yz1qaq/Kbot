package iam

import (
	"context"
	"testing"

	"my_kbot/internal/domain"
)

func TestGetUserByEmailReturnsCopy(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	user := &domain.User{
		ID:    "user-001",
		Email: "test@example.com",
		Name:  "原始名字",
	}

	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	got, err := store.GetUserByEmail(ctx, "TEST@example.com")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if got == nil {
		t.Fatal("got nil user")
	}

	if got.ID != user.ID || got.Name != "原始名字" {
		t.Fatalf("unexpected user: %+v", got)
	}

	got.Name = "调用方修改的名字"

	again, err := store.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("get user again: %v", err)
	}
	if again == nil {
		t.Fatal("got nil user on second lookup")
	}

	if again.Name != "原始名字" {
		t.Fatalf("stored name = %q, want 原始名字", again.Name)
	}
}
