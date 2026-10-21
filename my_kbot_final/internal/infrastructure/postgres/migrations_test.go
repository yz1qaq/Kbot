package postgres

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// 在独立 schema 和回滚事务中检查 SQL，不安装正式迁移或保留测试用户。
func TestUsersMigration(t *testing.T) {
	ctx, tx := beginIAMMigrationTest(t)

	var role, status, passwordHash, name string
	var createdAt, updatedAt time.Time
	err := tx.QueryRow(ctx, `
		INSERT INTO users (id, email) VALUES ($1, $2)
		RETURNING role, status, password_hash, name, created_at, updated_at`,
		uuid.NewString(), "schema-check@example.com",
	).Scan(&role, &status, &passwordHash, &name, &createdAt, &updatedAt)
	if err != nil {
		t.Fatalf("insert user with database defaults: %v", err)
	}
	if role != "member" || status != "active" || passwordHash != "" || name != "" {
		t.Fatalf("unexpected defaults: role=%q status=%q passwordHash=%q name=%q", role, status, passwordHash, name)
	}
	if createdAt.IsZero() || updatedAt.IsZero() {
		t.Fatal("database did not populate timestamps")
	}

	_, err = tx.Exec(ctx, "INSERT INTO users (id, email) VALUES ($1, $2)",
		uuid.NewString(), "schema-check@example.com")
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate email should violate a unique constraint, got %v", err)
	}
}

// 删除父空间后，子空间应继续存在，并解除父子关系。
func TestWorkspacesMigration(t *testing.T) {
	ctx, tx := beginIAMMigrationTest(t)
	if _, err := tx.Exec(ctx, "INSERT INTO workspaces (id, name) VALUES ($1, $2)",
		"operations", "运营中心"); err != nil {
		t.Fatalf("insert parent workspace: %v", err)
	}

	var description string
	var parentID *string
	var createdAt, updatedAt time.Time
	err := tx.QueryRow(ctx, `
		INSERT INTO workspaces (id, name, parent_id) VALUES ($1, $2, $3)
		RETURNING description, parent_id, created_at, updated_at`,
		"operations-cn", "中国区运营", "operations",
	).Scan(&description, &parentID, &createdAt, &updatedAt)
	if err != nil {
		t.Fatalf("insert child workspace: %v", err)
	}
	if description != "" || parentID == nil || *parentID != "operations" {
		t.Fatalf("unexpected workspace: description=%q parentID=%v", description, parentID)
	}
	if createdAt.IsZero() || updatedAt.IsZero() {
		t.Fatal("database did not populate workspace timestamps")
	}

	if _, err := tx.Exec(ctx, "DELETE FROM workspaces WHERE id = $1", "operations"); err != nil {
		t.Fatalf("delete parent workspace: %v", err)
	}
	if err := tx.QueryRow(ctx, "SELECT parent_id FROM workspaces WHERE id = $1", "operations-cn").Scan(&parentID); err != nil {
		t.Fatalf("child should survive deletion of its parent: %v", err)
	}
	if parentID != nil {
		t.Fatalf("parent_id should become NULL, got %q", *parentID)
	}
}

func beginIAMMigrationTest(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL to test the IAM schema with PostgreSQL")
	}
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatalf("read IAM migration: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	schema := pgx.Identifier{"kbot_iam_test_" + uuid.NewString()}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('search_path', $1, true)", schema+", pg_catalog"); err != nil {
		t.Fatalf("set transaction search path: %v", err)
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("execute IAM migration: %v", err)
	}

	return ctx, tx
}
