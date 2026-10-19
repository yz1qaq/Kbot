package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"my_kbot/internal/runtime/tooling"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type sequenceChatModel struct {
	calls         int
	sawToolResult bool
	alwaysTool    bool
}

func (m *sequenceChatModel) Generate(
	_ context.Context,
	messages []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	m.calls++

	for _, message := range messages {
		if message.Role == schema.Tool {
			m.sawToolResult = true
		}
	}

	if m.calls == 1 || m.alwaysTool {
		return schema.AssistantMessage("", []schema.ToolCall{
			{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "search_inventory",
					Arguments: `{"sku":"SKU-BLACK-M-01"}`,
				},
			},
		}), nil
	}

	return schema.AssistantMessage("库存还有12件", nil), nil
}

func (m *sequenceChatModel) Stream(
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

var _ model.BaseChatModel = (*sequenceChatModel)(nil)

type recordingToolExecutor struct {
	call  tooling.Call
	calls int
}

func (e *recordingToolExecutor) Execute(
	_ context.Context,
	call tooling.Call,
) (tooling.Result, error) {
	e.call = call
	e.calls++

	return tooling.Result{
		StatusCode: 200,
		Body:       []byte(`{"available":12}`),
	}, nil
}

func TestBindingToolForwardsCall(t *testing.T) {
	executor := &recordingToolExecutor{}

	tool := &bindingTool{
		binding: ToolBinding{
			Name:      "search_inventory",
			VersionID: "inventory-v1",
		},
		executor:    executor,
		workspaceID: "workspace-1",
	}

	ctx := context.WithValue(
		context.Background(),
		toolCallIDKey{},
		"call-1",
	)
	arguments := `{"sku":"SKU-BLACK-M-01"}`

	output, err := tool.InvokableRun(ctx, arguments)
	if err != nil {
		t.Fatalf("InvokableRun(): %v", err)
	}

	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}

	call := executor.call
	if call.WorkspaceID != "workspace-1" ||
		call.ToolVersionID != "inventory-v1" {
		t.Fatalf("unexpected call target: %+v", call)
	}

	if string(call.Arguments) != arguments {
		t.Fatalf("arguments = %q, want %q", call.Arguments, arguments)
	}

	if call.IdempotencyKey != "reactcall-1" {
		t.Fatalf(
			"idempotency key = %q, want reactcall-1",
			call.IdempotencyKey,
		)
	}

	if output != `{"available":12}` {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestToolEventMiddlewareReturnsToolErrorResult(t *testing.T) {
	middleware := toolEventMiddleware(nil)

	wrapped := middleware.Invokable(
		func(
			ctx context.Context,
			input *compose.ToolInput,
		) (*compose.ToolOutput, error) {
			return nil, errors.New("tool failed")
		},
	)

	output, err := wrapped(
		context.Background(),
		&compose.ToolInput{
			Name:   "search_inventory",
			CallID: "call-1",
		},
	)

	if err != nil {
		t.Fatalf("middleware returned error: %v", err)
	}
	if output == nil {
		t.Fatal("expected tool output")
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(output.Result), &payload); err != nil {
		t.Fatalf("invalid result JSON %q: %v", output.Result, err)
	}

	if payload["error"] != "tool failed" {
		t.Fatalf("error message = %q, want tool failed", payload["error"])
	}
}

func TestToolEventMiddlewareWrapsBindingTool(t *testing.T) {
	executor := &recordingToolExecutor{}
	tool := &bindingTool{
		binding: ToolBinding{
			Name:      "search_inventory",
			VersionID: "inventory-v1",
		},
		executor:    executor,
		workspaceID: "workspace-1",
	}

	var events []string
	middleware := toolEventMiddleware(func(event Event) error {
		events = append(events, event.Type)
		return nil
	})

	wrapped := middleware.Invokable(
		func(
			ctx context.Context,
			input *compose.ToolInput,
		) (*compose.ToolOutput, error) {
			events = append(events, "execute")

			result, err := tool.InvokableRun(ctx, input.Arguments)
			if err != nil {
				return nil, err
			}

			return &compose.ToolOutput{Result: result}, nil
		},
	)

	output, err := wrapped(
		context.Background(),
		&compose.ToolInput{
			Name:      "search_inventory",
			Arguments: `{"sku":"SKU-BLACK-M-01"}`,
			CallID:    "call-1",
		},
	)
	if err != nil {
		t.Fatalf("wrapped call: %v", err)
	}

	wantOrder := "tool_started -> execute -> tool_finished"
	if got := strings.Join(events, " -> "); got != wantOrder {
		t.Fatalf("event order = %q, want %q", got, wantOrder)
	}

	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}

	if executor.call.IdempotencyKey != "reactcall-1" {
		t.Fatalf(
			"idempotency key = %q, want reactcall-1",
			executor.call.IdempotencyKey,
		)
	}

	if output == nil || output.Result != `{"available":12}` {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestADKRunnerRejectsNonPositiveMaxSteps(t *testing.T) {
	runner := NewADKRunner(
		replyChatModel{},
		&recordingToolExecutor{},
		"workspace-1",
	)

	for _, maxSteps := range []int{-1, 0} {
		_, err := runner.Run(
			context.Background(),
			nil,
			nil,
			maxSteps,
			nil,
		)

		if err == nil || err.Error() != "max steps must be positive" {
			t.Fatalf(
				"Run(maxSteps=%d) error = %v, want max steps must be positive",
				maxSteps,
				err,
			)
		}
	}
}

func TestADKRunnerFeedsToolResultBackToModel(t *testing.T) {
	chatModel := &sequenceChatModel{}
	executor := &recordingToolExecutor{}

	binding := ToolBinding{
		Name:      "search_inventory",
		VersionID: "inventory-v1",
		Info: &schema.ToolInfo{
			Name: "search_inventory",
			Desc: "根据 SKU 查询库存",
			ParamsOneOf: schema.NewParamsOneOfByParams(
				map[string]*schema.ParameterInfo{
					"sku": {
						Type:     schema.String,
						Required: true,
					},
				},
			),
		},
	}

	runner := NewADKRunner(chatModel, executor, "workspace-1")

	var events []string
	answer, err := runner.Run(
		context.Background(),
		[]*schema.Message{
			schema.UserMessage("查询 SKU-BLACK-M-01 的库存"),
		},
		[]ToolBinding{binding},
		4,
		func(event Event) error {
			events = append(events, event.Type)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}

	if answer == nil || answer.Content != "库存还有12件" {
		t.Fatalf("unexpected answer: %+v", answer)
	}

	if chatModel.calls != 2 || !chatModel.sawToolResult {
		t.Fatalf(
			"model calls = %d, saw tool result = %v",
			chatModel.calls,
			chatModel.sawToolResult,
		)
	}

	if executor.calls != 1 ||
		executor.call.ToolVersionID != "inventory-v1" ||
		executor.call.WorkspaceID != "workspace-1" {
		t.Fatalf(
			"executor calls = %d, call = %+v",
			executor.calls,
			executor.call,
		)
	}

	if executor.call.IdempotencyKey != "reactcall-1" {
		t.Fatalf("unexpected idempotency key: %q", executor.call.IdempotencyKey)
	}

	wantEvents := "tool_started -> tool_finished"
	if got := strings.Join(events, " -> "); got != wantEvents {
		t.Fatalf("events = %q, want %q", got, wantEvents)
	}
}

func TestADKRunnerStopsAtMaxSteps(t *testing.T) {
	chatModel := &sequenceChatModel{
		alwaysTool: true,
	}
	executor := &recordingToolExecutor{}

	binding := ToolBinding{
		Name:      "search_inventory",
		VersionID: "inventory-v1",
		Info: &schema.ToolInfo{
			Name: "search_inventory",
			Desc: "根据 SKU 查询库存",
			ParamsOneOf: schema.NewParamsOneOfByParams(
				map[string]*schema.ParameterInfo{
					"sku": {
						Type:     schema.String,
						Required: true,
					},
				},
			),
		},
	}

	runner := NewADKRunner(chatModel, executor, "workspace-1")
	answer, err := runner.Run(
		context.Background(),
		[]*schema.Message{
			schema.UserMessage("查询库存"),
		},
		[]ToolBinding{binding},
		2,
		nil,
	)

	if !errors.Is(err, adk.ErrExceedMaxIterations) {
		t.Fatalf(
			"Run() error = %v, want ErrExceedMaxIterations",
			err,
		)
	}

	if answer != nil {
		t.Fatalf("expected no final answer, got %+v", answer)
	}

	if chatModel.calls != 2 {
		t.Fatalf("model calls = %d, want 2", chatModel.calls)
	}
}
