package iam

import (
	"context"
	"errors"
	"testing"

	"my_kbot/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

func TestWorkspaceRoleValidatesRole(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{
			name: "合法角色允许通过",
			role: WorkspaceRoleOwner,
		},
		{
			name:    "非法角色拒绝访问",
			role:    "root",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := NewMemoryStore()

			user := &domain.User{
				ID:    "user-001",
				Email: "test@example.com",
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

			if err := store.AddMembership(ctx, &domain.Membership{
				UserID:      user.ID,
				WorkspaceID: workspace.ID,
				Role:        tt.role,
			}); err != nil {
				t.Fatalf("add membership: %v", err)
			}

			service := New(store, "", "")
			got, err := service.WorkspaceRole(
				ctx, user.ID, workspace.ID,
			)

			if tt.wantErr {
				if !errors.Is(err, ErrWorkspaceForbidden) {
					t.Fatalf("error = %v, want ErrWorkspaceForbidden", err)
				}
				if got != "" {
					t.Fatalf("role = %q, want empty role", got)
				}
				return
			}

			if err != nil {
				t.Fatalf("workspace role: %v", err)
			}
			if got != tt.role {
				t.Fatalf("role = %q, want %q", got, tt.role)
			}
		})
	}
}

func TestRegisterCreatesDefaultWorkspace(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	service := New(store, "", "")

	password := "password123"
	user, err := service.Register(
		ctx,
		" TEST@example.com ",
		password,
		" 小明 ",
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user == nil || user.ID == "" {
		t.Fatal("expected a user with a non-empty ID")
	}
	if user.Email != "test@example.com" || user.Name != "小明" {
		t.Fatalf("unexpected user: %+v", user)
	}

	stored, err := store.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("get stored user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword(
		[]byte(stored.PasswordHash),
		[]byte(password),
	); err != nil {
		t.Fatalf("stored password hash does not match: %v", err)
	}

	workspaces, err := service.ListUserWorkspaces(ctx, user.ID)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("workspace count = %d, want 1", len(workspaces))
	}
	if workspaces[0] == nil || workspaces[0].ID == "" {
		t.Fatal("expected a workspace with a non-empty ID")
	}

	role, err := service.WorkspaceRole(
		ctx, user.ID, workspaces[0].ID,
	)
	if err != nil {
		t.Fatalf("workspace role: %v", err)
	}
	if role != WorkspaceRoleOwner {
		t.Fatalf("role = %q, want owner", role)
	}
}
