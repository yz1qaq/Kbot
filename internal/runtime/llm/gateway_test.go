package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"my_kbot/internal/config"
)

func TestGatewayGenerate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost ||
			r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1,
			"model":   "test-model",
			"choices": []map[string]any{
				{
					"index": 0,
					"message": map[string]any{
						"role":    "assistant",
						"content": "课堂模型回复",
					},
					"finish_reason": "stop",
				},
			},
		})
	}))
	defer server.Close()

	gateway, err := NewGateway(config.Config{
		LLMBaseURL: server.URL + "/v1",
		LLMAPIKey:  "test-key",
		LLMModel:   "test-model",
		LLMTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("new gateway: %v", err)
	}

	response, err := gateway.Generate(
		context.Background(),
		[]*schema.Message{
			schema.UserMessage("你好"),
		},
	)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if response == nil || response.Content != "课堂模型回复" {
		t.Fatalf("unexpected response: %+v", response)
	}
}
