package iam

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"my_kbot_final/internal/domain"
	"strings"
	"testing"
	"time"
)

// ---- postgres_store_workspace_member_read_test.go ----

func TestPostgresStoreWorkspaceMemberRead(t *testing.T) {
	ctx, store, tx := newPostgresStoreTransactionTest(t)
	x, y := uuid.NewString(), uuid.NewString()
	for _, user := range []*domain.User{
		{ID: x, Email: "x@example.com", Name: "小明", Role: GlobalRoleMember},
		{ID: y, Email: "y@example.com", Name: "小红", Role: GlobalRoleAdmin},
	} {
		if err := store.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"space-a", "space-b", "empty-space"} {
		if err := store.CreateWorkspace(ctx, &domain.Workspace{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for index, fixture := range []struct{ workspace, user, role string }{
		{"space-a", x, "owner"}, {"space-a", y, "viewer"}, {"space-b", x, "editor"},
	} {
		if _, err := tx.Exec(ctx, "INSERT INTO workspace_members (workspace_id,user_id,role,created_at) VALUES ($1,$2,$3,$4)", fixture.workspace, uuid.MustParse(fixture.user), fixture.role, base.Add(time.Duration(index)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	member, err := store.GetWorkspaceMember(ctx, "space-a", x)
	if err != nil || member == nil {
		t.Fatalf("get member: member=%v err=%v", member, err)
	}
	if member.WorkspaceID != "space-a" || member.UserID != x || member.Email != "x@example.com" || member.Name != "小明" || member.Role != "owner" || !member.CreatedAt.Equal(base) {
		t.Fatalf("membership fields/identity join lost: %+v", member)
	}
	other, err := store.GetWorkspaceMember(ctx, "space-b", x)
	if err != nil || other == nil || other.Role != "editor" || other.WorkspaceID != "space-b" {
		t.Fatalf("same user in another workspace: %+v err=%v", other, err)
	}
	members, err := store.ListWorkspaceMembers(ctx, "space-a")
	if err != nil || len(members) != 2 {
		t.Fatalf("members=%v err=%v", members, err)
	}
	if members[0] == nil || members[1] == nil || members[0].UserID != x || members[1].UserID != y || members[1].Role != "viewer" || members[1].Email != "y@example.com" || members[1].Name != "小红" || !members[1].CreatedAt.Equal(base.Add(time.Minute)) {
		t.Fatal("members must be scoped to the workspace and ordered by membership time, with workspace roles")
	}
	// 用户资料属于 users；无需修改成员记录，关联查询就应读到最新资料。
	if _, err := tx.Exec(ctx, "UPDATE users SET email=$2,name=$3 WHERE id=$1", uuid.MustParse(x), "x-new@example.com", "小明新名字"); err != nil {
		t.Fatal(err)
	}
	member, err = store.GetWorkspaceMember(ctx, "space-a", x)
	if err != nil || member == nil || member.Email != "x-new@example.com" || member.Name != "小明新名字" || member.Role != "owner" {
		t.Fatalf("joined user profile is stale: %+v err=%v", member, err)
	}
	members, err = store.ListWorkspaceMembers(ctx, "space-a")
	if err != nil || len(members) != 2 || members[0].Name != "小明新名字" {
		t.Fatal("list must also join current user data")
	}
	empty, err := store.ListWorkspaceMembers(ctx, "empty-space")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list=%v err=%v", empty, err)
	}
}

func TestPostgresStoreWorkspaceMemberMissing(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	member, err := store.GetWorkspaceMember(ctx, "missing-space", uuid.NewString())
	if member != nil || err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing member: %+v err=%v", member, err)
	}
}

func TestPostgresStoreWorkspaceMemberInvalidID(t *testing.T) {
	store := NewPostgresStore(nil, nil)
	for _, id := range []string{"", "invalid-uuid"} {
		member, err := store.GetWorkspaceMember(context.Background(), "space-a", id)
		if member != nil || err == nil {
			t.Fatalf("invalid UUID should fail before DB access: %+v err=%v", member, err)
		}
	}
}

func TestPostgresStoreWorkspaceMemberCancellation(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := store.GetWorkspaceMember(canceled, "space-a", uuid.NewString())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("get cancellation lost: %v", err)
	}
	_, err = store.ListWorkspaceMembers(canceled, "space-a")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("list cancellation lost: %v", err)
	}
}

// ---- postgres_store_workspace_member_upsert_test.go ----

func TestPostgresStoreUpsertWorkspaceMember(t *testing.T) {
	ctx, store, tx := newPostgresStoreTransactionTest(t)
	id := uuid.New()
	user := &domain.User{ID: id.String(), Email: "member@example.com", Name: "小明"}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	for _, space := range []string{"space-a", "space-b"} {
		if err := store.CreateWorkspace(ctx, &domain.Workspace{ID: space, Name: space}); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	inputUserID := strings.ReplaceAll(id.String(), "-", "")
	member := &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: inputUserID, Role: "member", CreatedAt: old}
	if err := store.UpsertWorkspaceMember(ctx, member); err != nil {
		t.Fatal(err)
	}
	if member.UserID != inputUserID {
		t.Fatal("upsert must preserve the caller-provided user ID string")
	}
	if member.CreatedAt.IsZero() || member.CreatedAt.Equal(old) {
		t.Fatal("database join time must be written back")
	}
	got, err := store.GetWorkspaceMember(ctx, "space-a", user.ID)
	if err != nil || got == nil || got.Role != "member" || got.Name != user.Name || got.Email != user.Email || !got.CreatedAt.Equal(member.CreatedAt) {
		t.Fatalf("inserted membership=%+v err=%v", got, err)
	}
	// 明确改变创建时间，避免事务内 now() 恒定掩盖错误的更新时间行为。
	joined := member.CreatedAt.Add(-time.Hour)
	if _, err := tx.Exec(ctx, "UPDATE workspace_members SET created_at=$3 WHERE workspace_id=$1 AND user_id=$2", "space-a", id, joined); err != nil {
		t.Fatal(err)
	}
	member.Role, member.CreatedAt = "editor", old
	if err := store.UpsertWorkspaceMember(ctx, member); err != nil {
		t.Fatal(err)
	}
	if member.Role != "editor" || !member.CreatedAt.Equal(joined) {
		t.Fatal("role update must preserve original join time and return it to caller")
	}
	members, err := store.ListWorkspaceMembers(ctx, "space-a")
	if err != nil || len(members) != 1 || members[0].Role != "editor" || !members[0].CreatedAt.Equal(joined) {
		t.Fatalf("upsert must update one membership rather than duplicate it: %v err=%v", members, err)
	}
	if err := store.UpsertWorkspaceMember(ctx, &domain.WorkspaceMember{WorkspaceID: "space-b", UserID: user.ID, Role: "viewer"}); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetWorkspaceMember(ctx, "space-a", user.ID)
	if err != nil || got == nil || got.Role != "editor" {
		t.Fatal("same user in another workspace must not overwrite the first membership")
	}
	other, err := store.GetWorkspaceMember(ctx, "space-b", user.ID)
	if err != nil || other == nil || other.Role != "viewer" {
		t.Fatal("second workspace membership missing")
	}
}

func TestPostgresStoreUpsertWorkspaceMemberForeignKeys(t *testing.T) {
	for _, missing := range []string{"user", "workspace"} {
		t.Run(missing, func(t *testing.T) {
			ctx, store := newPostgresUserStoreTest(t)
			user := &domain.User{ID: uuid.NewString(), Email: "member@example.com"}
			if err := store.CreateUser(ctx, user); err != nil {
				t.Fatal(err)
			}
			if err := store.CreateWorkspace(ctx, &domain.Workspace{ID: "space-a", Name: "A"}); err != nil {
				t.Fatal(err)
			}
			member := &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: user.ID, Role: "member"}
			if missing == "user" {
				member.UserID = uuid.NewString()
			} else {
				member.WorkspaceID = "missing-space"
			}
			err := store.UpsertWorkspaceMember(ctx, member)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
				t.Fatalf("missing %s must preserve foreign-key violation, got %v", missing, err)
			}
			if !member.CreatedAt.IsZero() {
				t.Fatal("failed write must not fill a success timestamp")
			}
		})
	}
}

