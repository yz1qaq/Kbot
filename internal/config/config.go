package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr  string
	JWTSecret string
	JWTIssuer string

	//llm相关
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
	LLMTimeout time.Duration

	// 沙箱服务连接配置
	SandboxRunnerURL   string
	SandboxRunnerToken string
}

func valueOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// durationOrDefault 读取正整数秒数，无效时返回默认时间。
func durationOrDefault(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

// 按顺序调用第一个非空的API key
func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func Load() Config {
	return Config{
		HTTPAddr:  valueOrDefault("KBOT_HTTP_ADDR", ":8080"),
		JWTSecret: os.Getenv("KBOT_JWT_SECRET"),
		JWTIssuer: valueOrDefault("KBOT_JWT_ISSUER", "kbot-course"),

		LLMBaseURL: valueOrDefault(
			"KBOT_LLM_BASE_URL",
			"https://api.deepseek.com",
		),
		LLMAPIKey: firstNonEmptyEnv("KBOT_LLM_API_KEY", "ARK_API_KEY", "DEEPSEEK_API_KEY"),
		LLMModel: valueOrDefault(
			"KBOT_LLM_MODEL",
			"deepseek-flash",
		),
		LLMTimeout: durationOrDefault(
			"KBOT_LLM_TIMEOUT_SECONDS",
			30*time.Second,
		),
		SandboxRunnerURL: valueOrDefault(
			"KBOT_SANDBOX_RUNNER_URL",
			"http://127.0.0.1:8081",
		),
		SandboxRunnerToken: os.Getenv("KBOT_SANDBOX_RUNNER_TOKEN"),
	}
}

func (c Config) Validate() error {
	if !strings.HasPrefix(c.HTTPAddr, ":") {
		return fmt.Errorf("KBOT_HTTP_ADDR must use :port format")
	}
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("KBOT_JWT_SECRET must contain at least 32 characters")
	}
	if strings.TrimSpace(c.JWTIssuer) == "" {
		return fmt.Errorf("KBOT_JWT_ISSUER is required")
	}

	if strings.TrimSpace(c.LLMBaseURL) == "" ||
		strings.TrimSpace(c.LLMModel) == "" {
		return fmt.Errorf("LLM base URL and model are required")
	}
	if strings.TrimSpace(c.LLMAPIKey) == "" {
		return fmt.Errorf(
			"KBOT_LLM_API_KEY, ARK_API_KEY or DEEPSEEK_API_KEY is required",
		)
	}
	if c.LLMTimeout <= 0 {
		return fmt.Errorf("LLM timeout must be positive")
	}

	if strings.TrimSpace(c.SandboxRunnerURL) == "" {
		return fmt.Errorf("KBOT_SANDBOX_RUNNER_URL is required")
	}

	if len(c.SandboxRunnerToken) < 32 {
		return fmt.Errorf(
			"KBOT_SANDBOX_RUNNER_TOKEN must contain at least 32 characters",
		)
	}

	return nil
}
