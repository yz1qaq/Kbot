package middleware

import (
	"context"
	"my_kbot_final/internal/platform/iam"
	"net/http"
	"strings"
)

type ContextKey string

const (
	ContextKeyUserID     ContextKey = "user_id"
	ContextKeyGlobalRole ContextKey = "global_role"
)

func GetUserIDFromContext(ctx context.Context) string {
	if userID, ok := ctx.Value(ContextKeyUserID).(string); ok {
		return userID
	}
	return ""
}

func GetGlobalRoleFromContext(ctx context.Context) string {
	role, _ := ctx.Value(ContextKeyGlobalRole).(string)
	return role
}

// Auth 验证请求身份，并将用户信息传给后续处理器。
func Auth(iamService *iam.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}
			const bearePrefix = "Bearer "
			if !strings.HasPrefix(authHeader, bearePrefix) {
				http.Error(w, "invalid authorization format", http.StatusUnauthorized)
				return
			}
			tokenString := authHeader[len(bearePrefix):]
			claims, err := iamService.ValidateToken(tokenString)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			user, err := iamService.GetUser(r.Context(), claims.UserID)
			if err != nil || user.Status != "active" {
				http.Error(w, "user account is unavailable", http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ContextKeyUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextKeyGlobalRole, user.Role)

			next.ServeHTTP(w, r.WithContext(ctx))

		})
	}
}

// 只允许全局管理员访问
func RequireGlobalAdmin() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := GetGlobalRoleFromContext(r.Context())
			if role != iam.GlobalRoleAdmin {
				http.Error(w, "global admin role required", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}


