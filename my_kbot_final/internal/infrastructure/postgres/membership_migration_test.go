package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestWorkspaceMembersMigration(t *testing.T) {
	setup := func(t *testing.T) (context.Context, pgx.Tx, string, string) {
		t.Helper()
		ctx, tx := beginIAMMigrationTest(t)
		firstUser, secondUser := uuid.NewString(), uuid.NewString()
		if _, err := tx.Exec(ctx, "INSERT INTO users (id, email) VALUES ($1, $2), ($3, $4)",
			firstUser, "first@example.com", secondUser, "second@example.com"); err != nil {
			t.Fatalf("create users: %v", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO workspaces (id, name) VALUES ('shop', '电商'), ('claims', '理赔')"); err != nil {
			t.Fatalf("create workspaces: %v", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO workspace_members (workspace_id, user_id)
			VALUES ('shop', $1), ('claims', $1), ('shop', $2)`, firstUser, secondUser); err != nil {
			t.Fatalf("create memberships across users and workspaces: %v", err)
		}
		return ctx, tx, firstUser, secondUser
	}

	t.Run("unique_pair_and_default_role", func(t *testing.T) {
		ctx, tx, firstUser, _ := setup(t)
		var role string
		if err := tx.QueryRow(ctx, "SELECT role FROM workspace_members WHERE workspace_id = 'shop' AND user_id = $1", firstUser).Scan(&role); err != nil {
			t.Fatalf("query membership: %v", err)
		}
		if role != "member" {
			t.Fatalf("default workspace role = %q, want member", role)
		}
		_, err := tx.Exec(ctx, "INSERT INTO workspace_members (workspace_id, user_id) VALUES ('shop', $1)", firstUser)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("duplicate membership should violate the primary key, got %v", err)
		}
	})

	t.Run("deleting_user_or_workspace_removes_only_related_memberships", func(t *testing.T) {
		ctx, tx, firstUser, secondUser := setup(t)
		if _, err := tx.Exec(ctx, "DELETE FROM users WHERE id = $1", firstUser); err != nil {
			t.Fatalf("delete first user: %v", err)
		}
		var remaining, workspaces int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM workspace_members").Scan(&remaining); err != nil || remaining != 1 {
			t.Fatalf("want only second user's membership to remain: count=%d err=%v", remaining, err)
		}
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM workspaces").Scan(&workspaces); err != nil || workspaces != 2 {
			t.Fatalf("deleting a user must preserve workspaces: count=%d err=%v", workspaces, err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM workspaces WHERE id = 'shop'"); err != nil {
			t.Fatalf("delete workspace: %v", err)
		}
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM workspace_members").Scan(&remaining); err != nil || remaining != 0 {
			t.Fatalf("workspace's memberships should be removed: count=%d err=%v", remaining, err)
		}
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE id = $1", secondUser).Scan(&remaining); err != nil || remaining != 1 {
			t.Fatalf("deleting a workspace must preserve its user: count=%d err=%v", remaining, err)
		}
	})
}
