package iam

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"my_kbot_final/internal/domain"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// ---- service_create_test.go ----

// 嵌入接口补齐未使用的方法；本轮只允许调用 CreateUser。
type createUserStoreStub struct {
	Store
	create func(context.Context, *domain.User) error
}

func (s *createUserStoreStub) CreateUser(ctx context.Context, user *domain.User) error {
	return s.create(ctx, user)
}

func TestServiceCreateUserRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name, email, password, displayName string
		want                               error
	}{
		{"invalid email", "not-an-email", "password123", "小明", ErrInvalidEmail},
		{"display address", "Alice <alice@example.com>", "password123", "小明", ErrInvalidEmail},
		{"short password", "alice@example.com", "1234567", "小明", ErrWeakPassword},
		{"blank name", "alice@example.com", "password123", " \t\n", ErrNameRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &createUserStoreStub{create: func(context.Context, *domain.User) error {
				t.Fatal("invalid input must not reach storage")
				return nil
			}}
			user, err := NewService(store, nil).CreateUser(context.Background(), tc.email, tc.password, tc.displayName)
			if user != nil || !errors.Is(err, tc.want) {
				t.Fatalf("expected nil user and %v, got userPresent=%t err=%v", tc.want, user != nil, err)
			}
		})
	}
}

func TestServiceCreateUserNormalizesAndHashes(t *testing.T) {
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "request-context")
	var saved *domain.User
	store := &createUserStoreStub{create: func(gotCtx context.Context, user *domain.User) error {
		if gotCtx.Value(contextKey{}) != "request-context" {
			t.Fatal("request context was not forwarded")
		}
		saved = user
		return nil
	}}
	service := NewService(store, nil)
	first, err := service.CreateUser(ctx, "  ALICE@Example.COM \t", "password123", " 小明 ")
	if err != nil || first == nil {
		t.Fatalf("create user: %v", err)
	}
	if first != saved || first.Email != "alice@example.com" || first.Name != "小明" || first.Role != GlobalRoleMember || first.Status != "active" {
		t.Fatal("created user has incorrect normalized fields, defaults, or storage result")
	}
	if _, err := uuid.Parse(first.ID); err != nil || first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatal("created user needs a valid ID and timestamps")
	}
	if first.PasswordHash == "password123" || bcrypt.CompareHashAndPassword([]byte(first.PasswordHash), []byte("password123")) != nil {
		t.Fatal("stored password must be a verifiable bcrypt hash")
	}
	if bcrypt.CompareHashAndPassword([]byte(first.PasswordHash), []byte("wrong-password")) == nil {
		t.Fatal("wrong password must not verify")
	}
	cost, err := bcrypt.Cost([]byte(first.PasswordHash))
	if err != nil || cost != bcrypt.DefaultCost {
		t.Fatal("unexpected bcrypt work factor")
	}
	second, err := service.CreateUser(ctx, "bob@example.com", "password123", "小红")
	if err != nil || second == nil {
		t.Fatalf("create second user: %v", err)
	}
	if first.ID == second.ID || first.PasswordHash == second.PasswordHash {
		t.Fatal("different users need distinct IDs and independently salted password hashes")
	}
	encoded, err := json.Marshal(first)
	if err != nil || strings.Contains(string(encoded), "password123") || strings.Contains(string(encoded), first.PasswordHash) {
		t.Fatal("user JSON must not expose the password or hash")
	}
}

func TestServiceCreateUserPreservesErrors(t *testing.T) {
	storageErr := errors.New("storage unavailable")
	calls := 0
	store := &createUserStoreStub{create: func(context.Context, *domain.User) error {
		calls++
		return storageErr
	}}
	service := NewService(store, nil)
	user, err := service.CreateUser(context.Background(), "alice@example.com", "password123", "小明")
	if user != nil || !errors.Is(err, storageErr) || calls != 1 {
		t.Fatal("storage failure should return no user and retain the original error")
	}
	user, err = service.CreateUser(context.Background(), "alice@example.com", strings.Repeat("a", 73), "小明")
	if user != nil || !errors.Is(err, bcrypt.ErrPasswordTooLong) || calls != 1 {
		t.Fatal("bcrypt length failure must be preserved and must not reach storage")
	}
}

