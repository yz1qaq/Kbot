package postgres

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---- workspace_queries_test.go ----

func TestWorkspaceQueriesRoundTrip(t *testing.T) {
	ctx, tx := beginIAMMigrationTest(t)
	q := pgstore.New(tx)
	parent, err := q.CreateWorkspace(ctx, pgstore.CreateWorkspaceParams{
		ID: "operations", Name: "运营中心", Description: "根空间",
	})
	if err != nil {
		t.Fatalf("create root workspace: %v", err)
	}
	if parent.ParentID.Valid || parent.CreatedAt.IsZero() || parent.UpdatedAt.IsZero() {
		t.Fatal("root workspace must have NULL parent and database timestamps")
	}
	child, err := q.CreateWorkspace(ctx, pgstore.CreateWorkspaceParams{
		ID: "operations-cn", Name: "中国区运营", Description: "子空间",
		ParentID: pgtype.Text{String: parent.ID, Valid: true},
	})
	if err != nil {
		t.Fatalf("create child workspace: %v", err)
	}
	for _, want := range []pgstore.Workspace{parent, child} {
		got, err := q.GetWorkspaceByID(ctx, want.ID)
		if err != nil {
			t.Fatalf("read workspace %s: %v", want.ID, err)
		}
		if got.ID != want.ID || got.Name != want.Name || got.Description != want.Description || got.ParentID != want.ParentID || !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
			t.Fatalf("workspace %s did not round-trip", want.ID)
		}
	}
}

func TestWorkspaceQueriesMissing(t *testing.T) {
	ctx, tx := beginIAMMigrationTest(t)
	_, err := pgstore.New(tx).GetWorkspaceByID(ctx, "missing-workspace")
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing workspace must preserve pgx.ErrNoRows, got %v", err)
	}
}

