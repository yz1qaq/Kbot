package middleware

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChainOrder(t *testing.T) {
	var events []string

	a := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			events = append(events, "a-before")
			next.ServeHTTP(w, r)
			events = append(events, "a-after")
		})
	}

	b := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			events = append(events, "b-before")
			next.ServeHTTP(w, r)
			events = append(events, "b-after")
		})
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		events = append(events, "handler")
		w.WriteHeader(http.StatusOK)
	})

	wrapped := Chain(handler, a, b)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	wrapped.ServeHTTP(recorder, req)

	got := strings.Join(events, ",")
	want := "a-before,b-before,handler,b-after,a-after"
	if got != want {
		t.Fatalf("execution order = %q, want %q", got, want)
	}
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "未提供时生成"},
		{name: "已提供时复用", input: "request-001"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var contextID string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contextID, _ = r.Context().Value(requestIDKey{}).(string)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.input != "" {
				req.Header.Set("X-Request-ID", tt.input)
			}
			recorder := httptest.NewRecorder()

			RequestID(handler).ServeHTTP(recorder, req)

			headerID := recorder.Result().Header.Get("X-Request-ID")
			if headerID == "" {
				t.Fatal("response is missing X-Request-ID")
			}
			if contextID != headerID {
				t.Fatalf("context ID = %q, response ID = %q", contextID, headerID)
			}

			if tt.input != "" {
				if headerID != tt.input {
					t.Fatalf("request ID = %q, want %q", headerID, tt.input)
				}
				return
			}

			if len(headerID) != 16 {
				t.Fatalf("generated ID length = %d, want 16", len(headerID))
			}
			if _, err := hex.DecodeString(headerID); err != nil {
				t.Fatalf("generated ID is not hexadecimal: %v", err)
			}
		})
	}
}

func TestRecovererHandlesPanic(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()

	Recoverer(handler).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}

	body := strings.TrimSpace(recorder.Body.String())
	if body != "internal server error" {
		t.Fatalf("body = %q, want internal server error", body)
	}
}