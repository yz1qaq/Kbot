package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"my_kbot_final/internal/domain"
	"my_kbot_final/internal/platform/iam"

	"golang.org/x/crypto/bcrypt"
)

type authLoginStoreStub struct {
	iam.Store
	lookup func(context.Context, string) (*domain.User, error)
}

func (s *authLoginStoreStub) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.lookup(ctx, email)
}

func TestAuthLoginInvalidBody(t *testing.T) {
	for _, body := range []string{
		"", `{"email":`, `{"email":"alice@example.com","role":"admin"}`,
		`{"email":"alice@example.com"} {}`, `{"email":123}`,
		`{"email":"` + strings.Repeat("a", 1<<20) + `"}`,
	} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		// 无效请求应在调用 Service 前返回；nil 依赖会暴露错误的执行顺序。
		NewAuthHandler(nil).Login(response, request)
		if response.Code != http.StatusBadRequest || response.Body.String() != "invalid request body\n" {
			t.Fatal("malformed requests must return a generic 400 before accessing the login service")
		}
	}
}

func TestAuthLoginSuccess(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{ID: "user-1", Email: "alice@example.com", Name: "小明", Role: "member", Status: "active", PasswordHash: string(hash)}
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "http-request-context")
	calls := 0
	store := &authLoginStoreStub{lookup: func(got context.Context, email string) (*domain.User, error) {
		calls++
		if got.Value(ctxKey{}) != "http-request-context" || email != "alice@example.com" {
			t.Fatal("handler must forward request context and login input to the Service")
		}
		return user, nil
	}}
	service := iam.NewService(store, []byte("learning-test-key-with-at-least-32-bytes"))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":" ALICE@Example.COM ","password":"password123"}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	NewAuthHandler(service).Login(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/json" || calls != 1 {
		t.Fatal("successful login must return JSON with status 200 and perform one lookup")
	}
	var result iam.LoginResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode HTTP login response: %v", err)
	}
	if result.User == nil || result.User.ID != user.ID || result.User.Name != user.Name || result.Token == "" || result.ExpiresAt.IsZero() {
		t.Fatal("login JSON must include token, expiry and user")
	}
	claims, err := service.ValidateToken(result.Token)
	if err != nil || claims == nil || claims.UserID != user.ID {
		t.Fatal("HTTP response must contain a verifiable token for the logged-in user")
	}
	if strings.Contains(response.Body.String(), "password123") || strings.Contains(response.Body.String(), string(hash)) || strings.Contains(response.Body.String(), "password_hash") {
		t.Fatal("HTTP login response must not expose password material")
	}
}

func TestAuthLoginRejectsCredentials(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, status, password string
		lookupErr              error
	}{
		{"missing user", "", "password123", errors.New("user not found")},
		{"wrong password", "active", "wrong-password", nil},
		{"inactive user", "inactive", "password123", nil},
		{"internal lookup failure", "", "password123", errors.New("private storage diagnostic")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &authLoginStoreStub{lookup: func(context.Context, string) (*domain.User, error) {
				if tc.lookupErr != nil {
					return nil, tc.lookupErr
				}
				return &domain.User{ID: "user-1", Email: "alice@example.com", Status: tc.status, PasswordHash: string(hash)}, nil
			}}
			service := iam.NewService(store, []byte("learning-test-key-with-at-least-32-bytes"))
			body, err := json.Marshal(iam.LoginRequest{Email: "alice@example.com", Password: tc.password})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(string(body)))
			response := httptest.NewRecorder()
			NewAuthHandler(service).Login(response, request)
			if response.Code != http.StatusUnauthorized || response.Body.String() != "invalid credentials\n" {
				t.Fatal("all login failures must return the same generic 401 response")
			}
		})
	}
}
