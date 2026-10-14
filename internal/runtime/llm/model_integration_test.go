//go:build integration

package llm

import (
	"context"
	"strings"
	"testing"
	"time"

	"my_kbot/internal/config"

	"github.com/cloudwego/eino/schema"
)

func TestRealModelGenerate(t *testing.T) {
	cfg := config.Load()
	if strings.TrimSpace(cfg.LLMAPIKey) == "" {
		t.Fatal("真实模型测试需要配置 LLM API Key")
	}
	cfg.LLMTimeout = 60 * time.Second

	gateway, err := NewGateway(cfg)
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		cfg.LLMTimeout,
	)
	defer cancel()

	response, err := gateway.Generate(ctx, []*schema.Message{
		schema.UserMessage("请用一句话介绍你自己。"),
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if response == nil || strings.TrimSpace(response.Content) == "" {
		t.Fatal("模型返回了空内容")
	}

	t.Logf("model=%s response=%s", cfg.LLMModel, response.Content)
}