func TestServiceCreateUserPostgres(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	service := NewService(store, nil)
	user, err := service.CreateUser(ctx, "  ALICE@Example.COM ", "password123", " 小明 ")
	if err != nil || user == nil {
		t.Fatalf("create user in PostgreSQL: %v", err)
	}
	got, err := store.GetUserByEmail(ctx, "alice@example.com")
	if err != nil || got == nil {
		t.Fatalf("read user from PostgreSQL: %v", err)
	}
	if got.ID != user.ID || got.Name != "小明" || got.Role != GlobalRoleMember || got.Status != "active" || !got.CreatedAt.Equal(user.CreatedAt) || !got.UpdatedAt.Equal(user.UpdatedAt) {
		t.Fatal("database user differs from the created user")
	}
	if bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte("password123")) != nil {
		t.Fatal("database password hash does not verify")
	}
	if got.CreatedAt.Before(time.Now().Add(-time.Minute)) {
		t.Fatal("database creation timestamp is unexpectedly old")
	}
	duplicate, err := service.CreateUser(ctx, "ALICE@example.com", "password456", "另一个人")
	var pgErr *pgconn.PgError
	if duplicate != nil || !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatal("normalized duplicate email must retain the PostgreSQL unique violation")
	}
}

// ---- service_login_test.go ----

type loginStoreStub struct {
	Store
	lookup func(context.Context, string) (*domain.User, error)
}

func (s *loginStoreStub) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.lookup(ctx, email)
}

func assertLoginToken(t *testing.T, result *LoginResponse, key []byte, user *domain.User, started time.Time) {
	t.Helper()
	if result == nil || result.User == nil || result.User.ID != user.ID || result.Token == "" {
		t.Fatal("login must return the user and a nonempty token")
	}
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(result.Token, claims, func(token *jwt.Token) (any, error) {
		return key, nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("kbot"), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		t.Fatalf("login token must have a valid signature and registered claims: %v", err)
	}
	if claims.UserID != user.ID || claims.Subject != user.ID || claims.Email != user.Email || claims.IssuedAt == nil || claims.NotBefore == nil || claims.ExpiresAt == nil {
		t.Fatal("login token is missing the expected identity or timestamps")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("encode login response: %v", err)
	}
	var response struct {
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if response.ExpiresAt.Before(started.Add(24*time.Hour)) || response.ExpiresAt.After(time.Now().Add(24*time.Hour)) {
		t.Fatal("login token must expire approximately 24 hours after signing")
	}
	if d := response.ExpiresAt.Sub(claims.ExpiresAt.Time); d < 0 || d >= time.Second {
		t.Fatal("response expiry and token expiry must agree within JWT timestamp precision")
	}
	if _, err := jwt.Parse(result.Token, func(*jwt.Token) (any, error) {
		return []byte("a-different-test-signing-key"), nil
	}, jwt.WithValidMethods([]string{"HS256"})); err == nil {
		t.Fatal("the token must not verify with a different signing key")
	}
	if strings.Contains(string(encoded), user.PasswordHash) || strings.Contains(string(encoded), "password123") {
		t.Fatal("login JSON must not expose password material")
	}
}

func TestServiceLoginSignsToken(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := &domain.User{ID: "user-1", Email: "alice@example.com", PasswordHash: string(hash), Status: "active"}
	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "login-context")
	store := &loginStoreStub{lookup: func(gotCtx context.Context, email string) (*domain.User, error) {
		if gotCtx.Value(ctxKey{}) != "login-context" || email != "alice@example.com" {
			t.Fatal("login must normalize the email and forward the context")
		}
		return user, nil
	}}
	key := []byte("learning-test-key-with-at-least-32-bytes")
	started := time.Now()
	result, err := NewService(store, key).Login(ctx, LoginRequest{Email: " ALICE@Example.COM ", Password: "password123"})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	assertLoginToken(t, result, key, user, started)
}

func TestServiceLoginRejectsInvalidCredentials(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, status, password, passwordHash string
		lookupErr                            error
	}{
		{name: "missing user", lookupErr: errors.New("user not found")},
		{name: "cancelled request", lookupErr: context.Canceled},
		{name: "wrong password", status: "active", password: "wrong-password", passwordHash: string(hash)},
		{name: "inactive account", status: "inactive", password: "password123", passwordHash: string(hash)},
		{name: "invalid stored hash", status: "active", password: "password123", passwordHash: "not-a-bcrypt-hash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &loginStoreStub{lookup: func(context.Context, string) (*domain.User, error) {
				if tc.lookupErr != nil {
					return nil, tc.lookupErr
				}
				return &domain.User{ID: "user-1", Status: tc.status, PasswordHash: tc.passwordHash}, nil
			}}
			result, err := NewService(store, []byte("test-key")).Login(context.Background(), LoginRequest{Email: "alice@example.com", Password: tc.password})
			if result != nil || err == nil {
				t.Fatal("invalid credentials must not produce a login response")
			}
			if tc.lookupErr != nil && !errors.Is(err, tc.lookupErr) {
				t.Fatal("lookup errors should retain their cause")
			}
		})
	}
}

