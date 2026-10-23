package llm

import (
	"context"
	"fmt"
	"my_kbot_final/internal/config"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type provider struct {
	model   model.ToolCallingChatModel
	system  string
	modelID string
}

// Gateway 统一管理模型调用。
type Gateway struct {
	cloud provider
}

// NewGateway 根据配置创建模型客户端。
func NewGateway(cfg config.Config) (*Gateway, error) {
	g := &Gateway{}

	// 沿用 Final 的可选配置行为。
	// 未提供密钥时，保留尚未配置模型的 Gateway。
	if cfg.LLMAPIKey == "" {
		return g, nil
	}

	cloudM, err := openai.NewChatModel(
		context.Background(),
		&openai.ChatModelConfig{
			APIKey:  cfg.LLMAPIKey,
			BaseURL: cfg.LLMBaseURL,
			Model:   cfg.LLMModel,
			Timeout: 30 * time.Second,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create cloud chat model: %w", err)
	}

	g.cloud = provider{
		model:   cloudM,
		system:  "openai-compatible",
		modelID: cfg.LLMModel,
	}

	return g, nil
}

func (g *Gateway) Generate(ctx context.Context, messages []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if g.cloud.model == nil {
		return nil, fmt.Errorf("llm model is not configured")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	response, err := g.cloud.model.Generate(ctx, messages, opts...)
	if err != nil {
		return nil, fmt.Errorf("generate model response %w", err)
	}
	return response, nil
}

func (g *Gateway) Stream(ctx context.Context, messages []*schema.Message, opt ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if g.cloud.model == nil {
		return nil, fmt.Errorf("llm model is not configured")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := g.cloud.model.Stream(ctx, messages, opt...)
	if err != nil {
		return nil, fmt.Errorf("generate model response %w", err)
	}
	return reader, nil
}
