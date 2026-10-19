package tooling

import (
	"context"
	"io"
	platformtool "my_kbot/internal/platform/tool"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type recordingSandbox struct {
	language string
	code     string
}

func (s *recordingSandbox) Run(_ context.Context, language, code string) (string, error) {
	s.language = language
	s.code = code
	return "42\n", nil
}

func TestExecutorValidateEndpoint(t *testing.T) {
	executor := &Executor{
		allowedHosts: map[string]struct{}{
			"example.com": {},
		},
	}

	if err := executor.validateEndpoint(nil); err == nil {
		t.Fatal("expected a nil endpoint to be rejected")
	}

	tests := []struct {
		name    string
		address string
		wantErr bool
	}{
		{
			name:    "http_allowed",
			address: "http://example.com/api",
		},
		{
			name:    "https_allowed",
			address: "https://example.com/api",
		},
		{
			name:    "port_allowed",
			address: "https://example.com:8443/api",
		},
		{
			name:    "unlisted_host",
			address: "https://other.example.com/api",
			wantErr: true,
		},
		{
			name:    "invalid_scheme",
			address: "ftp://example.com/file",
			wantErr: true,
		},
		{
			name:    "relative_path",
			address: "/api/inventory",
			wantErr: true,
		},
		{
			name:    "userinfo",
			address: "https://user:pass@example.com/api",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := url.Parse(tt.address)
			if err != nil {
				t.Fatalf("parse test URL: %v", err)
			}

			err = executor.validateEndpoint(endpoint)
			if (err != nil) != tt.wantErr {
				t.Fatalf(
					"validateEndpoint(%q): error = %v, wantErr = %v",
					tt.address, err, tt.wantErr,
				)
			}
		})
	}
}

func TestNewExecutorPreservesCallerClient(t *testing.T) {
	callerTransport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	callerClient := &http.Client{
		Transport: callerTransport,
	}

	executor := NewExecutor(nil, callerClient, "example.com")

	if callerClient.Timeout != 0 {
		t.Fatal("constructor changed the caller's timeout")
	}
	if callerClient.CheckRedirect != nil {
		t.Fatal("constructor changed the caller's redirect policy")
	}
	if callerClient.Transport != callerTransport {
		t.Fatal("constructor replaced the caller's transport")
	}

	if callerTransport.Proxy == nil || callerTransport.DialContext != nil {
		t.Fatal("constructor changed the caller's transport configuration")
	}

	if executor.client.Timeout != 10*time.Second {
		t.Fatalf(
			"executor timeout = %v, want 10s",
			executor.client.Timeout,
		)
	}
}

