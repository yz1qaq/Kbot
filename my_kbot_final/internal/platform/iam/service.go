package iam

import (
	"context"
	"errors"
	"fmt"
	"my_kbot_final/internal/domain"
	"my_kbot_final/internal/util"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	WorkspaceRoleOwner  = "owner"
	WorkspaceRoleAdmin  = "admin"
	WorkspaceRoleEditor = "editor"
	WorkspaceRoleMember = "member"
	WorkspaceRoleViewer = "viewer"
)

type Store interface {
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByID(ctx context.Context, userID string) (*domain.User, error)
	ListUsers(ctx context.Context, limit, offset int32) ([]*domain.User, error)
	SetUserRole(ctx context.Context, userID string, role string) error

	GetWorkspaceByID(ctx context.Context, workspaceID string) (*domain.Workspace, error)
	CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error
	ListWorkspaces(ctx context.Context, limit, offset int32) ([]*domain.Workspace, error)
	ListUserWorkspaces(ctx context.Context, userID string, limit, offset int32) ([]*domain.Workspace, error)

	GetWorkspaceMember(ctx context.Context, workspaceID, userID string) (*domain.WorkspaceMember, error)
	ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]*domain.WorkspaceMember, error)

	UpsertWorkspaceMember(ctx context.Context, member *domain.WorkspaceMember) error

	DeleteWorkspaceMember(ctx context.Context, workspaceID, userID string) (bool, error)
}

type workspaceOwnerStore interface {
	CreateWorkspaceWithOwner(ctx context.Context, workspace *domain.Workspace, userID string) error
}

type guardedMembershipStore interface {
	UpsertWorkspaceMemberGuarded(ctx context.Context, member *domain.WorkspaceMember, allowOwnerManagement bool) error

	DeleteWorkspaceMemberGuarded(ctx context.Context, workspaceID, userID string, allowOwnerManagement bool) (bool, error)
}

type Service struct {
	store  Store
	jwtKey []byte
}

type LoginResponse struct {
	Token     string       `json:"token"`
	ExpiresAt time.Time    `json:"expires_at"`
	User      *domain.User `json:"user"`
}

// LoginRequest 表示用户输入的登录信息。
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Claims 表示 JWT 中携带的身份信息和标准声明。
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

var (
	ErrInvalidEmail = errors.New("invalid email")
	ErrWeakPassword = errors.New("password must be at least 8 characters")
	ErrNameRequired = errors.New("name is required")
	ErrForbidden    = errors.New("forbidden")
	ErrLastOwner    = errors.New("workspace must keep at least one owner")
)

func NewService(store Store, jwtKey []byte) *Service {
	return &Service{
		store:  store,
		jwtKey: jwtKey,
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Service) CreateUser(ctx context.Context, email, password, name string) (*domain.User, error) {
	email = normalizeEmail(email)
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email {
		return nil, ErrInvalidEmail
	}

	if len(password) < 8 {
		return nil, ErrWeakPassword
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNameRequired
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		ID:           util.GenerateID(),
		Email:        email,
		Name:         name,
		Role:         GlobalRoleMember,
		PasswordHash: string(hashedPassword),
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := s.store.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

// 登陆功能
func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	//检查用户是否存在
	user, err := s.store.GetUserByEmail(ctx, normalizeEmail(req.Email))
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash),
		[]byte(req.Password)); err != nil {
		return nil, fmt.Errorf("invalid password")
	}
	if user.Status != "active" {
		return nil, fmt.Errorf("user account is not active")
	}

	expireAt := time.Now().Add(24 * time.Hour)

	claims := &Claims{
		UserID: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expireAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "kbot",
			Subject:   user.ID,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(s.jwtKey)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}

	return &LoginResponse{
		Token:     tokenString,
		ExpiresAt: expireAt,
		User:      user,
	}, nil
}

// ValidateToken 验证 JWT，并返回其中的身份声明。
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{},
		func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method:%v", token.Header["alg"])
			}
			return s.jwtKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer("kbot"),
	)
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}

// 确保指定邮箱的账号存在,并拥有全局管理员角色
func (s *Service) EnsureSeedAdmin(ctx context.Context, email, password string) error {
	user, err := s.store.GetUserByEmail(ctx, normalizeEmail(email))
	if err != nil {
		user, err = s.CreateUser(ctx, email, password, "Admin")
		if err != nil {
			return fmt.Errorf("ensure seed admin %w", err)
		}
	}
	if err := s.store.SetUserRole(ctx, user.ID, GlobalRoleAdmin); err != nil {
		return fmt.Errorf("grant seed admin role %w", err)
	}
	return nil
}

