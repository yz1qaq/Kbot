package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"my_kbot_final/internal/config"
)

func TestNewGateway(t *testing.T) {
	t.Run("absent credential leaves model disabled", func(t *testing.T) {
		gateway, err := NewGateway(config.Config{})
		if err != nil {
			t.Fatalf("optional model configuration should not fail: %v", err)
		}
		if gateway == nil || gateway.cloud.model != nil {
			t.Fatal("missing credential must leave a non-nil gateway with no enabled model")
		}
	})

	t.Run("configured client is created without a provider request", func(t *testing.T) {
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		gateway, err := NewGateway(config.Config{
			LLMBaseURL: server.URL + "/v1",
			LLMAPIKey:  "test-only-constructor-key",
			LLMModel:   "test-model",
		})
		if err != nil {
			t.Fatalf("construct configured model: %v", err)
		}
		if gateway == nil || gateway.cloud.model == nil {
			t.Fatal("configured gateway must hold a model client")
		}
		if gateway.cloud.system != "openai-compatible" || gateway.cloud.modelID != "test-model" {
			t.Fatal("model metadata was not retained")
		}
		if requests.Load() != 0 {
			t.Fatal("construction must not contact the provider")
		}
	})
}

func newHTTPTestGateway(t *testing.T, handler http.Handler) *Gateway {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	gateway, err := NewGateway(config.Config{
		LLMBaseURL: server.URL + "/v1", LLMAPIKey: "test-only-provider-key", LLMModel: "test-model",
	})
	if err != nil {
		t.Fatalf("construct local test gateway: %v", err)
	}
	return gateway
}

