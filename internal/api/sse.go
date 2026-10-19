package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"my_kbot/internal/api/middleware"
	"my_kbot/internal/runtime/engine"
	"net/http"
)

type ChatRuntime interface {
	ChatStream(ctx context.Context, req engine.ChatRequest, emit engine.Emitter) error
}

type StreamHandler struct {
	runtime ChatRuntime
}

func NewstreamHandler(runtime ChatRuntime) *StreamHandler {
	return &StreamHandler{
		runtime: runtime,
	}
}

var _ ChatRuntime = (*engine.Engine)(nil)

func (h *StreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.runtime == nil {
		http.Error(w, "chat runtime is unavaliable", http.StatusServiceUnavailable)
		return
	}
	var req engine.ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	req.WorkspaceID = middleware.WorkspaceID(r.Context())

	//SSE响应
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	errorEventWritten := false

	emit := func(event engine.Event) error {
		if event.Type == "error" {
			errorEventWritten = true
		}
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	err := h.runtime.ChatStream(r.Context(), req, emit)
	//如果Runtime失败,且尚未报告错误事件时,补发
	if err != nil && !errorsIsCanceled(err) && !errorEventWritten {
		_ = emit(engine.Event{
			Type: "error",
			Data: map[string]string{"message": err.Error()},
		})
	}
}

func errorsIsCanceled(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}
