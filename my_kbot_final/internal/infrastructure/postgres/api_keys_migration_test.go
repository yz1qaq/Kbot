package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

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