// 根据userID查询user
func (s *Service) GetUser(ctx context.Context, userID string) (*domain.User, error) {
	return s.store.GetUserByID(ctx, userID)
}

func (s *Service) ListUsers(ctx context.Context, limit, offset int32) ([]*domain.User, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListUsers(ctx, limit, offset)
}

// 为某个用户创建工作空间,该用户为owner
func (s *Service) CreateWorkspaceForUser(ctx context.Context, actorID, name,
	description string, parentID *string) (*domain.Workspace, error) {
	workspace := &domain.Workspace{
		ID:          util.GenerateID(),
		Name:        name,
		Description: description,
		ParentID:    parentID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if atomicStore, ok := s.store.(workspaceOwnerStore); ok {
		if err := atomicStore.CreateWorkspaceWithOwner(
			ctx,
			workspace,
			actorID,
		); err != nil {
			return nil, fmt.Errorf("create workspace with owner: %w", err)
		}
		return workspace, nil
	}

	if err := s.store.CreateWorkspace(ctx, workspace); err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	member := &domain.WorkspaceMember{
		WorkspaceID: workspace.ID,
		UserID:      actorID,
		Role:        WorkspaceRoleOwner,
		CreatedAt:   time.Now(),
	}
	if err := s.store.UpsertWorkspaceMember(ctx, member); err != nil {
		return nil, fmt.Errorf("assign workspace owner: %w", err)
	}

	return workspace, nil
}

// 判断是不是管理员
func (s *Service) IsGlobalAdmin(ctx context.Context, userID string) (bool, error) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return user.Status == "active" &&
		user.Role == GlobalRoleAdmin, nil
}

// 展示该用户可以访问的工作空间
func (s *Service) ListAccessibleWorkspaces(ctx context.Context, userID string, limit, offset int32) ([]*domain.Workspace, error) {
	if limit <= 0 {
		limit = 50
	}

	admin, err := s.IsGlobalAdmin(ctx, userID)
	if err != nil {
		return nil, err
	}

	if admin {
		return s.store.ListWorkspaces(ctx, limit, offset)
	}
	return s.store.ListUserWorkspaces(ctx, userID, limit, offset)
}

// 返回用户在指定空间的有效角色
func (s *Service) WorkspaceRole(ctx context.Context, userID, workspaceID string) (string, error) {
	if workspaceID == "" || userID == "" {
		return "", ErrForbidden
	}
	//判断user是否有效
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if user.Status != "active" {
		return "", ErrForbidden
	}

	//如果是管理员,直接返回
	if user.Role == GlobalRoleAdmin {
		return WorkspaceRoleAdmin, nil
	}

	member, err := s.store.GetWorkspaceMember(ctx, workspaceID, user.ID)
	if err != nil {
		return "", ErrForbidden
	}
	return member.Role, nil
}

// CanManageWorkspace 判断是否拥有成员管理权限。
func (s *Service) CanManageWorkspace(ctx context.Context, actorID, workspaceID string) (bool, error) {
	role, err := s.WorkspaceRole(ctx, actorID, workspaceID)
	if err != nil {
		return false, err
	}
	return role == WorkspaceRoleOwner ||
		role == WorkspaceRoleAdmin, nil
}

// 查询工作空间的成员,必须是owner或者admin
func (s *Service) ListWorkspaceMembers(ctx context.Context, actorID, workspaceID string) ([]*domain.WorkspaceMember, error) {
	ok, err := s.CanManageWorkspace(ctx, actorID, workspaceID)
	if err != nil || !ok {
		return nil, ErrForbidden
	}
	return s.store.ListWorkspaceMembers(ctx, workspaceID)
}

// workspaceManagementAuthority 判断成员管理资格及 owner 管理权限, err == nil 表示可以管理成员；bool 表示是否可以操作 owner。
func (s *Service) workspaceManagementAuthority(ctx context.Context, actorID, workspaceID string) (bool, error) {
	if actorID == "" || workspaceID == "" {
		return false, ErrForbidden
	}

	user, err := s.store.GetUserByID(ctx, actorID)
	if err != nil || user.Status != "active" {
		return false, ErrForbidden
	}

	if user.Role == GlobalRoleAdmin {
		return true, nil
	}

	member, err := s.store.GetWorkspaceMember(ctx, workspaceID, actorID)
	if err != nil {
		return false, ErrForbidden
	}

	switch member.Role {
	case WorkspaceRoleOwner:
		return true, nil
	case WorkspaceRoleAdmin:
		return false, nil
	default:
		return false, ErrForbidden
	}
}

