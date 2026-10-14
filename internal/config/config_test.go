package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadAndValidate(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"

	t.Setenv("KBOT_HTTP_ADDR", ":9090")
	t.Setenv("KBOT_JWT_SECRET", secret)
	t.Setenv("KBOT_JWT_ISSUER", "kbot-test")
	t.Setenv("KBOT_LLM_BASE_URL", "https://example.test/v1")
	t.Setenv("KBOT_LLM_API_KEY", "test-api-key")
	t.Setenv("KBOT_LLM_MODEL", "test-model")
	t.Setenv("KBOT_LLM_TIMEOUT_SECONDS", "15")

	cfg := Load()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate config: %v", err)
	}

	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}

	if cfg.JWTSecret != secret {
		t.Fatalf("JWTSecret was not loaded correctly")
	}

	if cfg.JWTIssuer != "kbot-test" {
		t.Fatalf("JWTIssuer = %q, want kbot-test", cfg.JWTIssuer)
	}
}

func TestValidateRejectsShortJWTSecret(t *testing.T) {
	cfg := Config{
		HTTPAddr:   ":8080",
		JWTSecret:  "short",
		JWTIssuer:  "kbot",
		LLMBaseURL: "https://example.test/v1",
		LLMAPIKey:  "test-api-key",
		LLMModel:   "test-model",
		LLMTimeout: 30 * time.Second,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected short JWT secret to fail")
	}
}
func TestLoadLLMConfig(t *testing.T) {
	t.Setenv("KBOT_LLM_BASE_URL", "https://example.test/v1")
	t.Setenv("KBOT_LLM_MODEL", "test-model")
	t.Setenv("KBOT_LLM_TIMEOUT_SECONDS", "15")

	tests := []struct {
		name     string
		kbot     string
		ark      string
		deepseek string
		wantKey  string
	}{
		{"项目配置优先", "kbot-key", "ark-key", "deepseek-key", "kbot-key"},
		{"回退到ARK", "", "ark-key", "deepseek-key", "ark-key"},
		{"回退到DeepSeek", "", "", "deepseek-key", "deepseek-key"},
		{"跳过空白并整理值", "   ", "   ", " deepseek-key ", "deepseek-key"},
		{"全部为空", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("KBOT_LLM_API_KEY", tt.kbot)
			t.Setenv("ARK_API_KEY", tt.ark)
			t.Setenv("DEEPSEEK_API_KEY", tt.deepseek)

			cfg := Load()

			if cfg.LLMBaseURL != "https://example.test/v1" {
				t.Fatalf("LLMBaseURL = %q, want configured URL", cfg.LLMBaseURL)
			}
			if cfg.LLMModel != "test-model" {
				t.Fatalf("LLMModel = %q, want test-model", cfg.LLMModel)
			}
			if cfg.LLMTimeout != 15*time.Second {
				t.Fatalf("LLMTimeout = %v, want 15s", cfg.LLMTimeout)
			}
			if cfg.LLMAPIKey != tt.wantKey {
				t.Fatal("LLMAPIKey did not follow the expected priority")
			}
		})
	}
}

func TestValidateRejectsInvalidLLMConfig(t *testing.T) {
	valid := Config{
		HTTPAddr:   ":8080",
		JWTSecret:  "0123456789abcdef0123456789abcdef",
		JWTIssuer:  "kbot-test",
		LLMBaseURL: "https://example.test/v1",
		LLMAPIKey:  "test-api-key",
		LLMModel:   "test-model",
		LLMTimeout: 30 * time.Second,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config was rejected: %v", err)
	}

	tests := []struct {
		name    string
		change  func(*Config)
		wantErr string
	}{
		{
			name:    "地址只有空白",
			change:  func(c *Config) { c.LLMBaseURL = "   " },
			wantErr: "base URL and model",
		},
		{
			name:    "模型名只有空白",
			change:  func(c *Config) { c.LLMModel = "   " },
			wantErr: "base URL and model",
		},
		{
			name:    "密钥只有空白",
			change:  func(c *Config) { c.LLMAPIKey = "   " },
			wantErr: "API_KEY",
		},
		{
			name:    "超时为零",
			change:  func(c *Config) { c.LLMTimeout = 0 },
			wantErr: "timeout",
		},
		{
			name:    "超时为负数",
			change:  func(c *Config) { c.LLMTimeout = -time.Second },
			wantErr: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.change(&cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected invalid config to be rejected")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
