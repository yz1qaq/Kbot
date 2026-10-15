package sandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeExecutionBackend struct {
	calls    int
	language string
	code     string
	result   Result
}

func (b *fakeExecutionBackend) Execute(
	ctx context.Context,
	language, code string,
) (Result, error) {
	b.calls++
	b.language = language
	b.code = code

	return b.result, nil
}

func (b *fakeExecutionBackend) Check(ctx context.Context) error {
	return nil
}

func TestHandlerRejectsMissingToken(t *testing.T) {
	backend := &fakeExecutionBackend{}
	handler := NewHandler(backend, "test-sandbox-token")

	request := httptest.NewRequest(
		http.MethodPost,
		executionsPath,
		strings.NewReader(`{"language":"python","code":"print(42)"}`),
	)
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status = %d, want %d; body = %s",
			recorder.Code,
			http.StatusUnauthorized,
			recorder.Body.String(),
		)
	}

	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response["error"] != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", response["error"])
	}

	if backend.calls != 0 {
		t.Fatalf("backend calls = %d, want 0", backend.calls)
	}
}

func TestHandlerExecutesWithValidToken(t *testing.T) {
	want := Result{
		ExecutionID: "execution-test",
		Stdout:      "42\n",
		ExitCode:    0,
	}

	backend := &fakeExecutionBackend{
		result: want,
	}
	handler := NewHandler(backend, "test-sandbox-token")

	request := httptest.NewRequest(
		http.MethodPost,
		executionsPath,
		strings.NewReader(`{"language":"python","code":"print(42)"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer test-sandbox-token")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"status = %d, want 200; body = %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	if backend.calls != 1 {
		t.Fatalf("backend calls = %d, want 1", backend.calls)
	}

	if backend.language != "python" || backend.code != "print(42)" {
		t.Fatalf(
			"backend received language=%q code=%q",
			backend.language,
			backend.code,
		)
	}

	var response ExecuteResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.Error != "" {
		t.Fatalf("unexpected execution error: %s", response.Error)
	}

	if response.Result != want {
		t.Fatalf("result = %+v, want %+v", response.Result, want)
	}
}

func TestClientCallsSandboxOverHTTP(t *testing.T) {
	const token = "test-sandbox-token"

	backend := &fakeExecutionBackend{
		result: Result{
			ExecutionID: "execution-http-test",
			Stdout:      "42\n",
			ExitCode:    0,
		},
	}

	server := httptest.NewServer(NewHandler(backend, token))
	defer server.Close()

	client, err := NewClient(server.URL, token)
	if err != nil {
		t.Fatalf("NewClient(): %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	if err := client.Check(ctx); err != nil {
		t.Fatalf("Check(): %v", err)
	}

	output, err := client.Run(ctx, "python", "print(42)")
	if err != nil {
		t.Fatalf("Run(): %v", err)
	}

	if output != "42\n" {
		t.Fatalf("output = %q, want %q", output, "42\n")
	}

	if backend.calls != 1 {
		t.Fatalf("backend calls = %d, want 1", backend.calls)
	}

	if backend.language != "python" || backend.code != "print(42)" {
		t.Fatalf(
			"backend received language=%q code=%q",
			backend.language,
			backend.code,
		)
	}
}