func TestWorkspaceQueriesPagination(t *testing.T) {
	ctx, tx := beginIAMMigrationTest(t)
	q := pgstore.New(tx)
	base := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for index, id := range []string{"oldest", "middle", "newest"} {
		if _, err := q.CreateWorkspace(ctx, pgstore.CreateWorkspaceParams{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
		// now() 在同一事务内相同，明确设置不同时间以检验排序，避免偶然通过。
		if _, err := tx.Exec(ctx, "UPDATE workspaces SET created_at=$2 WHERE id=$1", id, base.Add(time.Duration(index)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name          string
		limit, offset int32
		ids           []string
	}{
		{"descending order", 10, 0, []string{"newest", "middle", "oldest"}},
		{"first page", 2, 0, []string{"newest", "middle"}},
		{"next page", 2, 2, []string{"oldest"}},
		{"empty page", 2, 3, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := q.ListWorkspaces(ctx, pgstore.ListWorkspacesParams{Limit: tc.limit, Offset: tc.offset})
			if err != nil || rows == nil || len(rows) != len(tc.ids) {
				t.Fatalf("rows=%v, err=%v, want ids=%v", rows, err, tc.ids)
			}
			for index, id := range tc.ids {
				if rows[index].ID != id {
					t.Fatalf("row %d id=%s, want %s", index, rows[index].ID, id)
				}
			}
		})
	}
}

// ---- workspace_owner_queries_test.go ----

// 独立 schema、自动提交连接；不用外层事务回滚掩盖半成功写入。
func newWorkspaceOwnerQueryTest(t *testing.T) (context.Context, *pgstore.Queries, *pgx.Conn) {
	t.Helper()
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL for workspace owner SQL tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	schema := pgx.Identifier{"kbot_workspace_owner_" + uuid.NewString()}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := conn.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("clean test schema: %v", err)
		}
	})
	if _, err := conn.Exec(ctx, "SELECT set_config('search_path',$1,false)", schema+",pg_catalog"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	return ctx, pgstore.New(conn), conn
}

func TestWorkspaceOwnerQueriesCreate(t *testing.T) {
	for _, child := range []bool{false, true} {
		name := "root"
		if child {
			name = "child"
		}
		t.Run(name, func(t *testing.T) {
			ctx, q, _ := newWorkspaceOwnerQueryTest(t)
			id := uuid.New()
			if _, err := q.CreateUser(ctx, pgstore.CreateUserParams{ID: id, Email: "owner@example.com", Name: "创建者", Role: "member", Status: "active"}); err != nil {
				t.Fatal(err)
			}
			var parent pgtype.Text
			if child {
				if _, err := q.CreateWorkspace(ctx, pgstore.CreateWorkspaceParams{ID: "parent", Name: "父空间"}); err != nil {
					t.Fatal(err)
				}
				parent = pgtype.Text{String: "parent", Valid: true}
			}
			if err := q.CreateWorkspaceWithOwner(ctx, pgstore.CreateWorkspaceWithOwnerParams{WorkspaceID: "created", Name: "新空间", Description: "原子创建测试", ParentID: parent, UserID: id}); err != nil {
				t.Fatal(err)
			}
			ws, err := q.GetWorkspaceByID(ctx, "created")
			if err != nil || ws.Name != "新空间" || ws.Description != "原子创建测试" || ws.ParentID != parent || ws.CreatedAt.IsZero() || ws.UpdatedAt.IsZero() {
				t.Fatalf("workspace=%+v err=%v", ws, err)
			}
			members, err := q.ListWorkspaceMembers(ctx, "created")
			if err != nil || len(members) != 1 || members[0].UserID != id || members[0].Role != "owner" || members[0].CreatedAt.IsZero() {
				t.Fatalf("owner membership=%+v err=%v", members, err)
			}
			spaces, err := q.ListUserWorkspaces(ctx, pgstore.ListUserWorkspacesParams{UserID: id, Limit: 10})
			if err != nil || len(spaces) != 1 || spaces[0].ID != "created" {
				t.Fatal("creator must see the new space through membership")
			}
		})
	}
}

func TestWorkspaceOwnerQueriesMissingCreatorRollsBack(t *testing.T) {
	ctx, q, conn := newWorkspaceOwnerQueryTest(t)
	err := q.CreateWorkspaceWithOwner(ctx, pgstore.CreateWorkspaceWithOwnerParams{WorkspaceID: "must-not-survive", Name: "不应残留", UserID: uuid.New()})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		t.Fatalf("missing creator must fail with foreign-key violation, got %v", err)
	}
	// 直接在仍可用的自动提交连接上查询，无任何手动回滚。
	if _, err := q.GetWorkspaceByID(ctx, "must-not-survive"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("failed owner insertion left a workspace behind: %v", err)
	}
	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM workspace_members WHERE workspace_id=$1", "must-not-survive").Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphan membership count=%d err=%v", count, err)
	}
}

func TestWorkspaceOwnerQueriesDuplicatePreservesExisting(t *testing.T) {
	ctx, q, _ := newWorkspaceOwnerQueryTest(t)
	x, y := uuid.New(), uuid.New()
	for index, id := range []uuid.UUID{x, y} {
		email := "x@example.com"
		if index == 1 {
			email = "y@example.com"
		}
		if _, err := q.CreateUser(ctx, pgstore.CreateUserParams{ID: id, Email: email, Role: "member", Status: "active"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.CreateWorkspaceWithOwner(ctx, pgstore.CreateWorkspaceWithOwnerParams{WorkspaceID: "same-id", Name: "原空间", UserID: x}); err != nil {
		t.Fatal(err)
	}
	err := q.CreateWorkspaceWithOwner(ctx, pgstore.CreateWorkspaceWithOwnerParams{WorkspaceID: "same-id", Name: "覆盖尝试", UserID: y})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate workspace ID must fail, got %v", err)
	}
	ws, err := q.GetWorkspaceByID(ctx, "same-id")
	if err != nil || ws.Name != "原空间" {
		t.Fatal("duplicate creation changed existing workspace")
	}
	members, err := q.ListWorkspaceMembers(ctx, "same-id")
	if err != nil || len(members) != 1 || members[0].UserID != x || members[0].Role != "owner" {
		t.Fatal("duplicate creation changed existing owner")
	}
}
