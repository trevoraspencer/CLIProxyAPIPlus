package cliproxy

import (
	"context"
	"strings"
	"testing"
	"time"

	internalregistry "github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/watcher/synthesizer"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/config"
)

func TestDeepSeekModelListing_ConfigAuthRegistryPath(t *testing.T) {
	cfg := deepSeekModelListingConfig(false)
	auth := synthesizeSingleOpenAICompatAuth(t, cfg)
	service := &Service{cfg: cfg}

	modelRegistry := internalregistry.GetGlobalRegistry()
	modelRegistry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	models := modelRegistry.GetModelsForClient(auth.ID)
	model := findRegistryModel(models, "deepseek-v4")
	if model == nil {
		t.Fatalf("expected configured DeepSeek alias deepseek-v4, got %+v", modelIDs(models))
	}
	if model.OwnedBy != "deepseek" {
		t.Fatalf("owned_by = %q, want deepseek", model.OwnedBy)
	}
	if model.Type != "openai-compatibility" {
		t.Fatalf("type = %q, want openai-compatibility", model.Type)
	}
	if upstream := findRegistryModel(models, "deepseek-chat"); upstream != nil {
		t.Fatalf("upstream model deepseek-chat should not be client-visible when alias differs: %+v", upstream)
	}
}

func TestDeepSeekModelListing_PrefixAndForcePrefixSemantics(t *testing.T) {
	tests := []struct {
		name        string
		forcePrefix bool
		wantIDs     []string
	}{
		{
			name:        "prefix adds prefixed and unprefixed aliases",
			forcePrefix: false,
			wantIDs:     []string{"deepseek-v4", "ds/deepseek-v4"},
		},
		{
			name:        "force prefix lists only prefixed alias",
			forcePrefix: true,
			wantIDs:     []string{"ds/deepseek-v4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := deepSeekModelListingConfig(tt.forcePrefix)
			auth := synthesizeSingleOpenAICompatAuth(t, cfg)
			auth.Prefix = "ds"
			service := &Service{cfg: cfg}

			modelRegistry := internalregistry.GetGlobalRegistry()
			modelRegistry.UnregisterClient(auth.ID)
			t.Cleanup(func() {
				modelRegistry.UnregisterClient(auth.ID)
			})

			service.registerModelsForAuth(auth)

			models := modelRegistry.GetModelsForClient(auth.ID)
			for _, id := range tt.wantIDs {
				if findRegistryModel(models, id) == nil {
					t.Fatalf("expected model %q, got %+v", id, modelIDs(models))
				}
			}
			if tt.forcePrefix && findRegistryModel(models, "deepseek-v4") != nil {
				t.Fatalf("unprefixed deepseek-v4 should be absent when ForceModelPrefix is true: %+v", modelIDs(models))
			}
		})
	}
}

func TestDeepSeekModelListing_NegativeAndRemovalCases(t *testing.T) {
	t.Run("disabled config does not expose alias", func(t *testing.T) {
		cfg := deepSeekModelListingConfig(false)
		auth := synthesizeSingleOpenAICompatAuth(t, cfg)
		cfg.OpenAICompatibility[0].Disabled = true
		service := &Service{cfg: cfg}

		modelRegistry := internalregistry.GetGlobalRegistry()
		modelRegistry.UnregisterClient(auth.ID)
		t.Cleanup(func() {
			modelRegistry.UnregisterClient(auth.ID)
		})

		service.registerModelsForAuth(auth)
		if models := modelRegistry.GetModelsForClient(auth.ID); findRegistryModel(models, "deepseek-v4") != nil {
			t.Fatalf("disabled config exposed deepseek-v4: %+v", modelIDs(models))
		}
	})

	t.Run("empty model config does not expose alias", func(t *testing.T) {
		cfg := deepSeekModelListingConfig(false)
		cfg.OpenAICompatibility[0].Models = nil
		auth := synthesizeSingleOpenAICompatAuth(t, cfg)
		service := &Service{cfg: cfg}

		modelRegistry := internalregistry.GetGlobalRegistry()
		modelRegistry.UnregisterClient(auth.ID)
		t.Cleanup(func() {
			modelRegistry.UnregisterClient(auth.ID)
		})

		service.registerModelsForAuth(auth)
		if models := modelRegistry.GetModelsForClient(auth.ID); findRegistryModel(models, "deepseek-v4") != nil {
			t.Fatalf("empty model config exposed deepseek-v4: %+v", modelIDs(models))
		}
	})

	t.Run("credentialless synthetic auth does not expose alias", func(t *testing.T) {
		cfg := deepSeekModelListingConfig(false)
		cfg.OpenAICompatibility[0].APIKeyEntries = nil
		auth := synthesizeSingleOpenAICompatAuth(t, cfg)
		service := &Service{cfg: cfg}

		modelRegistry := internalregistry.GetGlobalRegistry()
		modelRegistry.UnregisterClient(auth.ID)
		t.Cleanup(func() {
			modelRegistry.UnregisterClient(auth.ID)
		})

		service.registerModelsForAuth(auth)
		if models := modelRegistry.GetModelsForClient(auth.ID); findRegistryModel(models, "deepseek-v4") != nil {
			t.Fatalf("credentialless synthetic auth exposed deepseek-v4: %+v", modelIDs(models))
		}
	})

	t.Run("header credential remains usable", func(t *testing.T) {
		cfg := deepSeekModelListingConfig(false)
		cfg.OpenAICompatibility[0].APIKeyEntries = nil
		cfg.OpenAICompatibility[0].Headers = map[string]string{"Authorization": "Bearer synthetic-header-key"}
		auth := synthesizeSingleOpenAICompatAuth(t, cfg)
		service := &Service{cfg: cfg}

		modelRegistry := internalregistry.GetGlobalRegistry()
		modelRegistry.UnregisterClient(auth.ID)
		t.Cleanup(func() {
			modelRegistry.UnregisterClient(auth.ID)
		})

		service.registerModelsForAuth(auth)
		if models := modelRegistry.GetModelsForClient(auth.ID); findRegistryModel(models, "deepseek-v4") == nil {
			t.Fatalf("header-credential synthetic auth should expose deepseek-v4, got %+v", modelIDs(models))
		}
	})

	t.Run("removed model clears stale registration", func(t *testing.T) {
		cfg := deepSeekModelListingConfig(false)
		auth := synthesizeSingleOpenAICompatAuth(t, cfg)
		service := &Service{cfg: cfg}

		modelRegistry := internalregistry.GetGlobalRegistry()
		modelRegistry.UnregisterClient(auth.ID)
		t.Cleanup(func() {
			modelRegistry.UnregisterClient(auth.ID)
		})

		service.registerModelsForAuth(auth)
		if models := modelRegistry.GetModelsForClient(auth.ID); findRegistryModel(models, "deepseek-v4") == nil {
			t.Fatalf("setup failed: expected deepseek-v4 before removal, got %+v", modelIDs(models))
		}

		cfg.OpenAICompatibility[0].Models = nil
		service.registerModelsForAuth(auth)
		if models := modelRegistry.GetModelsForClient(auth.ID); findRegistryModel(models, "deepseek-v4") != nil {
			t.Fatalf("removed config left stale deepseek-v4: %+v", modelIDs(models))
		}
	})

	t.Run("no hardcoded default alias", func(t *testing.T) {
		if model := internalregistry.LookupStaticModelInfo("deepseek-v4"); model != nil {
			t.Fatalf("deepseek-v4 must not be a static default model: %+v", model)
		}
	})
}

