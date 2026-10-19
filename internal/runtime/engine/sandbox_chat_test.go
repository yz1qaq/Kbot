package engine

import (
	"context"

	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"my_kbot/internal/domain"
	platformtool "my_kbot/internal/platform/tool"
	"my_kbot/internal/runtime/tooling"
)

type codeToolChatModel struct {
	calls         int
	sawToolResult bool
}

func (m *codeToolChatModel) Generate(
	_ context.Context,
	messages []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	m.calls++

	for _, message := range messages {
		if message.Role == schema.Tool &&
			message.Content == "42\n" {
			m.sawToolResult = true
		}
	}

	if m.calls == 1 {
		return schema.AssistantMessage("", []schema.ToolCall{
			{
				ID:   "code-call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "code_run_python",
					Arguments: `{"code":"print(6 * 7)"}`,
				},
			},
		}), nil
	}

	return schema.AssistantMessage("计算结果是 42", nil), nil
}

func (m *codeToolChatModel) Stream(
	ctx context.Context,
	messages []*schema.Message,
	opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	response, err := m.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}

	return schema.StreamReaderFromArray(
		[]*schema.Message{response},
	), nil
}

var _ model.BaseChatModel = (*codeToolChatModel)(nil)

type classroomSandbox struct {
	language string
	code     string
}

func (s *classroomSandbox) Run(
	_ context.Context,
	language, code string,
) (string, error) {
	s.language = language
	s.code = code

	return "42\n", nil
}

func TestChatStreamRunsPinnedCodeToolThroughSandbox(t *testing.T) {
	ctx := context.Background()

	// 注册代码工具版本。
	registry := platformtool.NewRegistry()
	err := registry.Register(ctx, platformtool.Version{
		ID:          "python-v1",
		ToolID:      "python-tool",
		WorkspaceID: "workspace-1",
		Name:        "code_run_python",
		SourceType:  "code_execution",
		Description: "执行 Python 代码并返回标准输出",
		Endpoint:    "python",
		Published:   true,
		InputSchema: []byte(
			`{"type":"object","properties":{"code":{"type":"string"}},"required":["code"],"additionalProperties":false}`,
		),
	})
	if err != nil {
		t.Fatalf("register code tool: %v", err)
	}

	// 对话绑定 Agent 版本，Agent 版本绑定代码工具版本。
	controlPlane := &fakePlatform{
		conversation: &domain.Conversation{
			ID:             "conversation-001",
			AgentVersionID: "version-001",
			WorkspaceID:    "workspace-1",
		},
		snapshots: map[string]*AgentSnapshot{
			"version-001": {
				AgentVersionID: "version-001",
				WorkspaceID:    "workspace-1",
				SystemPrompt:   "需要计算时使用 Python 工具",
				MaxSteps:       4,
				ToolVersionIDs: []string{"python-v1"},
			},
		},
	}

	chatModel := &codeToolChatModel{}
	runner := &classroomSandbox{}

	executor := tooling.NewExecutor(
		registry,
		nil,
	).WithSandbox(runner)

	runtime := New(controlPlane, chatModel).WithTools(executor)

	var answer string
	err = runtime.ChatStream(
		ctx,
		ChatRequest{
			ConversationID: "conversation-001",
			WorkspaceID:    "workspace-1",
			Message:        "计算 6 * 7",
		},
		func(event Event) error {
			if event.Type == "answer_done" {
				answer = event.Text
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStream(): %v", err)
	}

	if runner.language != "python" || runner.code != "print(6 * 7)" {
		t.Fatalf(
			"sandbox received language=%q code=%q",
			runner.language,
			runner.code,
		)
	}

	if chatModel.calls != 2 || !chatModel.sawToolResult {
		t.Fatalf(
			"model calls = %d, saw tool result = %v",
			chatModel.calls,
			chatModel.sawToolResult,
		)
	}

	if answer != "计算结果是 42" {
		t.Fatalf("answer = %q, want 计算结果是 42", answer)
	}
}
