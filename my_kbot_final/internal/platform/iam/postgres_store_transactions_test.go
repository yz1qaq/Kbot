package iam

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- postgres_store_workspace_lock_test.go ----

// Two independent sessions observe committed fixture rows; no shared transaction
// can make the lock test accidentally pass without a real database lock.
func newWorkspaceLockConnections(t *testing.T) (context.Context, *pgx.Conn, *pgx.Conn) {
	t.Helper()
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL for real workspace row-lock tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	first, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close(context.Background()) })
	schema := pgx.Identifier{"kbot_workspace_lock_" + uuid.NewString()}.Sanitize()
	if _, err := first.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := first.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("clean lock test schema: %v", err)
		}
	})
	if _, err := first.Exec(ctx, "SELECT set_config('search_path',$1,false)", schema+",pg_catalog"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(ctx, "INSERT INTO workspaces(id,name) VALUES ('space-a','A'),('space-b','B')"); err != nil {
		t.Fatal(err)
	}
	second, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close(context.Background()) })
	if _, err := second.Exec(ctx, "SELECT set_config('search_path',$1,false)", schema+",pg_catalog"); err != nil {
		t.Fatal(err)
	}
	return ctx, first, second
}

func TestLockWorkspaceTransactionLifetime(t *testing.T) {
	for _, release := range []string{"commit", "rollback"} {
		t.Run(release, func(t *testing.T) {
			ctx, first, second := newWorkspaceLockConnections(t)
			tx, err := first.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
			if err := lockWorkspace(ctx, tx, "space-a"); err != nil {
				t.Fatal(err)
			}
			var id string
			// A plain SELECT can read the committed row while the write lock is held.
			if err := second.QueryRow(ctx, "SELECT id FROM workspaces WHERE id=$1", "space-a").Scan(&id); err != nil || id != "space-a" {
				t.Fatalf("ordinary reader blocked/failed: id=%q err=%v", id, err)
			}
			// A different workspace has a different row lock.
			if err := second.QueryRow(ctx, "SELECT id FROM workspaces WHERE id=$1 FOR UPDATE NOWAIT", "space-b").Scan(&id); err != nil {
				t.Fatalf("unrelated workspace blocked: %v", err)
			}
			// NOWAIT gives deterministic evidence of contention without sleeps.
			err = second.QueryRow(ctx, "SELECT id FROM workspaces WHERE id=$1 FOR UPDATE NOWAIT", "space-a").Scan(&id)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
				t.Fatalf("same workspace must remain locked after helper returns: %v", err)
			}
			if release == "commit" {
				err = tx.Commit(ctx)
			} else {
				err = tx.Rollback(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := second.QueryRow(ctx, "SELECT id FROM workspaces WHERE id=$1 FOR UPDATE NOWAIT", "space-a").Scan(&id); err != nil {
				t.Fatalf("transaction end must release lock: %v", err)
			}
		})
	}
}

func TestLockWorkspaceMissing(t *testing.T) {
	ctx, _, tx := newPostgresStoreTransactionTest(t)
	err := lockWorkspace(ctx, tx, "missing-space")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing workspace should return not found, got %v", err)
	}
}

