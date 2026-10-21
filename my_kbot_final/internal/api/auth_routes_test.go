package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"my_kbot_final/internal/domain"
	"my_kbot_final/internal/platform/iam"
)

type routeLoginStoreStub struct {
	iam.Store
	calls int
}

func (s *routeLoginStoreStub) GetUserByEmail(context.Context, string) (*domain.User, error) {
	s.calls++
	return nil, errors.New("user not found")
}

func TestLoginRoute(t *testing.T) {
	tests := []struct {
		name, method, body  string
		status, lookupCalls int
	}{
		{"malformed JSON", http.MethodPost, `{"email":`, http.StatusBadRequest, 0},
		{"failed credentials", http.MethodPost, `{"email":"missing@example.com","password":"password123"}`, http.StatusUnauthorized, 1},
		{"wrong method", http.MethodGet, "", http.StatusMethodNotAllowed, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &routeLoginStoreStub{}
			service := iam.NewService(store, []byte("learning-test-key-with-at-least-32-bytes"))
			router := NewHandler(service).Routes()
			request := httptest.NewRequest(tc.method, "/api/v1/auth/login", strings.NewReader(tc.body))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status || store.calls != tc.lookupCalls {
				t.Fatalf("login route status=%d calls=%d, want status=%d calls=%d", response.Code, store.calls, tc.status, tc.lookupCalls)
			}
		})
	}
}
