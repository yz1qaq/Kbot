package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"my_kbot/internal/domain"
	"my_kbot/internal/platform/iam"
)

func TestAuth(t *testing.T) {
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

	result, err := service.Login(
		ctx, "test@example.com", "password123",
	)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result == nil {
		t.Fatal("expected a login result")
	}

	tests := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{
			name:       "有效令牌",
			header:     "Bearer " + result.Token,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "缺少令牌",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "无效令牌",
			header:     "Bearer invalid-token",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			var receivedUserID string

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				receivedUserID = UserID(r.Context())
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			recorder := httptest.NewRecorder()

			wrapped := Chain(handler, Auth(service))
			wrapped.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}

			wantCalled := tt.wantStatus == http.StatusNoContent
			if called != wantCalled {
				t.Fatalf("handler called = %v, want %v", called, wantCalled)
			}
			if called && receivedUserID != user.ID {
				t.Fatalf("user ID = %q, want %q", receivedUserID, user.ID)
			}
		})
	}
}

func TestWorkspaceMethodAllowed(t *testing.T) {
	tests := []struct {
		name   string
		role   string
		method string
		path   string
		want   bool
	}{
		{"owner可以审批", iam.WorkspaceRoleOwner, http.MethodPost, "/api/approvals/001", true},
		{"admin可以审批", iam.WorkspaceRoleAdmin, http.MethodPost, "/api/approvals/001", true},
		{"editor不能审批", iam.WorkspaceRoleEditor, http.MethodPost, "/api/approvals/001", false},
		{"editor可以修改", iam.WorkspaceRoleEditor, http.MethodPost, "/api/agents", true},
		{"viewer可以GET", iam.WorkspaceRoleViewer, http.MethodGet, "/api/agents", true},
		{"viewer可以HEAD", iam.WorkspaceRoleViewer, http.MethodHead, "/api/agents", true},
		{"viewer不能运行", iam.WorkspaceRoleViewer, http.MethodPost, "/api/chat", false},
		{"member可以聊天", iam.WorkspaceRoleMember, http.MethodPost, "/api/chat", true},
		{"member可以搜索", iam.WorkspaceRoleMember, http.MethodPost, "/api/search", true},
		{"member可以运行", iam.WorkspaceRoleMember, http.MethodPost, "/api/runs", true},
		{"member不能修改配置", iam.WorkspaceRoleMember, http.MethodPost, "/api/agents", false},
		{"member可以访问流式路径", iam.WorkspaceRoleMember, http.MethodPost, "/api/stream/run-001", true},
		{"仅以stream结尾不放行", iam.WorkspaceRoleMember, http.MethodPost, "/api/stream", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workspaceMethodAllowed(tt.role, tt.method, tt.path)
			if got != tt.want {
				t.Fatalf(
					"allowed(%q, %q, %q) = %v, want %v",
					tt.role, tt.method, tt.path, got, tt.want,
				)
			}
		})
	}
}

