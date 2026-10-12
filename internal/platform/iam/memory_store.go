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
