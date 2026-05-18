package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/zai"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestExtractAccessToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]any
		expected string
	}{
		{
			"antigravity top-level access_token",
			map[string]any{"access_token": "tok-abc"},
			"tok-abc",
		},
		{
			"gemini nested token.access_token",
			map[string]any{
				"token": map[string]any{"access_token": "tok-nested"},
			},
			"tok-nested",
		},
		{
			"top-level takes precedence over nested",
			map[string]any{
				"access_token": "tok-top",
				"token":        map[string]any{"access_token": "tok-nested"},
			},
			"tok-top",
		},
		{
			"empty metadata",
			map[string]any{},
			"",
		},
		{
			"whitespace-only access_token",
			map[string]any{"access_token": "   "},
			"",
		},
		{
			"wrong type access_token",
			map[string]any{"access_token": 12345},
			"",
		},
		{
			"token is not a map",
			map[string]any{"token": "not-a-map"},
			"",
		},
		{
			"nested whitespace-only",
			map[string]any{
				"token": map[string]any{"access_token": "  "},
			},
			"",
		},
		{
			"fallback to nested when top-level empty",
			map[string]any{
				"access_token": "",
				"token":        map[string]any{"access_token": "tok-fallback"},
			},
			"tok-fallback",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractAccessToken(tt.metadata)
			if got != tt.expected {
				t.Errorf("extractAccessToken() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFileTokenStoreListDisablesZAIMissingOrBlankAPIKey(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := map[string]string{
		"missing.json": `{"type":"zai"}`,
		"blank.json":   `{"type":"zai","api_key":"   "}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	store := NewFileTokenStore()
	store.SetBaseDir(dir)
	auths, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(auths) != len(files) {
		t.Fatalf("List returned %d auths, want %d", len(auths), len(files))
	}
	for _, auth := range auths {
		if !auth.Disabled || auth.Status != cliproxyauth.StatusDisabled {
			t.Fatalf("%s: expected disabled Z.AI auth, disabled=%v status=%s", auth.ID, auth.Disabled, auth.Status)
		}
		if got := auth.Attributes["api_key"]; got != "" {
			t.Fatalf("%s: expected no api_key attribute, got %q", auth.ID, got)
		}
	}
}

func TestFileTokenStoreListPreservesValidZAIKeys(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := map[string]string{
		"snake.json":    `{"type":"zai","api_key":" snake-key "}`,
		"kebab.json":    `{"type":"zai","api-key":"kebab-key"}`,
		"fallback.json": `{"type":"zai","api_key":"   ","api-key":"fallback-key"}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	store := NewFileTokenStore()
	store.SetBaseDir(dir)
	auths, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	gotKeys := make(map[string]string, len(auths))
	for _, auth := range auths {
		if auth.Disabled || auth.Status != cliproxyauth.StatusActive {
			t.Fatalf("%s: expected active Z.AI auth, disabled=%v status=%s", auth.ID, auth.Disabled, auth.Status)
		}
		if got := auth.Attributes["base_url"]; got != zai.DefaultCodingBaseURL {
			t.Fatalf("%s: base_url = %q, want %q", auth.ID, got, zai.DefaultCodingBaseURL)
		}
		gotKeys[auth.ID] = auth.Attributes["api_key"]
	}
	if gotKeys["snake.json"] != "snake-key" || gotKeys["kebab.json"] != "kebab-key" || gotKeys["fallback.json"] != "fallback-key" {
		t.Fatalf("unexpected keys: %#v", gotKeys)
	}
}
