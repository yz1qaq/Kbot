package config

import "testing"

func TestLoadAndValidate(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"

	t.Setenv("KBOT_HTTP_ADDR", ":9090")
	t.Setenv("KBOT_JWT_SECRET", secret)
	t.Setenv("KBOT_JWT_ISSUER", "kbot-test")

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
		HTTPAddr:  ":8080",
		JWTSecret: "short",
		JWTIssuer: "kbot",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected short JWT secret to fail")
	}
}