package postgres

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRolePermissionsMigration(t *testing.T) {
	ctx, tx := beginIAMMigrationTest(t)
	_, err := tx.Exec(ctx, `
		INSERT INTO role_permissions (role, resource, action) VALUES
		('viewer', 'kb', 'read'),
		('viewer', 'tool', 'read'),
		('editor', 'kb', 'read'),
		('editor', 'kb', 'write')`)
	if err != nil {
		t.Fatalf("store distinct role-resource-action rules: %v", err)
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM role_permissions").Scan(&count); err != nil || count != 4 {
		t.Fatalf("distinct rules should coexist: count=%d err=%v", count, err)
	}
	_, err = tx.Exec(ctx, "INSERT INTO role_permissions (role, resource, action) VALUES ('viewer', 'kb', 'read')")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate permission rule must be rejected, got %v", err)
	}
}
