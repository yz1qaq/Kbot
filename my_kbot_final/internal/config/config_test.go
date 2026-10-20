package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		t.Setenv("KBOT_ADDR", "")
		t.Setenv("KBOT_DATABASE_URL", "")

		cfg := Load()

		if cfg.Addr != ":18090" {
			t.Fatalf("Addr = %q, want :18090", cfg.Addr)
		}

		wantURL := "postgres://kbot:kbot@localhost:15432/kbot?sslmode=disable"
		if cfg.DatabaseURL != wantURL {
			t.Fatalf(
				"DatabaseURL = %q, want %q",
				cfg.DatabaseURL, wantURL,
			)
		}
	})

	t.Run("environment_overrides_defaults", func(t *testing.T) {
		wantAddr := ":19090"
		wantURL := "postgres://tester:testpass@localhost:25432/kbot_test?sslmode=disable"

		t.Setenv("KBOT_ADDR", wantAddr)
		t.Setenv("KBOT_DATABASE_URL", wantURL)

		cfg := Load()

		if cfg.Addr != wantAddr {
			t.Fatalf("Addr = %q, want %q", cfg.Addr, wantAddr)
		}
		if cfg.DatabaseURL != wantURL {
			t.Fatalf(
				"DatabaseURL = %q, want %q",
				cfg.DatabaseURL, wantURL,
			)
		}
	})
}

func TestValidate(t *testing.T) {
	const databaseURL = "postgres://kbot:kbot@localhost:15432/kbot?sslmode=disable"
	tests := []struct {
		name        string
		cfg         Config
		wantMissing []string
	}{
		{
			name: "valid",
			cfg:  Config{Addr: ":18090", DatabaseURL: databaseURL},
		},
		{
			name:        "missing_address",
			cfg:         Config{DatabaseURL: databaseURL},
			wantMissing: []string{"KBOT_ADDR"},
		},
		{
			name:        "missing_database_url",
			cfg:         Config{Addr: ":18090"},
			wantMissing: []string{"KBOT_DATABASE_URL"},
		},
		{
			name:        "all_missing",
			cfg:         Config{},
			wantMissing: []string{"KBOT_ADDR", "KBOT_DATABASE_URL"},
		},
		{
			name:        "whitespace_only",
			cfg:         Config{Addr: " \t", DatabaseURL: " \n"},
			wantMissing: []string{"KBOT_ADDR", "KBOT_DATABASE_URL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if len(tt.wantMissing) == 0 {
				if err != nil {
					t.Fatalf("Validate() should succeed: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Validate() should reject missing configuration")
			}
			for _, key := range tt.wantMissing {
				if !strings.Contains(err.Error(), key) {
					t.Fatalf("error %q should mention %s", err.Error(), key)
				}
			}
		})
	}
}
