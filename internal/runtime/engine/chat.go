package engine

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// ChatRequest 是一次聊天请求。
type ChatRequest struct {
	ConversationID string `json:"conversation_id"`
	Message        string `json:"message"`
}

// Event 是引擎向外输出的事件。
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
	Text string `json:"text,omitempty"`
}

func (e *Engine) ChatStream(ctx context.Context, req ChatRequest, emit Emitter) error {
	//参数检查
	conversationID := strings.TrimSpace(req.ConversationID)
	message := strings.TrimSpace(req.Message)
	if conversationID == "" || message == "" {
		return fmt.Errorf("conversation_id and message are required")
	}
	if emit == nil {
		return fmt.Errorf("emitter is required")
	}
	//获取对应对话的快照
	snapshot, err := e.ResolveSnapshot(ctx, conversationID)
	if err != nil {
		return err
	}
	if err = emitContext(ctx, emit, Event{
		Type: "run_started",
		Data: map[string]string{
			"conversation_id":  conversationID,
			"agent_version_id": snapshot.AgentVersionID,
		},
	}); err != nil {
		return err
	}

	//接入大模型对话
	messages := []*schema.Message{
		schema.SystemMessage(snapshot.SystemPrompt),
		schema.UserMessage(message),
	}
	if e.model == nil {
		return fmt.Errorf("chat model is required")
	}
	stream, err := e.model.Stream(ctx, messages)
	if err != nil {
		_ = emitContext(ctx, emit, Event{
			Type: "error",
			Data: map[string]string{"message": err.Error()},
		})
		return fmt.Errorf("stream: %w", err)
	}
	defer stream.Close()

	var answer strings.Builder

	for {
		chunk, recvErr := stream.Recv()
		if recvErr == io.EOF {
			break
		}
		if recvErr != nil {
			return fmt.Errorf("receive stream: %w", recvErr)
		}
		if chunk == nil || chunk.Content == "" {
			continue
		}

		answer.WriteString(chunk.Content)

		if err := emitContext(ctx, emit, Event{
			Type: "answer_delta",
			Text: chunk.Content,
		}); err != nil {
			return err
		}
	}

	if err := emitContext(ctx, emit, Event{
		Type: "answer_done",
		Text: answer.String(),
	}); err != nil {
		return err
	}

	return emitContext(ctx, emit, Event{
		Type: "run_finished",
		Data: map[string]string{"status": "completed"},
	})

}

// Emitter 是负责输出一个事件的函数类型。
type Emitter func(Event) error

func emitContext(ctx context.Context, emit Emitter, event Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err() //如果取消了,就返回取消原因
	default:
		return emit(event) //没取消就调用emit,并且返回执行结果
	}
}
