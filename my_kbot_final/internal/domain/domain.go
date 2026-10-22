// Package domain 定义业务中使用的领域类型。
package domain

import "time"

// User 表示平台用户。
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Workspace 表示工作空间，例如一个团队或业务项目。
type Workspace struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ParentID    *string   `json:"parent_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkspaceMember 表示用户在某个工作空间中的成员身份。
type WorkspaceMember struct {
	WorkspaceID string    `json:"workspace_id"`
	UserID      string    `json:"user_id"`
	Email       string    `json:"email,omitempty"`
	Name        string    `json:"name,omitempty"`
	Role        string    `json:"role"` // owner / admin / editor / member / viewer
	CreatedAt   time.Time `json:"created_at"`
}
