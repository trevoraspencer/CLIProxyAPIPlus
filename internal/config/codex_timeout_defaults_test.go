package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigCodexTimeoutDefaultsAndZeroDisable(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("api-keys:\n  - test-key\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if cfg.CodexResponseHeaderTimeout != 30 {
			t.Fatalf("CodexResponseHeaderTimeout = %d, want 30", cfg.CodexResponseHeaderTimeout)
		}
		if cfg.CodexTimeoutRetries != 2 {
			t.Fatalf("CodexTimeoutRetries = %d, want 2", cfg.CodexTimeoutRetries)
		}
		if cfg.CodexTimeoutCooldownSeconds != 30 {
			t.Fatalf("CodexTimeoutCooldownSeconds = %d, want 30", cfg.CodexTimeoutCooldownSeconds)
		}

		parsed, err := ParseConfigBytes([]byte("api-keys:\n  - test-key\n"))
		if err != nil {
			t.Fatalf("ParseConfigBytes() error = %v", err)
		}
		if parsed.CodexResponseHeaderTimeout != 30 {
			t.Fatalf("ParseConfigBytes CodexResponseHeaderTimeout = %d, want 30", parsed.CodexResponseHeaderTimeout)
		}
		if parsed.CodexTimeoutRetries != 2 {
			t.Fatalf("ParseConfigBytes CodexTimeoutRetries = %d, want 2", parsed.CodexTimeoutRetries)
		}
		if parsed.CodexTimeoutCooldownSeconds != 30 {
			t.Fatalf("ParseConfigBytes CodexTimeoutCooldownSeconds = %d, want 30", parsed.CodexTimeoutCooldownSeconds)
		}
	})

	t.Run("explicit zero disables", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		data := []byte(`
api-keys:
  - test-key
codex-response-header-timeout: 0
codex-timeout-retries: 0
codex-timeout-cooldown-seconds: 0
`)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
		cfg, err := LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if cfg.CodexResponseHeaderTimeout != 0 {
			t.Fatalf("CodexResponseHeaderTimeout = %d, want 0", cfg.CodexResponseHeaderTimeout)
		}
		if cfg.CodexTimeoutRetries != 0 {
			t.Fatalf("CodexTimeoutRetries = %d, want 0", cfg.CodexTimeoutRetries)
		}
		if cfg.CodexTimeoutCooldownSeconds != 0 {
			t.Fatalf("CodexTimeoutCooldownSeconds = %d, want 0", cfg.CodexTimeoutCooldownSeconds)
		}

		parsed, err := ParseConfigBytes(data)
		if err != nil {
			t.Fatalf("ParseConfigBytes() error = %v", err)
		}
		if parsed.CodexResponseHeaderTimeout != 0 {
			t.Fatalf("ParseConfigBytes CodexResponseHeaderTimeout = %d, want 0", parsed.CodexResponseHeaderTimeout)
		}
		if parsed.CodexTimeoutRetries != 0 {
			t.Fatalf("ParseConfigBytes CodexTimeoutRetries = %d, want 0", parsed.CodexTimeoutRetries)
		}
		if parsed.CodexTimeoutCooldownSeconds != 0 {
			t.Fatalf("ParseConfigBytes CodexTimeoutCooldownSeconds = %d, want 0", parsed.CodexTimeoutCooldownSeconds)
		}
	})
}