func TestWorkspacePassesContext(t *testing.T) {
	ctx := context.Background()
	service := iam.New(
		iam.NewMemoryStore(),
		"0123456789abcdef0123456789abcdef",
		"kbot-test",
	)

	user, err := service.Register(
		ctx, "owner@example.com", "password123", "小明",
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	workspaces, err := service.ListUserWorkspaces(ctx, user.ID)
	if err != nil {
		t.Fatalf("list workspaces: %v", err)
	}
	if len(workspaces) != 1 || workspaces[0] == nil {
		t.Fatal("expected one default workspace")
	}
	workspace := workspaces[0]

	result, err := service.Login(
		ctx, user.Email, "password123",
	)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result == nil {
		t.Fatal("expected a login result")
	}

	var gotUserID, gotWorkspaceID, gotRole string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = UserID(r.Context())
		gotWorkspaceID = WorkspaceID(r.Context())
		gotRole = WorkspaceRole(r.Context())
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer "+result.Token)
	req.Header.Set("X-Workspace-ID", workspace.ID)
	recorder := httptest.NewRecorder()

	wrapped := Chain(handler, Auth(service), Workspace(service))
	wrapped.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if gotUserID != user.ID {
		t.Fatalf("user ID = %q, want %q", gotUserID, user.ID)
	}
	if gotWorkspaceID != workspace.ID {
		t.Fatalf("workspace ID = %q, want %q", gotWorkspaceID, workspace.ID)
	}
	if gotRole != iam.WorkspaceRoleOwner {
		t.Fatalf("role = %q, want owner", gotRole)
	}
}
func TestWorkspaceRejectsRequests(t *testing.T) {
	ctx := context.Background()
	store := iam.NewMemoryStore()
	service := iam.New(
		store,
		"0123456789abcdef0123456789abcdef",
		"kbot-test",
	)

	user, err := service.Register(
		ctx, "owner@example.com", "password123", "小明",
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	result, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result == nil {
		t.Fatal("expected a login result")
	}

	other := &domain.Workspace{
		ID:   "workspace-other",
		Name: "未加入的空间",
	}
	if err := store.CreateWorkspace(ctx, other); err != nil {
		t.Fatalf("create other workspace: %v", err)
	}

	tests := []struct {
		name             string
		workspaceID      string
		workspaceService *iam.Service
		wantStatus       int
	}{
		{"未选择空间", "", service, http.StatusBadRequest},
		{"空间ID只有空格", "   ", service, http.StatusBadRequest},
		{"没有成员关系", other.ID, service, http.StatusForbidden},
		{"服务未配置", other.ID, nil, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
			req.Header.Set("Authorization", "Bearer "+result.Token)
			if tt.workspaceID != "" {
				req.Header.Set("X-Workspace-ID", tt.workspaceID)
			}
			recorder := httptest.NewRecorder()

			wrapped := Chain(
				handler,
				Auth(service),
				Workspace(tt.workspaceService),
			)
			wrapped.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if called {
				t.Fatal("business handler must not run")
			}
		})
	}
}

func TestWorkspaceViewerPermissions(t *testing.T) {
	ctx := context.Background()
	store := iam.NewMemoryStore()
	service := iam.New(
		store,
		"0123456789abcdef0123456789abcdef",
		"kbot-test",
	)

	user, err := service.Register(
		ctx, "viewer@example.com", "password123", "小明",
	)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	workspace := &domain.Workspace{
		ID:   "workspace-team",
		Name: "测试团队空间",
	}
	if err := store.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := store.AddMembership(ctx, &domain.Membership{
		UserID:      user.ID,
		WorkspaceID: workspace.ID,
		Role:        iam.WorkspaceRoleViewer,
	}); err != nil {
		t.Fatalf("add viewer membership: %v", err)
	}

	result, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if result == nil {
		t.Fatal("expected a login result")
	}

	tests := []struct {
		name       string
		method     string
		wantStatus int
	}{
		{"读取允许", http.MethodGet, http.StatusNoContent},
		{"修改拒绝", http.MethodPost, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			var receivedRole string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				receivedRole = WorkspaceRole(r.Context())
				w.WriteHeader(http.StatusNoContent)
			})

			req := httptest.NewRequest(tt.method, "/api/agents", nil)
			req.Header.Set("Authorization", "Bearer "+result.Token)
			req.Header.Set("X-Workspace-ID", workspace.ID)
			recorder := httptest.NewRecorder()

			wrapped := Chain(handler, Auth(service), Workspace(service))
			wrapped.ServeHTTP(recorder, req)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			wantCalled := tt.wantStatus == http.StatusNoContent
			if called != wantCalled {
				t.Fatalf("handler called = %v, want %v", called, wantCalled)
			}
			if called && receivedRole != iam.WorkspaceRoleViewer {
				t.Fatalf("role = %q, want viewer", receivedRole)
			}
		})
	}
}
