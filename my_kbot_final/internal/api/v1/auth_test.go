package v1

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"my_kbot_final/internal/domain"
	"my_kbot_final/internal/platform/iam"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- auth_test.go ----

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

// ---- auth_create_test.go ----

type authCreateUserStoreStub struct {
	iam.Store
	create func(context.Context, *domain.User) error
}

func (s *authCreateUserStoreStub) CreateUser(ctx context.Context, user *domain.User) error {
	return s.create(ctx, user)
}

func TestAuthCreateUserRejectsInvalidInput(t *testing.T) {
	for _, body := range []string{
		"", `{"email":`, `{"email":123}`, `{} {}`,
		`{"email":"alice@example.com","password":"password123","name":"小明","role":"admin"}`,
		`{"email":"invalid","password":"password123","name":"小明"}`,
		`{"email":"alice@example.com","password":"short","name":"小明"}`,
		`{"email":"alice@example.com","password":"password123","name":"  "}`,
	} {
		store := &authCreateUserStoreStub{create: func(context.Context, *domain.User) error {
			t.Fatal("invalid input must not be sent to storage")
			return nil
		}}
		service := iam.NewService(store, nil)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		response := httptest.NewRecorder()
		NewAuthHandler(service).CreateUser(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid input must return 400, got %d", response.Code)
		}
	}
}

func TestAuthCreateUserSuccess(t *testing.T) {
	type traceKey struct{}
	ctx := context.WithValue(context.Background(), traceKey{}, "create-request")
	var saved *domain.User
	store := &authCreateUserStoreStub{create: func(got context.Context, user *domain.User) error {
		if got.Value(traceKey{}) != "create-request" {
			t.Fatal("handler must propagate request context to the Store through the Service")
		}
		saved = user
		return nil
	}}
	service := iam.NewService(store, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":" ALICE@Example.COM ","password":"password123","name":" 小明 "}`)).WithContext(ctx)
	response := httptest.NewRecorder()
	NewAuthHandler(service).CreateUser(response, request)
	if response.Code != http.StatusCreated || response.Header().Get("Content-Type") != "application/json" || saved == nil {
		t.Fatal("successful user creation must store the user and return JSON with status 201")
	}
	var result domain.User
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode created user response: %v", err)
	}
	if result.ID != saved.ID || result.ID == "" || result.Email != "alice@example.com" || result.Name != "小明" || result.Role != "member" || result.Status != "active" {
		t.Fatal("response must contain the created member user with normalized fields")
	}
	if bcrypt.CompareHashAndPassword([]byte(saved.PasswordHash), []byte("password123")) != nil {
		t.Fatal("stored password must be a verifiable bcrypt hash")
	}
	if strings.Contains(response.Body.String(), "password123") || strings.Contains(response.Body.String(), saved.PasswordHash) || strings.Contains(response.Body.String(), "password_hash") {
		t.Fatal("created user JSON must not disclose password material")
	}
}

func TestAuthCreateUserStorageFailure(t *testing.T) {
	store := &authCreateUserStoreStub{create: func(context.Context, *domain.User) error {
		return errors.New("storage unavailable")
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"alice@example.com","password":"password123","name":"小明"}`))
	response := httptest.NewRecorder()
	NewAuthHandler(iam.NewService(store, nil)).CreateUser(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("storage failure must return 500, got %d", response.Code)
	}
}
