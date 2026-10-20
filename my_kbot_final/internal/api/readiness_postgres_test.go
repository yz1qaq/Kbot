package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"my_kbot_final/internal/infrastructure/postgres"
)

func TestReadinessWithPostgres(t *testing.T) {
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL to test readiness with real PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := postgres.Open(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer pool.Close()

	handler := (&Handler{}).SetReadiness(ReadinessCheck{
		Name: "postgres", Check: pool.Ping,
	})
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK || response.Body.String() != "ok" {
		t.Fatalf("readiness with real database: status=%d body=%q", response.Code, response.Body.String())
	}
}
