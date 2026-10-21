package iam

import (
	"context"
	"fmt"
	"my_kbot_final/internal/domain"
	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	db *pgxpool.Pool
	q  *pgstore.Queries
}

const (
	GlobalRoleAdmin  = "admin"
	GlobalRoleMember = "member"
)

func NewPostgresStore(db *pgxpool.Pool, q *pgstore.Queries) *PostgresStore {
	return &PostgresStore{
		db: db,
		q:  q,
	}
}

// 把数据库记录转换为业务用户
func userFromRow(r pgstore.User) *domain.User {
	return &domain.User{
		ID:           r.ID.String(),
		Email:        r.Email,
		PasswordHash: r.PasswordHash,
		Name:         r.Name,
		Role:         r.Role,
		Status:       r.Status,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
}

func notFound(what string, err error) error {
	if err == pgx.ErrNoRows {
		return fmt.Errorf("%s not found", what)
	}
	return fmt.Errorf("get %s: %w", what, err)
}

func (s *PostgresStore) GetUserByEmail(
	ctx context.Context,
	email string,
) (*domain.User, error) {
	row, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, notFound("user", err)
	}
	return userFromRow(row), nil
}

func (s *PostgresStore) GetUserByID(
	ctx context.Context,
	userID string,
) (*domain.User, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}

	row, err := s.q.GetUserByID(ctx, id)
	if err != nil {
		return nil, notFound("user", err)
	}
	return userFromRow(row), nil
}

func (s *PostgresStore) CreateUser(
	ctx context.Context,
	user *domain.User,
) error {
	id, err := uuid.Parse(user.ID)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}

	role := user.Role
	if role == "" {
		role = GlobalRoleMember
	}
	status := user.Status
	if status == "" {
		status = "active"
	}

	row, err := s.q.CreateUser(ctx, pgstore.CreateUserParams{
		ID:           id,
		Email:        user.Email,
		PasswordHash: user.PasswordHash,
		Name:         user.Name,
		Role:         role,
		Status:       status,
	})
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	*user = *userFromRow(row)
	return nil
}

func (s *PostgresStore) ListUsers(
	ctx context.Context,
	limit, offset int32,
) ([]*domain.User, error) {
	rows, err := s.q.ListUsers(ctx, pgstore.ListUsersParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	users := make([]*domain.User, 0, len(rows))
	for _, row := range rows {
		users = append(users, userFromRow(row))
	}
	return users, nil
}

func (s *PostgresStore) SetUserRole(
	ctx context.Context,
	userID, role string,
) error {
	id, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}

	return s.q.SetUserRole(ctx, pgstore.SetUserRoleParams{
		ID:   id,
		Role: role,
	})
}


