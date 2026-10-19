/*
执行引擎,后续负责读取配置,调用模型
关心的是,属于哪个版本?属于哪个agnet?哪个workspace?什么提示词?最多执行多少步?
*/

// Package engine 定义 Agent 数据面的稳定运行边界。
package engine

import (
	"context"
	"fmt"
	"my_kbot/internal/domain"
	"my_kbot/internal/runtime/tooling"

	"github.com/cloudwego/eino/components/model"
)

// AgentSnapshot 是 Runtime 已解析好的固定配置。
type AgentSnapshot struct {
	AgentVersionID string //保存的版本id,也就是version-001这种
	AgentID        string
	WorkspaceID    string
	SystemPrompt   string
	MaxSteps       int
	ToolVersionIDs []string //工具的版本列表
}

// platform 定义引擎读取绘画和配置快照所需的能力
type Platform interface {
	LoadConversation(ctx context.Context, conversationID string) (*domain.Conversation, error)
	GetAgentSnapshotByVersion(ctx context.Context, agentVersionID string) (*AgentSnapshot, error)
}

// ToolRuntime 定义聊天引擎需要的工具能力。
type ToolRuntime interface {
	Bind(ctx context.Context, workspaceID string, versionIDs []string) ([]tooling.Binding, error)
	Execute(ctx context.Context, call tooling.Call) (tooling.Result, error)
}

type Engine struct {
	platform Platform
	model    model.BaseChatModel
	tools    ToolRuntime //还没有保存好工具说明,需要先bind绑定
}

func New(p Platform, model model.BaseChatModel) *Engine {
	return &Engine{
		platform: p,
		model:    model,
	}
}

func (e *Engine) WithTools(tools ToolRuntime) *Engine {
	e.tools = tools
	return e
}

// 获取对应conversationID的agent快照
func (e *Engine) ResolveSnapshot(ctx context.Context, conversationID string) (*AgentSnapshot, error) {
	if e.platform == nil {
		return nil, fmt.Errorf("platform is required")
	}
	conversation, err := e.platform.LoadConversation(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("load conversation :%w", err)
	}
	if conversation.AgentVersionID == "" {
		return nil, fmt.Errorf("conversation has no pinned agent version")
	}

	snapshot, err := e.platform.GetAgentSnapshotByVersion(ctx, conversation.AgentVersionID)
	if err != nil {
		return nil, fmt.Errorf("get agent snapshot:%w", err)
	}
	if snapshot.AgentVersionID != conversation.AgentVersionID {
		return nil, fmt.Errorf("snapshot version mismatch:want %s,got %s", conversation.AgentVersionID, snapshot.AgentVersionID)
	}
	//会话的工作空间要和这个快照的工作空间一致
	if conversation.WorkspaceID != "" && snapshot.WorkspaceID != "" && conversation.WorkspaceID != snapshot.WorkspaceID {
		return nil, fmt.Errorf(
			"conversation and agent snapshot belong to different workspaces",
		)
	}
	return snapshot, nil
}