func TestLockWorkspaceCancellation(t *testing.T) {
	ctx, _, tx := newPostgresStoreTransactionTest(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := lockWorkspace(canceled, tx, "space-a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("request cancellation lost: %v", err)
	}
}

// ---- postgres_store_membership_transaction_read_test.go ----

func TestWorkspaceMemberRoleTransaction(t *testing.T) {
	ctx, first, second := newWorkspaceLockConnections(t)
	userID := uuid.New()
	if _, err := first.Exec(ctx, "INSERT INTO users(id,email) VALUES ($1,$2)", userID, "member@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role) VALUES ('space-a',$1,'viewer')", userID); err != nil {
		t.Fatal(err)
	}
	tx, err := first.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if err := lockWorkspace(ctx, tx, "space-a"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"owner", "admin", "editor", "member", "viewer"} {
		if _, err := tx.Exec(ctx, "UPDATE workspace_members SET role=$2 WHERE workspace_id='space-a' AND user_id=$1", userID, want); err != nil {
			t.Fatal(err)
		}
		role, exists, err := workspaceMemberRole(ctx, tx, "space-a", userID)
		if err != nil || !exists || role != want {
			t.Fatalf("role=%q exists=%v err=%v, want %q", role, exists, err, want)
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE workspace_members SET role='owner' WHERE workspace_id='space-a' AND user_id=$1", userID); err != nil {
		t.Fatal(err)
	}
	role, exists, err := workspaceMemberRole(ctx, tx, "space-a", userID)
	if err != nil || !exists || role != "owner" {
		t.Fatal("helper must read the supplied transaction's uncommitted update")
	}
	var outsideRole string
	if err := second.QueryRow(ctx, "SELECT role FROM workspace_members WHERE workspace_id='space-a' AND user_id=$1", userID).Scan(&outsideRole); err != nil || outsideRole != "viewer" {
		t.Fatalf("independent reader must still see committed viewer: role=%q err=%v", outsideRole, err)
	}
	for _, tc := range []struct {
		space string
		user  uuid.UUID
	}{{"space-b", userID}, {"space-a", uuid.New()}, {"missing-space", userID}} {
		role, exists, err := workspaceMemberRole(ctx, tx, tc.space, tc.user)
		if err != nil || exists || role != "" {
			t.Fatalf("absent membership is not a query error: role=%q exists=%v err=%v", role, exists, err)
		}
	}
}

func TestWorkspaceOwnerCountTransaction(t *testing.T) {
	ctx, first, second := newWorkspaceLockConnections(t)
	one, two, editor, other := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{one, two, editor, other} {
		if _, err := first.Exec(ctx, "INSERT INTO users(id,email) VALUES ($1,$2)", id, id.String()+"@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		space string
		user  uuid.UUID
		role  string
	}{{"space-a", one, "owner"}, {"space-a", editor, "editor"}, {"space-b", other, "owner"}} {
		if _, err := first.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role) VALUES ($1,$2,$3)", tc.space, tc.user, tc.role); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := first.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if err := lockWorkspace(ctx, tx, "space-a"); err != nil {
		t.Fatal(err)
	}
	assertCount := func(space string, want int) {
		t.Helper()
		count, err := workspaceOwnerCount(ctx, tx, space)
		if err != nil || count != want {
			t.Fatalf("space=%s count=%d err=%v, want %d", space, count, err, want)
		}
	}
	assertCount("space-a", 1)
	assertCount("space-b", 1)
	assertCount("missing-space", 0)
	if _, err := tx.Exec(ctx, "INSERT INTO workspace_members(workspace_id,user_id,role) VALUES ('space-a',$1,'owner')", two); err != nil {
		t.Fatal(err)
	}
	assertCount("space-a", 2)
	var outsideCount int
	if err := second.QueryRow(ctx, "SELECT COUNT(*) FROM workspace_members WHERE workspace_id='space-a' AND role='owner'").Scan(&outsideCount); err != nil || outsideCount != 1 {
		t.Fatalf("transaction count must differ from committed view: count=%d err=%v", outsideCount, err)
	}
	if _, err := tx.Exec(ctx, "UPDATE workspace_members SET role='viewer' WHERE workspace_id='space-a' AND user_id=$1", one); err != nil {
		t.Fatal(err)
	}
	assertCount("space-a", 1)
	if _, err := tx.Exec(ctx, "DELETE FROM workspace_members WHERE workspace_id='space-a' AND user_id=$1", two); err != nil {
		t.Fatal(err)
	}
	assertCount("space-a", 0)
	assertCount("space-b", 1)
}

func TestWorkspaceMembershipTransactionReadErrors(t *testing.T) {
	ctx, _, tx := newPostgresStoreTransactionTest(t)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if role, exists, err := workspaceMemberRole(canceled, tx, "space-a", uuid.New()); role != "" || exists || !errors.Is(err, context.Canceled) {
		t.Fatalf("role query cancellation lost: role=%q exists=%v err=%v", role, exists, err)
	}
	if _, err := workspaceOwnerCount(canceled, tx, "space-a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("count query cancellation lost: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if role, exists, err := workspaceMemberRole(ctx, tx, "space-a", uuid.New()); role != "" || exists || !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("closed tx is an error, not absence: role=%q exists=%v err=%v", role, exists, err)
	}
	if _, err := workspaceOwnerCount(ctx, tx, "space-a"); !errors.Is(err, pgx.ErrTxClosed) {
		t.Fatalf("count query lost closed transaction error: %v", err)
	}
}
