package store

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/zai"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func TestApplyZAIFileAPIKeyAttributesDisablesMissingOrBlankKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]any
	}{
		{name: "missing api key", metadata: map[string]any{"type": "zai"}},
		{name: "blank snake case api key", metadata: map[string]any{"type": "zai", "api_key": "   "}},
		{name: "blank kebab case api key", metadata: map[string]any{"type": "zai", "api-key": "\t"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			auth := &cliproxyauth.Auth{
				Provider:   "zai",
				Status:     cliproxyauth.StatusActive,
				Attributes: map[string]string{},
			}

			applyZAIFileAPIKeyAttributes(auth, tt.metadata)

			if !auth.Disabled || auth.Status != cliproxyauth.StatusDisabled {
				t.Fatalf("expected unusable Z.AI auth, disabled=%v status=%s", auth.Disabled, auth.Status)
			}
			if got := auth.Attributes["api_key"]; got != "" {
				t.Fatalf("expected no api_key attribute, got %q", got)
			}
			if got := auth.Attributes["base_url"]; got != zai.DefaultCodingBaseURL {
				t.Fatalf("base_url = %q, want default %q", got, zai.DefaultCodingBaseURL)
			}
		})
	}
}

func TestApplyZAIFileAPIKeyAttributesPreservesValidKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata map[string]any
		wantKey  string
	}{
		{name: "snake case api key", metadata: map[string]any{"type": "zai", "api_key": " zai-key "}, wantKey: "zai-key"},
		{name: "kebab case api key", metadata: map[string]any{"type": "zai", "api-key": "zai-kebab-key"}, wantKey: "zai-kebab-key"},
		{name: "blank snake case falls back to kebab case api key", metadata: map[string]any{"type": "zai", "api_key": "   ", "api-key": "zai-fallback-key"}, wantKey: "zai-fallback-key"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			auth := &cliproxyauth.Auth{
				Provider:   "zai",
				Status:     cliproxyauth.StatusActive,
				Attributes: map[string]string{},
			}

			applyZAIFileAPIKeyAttributes(auth, tt.metadata)

			if auth.Disabled || auth.Status != cliproxyauth.StatusActive {
				t.Fatalf("expected active Z.AI auth, disabled=%v status=%s", auth.Disabled, auth.Status)
			}
			if got := auth.Attributes["api_key"]; got != tt.wantKey {
				t.Fatalf("api_key = %q, want %q", got, tt.wantKey)
			}
			if got := auth.Attributes["base_url"]; got != zai.DefaultCodingBaseURL {
				t.Fatalf("base_url = %q, want default %q", got, zai.DefaultCodingBaseURL)
			}
		})
	}
}
