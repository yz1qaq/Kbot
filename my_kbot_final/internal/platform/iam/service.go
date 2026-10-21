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

type Store interface {
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	CreateUser(ctx context.Context, user *domain.User) error
	GetUserByID(ctx context.Context, userID string) (*domain.User, error)
	ListUsers(ctx context.Context, limit, offset int32) ([]*domain.User, error)
	SetUserRole(ctx context.Context, userID string, role string) error
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