func TestExecutorCallsHTTPTool(t *testing.T) {
	ctx := context.Background()
	const arguments = `{"sku":"SKU-BLACK-M-01"}`
	const responseBody = `{"available":12}`

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Error("missing JSON content type")
			}
			if r.Header.Get("Authorization") != "Bearer demo-key" {
				t.Error("missing tool credential")
			}
			if r.Header.Get("Idempotency-Key") != "run-1:call-1" {
				t.Error("missing idempotency key")
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				return
			}
			if string(body) != arguments {
				t.Errorf("body = %s, want %s", body, arguments)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, responseBody)
		},
	))
	defer server.Close()

	registry := platformtool.NewRegistry()
	err := registry.Register(ctx, platformtool.Version{
		ID:          "inventory-v1",
		ToolID:      "inventory-tool",
		WorkspaceID: "workspace-1",
		Name:        "search_inventory",
		SourceType:  "http",
		Endpoint:    server.URL,
		InputSchema: []byte(`{
			"type":"object",
			"properties":{"sku":{"type":"string"}},
			"required":["sku"]
		}`),
		AuthConfig: `{"headers":{"Authorization":"Bearer demo-key"}}`,
		Published:  true,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	executor := NewExecutor(registry, server.Client(), "127.0.0.1")
	result, err := executor.Execute(ctx, Call{
		WorkspaceID:    "workspace-1",
		ToolVersionID:  "inventory-v1",
		Arguments:      []byte(arguments),
		IdempotencyKey: "run-1:call-1",
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if result.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", result.StatusCode)
	}
	if string(result.Body) != responseBody {
		t.Fatalf("body = %s, want %s", result.Body, responseBody)
	}
}

func TestExecutorRejectsInvalidArgumentsBeforeHTTP(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusOK)
		},
	))
	defer server.Close()

	registry := platformtool.NewRegistry()
	err := registry.Register(ctx, platformtool.Version{
		ID:          "transfer-v1",
		ToolID:      "transfer-tool",
		WorkspaceID: "workspace-1",
		Name:        "transfer_inventory",
		SourceType:  "http",
		Endpoint:    server.URL,
		Published:   true,
		InputSchema: []byte(`{
			"type":"object",
			"properties":{
				"quantity":{"type":"integer","minimum":1}
			},
			"required":["quantity"],
			"additionalProperties":false
		}`),
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	executor := NewExecutor(registry, server.Client(), "127.0.0.1")

	tests := []struct {
		name      string
		arguments string
	}{
		{"wrong_type", `{"quantity":"2"}`},
		{"missing_required", `{}`},
		{"too_small", `{"quantity":0}`},
		{"unexpected_field", `{"quantity":2,"extra":true}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executor.Execute(ctx, Call{
				WorkspaceID:   "workspace-1",
				ToolVersionID: "transfer-v1",
				Arguments:     []byte(tt.arguments),
			})

			if err == nil ||
				!strings.Contains(err.Error(), "tool arguments violate schema") {
				t.Fatalf("expected schema rejection, got %v", err)
			}
		})
	}

	if got := calls.Load(); got != 0 {
		t.Fatalf("tool received %d requests, want 0", got)
	}
}

func TestExecutorRejectsCrossOriginRedirect(t *testing.T) {
	ctx := context.Background()
	var targetCalls atomic.Int32

	target := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			targetCalls.Add(1)
			w.WriteHeader(http.StatusOK)
		},
	))
	defer target.Close()

	targetURL := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)

	source := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, targetURL, http.StatusFound)
		},
	))
	defer source.Close()

	registry := platformtool.NewRegistry()
	err := registry.Register(ctx, platformtool.Version{
		ID:          "redirect-v1",
		ToolID:      "redirect-tool",
		WorkspaceID: "workspace-1",
		Name:        "redirect_test",
		SourceType:  "http",
		Endpoint:    source.URL,
		InputSchema: []byte(`{"type":"object"}`),
		Published:   true,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	executor := NewExecutor(
		registry,
		source.Client(),
		"127.0.0.1",
		"localhost",
	)
	_, err = executor.Execute(ctx, Call{
		WorkspaceID:   "workspace-1",
		ToolVersionID: "redirect-v1",
		Arguments:     []byte(`{}`),
	})

	if err == nil || !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("expected cross-origin rejection, got %v", err)
	}
	if got := targetCalls.Load(); got != 0 {
		t.Fatalf("redirect target received %d requests, want 0", got)
	}
}

func TestExecutorRoutesCodeExecutionToSandboxRunner(t *testing.T) {
	ctx := context.Background()
	registry := platformtool.NewRegistry()

	err := registry.Register(ctx, platformtool.Version{
		ID:          "python-v1",
		ToolID:      "python-tool",
		WorkspaceID: "workspace-1",
		Name:        "code_run_python",
		SourceType:  "code_execution",
		Endpoint:    "python",
		Published:   true,
		InputSchema: []byte(`{
			"type":"object",
			"properties":{"code":{"type":"string"}},
			"required":["code"],
			"additionalProperties":false
		}`),
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	runner := &recordingSandbox{}
	executor := NewExecutor(registry, nil).WithSandbox(runner)

	result, err := executor.Execute(ctx, Call{
		WorkspaceID:   "workspace-1",
		ToolVersionID: "python-v1",
		Arguments:     []byte(`{"code":"print(6 * 7)"}`),
	})
	if err != nil {
		t.Fatalf("execute code tool: %v", err)
	}

	if runner.language != "python" || runner.code != "print(6 * 7)" {
		t.Fatalf(
			"unexpected runner arguments: language=%q, code=%q",
			runner.language, runner.code,
		)
	}
	if result.StatusCode != http.StatusOK || string(result.Body) != "42\n" {
		t.Fatalf(
			"unexpected result: status=%d, body=%q",
			result.StatusCode, result.Body,
		)
	}
}

func TestExecutorBindBuildsToolInfo(t *testing.T) {
	ctx := context.Background()
	registry := platformtool.NewRegistry()

	version := platformtool.Version{
		ID:          "inventory-v1",
		ToolID:      "inventory-tool",
		WorkspaceID: "workspace-1",
		Name:        "search_inventory",
		SourceType:  "http",
		Description: "根据 SKU 查询库存",
		Endpoint:    "https://example.com/inventory",
		InputSchema: []byte(`{
			"type": "object",
			"properties": {
				"sku": {"type": "string"}
			},
			"required": ["sku"]
		}`),
		Published: true,
	}

	if err := registry.Register(ctx, version); err != nil {
		t.Fatalf("register: %v", err)
	}

	executor := NewExecutor(registry, nil)
	bindings, err := executor.Bind(
		ctx,
		"workspace-1",
		[]string{"inventory-v1"},
	)
	if err != nil {
		t.Fatalf("Bind(): %v", err)
	}

	if len(bindings) != 1 {
		t.Fatalf("bindings count = %d, want 1", len(bindings))
	}

	binding := bindings[0]
	if binding.Name != version.Name || binding.VersionID != version.ID {
		t.Fatalf("unexpected binding: %+v", binding)
	}

	if binding.Info == nil || binding.Info.ParamsOneOf == nil {
		t.Fatal("tool info or parameter description is missing")
	}

	if binding.Info.Name != version.Name ||
		binding.Info.Desc != version.Description {
		t.Fatalf("unexpected tool info: %+v", binding.Info)
	}

	params, err := binding.Info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema(): %v", err)
	}

	if params == nil || params.Type != "object" || params.Properties == nil {
		t.Fatalf("unexpected parameter schema: %+v", params)
	}

	sku, ok := params.Properties.Get("sku")
	if !ok || sku == nil || sku.Type != "string" {
		t.Fatalf("sku schema = %+v, want string", sku)
	}

	if len(params.Required) != 1 || params.Required[0] != "sku" {
		t.Fatalf("required = %v, want [sku]", params.Required)
	}
}
