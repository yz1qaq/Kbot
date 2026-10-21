package config

import (
	"strings"
	"testing"
)

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"KBOT_ADDR", "KBOT_DATABASE_URL", "KBOT_JWT_SECRET_KEY",
		"KBOT_AUTOSEED_ADMIN", "KBOT_AUTOSEED_ADMIN_EMAIL", "KBOT_AUTOSEED_ADMIN_PASSWORD",
	} {
		t.Setenv(key, "")
	}
}

func TestLoad(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		clearConfigEnvironment(t)
		cfg := Load()
		if cfg.Addr != ":18090" || cfg.DatabaseURL != "postgres://kbot:kbot@localhost:15432/kbot?sslmode=disable" {
			t.Fatal("unexpected server or database defaults")
		}
		if cfg.JWTSecretKey != "" || cfg.AutoseedAdmin || cfg.AutoseedAdminEmail != "admin@example.com" || cfg.AutoseedAdminPassword != "admin12345" {
			t.Fatal("JWT must have no default secret; admin bootstrap should be disabled with reference account defaults")
		}
	})
	t.Run("environment overrides defaults", func(t *testing.T) {
		clearConfigEnvironment(t)
		t.Setenv("KBOT_ADDR", ":19090")
		t.Setenv("KBOT_DATABASE_URL", "postgres://tester:testpass@localhost:25432/kbot_test?sslmode=disable")
		t.Setenv("KBOT_JWT_SECRET_KEY", strings.Repeat("k", 32))
		t.Setenv("KBOT_AUTOSEED_ADMIN", "true")
		t.Setenv("KBOT_AUTOSEED_ADMIN_EMAIL", "test-admin@example.com")
		t.Setenv("KBOT_AUTOSEED_ADMIN_PASSWORD", "test-admin-password")
		cfg := Load()
		if cfg.Addr != ":19090" || cfg.DatabaseURL != "postgres://tester:testpass@localhost:25432/kbot_test?sslmode=disable" {
			t.Fatal("server/database environment values were not loaded")
		}
		if cfg.JWTSecretKey != strings.Repeat("k", 32) || !cfg.AutoseedAdmin || cfg.AutoseedAdminEmail != "test-admin@example.com" || cfg.AutoseedAdminPassword != "test-admin-password" {
			t.Fatal("authentication environment values were not loaded")
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("explicit configuration should validate: %v", err)
		}
	})
	t.Run("bootstrap requires exact true", func(t *testing.T) {
		clearConfigEnvironment(t)
		for _, value := range []string{"false", "1", "TRUE", "true"} {
			t.Setenv("KBOT_AUTOSEED_ADMIN", value)
			if got := Load().AutoseedAdmin; got != (value == "true") {
				t.Fatalf("unexpected bootstrap boolean for %q", value)
			}
		}
	})
}

func TestValidate(t *testing.T) {
	base := Config{
		Addr: ":18090", DatabaseURL: "postgres://kbot:kbot@localhost:15432/kbot?sslmode=disable",
		JWTSecretKey: strings.Repeat("k", 32),
	}
	tests := []struct {
		name     string
		change   func(*Config)
		wantKeys []string
	}{
		{"valid", func(*Config) {}, nil},
		{"missing address", func(c *Config) { c.Addr = "" }, []string{"KBOT_ADDR"}},
		{"missing database", func(c *Config) { c.DatabaseURL = "" }, []string{"KBOT_DATABASE_URL"}},
		{"missing JWT key", func(c *Config) { c.JWTSecretKey = "" }, []string{"KBOT_JWT_SECRET_KEY"}},
		{"all missing", func(c *Config) { *c = Config{} }, []string{"KBOT_ADDR", "KBOT_DATABASE_URL", "KBOT_JWT_SECRET_KEY"}},
		{"whitespace only", func(c *Config) { c.Addr = " \t"; c.DatabaseURL = " \n"; c.JWTSecretKey = strings.Repeat(" ", 32) }, []string{"KBOT_ADDR", "KBOT_DATABASE_URL", "KBOT_JWT_SECRET_KEY"}},
		{"31 byte JWT key", func(c *Config) { c.JWTSecretKey = strings.Repeat("k", 31) }, []string{"KBOT_JWT_SECRET_KEY"}},
		{"longer JWT key", func(c *Config) { c.JWTSecretKey = strings.Repeat("k", 64) }, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.change(&cfg)
			err := cfg.Validate()
			if len(tc.wantKeys) == 0 {
				if err != nil {
					t.Fatalf("valid configuration was rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid configuration must be rejected")
			}
			for _, key := range tc.wantKeys {
				if !strings.Contains(err.Error(), key) {
					t.Fatalf("validation error must identify %s", key)
				}
			}
		})
	}
}

func TestJWTKeyBytes(t *testing.T) {
	cfg := Config{JWTSecretKey: strings.Repeat("k", 32)}
	if string(cfg.JWTKeyBytes()) != cfg.JWTSecretKey {
		t.Fatal("JWTKeyBytes must provide the configured signing key")
	}
}
