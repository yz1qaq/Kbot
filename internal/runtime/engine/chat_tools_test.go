package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"my_kbot/internal/domain"
	platformtool "my_kbot/internal/platform/tool"
	"my_kbot/internal/runtime/tooling"
)

func TestChatStreamRunsPinnedHTTPTool(t *testing.T) {
	// 临时 HTTP 服务收到工具请求的次数。
	var toolCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			toolCalls.Add(1)

			if r.Method != http.MethodPost {
				t.Errorf("method = %q, want POST", r.Method)
			}
			if got := r.Header.Get("Idempotency-Key"); got != "reactcall-1" {
				t.Errorf(
					"idempotency key = %q, want reactcall-1",
					got,
				)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"available":12}`))
		},
	))
	defer server.Close()

	// 注册一个真实的工具版本，地址指向临时 HTTP 服务。
	registry := platformtool.NewRegistry()
	err := registry.Register(context.Background(), platformtool.Version{
		ID:          "inventory-v1",
		ToolID:      "inventory-tool",
		WorkspaceID: "workspace-1",
		Name:        "search_inventory",
		Description: "根据 SKU 查询库存",
		Endpoint:    server.URL,
		Published:   true,
		InputSchema: []byte(
			`{"type":"object","properties":{"sku":{"type":"string"}},"required":["sku"],"additionalProperties":false}`,
		),
	})
	if err != nil {
		t.Fatalf("register tool: %v", err)
	}

	// 对话固定使用 version-001，该快照固定使用 inventory-v1。
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
				SystemPrompt:   "你是一个库存助手",
				MaxSteps:       4,
				ToolVersionIDs: []string{"inventory-v1"},
			},
		},
	}

	chatModel := &sequenceChatModel{}
	executor := tooling.NewExecutor(
		registry,
		server.Client(),
		"127.0.0.1",
	)
	runtime := New(controlPlane, chatModel).WithTools(executor)

	var eventTypes []string
	var answer string

	err = runtime.ChatStream(
		context.Background(),
		ChatRequest{
			ConversationID: "conversation-001",
			WorkspaceID:    "workspace-1",
			Message:        "查询 SKU-BLACK-M-01 的库存",
		},
		func(event Event) error {
			eventTypes = append(eventTypes, event.Type)
			if event.Type == "answer_done" {
				answer = event.Text
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStream(): %v", err)
	}

	if got := toolCalls.Load(); got != 1 {
		t.Fatalf("HTTP tool calls = %d, want 1", got)
	}
	if chatModel.calls != 2 || !chatModel.sawToolResult {
		t.Fatalf(
			"model calls = %d, saw tool result = %v",
			chatModel.calls,
			chatModel.sawToolResult,
		)
	}
	if answer != "库存还有12件" {
		t.Fatalf("answer = %q, want 库存还有12件", answer)
	}

	want := "run_started -> tool_started -> tool_finished -> answer_delta -> answer_done -> run_finished"
	if got := strings.Join(eventTypes, " -> "); got != want {
		t.Fatalf("events = %q, want %q", got, want)
	}
}