func TestGenerate(t *testing.T) {
	messages := []*schema.Message{schema.SystemMessage("你是 Go 助手。"), schema.UserMessage("什么是接口？")}

	t.Run("unconfigured model is rejected", func(t *testing.T) {
		gateway, err := NewGateway(config.Config{})
		if err != nil {
			t.Fatal(err)
		}
		response, err := gateway.Generate(context.Background(), messages)
		if err == nil || response != nil {
			t.Fatal("an unconfigured model must return an error and no response")
		}
	})

	t.Run("HTTP messages options and full response are preserved", func(t *testing.T) {
		type requestBody struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			MaxTokens int `json:"max_tokens"`
		}
		requests := make(chan requestBody, 1)
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
				t.Error("unexpected provider method or path")
			}
			if r.Header.Get("Authorization") != "Bearer test-only-provider-key" {
				t.Error("provider credential was not forwarded")
			}
			var body requestBody
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error("provider request must contain valid JSON")
				http.Error(w, "invalid JSON", http.StatusBadRequest)
				return
			}
			requests <- body
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"test-response","object":"chat.completion","model":"override-model","choices":[{"index":0,"message":{"role":"assistant","content":"接口是一组方法约定。"},"finish_reason":"stop"}],"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`)
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		response, err := gateway.Generate(ctx, messages, model.WithModel("override-model"), model.WithMaxTokens(64))
		if err != nil {
			t.Fatalf("generate against local HTTP provider: %v", err)
		}
		if response == nil || response.Role != schema.Assistant || response.Content != "接口是一组方法约定。" {
			t.Fatal("assistant response was not preserved")
		}
		if response.ResponseMeta == nil || response.ResponseMeta.Usage == nil || response.ResponseMeta.Usage.TotalTokens != 20 || response.ResponseMeta.FinishReason != "stop" {
			t.Fatal("full response metadata must remain available")
		}
		select {
		case request := <-requests:
			if request.Model != "override-model" || request.MaxTokens != 64 {
				t.Fatal("call options were not forwarded")
			}
			if len(request.Messages) != 2 || request.Messages[0].Role != "system" || request.Messages[0].Content != messages[0].Content || request.Messages[1].Role != "user" || request.Messages[1].Content != messages[1].Content {
				t.Fatal("message roles, order and content were not forwarded")
			}
		default:
			t.Fatal("provider did not receive the request")
		}
	})

	t.Run("provider error remains discoverable", func(t *testing.T) {
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"test provider unavailable","type":"server_error","code":"unavailable"}}`)
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		response, err := gateway.Generate(ctx, messages)
		var apiErr *openai.APIError
		if response != nil || !errors.As(err, &apiErr) || apiErr.HTTPStatusCode != http.StatusServiceUnavailable {
			t.Fatalf("provider error chain was lost: %v", err)
		}
	})

	t.Run("already canceled request does not contact provider", func(t *testing.T) {
		var requests atomic.Int32
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		response, err := gateway.Generate(ctx, messages)
		if response != nil || !errors.Is(err, context.Canceled) || requests.Load() != 0 {
			t.Fatal("canceled call must preserve cancellation without contacting the provider")
		}
	})

	t.Run("cancellation interrupts an in flight HTTP request", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		defer close(release)
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			close(entered)
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan error, 1)
		go func() { _, err := gateway.Generate(ctx, messages); done <- err }()
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("request did not reach local provider")
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("HTTP cancellation error was lost: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("cancel did not interrupt the in flight request")
		}
	})
}

func writeTestSSEChunk(w http.ResponseWriter, content string) {
	payload, _ := json.Marshal(map[string]any{
		"id": "test-stream", "object": "chat.completion.chunk", "model": "test-model",
		"choices": []map[string]any{{"index": 0, "delta": map[string]string{"role": "assistant", "content": content}}},
	})
	_, _ = io.WriteString(w, "data: "+string(payload)+"\n\n")
	w.(http.Flusher).Flush()
}

func TestStream(t *testing.T) {
	messages := []*schema.Message{schema.UserMessage("你好")}

	t.Run("unconfigured model is rejected", func(t *testing.T) {
		gateway, err := NewGateway(config.Config{})
		if err != nil {
			t.Fatal(err)
		}
		reader, err := gateway.Stream(context.Background(), messages)
		if err == nil || reader != nil {
			t.Fatal("an unconfigured model must return an error and no reader")
		}
	})

	t.Run("first chunk arrives before completion and options reach HTTP", func(t *testing.T) {
		allowRest := make(chan struct{}, 1)
		defer close(allowRest)
		finished := make(chan struct{})
		type streamRequest struct {
			Stream    bool   `json:"stream"`
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		requests := make(chan streamRequest, 1)
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer close(finished)
			var body streamRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error("invalid stream request JSON")
				http.Error(w, "invalid JSON", 400)
				return
			}
			requests <- body
			if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-only-provider-key" {
				t.Error("unexpected stream endpoint or credential")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			writeTestSSEChunk(w, "你")
			select {
			case <-allowRest:
			case <-r.Context().Done():
				return
			}
			writeTestSSEChunk(w, "好")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			w.(http.Flusher).Flush()
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		reader, err := gateway.Stream(ctx, messages, model.WithModel("stream-override"), model.WithMaxTokens(32))
		if err != nil {
			t.Fatalf("open local stream: %v", err)
		}
		defer reader.Close()
		first, err := reader.Recv()
		if err != nil || first == nil || first.Content != "你" {
			t.Fatalf("first incremental chunk unavailable: %v", err)
		}
		select {
		case <-finished:
			t.Fatal("provider finished before the first chunk was observed")
		default:
		}
		allowRest <- struct{}{}
		answer := first.Content
		nonEmptyChunks := 1
		for {
			chunk, err := reader.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("receive remaining chunks: %v", err)
			}
			if chunk != nil && chunk.Content != "" {
				answer += chunk.Content
				nonEmptyChunks++
			}
		}
		if answer != "你好" || nonEmptyChunks != 2 {
			t.Fatal("incremental chunks were lost or combined before delivery")
		}
		select {
		case request := <-requests:
			if !request.Stream || request.Model != "stream-override" || request.MaxTokens != 32 || len(request.Messages) != 1 || request.Messages[0].Role != "user" || request.Messages[0].Content != "你好" {
				t.Fatal("stream flag, messages or call options were not forwarded")
			}
		default:
			t.Fatal("provider did not receive a stream request")
		}
	})

	t.Run("HTTP failure is returned while opening stream", func(t *testing.T) {
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"test stream unavailable","type":"server_error","code":"unavailable"}}`)
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		reader, err := gateway.Stream(ctx, messages)
		var apiErr *openai.APIError
		if reader != nil || !errors.As(err, &apiErr) || apiErr.HTTPStatusCode != http.StatusServiceUnavailable {
			t.Fatalf("stream opening error chain was lost: %v", err)
		}
	})

	t.Run("malformed later frame fails during Recv", func(t *testing.T) {
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			writeTestSSEChunk(w, "先返回一段")
			_, _ = io.WriteString(w, "data: {invalid-json}\n\n")
			w.(http.Flusher).Flush()
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		reader, err := gateway.Stream(ctx, messages)
		if err != nil {
			t.Fatalf("stream should open before later frame error: %v", err)
		}
		defer reader.Close()
		first, err := reader.Recv()
		if err != nil || first == nil || first.Content != "先返回一段" {
			t.Fatalf("valid first chunk unavailable: %v", err)
		}
		_, err = reader.Recv()
		if err == nil || errors.Is(err, io.EOF) {
			t.Fatal("malformed frame must surface as a read error rather than normal EOF")
		}
	})

	t.Run("pre canceled stream does not contact provider", func(t *testing.T) {
		var requests atomic.Int32
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(503) }))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		reader, err := gateway.Stream(ctx, messages)
		if reader != nil || !errors.Is(err, context.Canceled) || requests.Load() != 0 {
			t.Fatal("pre canceled stream must preserve error and avoid HTTP")
		}
	})

	t.Run("cancel during reading releases the HTTP request", func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		providerCanceled := make(chan struct{})
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			writeTestSSEChunk(w, "第一段")
			select {
			case <-r.Context().Done():
				close(providerCanceled)
			case <-release:
			}
		}))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		reader, err := gateway.Stream(ctx, messages)
		if err != nil {
			t.Fatalf("open stream before cancel: %v", err)
		}
		defer reader.Close()
		first, err := reader.Recv()
		if err != nil || first == nil || first.Content != "第一段" {
			t.Fatalf("first chunk missing: %v", err)
		}
		cancel()
		done := make(chan error, 1)
		go func() { _, err := reader.Recv(); done <- err }()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("stream cancellation error was lost: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Recv did not unblock after cancel")
		}
		select {
		case <-providerCanceled:
		case <-time.After(5 * time.Second):
			t.Fatal("provider did not observe cancellation")
		}
	})

	t.Run("early reader close with cancel stops provider", func(t *testing.T) {
		release := make(chan struct{})
		defer close(release)
		providerCanceled := make(chan struct{})
		gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			writeTestSSEChunk(w, "第一段")
			select {
			case <-r.Context().Done():
				close(providerCanceled)
			case <-release:
			}
		}))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		reader, err := gateway.Stream(ctx, messages)
		if err != nil {
			t.Fatalf("open stream before close: %v", err)
		}
		_, err = reader.Recv()
		if err != nil {
			reader.Close()
			t.Fatalf("read initial chunk: %v", err)
		}
		reader.Close()
		cancel()
		select {
		case <-providerCanceled:
		case <-time.After(5 * time.Second):
			t.Fatal("early close plus cancel did not stop provider")
		}
	})
}

