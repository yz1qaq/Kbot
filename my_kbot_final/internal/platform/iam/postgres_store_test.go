package iam

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"my_kbot_final/internal/domain"
	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- postgres_store_test.go ----

func newPostgresUserStoreTest(t *testing.T) (context.Context, *PostgresStore) {
	t.Helper()
	ctx, store, _ := newPostgresStoreTransactionTest(t)
	return ctx, store
}

// 同一个隔离事务也用于准备成员关系等测试数据。
func newPostgresStoreTransactionTest(t *testing.T) (context.Context, *PostgresStore, pgx.Tx) {
	t.Helper()
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL to test the IAM PostgreSQL Store")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create test pool: %v", err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin isolated transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	schema := pgx.Identifier{"kbot_iam_store_" + uuid.NewString()}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create test schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('search_path', $1, true)", schema+", pg_catalog"); err != nil {
		t.Fatalf("set test search path: %v", err)
	}
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatalf("read IAM schema: %v", err)
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("create IAM tables: %v", err)
	}
	return ctx, NewPostgresStore(pool, pgstore.New(tx)), tx
}

func TestPostgresStoreReadUser(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	row, err := store.q.CreateUser(ctx, pgstore.CreateUserParams{
		ID: uuid.New(), Email: "read-user@example.com", Name: "读取测试用户",
		PasswordHash: "test-password-hash", Role: "member", Status: "active",
	})
	if err != nil {
		t.Fatalf("insert fixture user: %v", err)
	}
	lookups := []struct {
		name string
		get  func() (*domain.User, error)
	}{
		{"email", func() (*domain.User, error) { return store.GetUserByEmail(ctx, row.Email) }},
		{"id", func() (*domain.User, error) { return store.GetUserByID(ctx, row.ID.String()) }},
	}
	for _, lookup := range lookups {
		t.Run(lookup.name, func(t *testing.T) {
			got, err := lookup.get()
			if err != nil || got == nil {
				t.Fatalf("read user: user=%v err=%v", got, err)
			}
			if got.ID != row.ID.String() || got.Email != row.Email || got.Name != row.Name || got.Role != row.Role || got.Status != row.Status || got.PasswordHash != row.PasswordHash {
				t.Fatal("database user fields did not survive the Store conversion")
			}
			if got.CreatedAt.IsZero() || !got.CreatedAt.Equal(row.CreatedAt) || !got.UpdatedAt.Equal(row.UpdatedAt) {
				t.Fatal("database timestamps did not survive the Store conversion")
			}
		})
	}
}

func TestPostgresStoreMissingUser(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	lookups := []func() (*domain.User, error){
		func() (*domain.User, error) { return store.GetUserByEmail(ctx, "missing@example.com") },
		func() (*domain.User, error) { return store.GetUserByID(ctx, uuid.NewString()) },
	}
	for _, lookup := range lookups {
		user, err := lookup()
		if user != nil || err == nil || !strings.Contains(err.Error(), "not found") {
			t.Fatalf("missing user should return nil and not-found error: user=%v err=%v", user, err)
		}
	}
}

func TestPostgresStoreRejectsInvalidUserID(t *testing.T) {
	store := NewPostgresStore(nil, nil)
	for _, id := range []string{"", "not-a-uuid"} {
		user, err := store.GetUserByID(context.Background(), id)
		if user != nil || err == nil {
			t.Fatalf("invalid ID %q should fail before any query: user=%v err=%v", id, user, err)
		}
	}
}

func TestPostgresStoreReadPreservesCancellation(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	user, err := store.GetUserByEmail(cancelled, "cancelled@example.com")
	if user != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("read should preserve context cancellation: user=%v err=%v", user, err)
	}
}

// ---- postgres_store_write_test.go ----

func TestPostgresStoreCreateUser(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	id := uuid.New()
	oldTime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	user := &domain.User{
		ID: strings.ReplaceAll(id.String(), "-", ""), Email: "created@example.com",
		Name: "创建测试", PasswordHash: "test-password-hash", CreatedAt: oldTime, UpdatedAt: oldTime,
	}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.ID != id.String() || user.Role != "member" || user.Status != "active" {
		t.Fatalf("canonical ID/defaults were not written back: id=%q role=%q status=%q", user.ID, user.Role, user.Status)
	}
	if user.CreatedAt.IsZero() || user.CreatedAt.Equal(oldTime) || user.UpdatedAt.Equal(oldTime) {
		t.Fatal("database timestamps were not written back to the caller's user")
	}
	got, err := store.GetUserByEmail(ctx, user.Email)
	if err != nil || got == nil {
		t.Fatalf("read created user: user=%v err=%v", got, err)
	}
	if got.ID != user.ID || got.Name != user.Name || got.PasswordHash != user.PasswordHash || !got.CreatedAt.Equal(user.CreatedAt) || !got.UpdatedAt.Equal(user.UpdatedAt) {
		t.Fatal("created user differs from the database record")
	}
}