func TestServiceLoginPostgres(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	key := []byte("learning-test-key-with-at-least-32-bytes")
	service := NewService(store, key)
	user, err := service.CreateUser(ctx, "alice@example.com", "password123", "小明")
	if err != nil {
		t.Fatalf("create login fixture: %v", err)
	}
	started := time.Now()
	result, err := service.Login(ctx, LoginRequest{Email: " ALICE@EXAMPLE.COM ", Password: "password123"})
	if err != nil {
		t.Fatalf("login using PostgreSQL: %v", err)
	}
	assertLoginToken(t, result, key, user, started)
}

// ---- service_token_test.go ----

func TestServiceValidateToken(t *testing.T) {
	key := []byte("learning-test-key-with-at-least-32-bytes")
	service := NewService(nil, key)
	newClaims := func() *Claims {
		return &Claims{
			UserID: "user-1", Email: "alice@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: "kbot", Subject: "user-1",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-time.Minute)),
				NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
			},
		}
	}
	sign := func(method jwt.SigningMethod, claims *Claims, signingKey []byte) string {
		t.Helper()
		raw, err := jwt.NewWithClaims(method, claims).SignedString(signingKey)
		if err != nil {
			t.Fatalf("sign fixture token: %v", err)
		}
		return raw
	}
	valid := sign(jwt.SigningMethodHS256, newClaims(), key)
	t.Run("valid", func(t *testing.T) {
		claims, err := service.ValidateToken(valid)
		if err != nil || claims == nil {
			t.Fatalf("valid token was rejected: %v", err)
		}
		if claims.UserID != "user-1" || claims.Email != "alice@example.com" || claims.Subject != "user-1" || claims.Issuer != "kbot" {
			t.Fatal("validated claims lost user identity or registered fields")
		}
	})
	expired := newClaims()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	future := newClaims()
	future.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour))
	wrongIssuer := newClaims()
	wrongIssuer.Issuer = "another-service"
	parts := strings.Split(valid, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	payload = []byte(strings.Replace(string(payload), "user-1", "user-2", 1))
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	tests := []struct{ name, token string }{
		{"empty", ""},
		{"malformed", "not-a-jwt"},
		{"wrong signing key", sign(jwt.SigningMethodHS256, newClaims(), []byte("wrong-key"))},
		{"tampered identity", strings.Join(parts, ".")},
		{"wrong algorithm", sign(jwt.SigningMethodHS384, newClaims(), key)},
		{"expired", sign(jwt.SigningMethodHS256, expired, key)},
		{"not yet valid", sign(jwt.SigningMethodHS256, future, key)},
		{"wrong issuer", sign(jwt.SigningMethodHS256, wrongIssuer, key)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := service.ValidateToken(tc.token)
			if err == nil || claims != nil {
				t.Fatal("invalid token must return an error and no claims")
			}
		})
	}
}

func TestServiceValidateTokenPostgresLogin(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	service := NewService(store, []byte("learning-test-key-with-at-least-32-bytes"))
	user, err := service.CreateUser(ctx, "alice@example.com", "password123", "小明")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	result, err := service.Login(ctx, LoginRequest{Email: "alice@example.com", Password: "password123"})
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	claims, err := service.ValidateToken(result.Token)
	if err != nil || claims == nil {
		t.Fatalf("validate token returned by database-backed login: %v", err)
	}
	if claims.UserID != user.ID || claims.Subject != user.ID || claims.Email != user.Email {
		t.Fatal("validated token must refer to the database user who logged in")
	}
}

// ---- service_seed_test.go ----

type seedAdminStoreStub struct {
	Store
	get     func(context.Context, string) (*domain.User, error)
	create  func(context.Context, *domain.User) error
	setRole func(context.Context, string, string) error
}

func (s *seedAdminStoreStub) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return s.get(ctx, email)
}

func (s *seedAdminStoreStub) CreateUser(ctx context.Context, user *domain.User) error {
	return s.create(ctx, user)
}

