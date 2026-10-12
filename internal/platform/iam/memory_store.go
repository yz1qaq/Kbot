package iam

import (
	"context"
	"errors"
	"my_kbot/internal/domain"
	"strings"
	"sync"
)

var ErrUserNotFound = errors.New("user not found")

type MemoryStore struct {
	mu           sync.RWMutex
	getUserbyID  map[string]*domain.User                 //使用用户ID匹配User
	getIDbyEmail map[string]string                       //使用用户EMAIL匹配ID
	workspaces   map[string]*domain.Workspace            //使用空间ID匹配Workspace
	memberships  map[string]map[string]domain.Membership //用户ID匹配该用户在各空间的成员关系 .用ID得到一个这个用户的(空间ID,关系)的字典
}

var ErrMembershipNotFound = errors.New("workspace membership not found")

// 模拟存储库,暂时使用内存来存储
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		getUserbyID:  make(map[string]*domain.User),
		getIDbyEmail: make(map[string]string),
		workspaces:   make(map[string]*domain.Workspace),
		memberships:  make(map[string]map[string]domain.Membership), //注意这里只有外层的map
	}
}

// 模拟创建用户,在内存中保存
func (s *MemoryStore) CreateUser(_ context.Context, user *domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(user.Email)
	if _, exist := s.getIDbyEmail[key]; exist {
		return errors.New("email already exists")
	}
	clone := *user
	s.getUserbyID[user.ID] = &clone
	s.getIDbyEmail[key] = user.ID
	return nil
}

//通过邮箱获得User
func (s *MemoryStore) GetUserByEmail(_ context.Context, Email string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := strings.ToLower(Email)
	id, exist := s.getIDbyEmail[key]
	if !exist {
		return nil, ErrUserNotFound
	}
	user, exist := s.getUserbyID[id]
	if !exist {
		return nil, ErrUserNotFound
	}
	//复制一份user返回给调用方
	clone := *user //取出user的值,复制给clone
	return &clone, nil
}

func (s *MemoryStore) GetUser(_ context.Context, id string) (*domain.User, error) {
	if id == "" {
		return nil, ErrUserNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.getUserbyID[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	clone := *user
	return &clone, nil
}

// 创建一个工作空间
func (s *MemoryStore) CreateWorkspace(_ context.Context, workspace *domain.Workspace) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.workspaces[workspace.ID]; exists {
		return errors.New("Workspace already exists")
	}

	clone := *workspace
	s.workspaces[workspace.ID] = &clone
	return nil

}

func (s *MemoryStore) AddMembership(_ context.Context, membership *domain.Membership) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	//判断用户是否存在
	if _, ok := s.getUserbyID[membership.UserID]; !ok {
		return errors.New("membership references an unknow user or workspace")
	}
	//判断工作空间是否存在
	if _, ok := s.workspaces[membership.WorkspaceID]; !ok {
		return errors.New("membership references an unknown user or workspace")
	}
	//判断用户是否已经加入到这个工作空间
	if _, ok := s.memberships[membership.UserID][membership.WorkspaceID]; ok {
		return errors.New("user has joined the workspace")
	}
	if s.memberships[membership.UserID] == nil {
		s.memberships[membership.UserID] = make(map[string]domain.Membership)
	}
	s.memberships[membership.UserID][membership.WorkspaceID] = *membership
	return nil

}

// 得到用户和工作空间的关系
func (s *MemoryStore) GetMembership(_ context.Context, userID, workspaceID string) (*domain.Membership, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if userID == "" {
		return nil, errors.New("userID is Invalied")
	}
	if workspaceID == "" {
		return nil, errors.New("workspaceID is Invalied")
	}
	if _, ok := s.getUserbyID[userID]; !ok {
		return nil, ErrUserNotFound
	}
	if _, ok := s.workspaces[workspaceID]; !ok {
		return nil, errors.New("workspace is not Found")
	}

	membership, ok := s.memberships[userID][workspaceID]
	if !ok {
		return nil, ErrMembershipNotFound
	}
	return &membership, nil
}

// 判断用户和这个工作空间是否有关系
func (s *MemoryStore) HasMembership(_ context.Context, userID, workspaceID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	//判断参数是否正确
	_, ok := s.memberships[userID][workspaceID]
	return ok, nil
}

// 列出这个用户的所有工作空间
func (s *MemoryStore) ListUserWorkspaces(_ context.Context, userID string) ([]*domain.Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if userID == "" {
		return nil, errors.New("userID is Invalied")
	}
	result := make([]*domain.Workspace, 0, len(s.memberships[userID]))

	for workspaceId := range s.memberships[userID] {
		workspace := s.workspaces[workspaceId]
		if workspace == nil {
			continue
		}
		clone := *workspace
		result = append(result, &clone)
	}
	return result, nil

}
