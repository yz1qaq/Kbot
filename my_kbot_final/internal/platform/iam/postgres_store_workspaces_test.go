package iam

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"my_kbot_final/internal/domain"
	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"
	"strings"
	"testing"
	"time"
)

// ---- workspace_mapping_test.go ----

func TestWorkspaceNullableText(t *testing.T) {
	empty, parent := "", "workspace-parent"
	for _, tc := range []struct {
		name  string
		input *string
		valid bool
		value string
	}{
		{"nil means SQL NULL", nil, false, ""},
		{"empty string is not SQL NULL", &empty, true, ""},
		{"parent id", &parent, true, parent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded := textFromPtr(tc.input)
			if encoded.Valid != tc.valid || encoded.String != tc.value {
				t.Fatalf("encoded=%+v, want valid=%v value=%q", encoded, tc.valid, tc.value)
			}
			decoded := ptrFromText(encoded)
			if !tc.valid {
				if decoded != nil {
					t.Fatal("SQL NULL must decode to nil")
				}
				return
			}
			if decoded == nil || *decoded != tc.value {
				t.Fatalf("non-NULL value did not round-trip: %v", decoded)
			}
			*decoded = "changed"
			if *tc.input != tc.value || encoded.String != tc.value {
				t.Fatal("changing decoded value must not change the source")
			}
		})
	}
	if ptrFromText(pgtype.Text{String: "ignored value", Valid: false}) != nil {
		t.Fatal("Valid determines NULL status, even if String contains text")
	}
}

func TestWorkspaceFromRow(t *testing.T) {
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for _, parent := range []pgtype.Text{{}, {String: "root", Valid: true}} {
		row := pgstore.Workspace{
			ID: "workspace-a", Name: "电商运营", Description: "测试空间",
			ParentID: parent, CreatedAt: now, UpdatedAt: now.Add(time.Minute),
		}
		got := workspaceFromRow(row)
		if got == nil || got.ID != row.ID || got.Name != row.Name || got.Description != row.Description || !got.CreatedAt.Equal(row.CreatedAt) || !got.UpdatedAt.Equal(row.UpdatedAt) {
			t.Fatal("workspace data lost while mapping database row")
		}
		if !parent.Valid {
			if got.ParentID != nil {
				t.Fatal("root workspace must have nil parent")
			}
			continue
		}
		if got.ParentID == nil || *got.ParentID != parent.String {
			t.Fatal("parent ID lost in mapping")
		}
		another := workspaceFromRow(row)
		*got.ParentID = "changed-parent"
		if another.ParentID == nil || *another.ParentID != "root" || row.ParentID.String != "root" {
			t.Fatal("mapped workspaces must not share mutable parent ID storage")
		}
	}
}

// ---- postgres_store_workspace_test.go ----

func TestPostgresStoreWorkspaceRoundTrip(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	empty, err := store.ListWorkspaces(ctx, 10, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty workspace list=%v err=%v", empty, err)
	}
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	rootID := "operations"
	workspaces := []*domain.Workspace{
		{ID: rootID, Name: "运营中心", Description: "根空间", CreatedAt: old, UpdatedAt: old},
		{ID: "operations-cn", Name: "中国区运营", Description: "子空间", ParentID: &rootID, CreatedAt: old, UpdatedAt: old},
	}
	for _, ws := range workspaces {
		if err := store.CreateWorkspace(ctx, ws); err != nil {
			t.Fatal(err)
		}
		if ws.CreatedAt.IsZero() || ws.CreatedAt.Equal(old) || ws.UpdatedAt.IsZero() || ws.UpdatedAt.Equal(old) {
			t.Fatal("database timestamps must be written back to the caller's workspace")
		}
		got, err := store.GetWorkspaceByID(ctx, ws.ID)
		if err != nil || got == nil {
			t.Fatalf("read workspace: got=%v err=%v", got, err)
		}
		if got.ID != ws.ID || got.Name != ws.Name || got.Description != ws.Description || !got.CreatedAt.Equal(ws.CreatedAt) || !got.UpdatedAt.Equal(ws.UpdatedAt) {
			t.Fatal("stored domain workspace did not round-trip")
		}
		if ws.ParentID == nil {
			if got.ParentID != nil {
				t.Fatal("NULL parent must map back to nil")
			}
		} else if got.ParentID == nil || *got.ParentID != *ws.ParentID {
			t.Fatal("child workspace lost parent ID")
		}
	}
	all, err := store.ListWorkspaces(ctx, 10, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("list length=%d err=%v", len(all), err)
	}
	seen := map[string]bool{}
	for _, ws := range all {
		if ws == nil {
			t.Fatal("list must not contain nil workspaces")
		}
		seen[ws.ID] = true
		if ws.ID == rootID && ws.ParentID != nil {
			t.Fatal("root list row must have nil parent")
		}
		if ws.ID == "operations-cn" && (ws.ParentID == nil || *ws.ParentID != rootID) {
			t.Fatal("list mapping lost parent")
		}
	}
	if !seen[rootID] || !seen["operations-cn"] {
		t.Fatal("list mapping lost workspace IDs")
	}
	page, err := store.ListWorkspaces(ctx, 1, 0)
	if err != nil || len(page) != 1 {
		t.Fatalf("limit not forwarded: count=%d err=%v", len(page), err)
	}
	page, err = store.ListWorkspaces(ctx, 10, 2)
	if err != nil || page == nil || len(page) != 0 {
		t.Fatalf("empty page=%v err=%v", page, err)
	}
}

