package tooling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	platformtool "my_kbot/internal/platform/tool"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"
)

type Registry interface {
	Resolve(ctx context.Context, workspaceID, versionID string) (platformtool.Version, error)
}

// 一次工具调用的输入数据
type Call struct {
	WorkspaceID    string //去哪个空间取得版本
	ToolVersionID  string
	Arguments      []byte //实际调用的参数
	IdempotencyKey string
}

// 一次工具调用返回的结果
type Result struct {
	StatusCode int
	Body       []byte
}

// 相应大小限制
const maxResponseBytes = 1 << 20

// SandboxRunner 是 code_execution 工具依赖的最小执行接口。
type SandboxRunner interface {
	Run(ctx context.Context, language, code string) (string, error)
}

type Executor struct {
	registry     Registry //根据工作空间和版本 ID，读取工具配置
	client       *http.Client
	allowedHosts map[string]struct{} //允许访问的主机名
	sandbox      SandboxRunner       //执行工具
}

var _ Registry = (*platformtool.Registry)(nil)

// 验证调用工具的接口地址
func (e *Executor) validateEndpoint(endpoint *url.URL) error {
	if endpoint == nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("tool endpoint must be an absolute HTPP URL without user info")
	}
	//判断是否是可允许的host
	host := strings.ToLower(endpoint.Hostname())
	if _, ok := e.allowedHosts[host]; !ok {
		return fmt.Errorf("tool endpoint host %q is not allowlisted", host)
	}
	return nil
}

// 获取URL的端口
func effectivePort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}

	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}

	return "80"
}

// 比较两个URL的协议主机和端口是否相同
func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(
			strings.TrimSuffix(left.Hostname(), "."),
			strings.TrimSuffix(right.Hostname(), "."),
		) &&
		effectivePort(left) == effectivePort(right)
}

// 给HTTP Transport安装一套自定义的连接规则
func (e *Executor) secureTransport(transport *http.Transport) *http.Transport {
	//关闭代理,准备实际负责连接的Dialer
	transport.Proxy = nil
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		//得到请求的network和address
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}

		hostKey := strings.ToLower(strings.TrimSuffix(host, "."))
		if _, ok := e.allowedHosts[hostKey]; !ok {
			return nil, fmt.Errorf("tool dial host %q is not allowlisted", host)
		}

		if ip := net.ParseIP(host); ip != nil {
			return dialer.DialContext(
				ctx, network, net.JoinHostPort(ip.String(), port),
			)
		}

		//如果host是域名,一个域名可能有多个ip,得到多个ip
		addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil || len(addresses) == 0 {
			return nil, fmt.Errorf("resolve tool host %q: %w", host, err)
		}

		//逐个尝试每个ip
		var dialErr error
		for _, candidate := range addresses {
			var connection net.Conn
			connection, dialErr = dialer.DialContext(
				ctx,
				network,
				net.JoinHostPort(candidate.IP.String(), port),
			)
			if dialErr == nil {
				return connection, nil
			}
		}

		return nil, fmt.Errorf("dial tool host %q: %w", host, dialErr)
	}

	return transport
}

func NewExecutor(registry Registry, client *http.Client, allowedHosts ...string) *Executor {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	} else if client.Timeout <= 0 {
		clone := *client
		clone.Timeout = 10 * time.Second
		client = &clone
	}
	allowed := make(map[string]struct{}, len(allowedHosts))
	for _, host := range allowedHosts {
		normlized := strings.ToLower(strings.TrimSpace(host))
		if normlized != "" {
			allowed[normlized] = struct{}{}
		}
	}
	executor := &Executor{
		registry:     registry,
		client:       client,
		allowedHosts: allowed,
	}

	clone := *client
	if transport, ok := clone.Transport.(*http.Transport); ok {
		clone.Transport = executor.secureTransport(transport.Clone())
	} else if clone.Transport == nil {
		clone.Transport = executor.secureTransport(http.DefaultTransport.(*http.Transport).Clone())
	}

	//先保存原来的规则
	previousRedirect := clone.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := executor.validateEndpoint(req.URL); err != nil {
			return err
		}
		if len(via) > 0 && !sameOrigin(via[0].URL, req.URL) {
			return fmt.Errorf("cross-origin tool redirect is blocked")
		}
		if previousRedirect != nil {
			return previousRedirect(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}

	executor.client = &clone
	return executor
}

