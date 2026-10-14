// Package platform 是 Agent 控制面。
package platform

import (
	"context"
	"fmt"
	"my_kbot/internal/domain"
	"my_kbot/internal/runtime/engine"
	"sync"
)

// Platform 是控制面服务的装配入口。
// 具体服务会在后续课程中逐步加入。
type Platform struct {
	mu            sync.RWMutex
	conversations map[string]*domain.Conversation  //会话id查询会话
	snapshots     map[string]*engine.AgentSnapshot //版本id查询快照
}

// New 创建控制面。
func New() *Platform {
	return &Platform{
		conversations: make(map[string]*domain.Conversation),
		snapshots:     make(map[string]*engine.AgentSnapshot),
	}
}

// 保存会话
func (p *Platform) PutConversation(conversation *domain.Conversation) {
	p.mu.Lock()
	defer p.mu.Unlock()
	copy := *conversation
	p.conversations[copy.ID] = &copy
}

// LoadConversation 根据 ID 查询会话，并返回副本。
func (p *Platform) LoadConversation(_ context.Context, id string) (*domain.Conversation, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	conversation, ok := p.conversations[id]
	if !ok {
		return nil, fmt.Errorf("conversation %s not found", id)
	}

	copy := *conversation
	return &copy, nil
}

func (p *Platform) PutSnapshot(snapshot *engine.AgentSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	copy := *snapshot
	p.snapshots[copy.AgentVersionID] = &copy
}

// 根据agentversion返回快照
func (p *Platform) GetAgentSnapshotByVersion(_ context.Context, id string) (*engine.AgentSnapshot, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	snapshot, ok := p.snapshots[id]
	if !ok {
		return nil, fmt.Errorf("agent snapshot %s not found", id)
	}
	copy := *snapshot
	return &copy, nil
}

var _ engine.Platform = (*Platform)(nil)