func TestPostgresStoreWorkspaceMissing(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	ws, err := store.GetWorkspaceByID(ctx, "missing-workspace")
	if ws != nil || err == nil || !strings.Contains(err.Error(), "workspace not found") {
		t.Fatalf("missing workspace must return nil and not-found error: ws=%v err=%v", ws, err)
	}
}

func TestPostgresStoreWorkspaceConstraintErrors(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		duplicate  bool
	}{
		{"duplicate ID", "23505", true},
		{"missing parent", "23503", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, store := newPostgresUserStoreTest(t)
			ws := &domain.Workspace{ID: "workspace-a", Name: "测试空间"}
			if tc.duplicate {
				if err := store.CreateWorkspace(ctx, &domain.Workspace{ID: ws.ID, Name: "已存在"}); err != nil {
					t.Fatal(err)
				}
			} else {
				parent := "missing-parent"
				ws.ParentID = &parent
			}
			old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			ws.CreatedAt, ws.UpdatedAt = old, old
			err := store.CreateWorkspace(ctx, ws)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tc.code {
				t.Fatalf("want SQLSTATE %s, got %v", tc.code, err)
			}
			if !ws.CreatedAt.Equal(old) || !ws.UpdatedAt.Equal(old) {
				t.Fatal("failed insert must not write back success timestamps")
			}
		})
	}
}

func TestPostgresStoreWorkspaceCancellation(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	for _, operation := range []func() error{
		func() error { _, err := store.GetWorkspaceByID(canceled, "workspace-a"); return err },
		func() error { _, err := store.ListWorkspaces(canceled, 10, 0); return err },
		func() error {
			return store.CreateWorkspace(canceled, &domain.Workspace{ID: "workspace-a", Name: "取消"})
		},
	} {
		if err := operation(); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation must survive Store wrapping, got %v", err)
		}
	}
}

// ---- postgres_store_user_workspaces_test.go ----

