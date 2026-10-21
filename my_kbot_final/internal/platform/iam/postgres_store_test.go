package iam

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"my_kbot_final/internal/domain"
	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newPostgresUserStoreTest(t *testing.T) (context.Context, *PostgresStore) {
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
	return ctx, NewPostgresStore(pool, pgstore.New(tx))
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
