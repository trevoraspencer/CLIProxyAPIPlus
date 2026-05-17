package store

import (
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/auth/zai"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

func applyZAIFileAPIKeyAttributes(auth *cliproxyauth.Auth, metadata map[string]any) {
	if auth == nil || metadata == nil || !strings.EqualFold(strings.TrimSpace(auth.Provider), zai.Provider) {
		return
	}
	auth.Label = "zai-apikey"
	if auth.Attributes == nil {
		auth.Attributes = make(map[string]string)
	}
	auth.Attributes["auth_kind"] = "apikey"
	if rawKey, ok := metadata["api_key"].(string); ok {
		if key := strings.TrimSpace(rawKey); key != "" {
			auth.Attributes["api_key"] = key
		}
	} else if rawKey, ok := metadata["api-key"].(string); ok {
		if key := strings.TrimSpace(rawKey); key != "" {
			auth.Attributes["api_key"] = key
		}
	}
	baseURL := ""
	if rawBase, ok := metadata["base_url"].(string); ok {
		baseURL = strings.TrimSpace(rawBase)
	}
	if baseURL == "" {
		if rawBase, ok := metadata["base-url"].(string); ok {
			baseURL = strings.TrimSpace(rawBase)
		}
	}
	if baseURL == "" {
		baseURL = zai.DefaultCodingBaseURL
	}
	auth.Attributes["base_url"] = baseURL
	if _, exists := metadata["auth_method"]; !exists {
		metadata["auth_method"] = "api_key"
	}
}
