package llm

import (
	"context"
	"fmt"
	"my_kbot/internal/config"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type Gateway struct {
	model model.BaseChatModel
}

var _ model.BaseChatModel = (*Gateway)(nil)

func NewGateway(cfg config.Config) (*Gateway, error) {
	chatModel, err := openai.NewChatModel(
		context.Background(),
		&openai.ChatModelConfig{
			APIKey:  cfg.LLMAPIKey,
			BaseURL: cfg.LLMBaseURL,
			Model:   cfg.LLMModel,
			Timeout: cfg.LLMTimeout,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}
	return &Gateway{model: chatModel}, nil
}

// Generate 调用底层模型，等待并返回完整响应。
func (g *Gateway) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if g == nil || g.model == nil {
		return nil, fmt.Errorf("chat model is required")
	}

	response, err := g.model.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("generate response: %w", err)
	}

	return response, nil
}

// Stream 调用底层模型，返回可逐步读取的响应流。
func (g *Gateway) Stream(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if g == nil || g.model == nil {
		return nil, fmt.Errorf("chat model is required")
	}

	stream, err := g.model.Stream(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("stream response: %w", err)
	}

	return stream, nil
}