func (s *seedAdminStoreStub) SetUserRole(ctx context.Context, id, role string) error {
	return s.setRole(ctx, id, role)
}

func TestServiceEnsureSeedAdminPostgres(t *testing.T) {
	ctx, store := newPostgresUserStoreTest(t)
	service := NewService(store, []byte("learning-test-key-with-at-least-32-bytes"))
	if err := service.EnsureSeedAdmin(ctx, " ADMIN@Example.COM ", "password123"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	first, err := store.GetUserByEmail(ctx, "admin@example.com")
	if err != nil || first == nil {
		t.Fatalf("read seed admin: %v", err)
	}
	if first.Role != GlobalRoleAdmin || first.Status != "active" || first.Name != "Admin" {
		t.Fatal("new seed user must be an active global admin")
	}
	if bcrypt.CompareHashAndPassword([]byte(first.PasswordHash), []byte("password123")) != nil {
		t.Fatal("seed admin password must be stored as a valid bcrypt hash")
	}
	if err := service.EnsureSeedAdmin(ctx, "admin@example.com", "changed-password456"); err != nil {
		t.Fatalf("repeat bootstrap: %v", err)
	}
	second, err := store.GetUserByEmail(ctx, "admin@example.com")
	if err != nil || second == nil {
		t.Fatalf("read repeated seed admin: %v", err)
	}
	if second.ID != first.ID || second.PasswordHash != first.PasswordHash || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatal("repeat bootstrap must retain the existing account and password")
	}
	users, err := store.ListUsers(ctx, 10, 0)
	if err != nil || len(users) != 1 {
		t.Fatal("repeat bootstrap must not create duplicate accounts")
	}
	if err := store.SetUserRole(ctx, first.ID, GlobalRoleMember); err != nil {
		t.Fatalf("prepare existing member: %v", err)
	}
	if err := service.EnsureSeedAdmin(ctx, "admin@example.com", "unused-password789"); err != nil {
		t.Fatalf("promote existing seed account: %v", err)
	}
	promoted, err := store.GetUserByID(ctx, first.ID)
	if err != nil || promoted == nil || promoted.Role != GlobalRoleAdmin || promoted.PasswordHash != first.PasswordHash {
		t.Fatal("bootstrap must ensure the admin role without replacing the password")
	}
	if _, err := service.Login(ctx, LoginRequest{Email: "admin@example.com", Password: "password123"}); err != nil {
		t.Fatalf("original password must still work: %v", err)
	}
	if result, err := service.Login(ctx, LoginRequest{Email: "admin@example.com", Password: "changed-password456"}); err == nil || result != nil {
		t.Fatal("repeat bootstrap must not activate the newly supplied password")
	}
}

func TestServiceEnsureSeedAdminPreservesErrors(t *testing.T) {
	t.Run("create failure", func(t *testing.T) {
		want := errors.New("insert failed")
		store := &seedAdminStoreStub{
			get: func(context.Context, string) (*domain.User, error) {
				return nil, errors.New("user not found")
			},
			create: func(context.Context, *domain.User) error { return want },
			setRole: func(context.Context, string, string) error {
				t.Fatal("must not set role after failed creation")
				return nil
			},
		}
		if err := NewService(store, nil).EnsureSeedAdmin(context.Background(), "admin@example.com", "password123"); !errors.Is(err, want) {
			t.Fatalf("creation error must retain its cause: %v", err)
		}
	})
	t.Run("role update failure", func(t *testing.T) {
		want := errors.New("update role failed")
		store := &seedAdminStoreStub{
			get: func(_ context.Context, email string) (*domain.User, error) {
				if email != "admin@example.com" {
					t.Fatal("seed lookup must normalize email")
				}
				return &domain.User{ID: "existing-user"}, nil
			},
			create: func(context.Context, *domain.User) error {
				t.Fatal("existing seed account must not be recreated")
				return nil
			},
			setRole: func(_ context.Context, id, role string) error {
				if id != "existing-user" || role != GlobalRoleAdmin {
					t.Fatal("role update must target the seed account and global admin role")
				}
				return want
			},
		}
		if err := NewService(store, nil).EnsureSeedAdmin(context.Background(), " ADMIN@EXAMPLE.COM ", "unused-password"); !errors.Is(err, want) {
			t.Fatalf("role error must retain its cause: %v", err)
		}
	})
}
