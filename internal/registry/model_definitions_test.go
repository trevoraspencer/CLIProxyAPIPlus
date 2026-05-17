package registry

import "testing"

func TestCodexFreeModelsExcludeGPT55(t *testing.T) {
	model := findModelInfo(GetCodexFreeModels(), "gpt-5.5")
	if model != nil {
		t.Fatal("expected codex free tier to NOT include gpt-5.5")
	}
}

func TestCodexStaticModelsIncludeGPT55(t *testing.T) {
	tierModels := map[string][]*ModelInfo{
		"team": GetCodexTeamModels(),
		"plus": GetCodexPlusModels(),
		"pro":  GetCodexProModels(),
	}

	for tier, models := range tierModels {
		t.Run(tier, func(t *testing.T) {
			model := findModelInfo(models, "gpt-5.5")
			if model == nil {
				t.Fatalf("expected codex %s tier to include gpt-5.5", tier)
			}
			assertGPT55ModelInfo(t, tier, model)
		})
	}

	model := LookupStaticModelInfo("gpt-5.5")
	if model == nil {
		t.Fatal("expected LookupStaticModelInfo to find gpt-5.5")
	}
	assertGPT55ModelInfo(t, "lookup", model)
}

func TestKiroStaticModelsAreDynamic(t *testing.T) {
	// Kiro model discovery is entirely dynamic (see fetchKiroModels in
	// sdk/cliproxy/service.go). The static registry intentionally returns
	// an empty list so new Kiro models (GLM, DeepSeek, MiniMax, future
	// additions) flow through without code changes. This test pins two
	// contracts:
	//   1. The slice is empty — no hardcoded models sneak back in.
	//   2. The slice is non-nil — GetStaticModelDefinitionsByChannel("kiro")
	//      must not look like an unknown channel to the management API,
	//      which 400s on a nil result.
	models := GetKiroModels()
	if models == nil {
		t.Fatal("GetKiroModels must return a non-nil slice so kiro stays a recognized channel")
	}
	if len(models) != 0 {
		t.Fatalf("GetKiroModels should be empty (dynamic discovery only), got %d entries", len(models))
	}
}

func TestXAIOAuthStaticModels(t *testing.T) {
	models := GetXAIOAuthModels()
	if findModelInfo(models, "grok-4.3") == nil {
		t.Fatal("expected xai-oauth models to include grok-4.3")
	}
	if findModelInfo(models, "grok-4.20-0309-reasoning") == nil {
		t.Fatal("expected xai-oauth models to include grok-4.20-0309-reasoning")
	}
	if findModelInfo(models, "grok-4.20-0309-non-reasoning") == nil {
		t.Fatal("expected xai-oauth models to include grok-4.20-0309-non-reasoning")
	}
	multiAgent := findModelInfo(models, "grok-4.20-multi-agent-0309")
	if multiAgent == nil {
		t.Fatal("expected xai-oauth models to include grok-4.20-multi-agent-0309")
	}
	if multiAgent.Thinking == nil || len(multiAgent.Thinking.Levels) != 3 {
		t.Fatalf("expected multi-agent model to expose low/medium/high thinking levels, got %+v", multiAgent.Thinking)
	}
	if channelModels := GetStaticModelDefinitionsByChannel("xai-oauth"); findModelInfo(channelModels, "grok-4.3") == nil {
		t.Fatal("expected xai-oauth static channel lookup to include grok-4.3")
	}
	if lookup := LookupStaticModelInfo("grok-4.3"); lookup == nil || lookup.Type != "xai-oauth" {
		t.Fatalf("LookupStaticModelInfo(grok-4.3) = %+v", lookup)
	}
}

func TestXAIOAuthModelsFallbackToRemoteXAISection(t *testing.T) {
	withModelsCatalog(t, &staticModelsJSON{
		XAI: []*ModelInfo{
			{
				ID:                  "grok-remote",
				Object:              "model",
				Created:             1773014400,
				OwnedBy:             "xai",
				Type:                "xai",
				Name:                "grok-remote",
				ContextLength:       1000000,
				MaxCompletionTokens: 65536,
			},
		},
	})

	models := GetXAIOAuthModels()
	model := findModelInfo(models, "grok-remote")
	if model == nil {
		t.Fatal("expected xai-oauth models to fall back to remote xai section")
	}
	if model.Type != "xai-oauth" {
		t.Fatalf("fallback model type = %q, want xai-oauth", model.Type)
	}
	if got := getModels().XAI[0].Type; got != "xai" {
		t.Fatalf("fallback mutated source xai model type = %q, want xai", got)
	}
}

