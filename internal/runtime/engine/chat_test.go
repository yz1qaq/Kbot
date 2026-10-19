package engine

import (
	"context"
	"errors"
	"my_kbot/internal/domain"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type replyChatModel struct {
	answer string
}

var _ model.BaseChatModel = replyChatModel{}

func (g replyChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	answer := g.answer
	if answer == "" {
		answer = "hello"
	}

	return schema.AssistantMessage(answer, nil), nil
}

func (g replyChatModel) Stream(
	ctx context.Context,
	messages []*schema.Message,
	opts ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	response, err := g.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, err
	}

	runes := []rune(response.Content)
	middle := len(runes) / 2
	if middle == 0 {
		middle = len(runes)
	}

	return schema.StreamReaderFromArray([]*schema.Message{
		schema.AssistantMessage(string(runes[:middle]), nil),
		schema.AssistantMessage(string(runes[middle:]), nil),
	}), nil
}

func TestChatStreamEventOrderAndAnswer(t *testing.T) {
	controlPlane := &fakePlatform{
		conversation: &domain.Conversation{
			ID:             "conversation-001",
			AgentVersionID: "version-001",
		},
		snapshots: map[string]*AgentSnapshot{
			"version-001": {
				AgentVersionID: "version-001",
				SystemPrompt:   "你是一个客服助手",
			},
		},
	}

	runtime := New(controlPlane, replyChatModel{
		answer: "你好世界",
	})

	var events []Event
	err := runtime.ChatStream(
		context.Background(),
		ChatRequest{
			ConversationID: "conversation-001",
			Message:        "介绍一下自己",
		},
		func(event Event) error {
			events = append(events, event)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("chat stream: %v", err)
	}

	want := []Event{
		{Type: "run_started"},
		{Type: "answer_delta", Text: "你好"},
		{Type: "answer_delta", Text: "世界"},
		{Type: "answer_done", Text: "你好世界"},
		{Type: "run_finished"},
	}

	if len(events) != len(want) {
		t.Fatalf("events = %+v, want %d events", events, len(want))
	}
	for i, expected := range want {
		if events[i].Type != expected.Type ||
			events[i].Text != expected.Text {
			t.Fatalf(
				"event[%d] = %+v, want type=%q text=%q",
				i, events[i], expected.Type, expected.Text,
			)
		}
	}
}

func TestEmitContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	err := emitContext(ctx, func(event Event) error {
		called = true
		return nil
	}, Event{
		Type: "answer_delta",
		Text: "这段内容不应发送",
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("emitter must not run after context cancellation")
	}
}

func TestChatStreamRejectsOtherActiveWorkspace(t *testing.T) {
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
				SystemPrompt:   "你是一个客服助手",
			},
		},
	}

	chatModel := &sequenceChatModel{}
	runtime := New(controlPlane, chatModel)

	var events []Event
	err := runtime.ChatStream(
		context.Background(),
		ChatRequest{
			ConversationID: "conversation-001",
			Message:        "查询库存",
			WorkspaceID:    "workspace-2",
		},
		func(event Event) error {
			events = append(events, event)
			return nil
		},
	)

	if err == nil {
		t.Fatal("expected active workspace mismatch error")
	}
	if !strings.Contains(
		err.Error(),
		"conversation is outside the active workspace",
	) {
		t.Fatalf("unexpected error: %v", err)
	}
	if chatModel.calls != 0 {
		t.Fatalf("model calls = %d, want 0", chatModel.calls)
	}
	if len(events) != 0 {
		t.Fatalf("events = %+v, want no events", events)
	}
}