// 检查是否为合法的工作空间角色
func validWorkspaceRole(role string) bool {
	switch role {
	case WorkspaceRoleOwner,
		WorkspaceRoleAdmin,
		WorkspaceRoleEditor,
		WorkspaceRoleMember,
		WorkspaceRoleViewer:
		return true
	default:
		return false
	}
}

// 添加成员或更新成员角色
func (s *Service) UpsertWorkspaceMember(ctx context.Context, actorID, workspaceID, userID, role string) (*domain.WorkspaceMember, error) {
	manageOwners, err := s.workspaceManagementAuthority(ctx, actorID, workspaceID)
	if err != nil {
		return nil, ErrForbidden
	}
	if !validWorkspaceRole(role) {
		return nil, fmt.Errorf("invalid workspace role %q", role)
	}

	if _, err := s.store.GetUserByID(ctx, userID); err != nil {
		return nil, fmt.Errorf("get member user: %w", err)
	}

	member := &domain.WorkspaceMember{
		WorkspaceID: workspaceID,
		UserID:      userID,
		Role:        role,
		CreatedAt:   time.Now(),
	}

	if guarded, ok := s.store.(guardedMembershipStore); ok {
		if err := guarded.UpsertWorkspaceMemberGuarded(ctx, member, manageOwners); err != nil {
			return nil, err
		}
	} else if err := s.upsertWorkspaceMemberFallback(
		ctx, member, manageOwners,
	); err != nil {
		return nil, fmt.Errorf(
			"upsert workspace member: %w", err,
		)
	}
	return s.store.GetWorkspaceMember(ctx, workspaceID, userID)

}

// upsertWorkspaceMemberFallback 兼容只实现基础 Store 的存储对象。
func (s *Service) upsertWorkspaceMemberFallback(ctx context.Context, member *domain.WorkspaceMember, manageOwners bool) error {
	//先判断这个用户是否已经存在在当前工作空间
	existing, existingErr := s.store.GetWorkspaceMember(
		ctx, member.WorkspaceID, member.UserID,
	)

	//不能操作管理员
	if member.Role == WorkspaceRoleOwner && !manageOwners {
		return ErrForbidden
	}

	if existingErr == nil && existing.Role == WorkspaceRoleOwner {
		if !manageOwners {
			return ErrForbidden
		}

		if member.Role != WorkspaceRoleOwner {
			members, err := s.store.ListWorkspaceMembers(
				ctx, member.WorkspaceID,
			)
			if err != nil {
				return err
			}

			owners := 0
			for _, candidate := range members {
				if candidate.Role == WorkspaceRoleOwner {
					owners++
				}
			}

			if owners <= 1 {
				return ErrLastOwner
			}
		}
	}

	return s.store.UpsertWorkspaceMember(ctx, member)
}

// DeleteWorkspaceMember 移除成员，但必须保留至少一位 owner。
func (s *Service) DeleteWorkspaceMember(ctx context.Context, actorID, workspaceID, userID string) error {
	manageOwners, err := s.workspaceManagementAuthority(
		ctx, actorID, workspaceID,
	)
	if err != nil {
		return ErrForbidden
	}

	if guarded, ok := s.store.(guardedMembershipStore); ok {
		deleted, err := guarded.DeleteWorkspaceMemberGuarded(
			ctx, workspaceID, userID, manageOwners,
		)
		if err != nil {
			return err
		}
		if !deleted {
			return fmt.Errorf("workspace member not found")
		}
		return nil
	}

	//如果没有安全删除能力
	members, err := s.store.ListWorkspaceMembers(
		ctx, workspaceID,
	)
	if err != nil {
		return err
	}

	owners := 0
	targetOwner := false

	//判断要被删除的user是不是owner
	for _, member := range members {
		if member.Role == WorkspaceRoleOwner {
			owners++
			if member.UserID == userID {
				targetOwner = true
			}
		}
	}

	//如果要被删除的是owner,且参数要求不能操作owner
	if targetOwner && !manageOwners {
		return ErrForbidden
	}

	//如果要删除的是最后一个owner,拒绝
	if targetOwner && owners <= 1 {
		return ErrLastOwner
	}

	deleted, err := s.store.DeleteWorkspaceMember(
		ctx, workspaceID, userID,
	)
	if err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("workspace member not found")
	}

	return nil
}
