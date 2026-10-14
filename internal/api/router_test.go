package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"my_kbot/internal/domain"
	"my_kbot/internal/platform/iam"
)

func TestContextRequiresWorkspace(t *testing.T) {
	ctx := context.Background()
	service := iam.New(
		iam.NewMemoryStore(),
		"0123456789abcdef0123456789abcdef",
		"kbot-test",
	)

	user, err := service.Register(
		ctx, "test@example.com", "password123", "小明",
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	result, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result == nil || result.Token == "" {
		t.Fatal("expected a login result with a token")
	}

	router := NewRouter(service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/context", nil)
	req.Header.Set("Authorization", "Bearer "+result.Token)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	body := strings.TrimSpace(recorder.Body.String())
	if body != "workspace is required" {
		t.Fatalf("body = %q, want workspace is required", body)
	}
}

func TestHTTPAuthFlow(t *testing.T) {
	service := iam.New(
		iam.NewMemoryStore(),
		"0123456789abcdef0123456789abcdef",
		"kbot-test",
	)
	router := NewRouter(service, stubRuntime{})

	send := func(
		method, path, body string,
		headers map[string]string,
	) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}

	// 1. 注册。
	registered := send(
		http.MethodPost,
		"/api/v1/auth/register",
		`{"email":"test@example.com","password":"password123","name":"小明"}`,
		nil,
	)
	if registered.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want 201", registered.Code)
	}

	var user domain.User
	if err := json.NewDecoder(registered.Body).Decode(&user); err != nil {
		t.Fatalf("decode registered user: %v", err)
	}
	if user.ID == "" {
		t.Fatal("expected a registered user ID")
	}

	// 2. 登录。
	loggedIn := send(
		http.MethodPost,
		"/api/v1/auth/login",
		`{"email":"test@example.com","password":"password123"}`,
		nil,
	)
	if loggedIn.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loggedIn.Code)
	}

	var result iam.LoginResult
	if err := json.NewDecoder(loggedIn.Body).Decode(&result); err != nil {
		t.Fatalf("decode login result: %v", err)
	}
	if result.Token == "" || result.User == nil || result.User.ID != user.ID {
		t.Fatal("expected a token and the registered user")
	}

	// 3. 查询空间。
	authHeaders := map[string]string{
		"Authorization": "Bearer " + result.Token,
	}
	listed := send(http.MethodGet, "/api/v1/workspaces", "", authHeaders)
	if listed.Code != http.StatusOK {
		t.Fatalf("workspaces status = %d, want 200", listed.Code)
	}

	var workspaces []domain.Workspace
	if err := json.NewDecoder(listed.Body).Decode(&workspaces); err != nil {
		t.Fatalf("decode workspaces: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0].ID == "" {
		t.Fatal("expected one default workspace with an ID")
	}

	// 4. 查询鉴权上下文。
	response := send(
		http.MethodGet,
		"/api/v1/context",
		"",
		map[string]string{
			"Authorization":  "Bearer " + result.Token,
			"X-Workspace-ID": workspaces[0].ID,
		},
	)
	if response.Code != http.StatusOK {
		t.Fatalf("context status = %d, want 200", response.Code)
	}

	var got map[string]string
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode context: %v", err)
	}
	if got["user_id"] != user.ID ||
		got["workspace_id"] != workspaces[0].ID ||
		got["role"] != iam.WorkspaceRoleOwner {
		t.Fatalf("unexpected context: %+v", got)
	}

	// 5. 携带身份和空间信息，访问流式聊天路由。
	streamed := send(
		http.MethodPost,
		"/stream/agents/demo/chat",
		`{"conversation_id":"demo-conversation","message":"你好"}`,
		map[string]string{
			"Authorization":  "Bearer " + result.Token,
			"X-Workspace-ID": workspaces[0].ID,
		},
	)

	if streamed.Code != http.StatusOK {
		t.Fatalf(
			"stream status = %d, want 200; body = %q",
			streamed.Code,
			streamed.Body.String(),
		)
	}
	if got := streamed.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("stream content type = %q", got)
	}

	want := `data: {"type":"answer_delta","text":"你好"}` + "\n\n"
	if got := streamed.Body.String(); got != want {
		t.Fatalf("stream body = %q, want %q", got, want)
	}
	if !streamed.Flushed {
		t.Fatal("expected stream response to be flushed")
	}

	rejectedCases := []struct {
		name       string
		headers    map[string]string
		wantStatus int
	}{
		{
			name: "missing_token",
			headers: map[string]string{
				"X-Workspace-ID": workspaces[0].ID,
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "missing_workspace",
			headers:    authHeaders,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range rejectedCases {
		t.Run(tt.name, func(t *testing.T) {
			response := send(
				http.MethodPost,
				"/stream/agents/demo/chat",
				`{"conversation_id":"demo-conversation","message":"你好"}`,
				tt.headers,
			)

			if response.Code != tt.wantStatus {
				t.Fatalf(
					"status = %d, want %d; body = %q",
					response.Code,
					tt.wantStatus,
					response.Body.String(),
				)
			}
			if response.Flushed {
				t.Fatal("rejected request must not enter SSE output")
			}
		})
	}
}
