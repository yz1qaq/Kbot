package middleware

import (
	"context"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"my_kbot_final/internal/domain"
	"my_kbot_final/internal/platform/iam"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---- auth_context_test.go ----

func TestIdentityContextValues(t *testing.T) {
	parent := context.Background()
	userContext := context.WithValue(parent, ContextKeyUserID, "user-1")
	ctx := context.WithValue(userContext, ContextKeyGlobalRole, "admin")
	if GetUserIDFromContext(ctx) != "user-1" || GetGlobalRoleFromContext(ctx) != "admin" {
		t.Fatal("context should preserve both identity fields")
	}
	if GetUserIDFromContext(parent) != "" || GetGlobalRoleFromContext(userContext) != "" {
		t.Fatal("adding a value to a derived context must not modify its parent")
	}
	second := context.WithValue(parent, ContextKeyUserID, "user-2")
	if GetUserIDFromContext(second) != "user-2" || GetUserIDFromContext(ctx) != "user-1" {
		t.Fatal("independent request contexts must not overwrite each other's identities")
	}
}

func TestIdentityContextMissingOrWrongType(t *testing.T) {
	contexts := []struct {
		name string
		ctx  context.Context
	}{
		{"missing", context.Background()},
		{"incorrect value types", context.WithValue(context.WithValue(context.Background(), ContextKeyUserID, 123), ContextKeyGlobalRole, true)},
		// 同名普通字符串不等于 ContextKey 类型；模拟其他代码使用同名 key。
		{"plain string keys", context.WithValue(context.WithValue(context.Background(), "user_id", "unexpected-user"), "global_role", "admin")},
	}
	for _, tc := range contexts {
		t.Run(tc.name, func(t *testing.T) {
			if GetUserIDFromContext(tc.ctx) != "" || GetGlobalRoleFromContext(tc.ctx) != "" {
				t.Fatal("missing or incorrectly typed identity values must be treated as absent")
			}
		})
	}
}

func TestIdentityContextPreservesCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := context.WithValue(parent, ContextKeyUserID, "user-1")
	cancel()
	if ctx.Err() != context.Canceled || GetUserIDFromContext(ctx) != "user-1" {
		t.Fatal("identity context must inherit cancellation while retaining its value")
	}
}

// ---- auth_test.go ----

type authUserStoreStub struct {
	iam.Store
	get func(context.Context, string) (*domain.User, error)
}

func (s *authUserStoreStub) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	return s.get(ctx, id)
}

func authSignedToken(t *testing.T, key []byte, method jwt.SigningMethod, expiresAt time.Time) string {
	t.Helper()
	claims := &iam.Claims{
		UserID: "user-1", Email: "alice@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "kbot", Subject: "user-1",
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAuthUsesCurrentUserIdentity(t *testing.T) {
	key := []byte("learning-test-key-with-at-least-32-bytes")
	token := authSignedToken(t, key, jwt.SigningMethodHS256, time.Now().Add(time.Hour))
	type traceKey struct{}
	parent := context.WithValue(context.Background(), traceKey{}, "request-trace")
	currentRole := "admin"
	lookups := 0
	store := &authUserStoreStub{get: func(ctx context.Context, id string) (*domain.User, error) {
		lookups++
		if id != "user-1" || ctx.Value(traceKey{}) != "request-trace" {
			t.Fatal("Auth must query the token identity and propagate the request context")
		}
		return &domain.User{ID: id, Role: currentRole, Status: "active"}, nil
	}}
	service := iam.NewService(store, key)
	var roles []string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if GetUserIDFromContext(r.Context()) != "user-1" || r.Context().Value(traceKey{}) != "request-trace" {
			t.Fatal("downstream handler did not receive the authenticated identity and original context")
		}
		roles = append(roles, GetGlobalRoleFromContext(r.Context()))
		w.WriteHeader(http.StatusNoContent)
	})
	handler := Auth(service)(next)
	for _, role := range []string{"admin", "member"} {
		currentRole = role
		request := httptest.NewRequest(http.MethodGet, "/protected", nil).WithContext(parent)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-User-ID", "forged-user")
		request.Header.Set("X-Global-Role", "admin")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || GetUserIDFromContext(request.Context()) != "" {
			t.Fatal("Auth must call next with a derived request without modifying the original context")
		}
	}
	if lookups != 2 || len(roles) != 2 || roles[0] != "admin" || roles[1] != "member" {
		t.Fatal("reusing a token must still observe the current stored role on every request")
	}
}

func TestAuthRejectsUnauthenticatedRequests(t *testing.T) {
	key := []byte("learning-test-key-with-at-least-32-bytes")
	valid := authSignedToken(t, key, jwt.SigningMethodHS256, time.Now().Add(time.Hour))
	tests := []struct {
		name, header, status string
		lookupErr            error
		wantLookups          int
	}{
		{name: "missing header"},
		{name: "wrong scheme", header: "Basic abc"},
		{name: "empty bearer", header: "Bearer "},
		{name: "invalid token", header: "Bearer not-a-jwt"},
		{name: "wrong key", header: "Bearer " + authSignedToken(t, []byte("wrong-key"), jwt.SigningMethodHS256, time.Now().Add(time.Hour))},
		{name: "wrong algorithm", header: "Bearer " + authSignedToken(t, key, jwt.SigningMethodHS384, time.Now().Add(time.Hour))},
		{name: "expired token", header: "Bearer " + authSignedToken(t, key, jwt.SigningMethodHS256, time.Now().Add(-time.Hour))},
		{name: "deleted user", header: "Bearer " + valid, lookupErr: errors.New("user not found"), wantLookups: 1},
		{name: "inactive user", header: "Bearer " + valid, status: "inactive", wantLookups: 1},
		{name: "storage error", header: "Bearer " + valid, lookupErr: errors.New("private storage diagnostic"), wantLookups: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookups := 0
			store := &authUserStoreStub{get: func(context.Context, string) (*domain.User, error) {
				lookups++
				if tc.lookupErr != nil {
					return nil, tc.lookupErr
				}
				return &domain.User{ID: "user-1", Status: tc.status}, nil
			}}
			service := iam.NewService(store, key)
			nextCalled := false
			handler := Auth(service)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true }))
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tc.header != "" {
				request.Header.Set("Authorization", tc.header)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || nextCalled || lookups != tc.wantLookups {
				t.Fatalf("rejected request: status=%d nextCalled=%t lookups=%d", response.Code, nextCalled, lookups)
			}
			if strings.Contains(response.Body.String(), "private storage diagnostic") {
				t.Fatal("Auth must not disclose storage diagnostics")
			}
		})
	}
}

// ---- admin_test.go ----

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
