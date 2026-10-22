package iam

import (
	"context"
	"errors"
	"fmt"
	"my_kbot_final/internal/domain"
	pgstore "my_kbot_final/internal/infrastructure/postgres/sqlc"

	"github.com/google/uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	db *pgxpool.Pool
	q  *pgstore.Queries
}

var _ Store = (*PostgresStore)(nil)

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

func (s *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	row, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, notFound("user", err)
	}
	return userFromRow(row), nil
}

func (s *PostgresStore) GetUserByID(ctx context.Context, userID string) (*domain.User, error) {
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

func (s *PostgresStore) CreateUser(ctx context.Context, user *domain.User) error {
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

func (s *PostgresStore) ListUsers(ctx context.Context, limit, offset int32) ([]*domain.User, error) {
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

func (s *PostgresStore) SetUserRole(ctx context.Context, userID, role string) error {
	id, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}

	return s.q.SetUserRole(ctx, pgstore.SetUserRoleParams{
		ID:   id,
		Role: role,
	})
}

func textFromPtr(p *string) pgtype.Text {
	if p == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{
		String: *p,
		Valid:  true,
	}
}

func ptrFromText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	s := t.String
	return &s
}

// 从数据库的格式变为domain.Workspace的格式
func workspaceFromRow(r pgstore.Workspace) *domain.Workspace {
	return &domain.Workspace{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		ParentID:    ptrFromText(r.ParentID),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func (s *PostgresStore) GetWorkspaceByID(ctx context.Context, workspaceID string) (*domain.Workspace, error) {
	row, err := s.q.GetWorkspaceByID(ctx, workspaceID)
	if err != nil {
		return nil, notFound("workspace", err)
	}
	return workspaceFromRow(row), nil
}

func (s *PostgresStore) CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error {
	row, err := s.q.CreateWorkspace(ctx, pgstore.CreateWorkspaceParams{
		ID:          workspace.ID,
		Name:        workspace.Name,
		Description: workspace.Description,
		ParentID:    textFromPtr(workspace.ParentID),
	})
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	*workspace = *workspaceFromRow(row)
	return nil
}

func (s *PostgresStore) ListWorkspaces(ctx context.Context, limit, offset int32) ([]*domain.Workspace, error) {
	rows, err := s.q.ListWorkspaces(ctx, pgstore.ListWorkspacesParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}

	workspaces := make([]*domain.Workspace, 0, len(rows))
	for _, row := range rows {
		workspaces = append(workspaces, workspaceFromRow(row))
	}
	return workspaces, nil
}

func (s *PostgresStore) ListUserWorkspaces(ctx context.Context,
	userID string, limit, offset int32) ([]*domain.Workspace, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user id: %w", err)
	}
	userWorkspaces, err := s.q.ListUserWorkspaces(ctx, pgstore.ListUserWorkspacesParams{
		UserID: id,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list user workspaces: %w", err)
	}

	workspaces := make([]*domain.Workspace, 0, len(userWorkspaces))
	for _, row := range userWorkspaces {
		workspaces = append(workspaces, workspaceFromRow(row))
	}
	return workspaces, nil
}

// 获取一个工作空间的member
func (s *PostgresStore) GetWorkspaceMember(ctx context.Context,
	workspaceID, userID string) (*domain.WorkspaceMember, error) {

	id, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user id %w", err)
	}
	row, err := s.q.GetWorkspaceMember(ctx, pgstore.GetWorkspaceMemberParams{
		WorkspaceID: workspaceID,
		UserID:      id,
	})
	if err != nil {
		return nil, notFound("workspaceMember", err)
	}

	return &domain.WorkspaceMember{
		WorkspaceID: row.WorkspaceID,
		UserID:      row.UserID.String(),
		Email:       row.Email,
		Name:        row.Name,
		Role:        row.Role,
		CreatedAt:   row.CreatedAt,
	}, nil

}

// 列出工作空间的member
func (s *PostgresStore) ListWorkspaceMembers(ctx context.Context,
	workspaceID string) ([]*domain.WorkspaceMember, error) {
	rows, err := s.q.ListWorkspaceMembers(ctx, workspaceID)
	if err != nil {
		return nil, notFound("workspace members", err)
	}

	workspaceMembers := make([]*domain.WorkspaceMember, 0, len(rows))
	for _, row := range rows {
		workspaceMembers = append(workspaceMembers, &domain.WorkspaceMember{
			WorkspaceID: row.WorkspaceID,
			UserID:      row.UserID.String(),
			Email:       row.Email,
			Name:        row.Name,
			Role:        row.Role,
			CreatedAt:   row.CreatedAt,
		})
	}
	return workspaceMembers, nil
}

// 插入一个新的WorkspaceMember
func (s *PostgresStore) UpsertWorkspaceMember(ctx context.Context, member *domain.WorkspaceMember) error {
	id, err := uuid.Parse(member.UserID)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}
	row, err := s.q.UpsertWorkspaceMember(ctx, pgstore.UpsertWorkspaceMemberParams{
		UserID:      id,
		WorkspaceID: member.WorkspaceID,
		Role:        member.Role,
	})
	if err != nil {
		return fmt.Errorf("upsert workspace meber: %w", err)
	}
	member.CreatedAt = row.CreatedAt
	//member.UserID = row.UserID.String()
	return nil
}

// 删除一个工作空间的member
func (s *PostgresStore) DeleteWorkspaceMember(ctx context.Context, workspaceID, userID string) (bool, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return false, fmt.Errorf("parse user id: %w", err)
	}
	rows, err := s.q.DeleteWorkspaceMember(ctx, pgstore.DeleteWorkspaceMemberParams{
		WorkspaceID: workspaceID,
		UserID:      id,
	})
	if err != nil {
		return false, fmt.Errorf("delete workspace member: %w", err)
	}
	return rows > 0, err

}

