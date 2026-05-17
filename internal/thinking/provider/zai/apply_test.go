package zai

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/thinking"
	"github.com/tidwall/gjson"
)

func TestZAIThinkingApplierEnablesThinking(t *testing.T) {
	body := []byte(`{"model":"glm-5.1","messages":[],"reasoning_effort":"high","thinking":{"clear_thinking":true}}`)

	out, err := thinking.ApplyThinking(body, "glm-5.1", "openai", "zai", "zai")
	if err != nil {
		t.Fatalf("ApplyThinking returned error: %v", err)
	}
	if gjson.GetBytes(out, "reasoning_effort").Exists() {
		t.Fatalf("expected reasoning_effort removed, got %s", out)
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "enabled" {
		t.Fatalf("thinking.type = %q", got)
	}
	if !gjson.GetBytes(out, "thinking.clear_thinking").Bool() {
		t.Fatalf("expected clear_thinking preserved, got %s", out)
	}
}

func TestZAIThinkingApplierDisablesThinking(t *testing.T) {
	body := []byte(`{"model":"glm-5.1","messages":[],"reasoning_effort":"none"}`)

	out, err := thinking.ApplyThinking(body, "glm-5.1", "openai", "zai", "zai")
	if err != nil {
		t.Fatalf("ApplyThinking returned error: %v", err)
	}
	if got := gjson.GetBytes(out, "thinking.type").String(); got != "disabled" {
		t.Fatalf("thinking.type = %q", got)
	}
	if gjson.GetBytes(out, "reasoning_effort").Exists() {
		t.Fatalf("expected reasoning_effort removed, got %s", out)
	}
}
