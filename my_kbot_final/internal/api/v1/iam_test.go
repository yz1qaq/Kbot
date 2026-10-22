package v1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"my_kbot_final/internal/domain"
	"my_kbot_final/internal/platform/iam"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- iam_test.go ----

type listUsersStoreStub struct {
	iam.Store
	list func(context.Context, int32, int32) ([]*domain.User, error)
}

func (s *listUsersStoreStub) ListUsers(ctx context.Context, limit, offset int32) ([]*domain.User, error) {
	return s.list(ctx, limit, offset)
}

func TestIAMListUsers(t *testing.T) {
	for _, tc := range []struct {
		name, query   string
		limit, offset int32
		empty         bool
	}{
		{"default page", "", 50, 0, false},
		{"explicit page", "?limit=20&offset=40", 20, 40, false},
		{"empty page", "?limit=20&offset=40", 20, 40, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type key struct{}
			ctx := context.WithValue(context.Background(), key{}, "request")
			calls := 0
			store := &listUsersStoreStub{list: func(got context.Context, limit, offset int32) ([]*domain.User, error) {
				calls++
				if got.Value(key{}) != "request" || limit != tc.limit || offset != tc.offset {
					t.Fatalf("context/page not forwarded: limit=%d offset=%d", limit, offset)
				}
				if tc.empty {
					return []*domain.User{}, nil
				}
				return []*domain.User{{ID: "user-1", Email: "alice@example.com", Name: "小明", Role: "member", PasswordHash: "hidden-password-hash"}}, nil
			}}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/users"+tc.query, nil).WithContext(ctx)
			NewIAMHandler(iam.NewService(store, nil)).ListUsers(response, request)
			if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" || calls != 1 {
				t.Fatalf("unexpected response status=%d calls=%d", response.Code, calls)
			}
			var users []domain.User
			if err := json.Unmarshal(response.Body.Bytes(), &users); err != nil || users == nil {
				t.Fatalf("response must be a JSON array, body=%q err=%v", response.Body.String(), err)
			}
			if tc.empty {
				if len(users) != 0 {
					t.Fatal("empty page must be []")
				}
			} else if len(users) != 1 || users[0].ID != "user-1" || users[0].Name != "小明" {
				t.Fatal("user fields lost from response")
			}
			if strings.Contains(response.Body.String(), "hidden-password-hash") || strings.Contains(response.Body.String(), "password_hash") {
				t.Fatal("list response must not expose password hashes")
			}
		})
	}
}

func TestIAMListUsersRejectsPaginationBeforeStore(t *testing.T) {
	for _, query := range []string{"?limit=0", "?limit=201", "?limit=abc", "?offset=-1", "?offset=2147483648"} {
		response := httptest.NewRecorder()
		// nil dependency catches accidental Service access before rejecting input.
		NewIAMHandler(nil).ListUsers(response, httptest.NewRequest(http.MethodGet, "/api/v1/users"+query, nil))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s status=%d", query, response.Code)
		}
	}
}

func TestIAMListUsersStoreFailure(t *testing.T) {
	store := &listUsersStoreStub{list: func(context.Context, int32, int32) ([]*domain.User, error) {
		return nil, errors.New("list unavailable")
	}}
	response := httptest.NewRecorder()
	NewIAMHandler(iam.NewService(store, nil)).ListUsers(response, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", response.Code)
	}
}

// ---- iam_error_test.go ----

func TestWriteIAMError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"forbidden", iam.ErrForbidden, http.StatusForbidden},
		{"wrapped forbidden", fmt.Errorf("member access: %w", iam.ErrForbidden), http.StatusForbidden},
		{"last owner", iam.ErrLastOwner, http.StatusConflict},
		{"wrapped last owner", fmt.Errorf("member write: %w", iam.ErrLastOwner), http.StatusConflict},
		{"invalid workspace role", errors.New(`invalid workspace role "root"`), http.StatusBadRequest},
		{"short error", errors.New("bad"), http.StatusInternalServerError},
		{"store error", errors.New("list failed"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeIAMError(w, tc.err)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s, want %d", w.Code, w.Body.String(), tc.status)
			}
		})
	}
}
