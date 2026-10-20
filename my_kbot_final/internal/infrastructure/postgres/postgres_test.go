package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestOpenRejectsInvalidConfig(t *testing.T) {
	pool, err := Open(context.Background(), "postgres://%zz")
	if pool != nil {
		pool.Close()
		t.Fatal("invalid configuration must not return a usable pool")
	}
	if err == nil || !strings.Contains(err.Error(), "parse database config") {
		t.Fatalf("expected configuration parsing error, got %v", err)
	}
}

func TestOpenWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL to run the real PostgreSQL test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Open(): %v", err)
	}
	defer pool.Close()

	var value int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&value); err != nil {
		t.Fatalf("query through pool: %v", err)
	}
	if value != 1 {
		t.Fatalf("SELECT 1 = %d, want 1", value)
	}
}