func (e *Executor) WithSandbox(runner SandboxRunner) *Executor {
	e.sandbox = runner
	return e
}

/*
	{
	  "headers": {
	    "Authorization": "Bearer demo-key",
	    "X-API-Key": "demo-api-key"
	  }
	}
*/
func toolAuthHeaders(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var config struct {
		Header map[string]string `json:"headers"`
	}
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return nil, fmt.Errorf("invalied tool auth config %w", err)
	}
	for name := range config.Header {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "", "host", "content-length", "connection", "transfer-encoding":
			return nil, fmt.Errorf("tool auth header %q is not allowed", name)
		}
	}
	return config.Header, nil
}

// 检查实际调用参数是否符合工具的JSONScchema
func validateArguments(schemaJSON, arguments []byte) error {

	//把schemaJson解析编译为校验器
	compiled, err := gojsonschema.NewSchema(
		gojsonschema.NewBytesLoader(schemaJSON),
	)
	if err != nil {
		return fmt.Errorf("complied stored tool schema:%w", err)
	}

	//使用校验器来检验arguments
	result, err := compiled.Validate(gojsonschema.NewBytesLoader(arguments))
	if err != nil {
		return fmt.Errorf("validate tool arguments: %w", err)
	}

	//如果不错误就返回
	if result.Valid() {
		return nil
	}
	problems := make([]string, 0, len(result.Errors()))
	for _, problem := range result.Errors() {
		problems = append(problems, problem.String())
	}
	return fmt.Errorf("tool arguments violate schema: %s", strings.Join(problems, ";"))

}

func (e *Executor) Execute(ctx context.Context, call Call) (Result, error) {
	if e.registry == nil {
		return Result{}, fmt.Errorf("tool registry is required")
	}
	//拿到工具
	version, err := e.registry.Resolve(ctx, call.WorkspaceID, call.ToolVersionID)
	if err != nil {
		return Result{}, fmt.Errorf("resolve tool:%w", err)
	}
	var arguments map[string]any
	decoder := json.NewDecoder(bytes.NewReader(call.Arguments))
	decoder.UseNumber()
	if err = decoder.Decode(&arguments); err != nil {
		return Result{}, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return Result{}, fmt.Errorf("arguments must contain one JSON object")
	}
	if err := validateArguments(version.InputSchema, call.Arguments); err != nil {
		return Result{}, err
	}

	if version.SourceType == "code_execution" {
		if e.sandbox == nil {
			return Result{}, fmt.Errorf("sandbox runner is not configured")
		}
		code, _ := arguments["code"].(string)
		if strings.TrimSpace(code) == "" {
			return Result{}, fmt.Errorf("code_execution requires a non-empty code argument")
		}
		language := strings.TrimSpace(version.Endpoint)
		if language == "" {
			language = "python"
		}
		output, err := e.sandbox.Run(ctx, language, code)
		if err != nil {
			return Result{}, fmt.Errorf("execute code tool: %w", err)
		}
		return Result{StatusCode: http.StatusOK, Body: []byte(output)}, nil
	}

	endpoint, err := url.Parse(version.Endpoint)
	if err != nil {
		return Result{}, fmt.Errorf("parse tool endpoint: %w", err)
	}
	if err = e.validateEndpoint(endpoint); err != nil {
		return Result{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(call.Arguments))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("content-Type", "application/json")
	headers, err := toolAuthHeaders(version.AuthConfig)
	if err != nil {
		return Result{}, err
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	if key := strings.TrimSpace(call.IdempotencyKey); key != "" {
		req.Header.Set("Idempotency-Key", key)
	}

	//调用工具的http服务
	response, err := e.client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("call tools: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(
		io.LimitReader(response.Body, maxResponseBytes+1),
	)
	if err != nil {
		return Result{}, fmt.Errorf("read tool response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return Result{}, fmt.Errorf("tool response exceeds %d bytes", maxResponseBytes)
	}

	result := Result{StatusCode: response.StatusCode, Body: body}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("tool returned HTTP %d", response.StatusCode)
	}
	return result, nil
}
