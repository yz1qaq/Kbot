package postgres

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// ---- migrations_test.go ----

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

// ---- api_keys_migration_test.go ----

func TestAPIKeysMigration(t *testing.T) {
	setup := func(t *testing.T) (context.Context, pgx.Tx, string) {
		t.Helper()
		ctx, tx := beginIAMMigrationTest(t)
		userID := uuid.NewString()
		if _, err := tx.Exec(ctx, "INSERT INTO users (id, email) VALUES ($1, 'api-key@example.com')", userID); err != nil {
			t.Fatalf("create key owner: %v", err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO api_keys (id, user_id, hash) VALUES ($1, $2, 'test-digest-1')", uuid.NewString(), userID); err != nil {
			t.Fatalf("create API key record: %v", err)
		}
		return ctx, tx, userID
	}

	t.Run("defaults_and_unique_hash", func(t *testing.T) {
		ctx, tx, userID := setup(t)
		var name string
		var scopes []string
		var lastUsedAt, expiresAt *time.Time
		var createdAt time.Time
		err := tx.QueryRow(ctx, `
			SELECT name, scopes, last_used_at, expires_at, created_at
			FROM api_keys WHERE user_id = $1`, userID,
		).Scan(&name, &scopes, &lastUsedAt, &expiresAt, &createdAt)
		if err != nil {
			t.Fatalf("read API key defaults: %v", err)
		}
		if name != "" || scopes == nil || len(scopes) != 0 || lastUsedAt != nil || expiresAt != nil || createdAt.IsZero() {
			t.Fatalf("unexpected defaults: name=%q scopes=%#v lastUsed=%v expires=%v created=%v", name, scopes, lastUsedAt, expiresAt, createdAt)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO api_keys (id, user_id, hash) VALUES ($1, $2, 'test-digest-2')", uuid.NewString(), userID); err != nil {
			t.Fatalf("same user should be able to own a second distinct key: %v", err)
		}
		_, err = tx.Exec(ctx, "INSERT INTO api_keys (id, user_id, hash) VALUES ($1, $2, 'test-digest-1')", uuid.NewString(), userID)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("duplicate key hash should be rejected, got %v", err)
		}
	})

	t.Run("delete_owner_removes_keys", func(t *testing.T) {
		ctx, tx, userID := setup(t)
		if _, err := tx.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Fatalf("delete key owner: %v", err)
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM api_keys").Scan(&count); err != nil || count != 0 {
			t.Fatalf("owner's keys should be removed: count=%d err=%v", count, err)
		}
	})
}

// ---- role_migration_test.go ----

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

// ---- role_permissions_migration_test.go ----

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

// ---- user_roles_migration_test.go ----

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

// ---- membership_migration_test.go ----

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

// ---- down_migration_test.go ----

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
