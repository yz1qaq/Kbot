package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"my_kbot_final/internal/domain"
	"my_kbot_final/internal/platform/iam"

	"github.com/golang-jwt/jwt/v5"
)

func TestRequireGlobalAdmin(t *testing.T) {
	tests := []struct {
		name    string
		role    any
		allowed bool
	}{
		{"admin", "admin", true},
		{"member", "member", false},
		{"workspace owner is not a global admin", "owner", false},
		{"missing", nil, false},
		{"empty", "", false},
		{"wrong value type", 123, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.role != nil {
				ctx = context.WithValue(ctx, ContextKeyGlobalRole, tc.role)
			}
			request := httptest.NewRequest(http.MethodPost, "/admin-only", nil).WithContext(ctx)
			request.Header.Set("X-Global-Role", "admin")
			calls := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r != request {
					t.Fatal("permission middleware should forward the existing request")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			RequireGlobalAdmin()(next).ServeHTTP(response, request)
			if tc.allowed {
				if response.Code != http.StatusNoContent || calls != 1 {
					t.Fatal("global admin must reach the downstream handler once")
				}
			} else if response.Code != http.StatusForbidden || calls != 0 || response.Body.String() != "global admin role required\n" {
				t.Fatal("non-admin must receive 403 without running the protected handler")
			}
		})
	}
}

func TestAuthAndRequireGlobalAdmin(t *testing.T) {
	key := []byte("learning-test-key-with-at-least-32-bytes")
	valid := authSignedToken(t, key, jwt.SigningMethodHS256, time.Now().Add(time.Hour))
	tests := []struct {
		name, token, role, status string
		wantStatus                int
	}{
		{"anonymous", "", "admin", "active", http.StatusUnauthorized},
		{"invalid token", "invalid", "admin", "active", http.StatusUnauthorized},
		{"active member", valid, "member", "active", http.StatusForbidden},
		{"active admin", valid, "admin", "active", http.StatusNoContent},
		{"inactive admin", valid, "admin", "inactive", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &authUserStoreStub{get: func(context.Context, string) (*domain.User, error) {
				return &domain.User{ID: "user-1", Role: tc.role, Status: tc.status}, nil
			}}
			service := iam.NewService(store, key)
			calls := 0
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if GetUserIDFromContext(r.Context()) != "user-1" || GetGlobalRoleFromContext(r.Context()) != "admin" {
					t.Fatal("protected handler must receive the authenticated admin identity")
				}
				w.WriteHeader(http.StatusNoContent)
			})
			handler := Auth(service)(RequireGlobalAdmin()(next))
			request := httptest.NewRequest(http.MethodPost, "/admin-only", nil)
			if tc.token != "" {
				request.Header.Set("Authorization", "Bearer "+tc.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			wantCalls := 0
			if tc.wantStatus == http.StatusNoContent {
				wantCalls = 1
			}
			if response.Code != tc.wantStatus || calls != wantCalls {
				t.Fatalf("middleware chain: status=%d calls=%d, want status=%d calls=%d", response.Code, calls, tc.wantStatus, wantCalls)
			}
		})
	}
}
