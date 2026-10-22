package api

import (
	"context"
	"errors"
	"my_kbot_final/internal/infrastructure/postgres"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"
)

// ---- router_test.go ----

func TestHealthz(t *testing.T) {
	handler := &Handler{}
	router := handler.Routes()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if response.Body.String() != "ok" {
		t.Fatalf("body = %q, want ok", response.Body.String())
	}
}

// ---- readiness_test.go ----

func TestReadinessHealthy(t *testing.T) {
	var calls []string
	deadlinePresent := false
	handler := (&Handler{}).SetReadiness(
		ReadinessCheck{Name: "unused"},
		ReadinessCheck{Name: "postgres", Check: func(ctx context.Context) error {
			_, deadlinePresent = ctx.Deadline()
			calls = append(calls, "postgres")
			return nil
		}},
		ReadinessCheck{Name: "redis", Check: func(context.Context) error {
			calls = append(calls, "redis")
			return nil
		}},
	)
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK || response.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q, want 200 and ok", response.Code, response.Body.String())
	}
	if !slices.Equal(calls, []string{"postgres", "redis"}) {
		t.Fatalf("checks ran in unexpected order: %v", calls)
	}
	if !deadlinePresent {
		t.Fatal("dependency check should receive a deadline")
	}
}

func TestReadinessDependencyFailure(t *testing.T) {
	failedCheckCalls := 0
	laterCheckCalled := false
	handler := (&Handler{}).SetReadiness(
		ReadinessCheck{Name: "postgres", Check: func(context.Context) error {
			failedCheckCalls++
			return errors.New("database unavailable")
		}},
		ReadinessCheck{Name: "redis", Check: func(context.Context) error {
			laterCheckCalled = true
			return nil
		}},
	)
	router := handler.Routes()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable || response.Body.String() != "not ready: postgres\n" {
		t.Fatalf("status=%d body=%q, want 503 and dependency name", response.Code, response.Body.String())
	}
	if laterCheckCalled {
		t.Fatal("checks after the failed dependency should not run")
	}

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK || health.Body.String() != "ok" {
		t.Fatalf("dependency failure should not break healthz: status=%d body=%q", health.Code, health.Body.String())
	}
	if failedCheckCalls != 1 {
		t.Fatal("healthz must not run dependency checks")
	}
}

func TestReadinessInheritsRequestCancellation(t *testing.T) {
	var observed error
	handler := (&Handler{}).SetReadiness(ReadinessCheck{
		Name: "postgres",
		Check: func(ctx context.Context) error {
			observed = ctx.Err()
			return observed
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, req)
	if !errors.Is(observed, context.Canceled) {
		t.Fatalf("check should inherit canceled context, got %v", observed)
	}
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", response.Code)
	}
}

// ---- readiness_postgres_test.go ----

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
