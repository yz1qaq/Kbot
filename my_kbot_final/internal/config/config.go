// Package config 管理服务启动所需的配置。
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config 保存当前阶段需要的服务和数据库配置。
type Config struct {
	// Addr 是 HTTP 服务监听地址。
	Addr string

	// DatabaseURL 是 PostgreSQL 数据库连接字符串。
	DatabaseURL string

	// JWT 签名和验签使用的密钥。
	JWTSecretKey string

	// 是否在启动时引导管理员账号。
	AutoseedAdmin bool

	AutoseedAdminEmail    string
	AutoseedAdminPassword string
}

// Load 从环境变量读取配置。
func Load() Config {
	return Config{
		Addr: getenv("KBOT_ADDR", ":18090"),
		DatabaseURL: getenv(
			"KBOT_DATABASE_URL",
			"postgres://kbot:kbot@localhost:15432/kbot?sslmode=disable",
		),
		JWTSecretKey: os.Getenv("KBOT_JWT_SECRET_KEY"),

		AutoseedAdmin:         getenv("KBOT_AUTOSEED_ADMIN", "false") == "true",
		AutoseedAdminEmail:    getenv("KBOT_AUTOSEED_ADMIN_EMAIL", "admin@example.com"),
		AutoseedAdminPassword: getenv("KBOT_AUTOSEED_ADMIN_PASSWORD", "admin12345"),
	}
}

// getenv 返回非空的环境变量值，否则返回默认值。
func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (c Config) Validate() error {
	var missing []string
	if strings.TrimSpace(c.Addr) == "" {
		missing = append(missing, "KBOT_ADDR")
	}
	if strings.TrimSpace(c.DatabaseURL) == "" {
		missing = append(missing, "KBOT_DATABASE_URL")
	}
	if strings.TrimSpace(c.JWTSecretKey) == "" {
		missing = append(missing, "KBOT_JWT_SECRET_KEY")
	}
	if len(missing) > 0 {
		return fmt.Errorf("缺少必填配置: %v", missing)
	}
	if len(c.JWTSecretKey) < 32 {
		return fmt.Errorf("KBOT_JWT_SECRET_KEY至少需要32字节")
	}
	return nil
}

func (c Config) JWTKeyBytes() []byte {
	return []byte(c.JWTSecretKey)
}
