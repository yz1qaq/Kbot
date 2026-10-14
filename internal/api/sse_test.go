package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"my_kbot/internal/runtime/engine"
)

type stubRuntime struct{}

func (stubRuntime) ChatStream(
	_ context.Context,
	_ engine.ChatRequest,
	emit engine.Emitter,
) error {
	return emit(engine.Event{
		Type: "answer_delta",
		Text: "你好",
	})
}

func TestStreamHandlerWritesSSEFrame(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/stream/agents/demo/chat",
		strings.NewReader(
			`{"conversation_id":"conversation-001","message":"hi"}`,
		),
	)
	recorder := httptest.NewRecorder()

	NewstreamHandler(stubRuntime{}).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q, want text/event-stream", got)
	}

	want := `data: {"type":"answer_delta","text":"你好"}` + "\n\n"
	if got := recorder.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if !recorder.Flushed {
		t.Fatal("expected response to be flushed")
	}
}

type reportedErrorRuntime struct{}

func (reportedErrorRuntime) ChatStream(
	_ context.Context,
	_ engine.ChatRequest,
	emit engine.Emitter,
) error {
	if err := emit(engine.Event{
		Type: "error",
		Data: map[string]string{"message": "boom"},
	}); err != nil {
		return err
	}

	return errors.New("boom")
}

func TestStreamHandlerDoesNotDuplicateRuntimeErrorEvent(t *testing.T) {
	req := httptest.NewRequest(
		http.MethodPost,
		"/stream/agents/demo/chat",
		strings.NewReader(
			`{"conversation_id":"conversation-001","message":"hi"}`,
		),
	)
	recorder := httptest.NewRecorder()

	NewstreamHandler(reportedErrorRuntime{}).ServeHTTP(recorder, req)

	body := recorder.Body.String()
	if count := strings.Count(body, `"type":"error"`); count != 1 {
		t.Fatalf("error event count = %d, want 1; body = %q", count, body)
	}
}
