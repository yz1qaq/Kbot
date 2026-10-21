package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestUserRolesMigration(t *testing.T) {
	setup := func(t *testing.T) (context.Context, pgx.Tx, string, string) {
		t.Helper()
		ctx, tx := beginIAMMigrationTest(t)
		userID, firstRole, secondRole := uuid.NewString(), uuid.NewString(), uuid.NewString()
		if _, err := tx.Exec(ctx, "INSERT INTO users (id, email) VALUES ($1, 'roles@example.com')", userID); err != nil {
			t.Fatalf("create user: %v", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO roles (id, name) VALUES ($1, '知识只读'), ($2, '工具只读')", firstRole, secondRole); err != nil {
			t.Fatalf("create role definitions: %v", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO workspaces (id, name) VALUES ('shop', '电商'), ('claims', '理赔')"); err != nil {
			t.Fatalf("create workspaces: %v", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_roles (user_id, role_id, workspace_id)
			VALUES ($1, $2, 'shop'), ($1, $3, 'shop'), ($1, $2, 'claims')`, userID, firstRole, secondRole); err != nil {
			t.Fatalf("assign roles across workspaces: %v", err)
		}
		return ctx, tx, userID, firstRole
	}

	t.Run("unique_triple", func(t *testing.T) {
		ctx, tx, userID, roleID := setup(t)
		_, err := tx.Exec(ctx, "INSERT INTO user_roles (user_id, role_id, workspace_id) VALUES ($1, $2, 'shop')", userID, roleID)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("duplicate user-role-workspace must be rejected, got %v", err)
		}
	})

	for _, entity := range []string{"user", "role", "workspace"} {
		t.Run("delete_"+entity, func(t *testing.T) {
			ctx, tx, userID, roleID := setup(t)
			var err error
			want := 1
			switch entity {
			case "user":
				_, err = tx.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
				want = 0
			case "role":
				_, err = tx.Exec(ctx, "DELETE FROM roles WHERE id = $1", roleID)
			case "workspace":
				_, err = tx.Exec(ctx, "DELETE FROM workspaces WHERE id = 'shop'")
			}
			if err != nil {
				t.Fatalf("delete %s: %v", entity, err)
			}
			var count int
			if err := tx.QueryRow(ctx, "SELECT count(*) FROM user_roles").Scan(&count); err != nil {
				t.Fatalf("count remaining role assignments: %v", err)
			}
			if count != want {
				t.Fatalf("assignments after deleting %s: got %d, want %d", entity, count, want)
			}
		})
	}
}
