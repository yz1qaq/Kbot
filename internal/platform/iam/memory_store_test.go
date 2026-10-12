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

func TestAddMembershipRejectsDuplicate(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	user := &domain.User{
		ID:    "user-001",
		Email: "test@example.com",
		Name:  "测试用户",
	}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	workspace := &domain.Workspace{
		ID:   "workspace-001",
		Name: "测试空间",
	}
	if err := store.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	membership := &domain.Membership{
		UserID:      user.ID,
		WorkspaceID: workspace.ID,
		Role:        "owner",
	}
	if err := store.AddMembership(ctx, membership); err != nil {
		t.Fatalf("first membership: %v", err)
	}

	changed := *membership
	changed.Role = "viewer"

	if err := store.AddMembership(ctx, &changed); err == nil {
		t.Fatal("expected duplicate membership to fail")
	}

	got, ok := store.memberships[user.ID][workspace.ID]
	if !ok {
		t.Fatal("original membership was lost")
	}
	if got.Role != "owner" {
		t.Fatalf("stored role = %q, want owner", got.Role)
	}
}

func TestListUserWorkspacesFiltersAndReturnsCopies(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	user := &domain.User{
		ID:    "user-001",
		Email: "test@example.com",
	}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	joined := &domain.Workspace{
		ID:   "workspace-A",
		Name: "已加入的空间",
	}
	other := &domain.Workspace{
		ID:   "workspace-B",
		Name: "未加入的空间",
	}

	for _, workspace := range []*domain.Workspace{joined, other} {
		if err := store.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("create workspace: %v", err)
		}
	}

	if err := store.AddMembership(ctx, &domain.Membership{
		UserID:      user.ID,
		WorkspaceID: joined.ID,
		Role:        "owner",
	}); err != nil {
		t.Fatalf("add membership: %v", err)
	}

	got, err := store.ListUserWorkspaces(ctx, user.ID)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("workspace count = %d, want 1", len(got))
	}
	if got[0] == nil || got[0].ID != joined.ID {
		t.Fatal("expected only the joined workspace")
	}

	got[0].Name = "调用方修改的名字"

	again, err := store.ListUserWorkspaces(ctx, user.ID)
	if err != nil {
		t.Fatalf("list workspaces again: %v", err)
	}
	if len(again) != 1 || again[0] == nil {
		t.Fatal("expected one workspace on second lookup")
	}
	if again[0].Name != joined.Name {
		t.Fatalf("stored name = %q, want %q",
			again[0].Name, joined.Name)
	}
}
