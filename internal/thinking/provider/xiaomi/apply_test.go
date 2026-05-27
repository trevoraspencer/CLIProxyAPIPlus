package xiaomi

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
)

func TestXiaomiApplierMapsCanonicalConfigToThinkingType(t *testing.T) {
	applier := NewApplier()
	model := &registry.ModelInfo{ID: "mimo-v2.5-pro", Thinking: &registry.ThinkingSupport{
		Levels:         []string{"none", "auto", "low", "medium", "high", "xhigh", "max"},
		ZeroAllowed:    true,
		DynamicAllowed: true,
	}}

	cases := []struct {
		name string
		cfg  thinking.ThinkingConfig
		want string
	}{
		{name: "none", cfg: thinking.ThinkingConfig{Mode: thinking.ModeNone, Budget: 0}, want: "disabled"},
		{name: "zero budget", cfg: thinking.ThinkingConfig{Mode: thinking.ModeBudget, Budget: 0}, want: "disabled"},
		{name: "auto", cfg: thinking.ThinkingConfig{Mode: thinking.ModeAuto, Budget: -1}, want: "enabled"},
		{name: "level", cfg: thinking.ThinkingConfig{Mode: thinking.ModeLevel, Level: thinking.LevelHigh}, want: "enabled"},
		{name: "positive budget", cfg: thinking.ThinkingConfig{Mode: thinking.ModeBudget, Budget: 8192}, want: "enabled"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := applier.Apply([]byte(`{"model":"mimo-v2.5-pro","messages":[],"reasoning_effort":"high","reasoning":{"effort":"high"},"output_config":{"effort":"high"}}`), tc.cfg, model)
			if err != nil {
				t.Fatalf("Apply returned error: %v", err)
			}
			if got := gjson.GetBytes(out, "thinking.type").String(); got != tc.want {
				t.Fatalf("thinking.type = %q, want %q; body=%s", got, tc.want, out)
			}
			for _, path := range []string{"reasoning_effort", "reasoning", "output_config"} {
				if gjson.GetBytes(out, path).Exists() {
					t.Fatalf("%s should be removed; body=%s", path, out)
				}
			}
		})
	}
}

func TestXiaomiApplyThinkingHonorsSuffixesAndBody(t *testing.T) {
	model := &registry.ModelInfo{ID: "mimo-v2.5-pro", Thinking: &registry.ThinkingSupport{
		Levels:         []string{"none", "auto", "minimal", "low", "medium", "high", "xhigh", "max"},
		ZeroAllowed:    true,
		DynamicAllowed: true,
	}}
	registry.GetGlobalRegistry().RegisterClient("xiaomi-thinking-test", "xiaomi", []*registry.ModelInfo{model})
	defer registry.GetGlobalRegistry().UnregisterClient("xiaomi-thinking-test")

	out, err := thinking.ApplyThinking([]byte(`{"model":"mimo-v2.5-pro","messages":[],"reasoning_effort":"high"}`), "mimo-v2.5-pro", "openai", "xiaomi", "xiaomi")
	if err != nil {
		t.Fatalf("ApplyThinking body config returned error: %v", err)
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "enabled" {
		t.Fatalf("body reasoning_effort should enable thinking, got %q; body=%s", got, out)
	}

	out, err = thinking.ApplyThinking([]byte(`{"model":"mimo-v2.5-pro","messages":[],"thinking":{"type":"enabled"}}`), "mimo-v2.5-pro(none)", "openai", "xiaomi", "xiaomi")
	if err != nil {
		t.Fatalf("ApplyThinking suffix none returned error: %v", err)
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "disabled" {
		t.Fatalf("suffix none should disable thinking, got %q; body=%s", got, out)
	}

	out, err = thinking.ApplyThinking([]byte(`{"model":"mimo-v2.5-pro","messages":[]}`), "mimo-v2.5-pro(8192)", "openai", "xiaomi", "xiaomi")
	if err != nil {
		t.Fatalf("ApplyThinking budget suffix returned error: %v", err)
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "enabled" {
		t.Fatalf("positive budget suffix should enable thinking, got %q; body=%s", got, out)
	}
}
