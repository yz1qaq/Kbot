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
