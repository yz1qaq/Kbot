package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"my_kbot_final/internal/domain"
	"my_kbot_final/internal/platform/iam"

	"github.com/golang-jwt/jwt/v5"
)

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