func TestPostgresStoreUpsertWorkspaceMemberInvalidID(t *testing.T) {
	store := NewPostgresStore(nil, nil)
	for _, id := range []string{"", "invalid-user-id"} {
		if err := store.UpsertWorkspaceMember(context.Background(), &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: id, Role: "member"}); err == nil {
			t.Fatal("invalid UUID must fail before DB access")
		}
	}
}

func TestPostgresStoreUpsertWorkspaceMemberCancellation(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	err := store.UpsertWorkspaceMember(canceled, &domain.WorkspaceMember{WorkspaceID: "space-a", UserID: uuid.NewString(), Role: "member"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

// ---- postgres_store_workspace_member_delete_test.go ----

func TestPostgresStoreDeleteWorkspaceMember(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	x, y := uuid.NewString(), uuid.NewString()
	for _, user := range []*domain.User{{ID: x, Email: "x@example.com"}, {ID: y, Email: "y@example.com"}} {
		if err := store.CreateUser(ctx, user); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"space-a", "space-b"} {
		if err := store.CreateWorkspace(ctx, &domain.Workspace{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	for _, member := range []*domain.WorkspaceMember{
		{WorkspaceID: "space-a", UserID: x, Role: "member"},
		{WorkspaceID: "space-b", UserID: x, Role: "member"},
		{WorkspaceID: "space-a", UserID: y, Role: "viewer"},
	} {
		if err := store.UpsertWorkspaceMember(ctx, member); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := store.DeleteWorkspaceMember(ctx, "space-a", x)
	if err != nil || !deleted {
		t.Fatalf("existing membership deletion=%v err=%v", deleted, err)
	}
	if member, err := store.GetWorkspaceMember(ctx, "space-a", x); err == nil || member != nil {
		t.Fatal("deleted membership must no longer be readable")
	}
	for _, membership := range []struct{ workspace, user string }{{"space-b", x}, {"space-a", y}} {
		if member, err := store.GetWorkspaceMember(ctx, membership.workspace, membership.user); err != nil || member == nil {
			t.Fatalf("unrelated membership was deleted: %+v err=%v", membership, err)
		}
	}
	if user, err := store.GetUserByID(ctx, x); err != nil || user == nil {
		t.Fatal("removing membership must not delete the user")
	}
	if ws, err := store.GetWorkspaceByID(ctx, "space-a"); err != nil || ws == nil {
		t.Fatal("removing membership must not delete the workspace")
	}
	spaces, err := store.ListUserWorkspaces(ctx, x, 10, 0)
	if err != nil || len(spaces) != 1 || spaces[0].ID != "space-b" {
		t.Fatalf("membership-filtered spaces not updated: %v err=%v", spaces, err)
	}
	deleted, err = store.DeleteWorkspaceMember(ctx, "space-a", x)
	if err != nil || deleted {
		t.Fatalf("repeat deletion should report no matching row: deleted=%v err=%v", deleted, err)
	}
}

func TestPostgresStoreDeleteWorkspaceMemberMissing(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	user := &domain.User{ID: uuid.NewString(), Email: "member@example.com"}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateWorkspace(ctx, &domain.Workspace{ID: "space-a", Name: "A"}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []struct{ workspace, user string }{
		{"space-a", user.ID}, {"missing-space", user.ID}, {"space-a", uuid.NewString()},
	} {
		deleted, err := store.DeleteWorkspaceMember(ctx, query.workspace, query.user)
		if err != nil || deleted {
			t.Fatalf("no matching membership must return false,nil: deleted=%v err=%v", deleted, err)
		}
	}
}

func TestPostgresStoreDeleteWorkspaceMemberInvalidID(t *testing.T) {
	store := NewPostgresStore(nil, nil)
	for _, id := range []string{"", "invalid-user-id"} {
		deleted, err := store.DeleteWorkspaceMember(context.Background(), "space-a", id)
		if err == nil || deleted {
			t.Fatal("invalid UUID must return false and an error before DB access")
		}
	}
}

func TestPostgresStoreDeleteWorkspaceMemberCancellation(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	deleted, err := store.DeleteWorkspaceMember(canceled, "space-a", uuid.NewString())
	if deleted || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation must be preserved: deleted=%v err=%v", deleted, err)
	}
}
