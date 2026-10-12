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

	"github.com/cloudwego/eino/schema"
)

// AgentSnapshot 是 Runtime 已解析好的固定配置。
type AgentSnapshot struct {
	ID           string //保存的版本id,也就是version-001这种
	AgentID      string
	WorkspaceID  string
	SystemPrompt string
	MaxSteps     int
}

// platform 定义引擎读取绘画和配置快照所需的能力
type Platform interface {
	LoadConversation(ctx context.Context, conversationID string) (*domain.Conversation, error)
	GetAgentSnapshotByVersion(ctx context.Context, agentVersionID string) (*AgentSnapshot, error)
}

// 定义引擎调用模型所需能力
type Generator interface {
	Generate(ctx context.Context, messages []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error)
}

type Engine struct {
	platform Platform
	gen      Generator
}

func New(p Platform, g Generator) *Engine {
	return &Engine{
		platform: p,
		gen:      g,
	}
}

func (e *Engine) ResolveSnapshot(ctx context.Context,conversationID string)(*AgentSnapshot,error){
	if e.platform==nil{
		return nil,fmt.Errorf("platform is required")
	}
	conversation,err:=e.platform.LoadConversation(ctx,conversationID)
	if err!=nil{
		return nil,fmt.Errorf("load conversation :%w",err)
	}
	if conversation.AgentVersionID == ""{
		return nil,fmt.Errorf("conversation has no pinned agent version")
	}

	snapshot,err:=e.platform.GetAgentSnapshotByVersion(ctx,conversation.AgentVersionID)
	if err!=nil{
		return nil,fmt.Errorf("get agent snapshot:%w",err)
	}
	if snapshot.ID != conversation.AgentVersionID{
		return  nil,fmt.Errorf("snapshot version mismatch:want %s,got %s",conversation.AgentVersionID,snapshot.ID)
	}
	return snapshot,nil
}