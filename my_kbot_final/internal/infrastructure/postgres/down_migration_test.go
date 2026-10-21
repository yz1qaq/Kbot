package postgres

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIAMDownMigration(t *testing.T) {
	ctx, tx := beginIAMMigrationTest(t)
	migrationDir := filepath.Join("..", "..", "..", "migrations")
	down, err := os.ReadFile(filepath.Join(migrationDir, "000001_init_iam.down.sql"))
	if err != nil {
		t.Fatalf("read down migration: %v", err)
	}
	tables := []string{"users", "workspaces", "workspace_members", "roles", "user_roles", "role_permissions", "api_keys"}
	assertTableCount := func(want int) {
		t.Helper()
		var count int
		err := tx.QueryRow(ctx, `
			SELECT count(*) FROM pg_catalog.pg_tables
			WHERE schemaname = current_schema() AND tablename = ANY($1::text[])`, tables,
		).Scan(&count)
		if err != nil || count != want {
			t.Fatalf("IAM table count = %d, want %d, err=%v", count, want, err)
		}
	}
	assertTableCount(7)
	if _, err := tx.Exec(ctx, "CREATE TABLE unrelated_test_record (id integer PRIMARY KEY)"); err != nil {
		t.Fatalf("create unrelated table: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := tx.Exec(ctx, string(down)); err != nil {
			t.Fatalf("execute down migration attempt %d: %v", attempt, err)
		}
		assertTableCount(0)
	}
	var unrelatedCount int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM unrelated_test_record").Scan(&unrelatedCount); err != nil {
		t.Fatalf("down migration should preserve unrelated tables: %v", err)
	}
	up, err := os.ReadFile(filepath.Join(migrationDir, "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatalf("read up migration: %v", err)
	}
	if _, err := tx.Exec(ctx, string(up)); err != nil {
		t.Fatalf("recreate IAM tables after down: %v", err)
	}
	assertTableCount(7)
}
