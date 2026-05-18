package cliproxy

import (
	"strings"
	"testing"

	internalregistry "github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestRegisterModelsForAuth_UsesPreMergedExcludedModelsAttribute(t *testing.T) {
	service := &Service{
		cfg: &config.Config{
			OAuthExcludedModels: map[string][]string{
				"gemini-cli": {"gemini-2.5-pro"},
			},
		},
	}
	auth := &coreauth.Auth{
		ID:       "auth-gemini-cli",
		Provider: "gemini-cli",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind":       "oauth",
			"excluded_models": "gemini-2.5-flash",
		},
	}

	registry := internalregistry.GetGlobalRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	models := registry.GetAvailableModelsByProvider("gemini-cli")
	if len(models) == 0 {
		t.Fatal("expected gemini-cli models to be registered")
	}

	for _, model := range models {
		if model == nil {
			continue
		}
		modelID := strings.TrimSpace(model.ID)
		if strings.EqualFold(modelID, "gemini-2.5-flash") {
			t.Fatalf("expected model %q to be excluded by auth attribute", modelID)
		}
	}

	seenGlobalExcluded := false
	for _, model := range models {
		if model == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(model.ID), "gemini-2.5-pro") {
			seenGlobalExcluded = true
			break
		}
	}
	if !seenGlobalExcluded {
		t.Fatal("expected global excluded model to be present when attribute override is set")
	}
}

func TestRegisterModelsForAuth_XAIOAuthModelsAndExclusions(t *testing.T) {
	service := &Service{
		cfg: &config.Config{
			OAuthExcludedModels: map[string][]string{
				"xai-oauth": {"grok-4.20-0309-non-reasoning"},
			},
		},
	}
	auth := &coreauth.Auth{
		ID:       "xai-oauth-auth.json",
		Provider: "xai-oauth",
		Status:   coreauth.StatusActive,
		Metadata: map[string]any{
			"type": "xai-oauth",
		},
	}

	registry := internalregistry.GetGlobalRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)
	models := registry.GetModelsForClient(auth.ID)
	if len(models) == 0 {
		t.Fatal("expected xai-oauth models to be registered")
	}

	seenDefault := false
	for _, model := range models {
		if model == nil {
			continue
		}
		switch strings.TrimSpace(model.ID) {
		case "grok-4.3":
			seenDefault = true
		case "grok-4.20-0309-non-reasoning":
			t.Fatalf("excluded xai-oauth model %q was registered", model.ID)
		}
	}
	if !seenDefault {
		t.Fatal("expected xai-oauth models to include grok-4.3")
	}
}

func TestRegisterModelsForAuth_ZAIAPIKeyAliasesAndExclusions(t *testing.T) {
	service := &Service{
		cfg: &config.Config{
			ZAIKey: []config.ZAIKey{
				{
					APIKey:         "zai-test-key",
					Models:         []config.ZAIModel{{Name: "glm-5.1", Alias: "glm51"}, {Name: "glm-5", Alias: "glm5"}},
					ExcludedModels: []string{"glm5"},
				},
			},
		},
	}
	auth := &coreauth.Auth{
		ID:       "zai-auth.json",
		Provider: "zai",
		Status:   coreauth.StatusActive,
		Attributes: map[string]string{
			"auth_kind": "apikey",
			"api_key":   "zai-test-key",
		},
	}

	registry := internalregistry.GetGlobalRegistry()
	registry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		registry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)
	models := registry.GetModelsForClient(auth.ID)
	if len(models) == 0 {
		t.Fatal("expected zai models to be registered")
	}

	seenAlias := false
	for _, model := range models {
		if model == nil {
			continue
		}
		switch strings.TrimSpace(model.ID) {
		case "glm51":
			seenAlias = true
			if model.Type != "zai" || model.OwnedBy != "zai" {
				t.Fatalf("zai alias metadata = %+v", model)
			}
			if model.DisplayName != "glm-5.1" {
				t.Fatalf("zai alias display name = %q, want upstream model name", model.DisplayName)
			}
			if model.Thinking == nil {
				t.Fatal("expected zai alias to preserve upstream thinking metadata")
			}
		case "glm5":
			t.Fatalf("excluded zai alias %q was registered", model.ID)
		case "glm-5.1", "glm-5":
			t.Fatalf("expected configured zai aliases instead of upstream model id %q", model.ID)
		}
	}
	if !seenAlias {
		t.Fatal("expected zai models to include configured alias glm51")
	}
}
