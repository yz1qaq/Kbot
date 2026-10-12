package iam

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"my_kbot/internal/domain"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrWorkspaceForbidden = errors.New("workspace access denied")

const (
	WorkspaceRoleOwner  = "owner"  //所有者
	WorkspaceRoleAdmin  = "admin"  //管理员
	WorkspaceRoleEditor = "editor" //编辑员
	WorkspaceRoleMember = "member" //普通成员
	WorkspaceRoleViewer = "viewer" //查看者
)

type Store interface {
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByEmail(Ctx context.Context, email string) (*domain.User, error)
	GetUser(ctx context.Context, id string) (*domain.User, error)

	CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error
	AddMembership(ctx context.Context, membership *domain.Membership) error
	HasMembership(ctx context.Context, userID, workspaceID string) (bool, error)
	GetMembership(ctx context.Context, userID, workspaceID string) (*domain.Membership, error)
	ListUserWorkspaces(ctx context.Context, userID string) ([]*domain.Workspace, error)
}

// 测试MemoryStore是否满足接口方法
var _ Store = (*MemoryStore)(nil)

type Service struct {
	store  Store
	secret []byte //JWT 签名和验证使用的密钥
	issuer string //JWT 的签发者标识
}

type LoginResult struct {
	Token string       `json:"token"` //登录成功后签发的令牌，后续请求携带它
	User  *domain.User `json:"user"`
}

var ErrInvalidCredentials = errors.New("invalid email or password")

/*
claims 定义 JWT 中的用户身份和标准声明。

	这个令牌属于哪个用户？
	由谁签发？
	什么时候签发？
	什么时候过期？
*/
type claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

func New(stroe Store, secret, issuer string) *Service {
	return &Service{
		store:  stroe,
		secret: []byte(secret),
		issuer: issuer,
	}
}

func newID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}

	return hex.EncodeToString(raw[:])
}

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

// 从服务端的成员关系中,取得用户所在指定空间的合法角色
func (s *Service) WorkspaceRole(ctx context.Context, userID, workspaceID string) (string, error) {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(workspaceID) == "" {
		return "", ErrWorkspaceForbidden
	}
	membership, err := s.store.GetMembership(ctx, userID, workspaceID)
	if err != nil || !validWorkspaceRole(membership.Role) {
		return "", ErrWorkspaceForbidden
	}
	return membership.Role, nil
}

// 检查用户是否具有指定工作空间的访问资格
func (s *Service) CheckWorkspaceAccess(ctx context.Context, userID, workspaceID string) error {
	_, err := s.WorkspaceRole(ctx, userID, workspaceID)
	return err
}

// 列出用户的所有工作空间
func (s *Service) ListUserWorkspaces(ctx context.Context, userID string) ([]*domain.Workspace, error) {
	return s.store.ListUserWorkspaces(ctx, userID)
}

// 注册
func (s *Service) Register(ctx context.Context, email, password, name string) (*domain.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return nil, fmt.Errorf("Invalid email")
	}
	if len(password) < 8 {
		return nil, fmt.Errorf("password must contain at least 8 characters")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	user := &domain.User{
		ID:           newID(),
		Email:        email,
		Name:         name,
		PasswordHash: string(hash),
		CreatedAt:    time.Now().UTC(),
	}
	err = s.store.CreateUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	workspace := &domain.Workspace{
		ID:        newID(),
		Name:      name + "Workspace",
		CreatedAt: time.Now().UTC(),
	}
	err = s.store.CreateWorkspace(ctx, workspace)
	if err != nil {
		return nil, fmt.Errorf("create workspace:%w", err)
	}

	if err := s.store.AddMembership(ctx, &domain.Membership{
		UserID:      user.ID,
		WorkspaceID: workspace.ID,
		Role:        WorkspaceRoleOwner,
		CreatedAt:   time.Now().UTC(),
	}); err != nil {
		return nil, fmt.Errorf("create Membership: %w", err)
	}

	return user, nil
}

// 登陆
func (s *Service) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	user, err := s.store.GetUserByEmail(ctx, strings.TrimSpace(email))
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return nil, ErrInvalidCredentials
	}
	now := time.Now()
	tokenClaims := claims{
		UserID: user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   user.ID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(8 * time.Hour)),
		},
	}

	token, err := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		tokenClaims,
	).SignedString(s.secret)
	if err != nil {
		return nil, fmt.Errorf("sign token: %w", err)
	}
	return &LoginResult{
		Token: token,
		User:  user,
	}, nil
}

// ParseToken 验证 JWT，并返回其中的用户 ID。
func (s *Service) ParseToken(raw string) (string, error) {
	parsed := &claims{}

	token, err := jwt.ParseWithClaims(
		raw,
		parsed,
		func(token *jwt.Token) (any, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf(
					"unexpected signing method %s",
					token.Method.Alg(),
				)
			}
			return s.secret, nil
		},
		jwt.WithIssuer(s.issuer),
		jwt.WithValidMethods([]string{
			jwt.SigningMethodHS256.Alg(),
		}),
	)
	if err != nil || !token.Valid || parsed.UserID == "" {
		return "", ErrInvalidCredentials
	}

	return parsed.UserID, nil
}