func TestXAIOAuthModelsFallbackToBuiltinsWhenCatalogMissing(t *testing.T) {
	withModelsCatalog(t, &staticModelsJSON{})

	models := GetXAIOAuthModels()
	if findModelInfo(models, "grok-4.3") == nil {
		t.Fatal("expected built-in xai-oauth fallback to include grok-4.3")
	}
	if findModelInfo(models, "grok-4.20-multi-agent-0309") == nil {
		t.Fatal("expected built-in xai-oauth fallback to include grok-4.20-multi-agent-0309")
	}
}

func TestDetectChangedProvidersIncludesEffectiveXAIOAuthModels(t *testing.T) {
	oldData := &staticModelsJSON{
		XAI: []*ModelInfo{{ID: "grok-old", Object: "model", Type: "xai"}},
	}
	newData := &staticModelsJSON{
		XAI: []*ModelInfo{{ID: "grok-new", Object: "model", Type: "xai"}},
	}

	changed := detectChangedProviders(oldData, newData)
	if !containsString(changed, "xai-oauth") {
		t.Fatalf("detectChangedProviders() = %v, want xai-oauth", changed)
	}
}

func withModelsCatalog(t *testing.T, data *staticModelsJSON) {
	t.Helper()

	previous := getModels()
	modelsCatalogStore.mu.Lock()
	modelsCatalogStore.data = data
	modelsCatalogStore.mu.Unlock()
	t.Cleanup(func() {
		modelsCatalogStore.mu.Lock()
		modelsCatalogStore.data = previous
		modelsCatalogStore.mu.Unlock()
	})
}

func findModelInfo(models []*ModelInfo, id string) *ModelInfo {
	for _, model := range models {
		if model != nil && model.ID == id {
			return model
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertGPT55ModelInfo(t *testing.T, source string, model *ModelInfo) {
	t.Helper()

	if model.ID != "gpt-5.5" {
		t.Fatalf("%s id mismatch: got %q", source, model.ID)
	}
	if model.Object != "model" {
		t.Fatalf("%s object mismatch: got %q", source, model.Object)
	}
	if model.Created != 1776902400 {
		t.Fatalf("%s created timestamp mismatch: got %d", source, model.Created)
	}
	if model.OwnedBy != "openai" {
		t.Fatalf("%s owned_by mismatch: got %q", source, model.OwnedBy)
	}
	if model.Type != "openai" {
		t.Fatalf("%s type mismatch: got %q", source, model.Type)
	}
	if model.DisplayName != "GPT 5.5" {
		t.Fatalf("%s display name mismatch: got %q", source, model.DisplayName)
	}
	if model.Version != "gpt-5.5" {
		t.Fatalf("%s version mismatch: got %q", source, model.Version)
	}
	if model.Description != "Frontier model for complex coding, research, and real-world work." {
		t.Fatalf("%s description mismatch: got %q", source, model.Description)
	}
	if model.ContextLength != 272000 {
		t.Fatalf("%s context length mismatch: got %d", source, model.ContextLength)
	}
	if model.MaxCompletionTokens != 128000 {
		t.Fatalf("%s max completion tokens mismatch: got %d", source, model.MaxCompletionTokens)
	}
	if len(model.SupportedParameters) != 1 || model.SupportedParameters[0] != "tools" {
		t.Fatalf("%s supported parameters mismatch: got %v", source, model.SupportedParameters)
	}
	if model.Thinking == nil {
		t.Fatalf("%s missing thinking support", source)
	}

	want := []string{"low", "medium", "high", "xhigh"}
	if len(model.Thinking.Levels) != len(want) {
		t.Fatalf("%s thinking level count mismatch: got %d, want %d", source, len(model.Thinking.Levels), len(want))
	}
	for i, level := range want {
		if model.Thinking.Levels[i] != level {
			t.Fatalf("%s thinking level %d mismatch: got %q, want %q", source, i, model.Thinking.Levels[i], level)
		}
	}
}