// These scenarios deliberately use the production NewGateway timeout.
// No replacement transport or shorter model-client timeout is injected.
func TestGatewayTimeout(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		mode := "generate"
		if streaming {
			mode = "stream"
		}
		t.Run(mode+" client timeout without caller deadline", func(t *testing.T) {
			t.Parallel()
			verifyGatewayDeadline(t, streaming, 0)
		})
		t.Run(mode+" shorter caller deadline wins", func(t *testing.T) {
			t.Parallel()
			verifyGatewayDeadline(t, streaming, time.Second)
		})
	}
}

func verifyGatewayDeadline(t *testing.T, streaming bool, callerLimit time.Duration) {
	t.Helper()
	release := make(chan struct{})
	defer close(release)
	entered := make(chan struct{})
	providerCanceled := make(chan struct{})
	gateway := newHTTPTestGateway(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			writeTestSSEChunk(w, "first")
		}
		close(entered)
		select {
		case <-r.Context().Done():
			close(providerCanceled)
		case <-release:
		}
	}))
	var ctx context.Context
	var cancel context.CancelFunc
	if callerLimit > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), callerLimit)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()
	started := time.Now()
	done := make(chan error, 1)
	go func() {
		messages := []*schema.Message{schema.UserMessage("test timeout")}
		if !streaming {
			_, err := gateway.Generate(ctx, messages)
			done <- err
			return
		}
		reader, err := gateway.Stream(ctx, messages)
		if err != nil {
			done <- err
			return
		}
		defer reader.Close()
		first, err := reader.Recv()
		if err != nil {
			done <- err
			return
		}
		if first == nil || first.Content != "first" {
			done <- errors.New("expected an initial chunk before waiting for timeout")
			return
		}
		_, err = reader.Recv()
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout test never reached local HTTP provider")
	}
	select {
	case err := <-done:
		elapsed := time.Since(started)
		var timeoutErr net.Error
		if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &timeoutErr) || !timeoutErr.Timeout() {
			t.Fatalf("expected a discoverable HTTP deadline error, got %v", err)
		}
		if callerLimit == 0 {
			if ctx.Err() != nil {
				t.Fatal("client timeout must fire independently of caller context")
			}
			if elapsed < 25*time.Second {
				t.Fatalf("production client timed out too early: %s", elapsed)
			}
		} else {
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) || elapsed > 5*time.Second {
				t.Fatal("shorter caller deadline must stop the request before the client limit")
			}
		}
		t.Logf("observed timeout after %s", elapsed.Round(time.Millisecond))
	case <-time.After(45 * time.Second):
		t.Fatal("model request failed to respect its timeout")
	}
	select {
	case <-providerCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("local provider did not observe timeout cancellation")
	}
}
