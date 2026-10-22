package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
	"io"
	"my_kbot_final/internal/domain"
	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"
	"my_kbot_final/internal/platform/iam"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- auth_routes_test.go ----

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

// ---- auth_create_postgres_test.go ----

func TestCreateUserRoutePostgres(t *testing.T) {
	databaseURL := os.Getenv("KBOT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set KBOT_TEST_DATABASE_URL to verify actual HTTP/PG permissions")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	schema := pgx.Identifier{"kbot_iam_route_" + uuid.NewString()}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('search_path',$1,true)", schema+",pg_catalog"); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "000001_init_iam.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	store := iam.NewPostgresStore(pool, pgstore.New(tx))
	service := iam.NewService(store, []byte("learning-test-key-with-at-least-32-bytes"))
	if err := service.EnsureSeedAdmin(ctx, "admin@example.com", "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateUser(ctx, "member@example.com", "password123", "普通用户"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewHandler(service).Routes())
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	type reply struct {
		status      int
		body        []byte
		contentType string
	}
	post := func(path, token, body string) reply {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		// 伪造角色头不能替代数据库中的账号角色。
		request.Header.Set("X-Global-Role", "admin")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatalf("HTTP request failed: %v", err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		if err != nil {
			t.Fatal(err)
		}
		return reply{response.StatusCode, data, response.Header.Get("Content-Type")}
	}
	login := func(email string) iam.LoginResponse {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"email": email, "password": "password123"})
		response := post("/api/v1/auth/login", "", string(body))
		var result iam.LoginResponse
		if response.status != http.StatusOK || json.Unmarshal(response.body, &result) != nil || result.Token == "" || result.User == nil {
			t.Fatalf("fixture login failed, status=%d", response.status)
		}
		return result
	}
	admin := login("admin@example.com")
	member := login("member@example.com")
	body := `{"email":"new-member@example.com","password":"password123","name":"新成员"}`
	for _, test := range []struct {
		name, token string
		status      int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"invalid token", "not-a-jwt", http.StatusUnauthorized},
		{"member", member.Token, http.StatusForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := post("/api/v1/auth/register", test.token, body)
			if response.status != test.status {
				t.Fatalf("status=%d, want %d", response.status, test.status)
			}
		})
	}
	assertCount := func(want int) {
		t.Helper()
		users, err := store.ListUsers(ctx, 100, 0)
		if err != nil || len(users) != want {
			t.Fatalf("unexpected stored user count: got %d, want %d, err=%v", len(users), want, err)
		}
	}
	assertCount(2)
	for _, invalid := range []string{
		`{"email":`,
		`{"email":"new-member@example.com","password":"password123","name":" "}`,
		`{"email":"new-member@example.com","password":"password123","name":"新成员","role":"admin"}`,
	} {
		if response := post("/api/v1/auth/register", admin.Token, invalid); response.status != http.StatusBadRequest {
			t.Fatalf("admin invalid input must return 400, got %d", response.status)
		}
	}
	assertCount(2)
	created := post("/api/v1/auth/register", admin.Token, body)
	var user domain.User
	if created.status != http.StatusCreated || created.contentType != "application/json" || json.Unmarshal(created.body, &user) != nil || user.ID == "" || user.Role != "member" {
		t.Fatalf("admin creation must return member user with status 201, got %d", created.status)
	}
	stored, err := store.GetUserByID(ctx, user.ID)
	if err != nil || stored == nil {
		t.Fatalf("read created database user: %v", err)
	}
	if stored.Email != "new-member@example.com" || bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("password123")) != nil {
		t.Fatal("created user must be persisted with a bcrypt password")
	}
	if strings.Contains(string(created.body), "password123") || strings.Contains(string(created.body), stored.PasswordHash) {
		t.Fatal("creation response must not expose password material")
	}
	assertCount(3)
	newMember := login("new-member@example.com")
	if response := post("/api/v1/auth/register", newMember.Token, body); response.status != http.StatusForbidden {
		t.Fatal("newly created member must not gain permission to create users")
	}

	// 查询用户列表也必须经过相同的认证和管理员门禁。
	t.Run("list users through HTTP and PostgreSQL", func(t *testing.T) {
		get := func(path, token string) reply {
			t.Helper()
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("X-Global-Role", "admin")
			if token != "" {
				request.Header.Set("Authorization", "Bearer "+token)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			if err != nil {
				t.Fatal(err)
			}
			return reply{response.StatusCode, data, response.Header.Get("Content-Type")}
		}
		for _, tc := range []struct {
			name, token, path string
			status            int
		}{
			{"anonymous", "", "/api/v1/users", http.StatusUnauthorized},
			{"invalid token", "invalid-jwt", "/api/v1/users", http.StatusUnauthorized},
			{"member", member.Token, "/api/v1/users", http.StatusForbidden},
			{"invalid limit", admin.Token, "/api/v1/users?limit=0", http.StatusBadRequest},
			{"invalid offset", admin.Token, "/api/v1/users?offset=-1", http.StatusBadRequest},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if result := get(tc.path, tc.token); result.status != tc.status {
					t.Fatalf("status=%d, want %d", result.status, tc.status)
				}
			})
		}
		for _, tc := range []struct {
			query string
			count int
		}{
			{"", 3}, {"?limit=1&offset=0", 1}, {"?limit=1&offset=2", 1}, {"?limit=1&offset=3", 0},
		} {
			result := get("/api/v1/users"+tc.query, admin.Token)
			var users []domain.User
			if result.status != http.StatusOK || result.contentType != "application/json" || json.Unmarshal(result.body, &users) != nil || users == nil || len(users) != tc.count {
				t.Fatalf("page %s: status=%d count=%d, want %d", tc.query, result.status, len(users), tc.count)
			}
			if strings.Contains(string(result.body), "password_hash") || strings.Contains(string(result.body), stored.PasswordHash) {
				t.Fatal("list response must not expose password hashes")
			}
		}
		assertCount(3)
	})

	if err := store.SetUserRole(ctx, admin.User.ID, iam.GlobalRoleMember); err != nil {
		t.Fatal(err)
	}
	if response := post("/api/v1/auth/register", admin.Token, body); response.status != http.StatusForbidden {
		t.Fatal("old admin token must observe the current demoted role")
	}
	if err := store.SetUserRole(ctx, admin.User.ID, iam.GlobalRoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "UPDATE users SET status='inactive' WHERE id=$1", uuid.MustParse(admin.User.ID)); err != nil {
		t.Fatal(err)
	}
	if response := post("/api/v1/auth/register", admin.Token, body); response.status != http.StatusUnauthorized {
		t.Fatal("old token for an inactive admin must be rejected")
	}
	assertCount(3)
}