func TestPostgresStoreDuplicateEmail(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	first := &domain.User{ID: uuid.NewString(), Email: "duplicate@example.com"}
	if err := store.CreateUser(ctx, first); err != nil {
		t.Fatalf("create first user: %v", err)
	}
	err := store.CreateUser(ctx, &domain.User{ID: uuid.NewString(), Email: first.Email})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("duplicate email should preserve PostgreSQL's unique violation, got %v", err)
	}
}

func TestPostgresStoreListAndSetUserRole(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	empty, err := store.ListUsers(ctx, 10, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list should be non-nil: users=%v err=%v", empty, err)
	}
	first := &domain.User{ID: uuid.NewString(), Email: "first@example.com", Role: "admin", Status: "inactive"}
	second := &domain.User{ID: uuid.NewString(), Email: "second@example.com"}
	for _, user := range []*domain.User{first, second} {
		if err := store.CreateUser(ctx, user); err != nil {
			t.Fatalf("create list fixture: %v", err)
		}
	}
	if first.Role != "admin" || first.Status != "inactive" {
		t.Fatal("explicit role/status should not be overwritten by defaults")
	}
	users, err := store.ListUsers(ctx, 10, 0)
	if err != nil || len(users) != 2 {
		t.Fatalf("list users: count=%d err=%v", len(users), err)
	}
	seen := make(map[string]bool)
	for _, user := range users {
		if user == nil {
			t.Fatal("list contains a nil user")
		}
		seen[user.ID] = true
	}
	if !seen[first.ID] || !seen[second.ID] {
		t.Fatal("list lost database user IDs during mapping")
	}
	page, err := store.ListUsers(ctx, 1, 0)
	if err != nil || len(page) != 1 {
		t.Fatalf("limit should constrain page size: count=%d err=%v", len(page), err)
	}
	page, err = store.ListUsers(ctx, 10, 2)
	if err != nil || page == nil || len(page) != 0 {
		t.Fatalf("offset beyond the list should return an empty slice: users=%v err=%v", page, err)
	}
	if err := store.SetUserRole(ctx, second.ID, "admin"); err != nil {
		t.Fatalf("set user role: %v", err)
	}
	updated, err := store.GetUserByID(ctx, second.ID)
	if err != nil || updated == nil || updated.Role != "admin" {
		t.Fatalf("role update did not persist: user=%v err=%v", updated, err)
	}
}

func TestPostgresStoreRejectsInvalidWriteIDs(t *testing.T) {
	store := NewPostgresStore(nil, nil)
	if err := store.CreateUser(context.Background(), &domain.User{ID: "invalid"}); err == nil {
		t.Fatal("create should reject an invalid ID before querying")
	}
	if err := store.SetUserRole(context.Background(), "invalid", "member"); err == nil {
		t.Fatal("role update should reject an invalid ID before querying")
	}
}

func TestPostgresStorePersistsAfterReconnect(t *testing.T) {
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL to test persistence across pools")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer admin.Close(context.Background())
	schema := pgx.Identifier{"kbot_iam_persist_" + uuid.NewString()}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create persistence schema: %v", err)
	}
	defer func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("cleanup persistence schema: %v", err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ", pg_catalog"
	firstPool, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatalf("create first pool: %v", err)
	}
	defer firstPool.Close()
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatalf("read IAM schema: %v", err)
	}
	if _, err := firstPool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("create isolated persistence tables: %v", err)
	}
	user := &domain.User{ID: uuid.NewString(), Email: "persisted@example.com", Name: "持久化测试"}
	firstStore := NewPostgresStore(firstPool, pgstore.New(firstPool))
	if err := firstStore.CreateUser(ctx, user); err != nil {
		t.Fatalf("create committed user: %v", err)
	}
	firstPool.Close()

	secondPool, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatalf("create replacement pool: %v", err)
	}
	defer secondPool.Close()
	secondStore := NewPostgresStore(secondPool, pgstore.New(secondPool))
	got, err := secondStore.GetUserByID(ctx, user.ID)
	if err != nil || got == nil {
		t.Fatalf("read after reconnect: user=%v err=%v", got, err)
	}
	if got.ID != user.ID || got.Email != user.Email || got.Name != user.Name || !got.CreatedAt.Equal(user.CreatedAt) {
		t.Fatal("committed user changed after reconnecting with a new pool and Store")
	}
}
