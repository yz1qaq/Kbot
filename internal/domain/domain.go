package domain

import "time"

/*
一个用户可以加入多个空间。
一个空间可以包含多个用户。
*/

// 平台用户
// User 是平台登录身份。
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
}

// Workspace 是业务资源的逻辑隔离边界。
type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// Membership 将用户关联到允许访问的工作空间。
type Membership struct {
	UserID      string    `json:"user_id"`
	WorkspaceID string    `json:"workspace_id"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"created_at"`
}

/*
Q:为什么要定义AgentVersion?
A:假设你要做一个跨境客服助手,提示词是:你是跨境客服助手,xxxxx. 后来你修改了提示词,如果直接覆盖旧配置,那么前面的每次对话会被后续的会话影响
所以项目思路是一个agent可以有多个配置版本,修改配置时创建新版本,保留旧版本
*/

type AgentVersion struct {
	ID           string //这一版配置的唯一标识符  version-001
	AgentID      string //属于哪个agent  agent-001
	WorkspaceID  string //属于哪个工作空间 workspace-001
	Version      int    //版本序号  1
	SystemPrompt string //系统提示词
	CreatedAt    time.Time
}

/*
Conversation 固定一次会话实际使用的 AgentVersion。
后续发布新版本不会改变已经创建的会话,表示一次持续的对话,里面可有多轮交流
*/
type Conversation struct {
	ID             string //这是哪一次对话
	WorkspaceID    string //这次会话属于哪个工作空间
	AgentID        string //这次对话用的哪个助手
	AgentVersionID string //具体实用助手的哪一版配置
	UserID         string //是哪个用户创建的会话
	CreatedAt      time.Time
}

// Message 表示会话中的一条消息记录。
type Message struct {
	ID             string //这条消息的唯一标识	message-001
	ConversationID string //属于哪次会话	demo-conversation
	Role           string //user assistant
	Content        string
	CreatedAt      time.Time
}
