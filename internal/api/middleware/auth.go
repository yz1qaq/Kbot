package middleware

import (
	"context"
	"my_kbot/internal/platform/iam"
	"net/http"
	"strings"
)

type userIDkey struct{}

type workspaceIDKey struct{}

// workspaceRoleKey 用于在请求上下文中存取工作空间角色。
type workspaceRoleKey struct{}

// 从上下文获取userID
func UserID(ctx context.Context) string {
	value, _ := ctx.Value(userIDkey{}).(string)
	return value
}

func Auth(service *iam.Service) Func {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if raw == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			userID, err := service.ParseToken(raw)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userIDkey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func WorkspaceID(ctx context.Context) string {
	value, _ := ctx.Value(workspaceIDKey{}).(string)
	return value
}

func WorkspaceRole(ctx context.Context) string {
	value, _ := ctx.Value(workspaceRoleKey{}).(string)
	return value
}

// 判断角色是否允许执行本次操作
func workspaceMethodAllowed(role, method, path string) bool {
	//如果请求不是GET,HEAD且路径包括/approvals/的时候,只允许owner和admin
	if method != http.MethodGet && method != http.MethodHead && strings.Contains(path, "/approvals/") {
		return role == iam.WorkspaceRoleOwner || role == iam.WorkspaceRoleAdmin
	}
	//这三类都放行
	if role == iam.WorkspaceRoleOwner || role == iam.WorkspaceRoleAdmin || role == iam.WorkspaceRoleEditor {
		return true
	}

	//读取请求都放行
	if method == http.MethodGet || method == http.MethodHead {
		return true
	}
	//当不是读取请求的时候,role是viewer直接拒绝
	if role == iam.WorkspaceRoleViewer {
		return false
	}

	//普通成员只能通过这四个
	return strings.HasSuffix(path, "/chat") || strings.HasSuffix(path, "/search") || strings.HasSuffix(path, "/runs") ||
		strings.Contains(path, "/stream/")
}

// 检查工作空间访问资格和操作权限,并传递空间信息
func Workspace(service *iam.Service) Func {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspaceID := strings.TrimSpace(r.Header.Get("X-Workspace-ID"))
			if workspaceID == "" {
				http.Error(w, "workspace is required", http.StatusBadRequest)
				return
			}
			if service == nil {
				http.Error(w, "workspace access denied", http.StatusForbidden)
				return
			}
			role, err := service.WorkspaceRole(r.Context(), UserID(r.Context()), workspaceID)
			if err != nil || !workspaceMethodAllowed(role, r.Method, r.URL.Path) {
				http.Error(w, "workspace access denied", http.StatusForbidden)
				return
			}
			ctx := context.WithValue(r.Context(), workspaceIDKey{}, workspaceID)
			ctx = context.WithValue(ctx, workspaceRoleKey{}, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}