func TestDeepSeekModelListing_NonDeepSeekProviderWithDeepSeekLookingUpstream(t *testing.T) {
	cfg := &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "openrouter",
				BaseURL: "https://openrouter.example.test/v1",
				Models: []config.OpenAICompatibilityModel{
					{Name: "deepseek/deepseek-r1", Alias: "router-r1"},
				},
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: "synthetic-openrouter-key"}},
			},
		},
	}
	auth := synthesizeSingleOpenAICompatAuth(t, cfg)
	service := &Service{cfg: cfg}

	modelRegistry := internalregistry.GetGlobalRegistry()
	modelRegistry.UnregisterClient(auth.ID)
	t.Cleanup(func() {
		modelRegistry.UnregisterClient(auth.ID)
	})

	service.registerModelsForAuth(auth)

	models := modelRegistry.GetModelsForClient(auth.ID)
	model := findRegistryModel(models, "router-r1")
	if model == nil {
		t.Fatalf("expected configured openrouter alias router-r1, got %+v", modelIDs(models))
	}
	if model.OwnedBy != "openrouter" {
		t.Fatalf("owned_by = %q, want openrouter", model.OwnedBy)
	}
	if findRegistryModel(models, "deepseek-v4") != nil {
		t.Fatalf("non-DeepSeek provider synthesized deepseek-v4: %+v", modelIDs(models))
	}
	if findRegistryModel(models, "deepseek/deepseek-r1") != nil {
		t.Fatalf("upstream DeepSeek-looking name should not be listed when alias differs: %+v", modelIDs(models))
	}
}

func deepSeekModelListingConfig(forcePrefix bool) *config.Config {
	return &config.Config{
		SDKConfig: config.SDKConfig{
			ForceModelPrefix: forcePrefix,
		},
		OpenAICompatibility: []config.OpenAICompatibility{
			{
				Name:    "deepseek",
				BaseURL: "https://api.deepseek.com/v1",
				Models: []config.OpenAICompatibilityModel{
					{Name: "deepseek-chat", Alias: "deepseek-v4"},
				},
				APIKeyEntries: []config.OpenAICompatibilityAPIKey{{APIKey: "synthetic-deepseek-key"}},
			},
		},
	}
}

func synthesizeSingleOpenAICompatAuth(t *testing.T, cfg *config.Config) *coreauth.Auth {
	t.Helper()

	_ = t.TempDir()
	synth := synthesizer.NewConfigSynthesizer()
	auths, err := synth.Synthesize(&synthesizer.SynthesisContext{
		Config:      cfg,
		Now:         time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC),
		IDGenerator: synthesizer.NewStableIDGenerator(),
	})
	if err != nil {
		t.Fatalf("synthesize auths: %v", err)
	}
	if len(auths) != 1 {
		t.Fatalf("expected exactly one synthetic auth, got %d", len(auths))
	}
	if auths[0].Provider != "openai-compatibility" && strings.TrimSpace(auths[0].Attributes["compat_name"]) == "" {
		t.Fatalf("expected OpenAI-compatible synthetic auth, got provider=%q attributes=%+v", auths[0].Provider, auths[0].Attributes)
	}

	if _, err := coreauth.NewManager(nil, nil, nil).Register(coreauth.WithSkipPersist(context.Background()), auths[0]); err != nil {
		t.Fatalf("synthetic auth should be registerable: %v", err)
	}

	return auths[0]
}

func findRegistryModel(models []*internalregistry.ModelInfo, id string) *internalregistry.ModelInfo {
	for _, model := range models {
		if model != nil && strings.TrimSpace(model.ID) == id {
			return model
		}
	}
	return nil
}

func modelIDs(models []*internalregistry.ModelInfo) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		if model != nil {
			ids = append(ids, model.ID)
		}
	}
	return ids
}