func TestPostgresStoreListUserWorkspaces(t *testing.T) {
	ctx, store, tx := newPostgresStoreTransactionTest(t)
	x, y, admin := uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, user := range []*domain.User{
		{ID: x, Email: "x@example.com", Role: GlobalRoleMember},
		{ID: y, Email: "y@example.com", Role: GlobalRoleMember},
		{ID: admin, Email: "admin@example.com", Role: GlobalRoleAdmin},
	} {
		if err := store.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	a := "space-a"
	base := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for index, ws := range []*domain.Workspace{
		{ID: a, Name: "X 的空间"},
		{ID: "space-b", Name: "Y 的空间"},
		{ID: "shared", Name: "共同空间"},
		{ID: "child", Name: "A 的子空间", ParentID: &a},
	} {
		if err := store.CreateWorkspace(ctx, ws); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, "UPDATE workspaces SET created_at=$2 WHERE id=$1", ws.ID, base.Add(time.Duration(index)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	for _, membership := range []struct{ workspace, user, role string }{
		{a, x, "owner"}, {"shared", x, "viewer"},
		{"space-b", y, "owner"}, {"shared", y, "editor"},
	} {
		if _, err := tx.Exec(ctx, "INSERT INTO workspace_members (workspace_id,user_id,role) VALUES ($1,$2,$3)", membership.workspace, uuid.MustParse(membership.user), membership.role); err != nil {
			t.Fatal(err)
		}
	}
	assertPage := func(user string, limit, offset int32, ids ...string) {
		t.Helper()
		workspaces, err := store.ListUserWorkspaces(ctx, user, limit, offset)
		if err != nil || workspaces == nil || len(workspaces) != len(ids) {
			t.Fatalf("count=%d err=%v, want %v", len(workspaces), err, ids)
		}
		for index, id := range ids {
			if workspaces[index] == nil || workspaces[index].ID != id || workspaces[index].Name == "" || workspaces[index].CreatedAt.IsZero() {
				t.Fatalf("unexpected workspace at index %d: %+v, want %s", index, workspaces[index], id)
			}
		}
	}
	assertPage(x, 10, 0, "shared", a)
	assertPage(y, 10, 0, "shared", "space-b")
	assertPage(x, 1, 1, a) // 先按成员过滤，再分页；不应得到 Y 的空间。
	assertPage(x, 10, 2)
	// 该 Store 查询只按成员关系过滤，全局管理员的业务绕过由 Service 决定。
	assertPage(admin, 10, 0)
	if _, err := tx.Exec(ctx, "DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2", a, uuid.MustParse(x)); err != nil {
		t.Fatal(err)
	}
	assertPage(x, 10, 0, "shared")
	assertPage(y, 10, 0, "shared", "space-b")
}

func TestPostgresStoreListUserWorkspacesInvalidID(t *testing.T) {
	store := NewPostgresStore(nil, nil)
	for _, id := range []string{"", "invalid-user-id"} {
		rows, err := store.ListUserWorkspaces(context.Background(), id, 10, 0)
		if err == nil || rows != nil {
			t.Fatalf("invalid UUID must fail before querying: rows=%v err=%v", rows, err)
		}
	}
}

func TestPostgresStoreListUserWorkspacesCancellation(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := store.ListUserWorkspaces(canceled, uuid.NewString(), 10, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("request cancellation lost: %v", err)
	}
}

// ---- postgres_store_workspace_owner_test.go ----

func TestPostgresStoreCreateWorkspaceWithOwner(t *testing.T) {
	for _, child := range []bool{false, true} {
		name := "root"
		if child {
			name = "child"
		}
		t.Run(name, func(t *testing.T) {
			ctx, store := newPostgresUserStoreTest(t)
			user := &domain.User{ID: uuid.NewString(), Email: "creator@example.com", Name: "创建者"}
			if err := store.CreateUser(ctx, user); err != nil {
				t.Fatal(err)
			}
			ws := &domain.Workspace{ID: "created-space", Name: "原子创建空间", Description: "Store 适配测试"}
			if child {
				if err := store.CreateWorkspace(ctx, &domain.Workspace{ID: "parent", Name: "父空间"}); err != nil {
					t.Fatal(err)
				}
				parent := "parent"
				ws.ParentID = &parent
			}
			var atomicStore workspaceOwnerStore = store
			if err := atomicStore.CreateWorkspaceWithOwner(ctx, ws, strings.ReplaceAll(user.ID, "-", "")); err != nil {
				t.Fatal(err)
			}
			got, err := store.GetWorkspaceByID(ctx, ws.ID)
			if err != nil || got == nil || got.Name != ws.Name || got.Description != ws.Description || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
				t.Fatalf("workspace=%+v err=%v", got, err)
			}
			if ws.ParentID == nil {
				if got.ParentID != nil {
					t.Fatal("root parent must be NULL")
				}
			} else if got.ParentID == nil || *got.ParentID != *ws.ParentID {
				t.Fatal("parent conversion lost")
			}
			member, err := store.GetWorkspaceMember(ctx, ws.ID, user.ID)
			if err != nil || member == nil || member.UserID != user.ID || member.Role != "owner" || member.Name != user.Name {
				t.Fatalf("owner=%+v err=%v", member, err)
			}
		})
	}
}

func TestPostgresStoreCreateWorkspaceWithOwnerInvalidID(t *testing.T) {
	store := NewPostgresStore(nil, nil)
	for _, id := range []string{"", "invalid-user-id"} {
		if err := store.CreateWorkspaceWithOwner(context.Background(), &domain.Workspace{ID: "space-a"}, id); err == nil {
			t.Fatal("invalid UUID must fail before DB access")
		}
	}
}

func TestPostgresStoreCreateWorkspaceWithOwnerForeignKey(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	err := store.CreateWorkspaceWithOwner(ctx, &domain.Workspace{ID: "space-a", Name: "A"}, uuid.NewString())
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("Store must preserve foreign-key error: %v", err)
	}
}

func TestPostgresStoreCreateWorkspaceWithOwnerCancellation(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	err := store.CreateWorkspaceWithOwner(canceled, &domain.Workspace{ID: "space-a", Name: "A"}, uuid.NewString())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