func (s *PostgresStore) CreateWorkspaceWithOwner(ctx context.Context, workspace *domain.Workspace, userID string) error {
	id, err := uuid.Parse(userID)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}
	err = s.q.CreateWorkspaceWithOwner(ctx, pgstore.CreateWorkspaceWithOwnerParams{
		UserID:      id,
		WorkspaceID: workspace.ID,
		Name:        workspace.Name,
		Description: workspace.Description,
		ParentID:    textFromPtr(workspace.ParentID),
	})
	if err != nil {
		return fmt.Errorf("create workspace with owner: %w", err)
	}
	return nil
}

// lockWorkspace 在当前事务中锁定指定工作空间行。
func lockWorkspace(ctx context.Context, tx pgx.Tx, workspaceID string) error {
	var id string
	err := tx.QueryRow(
		ctx,
		`SELECT id FROM workspaces WHERE id = $1 FOR UPDATE`,
		workspaceID,
	).Scan(&id)
	if err != nil {
		return notFound("workspace", err)
	}

	return nil
}

// workspaceMemberRole 查询目标成员在当前事务中的实际空间角色。
func workspaceMemberRole(ctx context.Context, tx pgx.Tx, workspaceID string, userID uuid.UUID) (string, bool, error) {
	var role string
	err := tx.QueryRow(
		ctx,
		`SELECT role
		 FROM workspace_members
		 WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID,
		userID,
	).Scan(&role)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}

	return role, err == nil, err
}

// workspaceOwnerCount 统计当前事务中指定空间的 owner 数量。
func workspaceOwnerCount(ctx context.Context, tx pgx.Tx, workspaceID string) (int, error) {
	var count int
	err := tx.QueryRow(
		ctx,
		`SELECT COUNT(*)
		 FROM workspace_members
		 WHERE workspace_id = $1 AND role = 'owner'`,
		workspaceID,
	).Scan(&count)

	return count, err
}

// 修改某用户在某工作空间的role
func (s *PostgresStore) UpsertWorkspaceMemberGuarded(ctx context.Context,
	member *domain.WorkspaceMember, allowOwnerManagement bool) error {
	if s.db == nil {
		return fmt.Errorf("membership transaction database is not configured")
	}
	uid, err := uuid.Parse(member.UserID)
	if err != nil {
		return fmt.Errorf("parse user id: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err = lockWorkspace(ctx, tx, member.WorkspaceID); err != nil {
		return err
	}

	existingRole, exists, err := workspaceMemberRole(ctx, tx, member.WorkspaceID, uid)
	if err != nil {
		return err
	}

	if (member.Role == WorkspaceRoleOwner || exists && existingRole == WorkspaceRoleOwner) && !allowOwnerManagement {
		return ErrForbidden
	}

	//如果用户已经在工作空间,并且是owner,修改为member.Role
	if exists && existingRole == WorkspaceRoleOwner && member.Role != WorkspaceRoleOwner {
		//先判断当前事务该空间有几个owner
		owners, err := workspaceOwnerCount(
			ctx, tx, member.WorkspaceID,
		)
		if err != nil {
			return err
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	//如果该工作空间有多个owner,则允许把其修改role
	row, err := s.q.WithTx(tx).UpsertWorkspaceMember(ctx,
		pgstore.UpsertWorkspaceMemberParams{
			WorkspaceID: member.WorkspaceID,
			UserID:      uid,
			Role:        member.Role,
		},
	)
	if err != nil {
		return fmt.Errorf("upsert workspace member: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	member.CreatedAt = row.CreatedAt

	return nil

}

func (s *PostgresStore) DeleteWorkspaceMemberGuarded(ctx context.Context,
	workspaceID, userID string, allowOwnerManagement bool) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("membership transaction database is not configured")
	}
	uid, err := uuid.Parse(userID)
	if err != nil {
		return false, fmt.Errorf("parse user id: %w", err)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err = lockWorkspace(ctx, tx, workspaceID); err != nil {
		return false, err
	}
	existingRole, exists, err := workspaceMemberRole(ctx, tx, workspaceID, uid)
	if err != nil || !exists {
		return false, err
	}

	// 如果用户是owner,则判断是否允许删除owner
	if existingRole == WorkspaceRoleOwner {
		//如果不允许删除owner
		if !allowOwnerManagement {
			return false, ErrForbidden
		}
		//如果允许,判断有多少owner
		owners, err := workspaceOwnerCount(ctx, tx, workspaceID)
		if err != nil {
			return false, err
		}
		//如果只有一个owner,则不允许删除
		if owners <= 1 {
			return false, ErrLastOwner
		}
	}
	rows, err := s.q.WithTx(tx).DeleteWorkspaceMember(ctx,
		pgstore.DeleteWorkspaceMemberParams{
			WorkspaceID: workspaceID,
			UserID:      uid,
		},
	)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return rows > 0, nil

}
