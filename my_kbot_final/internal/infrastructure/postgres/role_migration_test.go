package postgres

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRolesMigration(t *testing.T) {
	ctx, tx := beginIAMMigrationTest(t)
	id := uuid.NewString()
	var permissions []string
	var createdAt time.Time
	err := tx.QueryRow(ctx, `
		INSERT INTO roles (id, name) VALUES ($1, $2)
		RETURNING permissions, created_at`, id, "知识库只读角色",
	).Scan(&permissions, &createdAt)
	if err != nil {
		t.Fatalf("create role with default permissions: %v", err)
	}
	if permissions == nil || len(permissions) != 0 {
		t.Fatalf("default permissions should be a non-NULL empty array, got %#v", permissions)
	}
	if createdAt.IsZero() {
		t.Fatal("database did not populate role timestamp")
	}

	want := []string{"kb:read", "tool:read"}
	if _, err := tx.Exec(ctx, "UPDATE roles SET permissions = $2 WHERE id = $1", id, want); err != nil {
		t.Fatalf("save permission array: %v", err)
	}
	if err := tx.QueryRow(ctx, "SELECT permissions FROM roles WHERE id = $1", id).Scan(&permissions); err != nil {
		t.Fatalf("read permission array: %v", err)
	}
	if !slices.Equal(permissions, want) {
		t.Fatalf("permission array = %#v, want %#v", permissions, want)
	}
}
