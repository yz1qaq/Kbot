package middleware

import (
	"context"
	"testing"
)

func TestIdentityContextValues(t *testing.T) {
	parent := context.Background()
	userContext := context.WithValue(parent, ContextKeyUserID, "user-1")
	ctx := context.WithValue(userContext, ContextKeyGlobalRole, "admin")
	if GetUserIDFromContext(ctx) != "user-1" || GetGlobalRoleFromContext(ctx) != "admin" {
		t.Fatal("context should preserve both identity fields")
	}
	if GetUserIDFromContext(parent) != "" || GetGlobalRoleFromContext(userContext) != "" {
		t.Fatal("adding a value to a derived context must not modify its parent")
	}
	second := context.WithValue(parent, ContextKeyUserID, "user-2")
	if GetUserIDFromContext(second) != "user-2" || GetUserIDFromContext(ctx) != "user-1" {
		t.Fatal("independent request contexts must not overwrite each other's identities")
	}
}

func TestIdentityContextMissingOrWrongType(t *testing.T) {
	contexts := []struct {
		name string
		ctx  context.Context
	}{
		{"missing", context.Background()},
		{"incorrect value types", context.WithValue(context.WithValue(context.Background(), ContextKeyUserID, 123), ContextKeyGlobalRole, true)},
		// 同名普通字符串不等于 ContextKey 类型；模拟其他代码使用同名 key。
		{"plain string keys", context.WithValue(context.WithValue(context.Background(), "user_id", "unexpected-user"), "global_role", "admin")},
	}
	for _, tc := range contexts {
		t.Run(tc.name, func(t *testing.T) {
			if GetUserIDFromContext(tc.ctx) != "" || GetGlobalRoleFromContext(tc.ctx) != "" {
				t.Fatal("missing or incorrectly typed identity values must be treated as absent")
			}
		})
	}
}

func TestIdentityContextPreservesCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := context.WithValue(parent, ContextKeyUserID, "user-1")
	cancel()
	if ctx.Err() != context.Canceled || GetUserIDFromContext(ctx) != "user-1" {
		t.Fatal("identity context must inherit cancellation while retaining its value")
	}
}
