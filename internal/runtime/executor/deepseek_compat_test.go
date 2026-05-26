package executor

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	"github.com/tidwall/gjson"
)

func TestDeepSeekDetectUsesProviderConfigAndBaseURLSignals(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		auth     *cliproxyauth.Auth
		compat   *config.OpenAICompatibility
		baseURL  string
		model    string
		want     bool
	}{
		{
			name:     "provider name with case and whitespace",
			provider: "  DeepSeek  ",
			baseURL:  "https://aggregator.example/v1",
			model:    "not-deepseek",
			want:     true,
		},
		{
			name:     "compat config name",
			provider: "custom",
			compat:   &config.OpenAICompatibility{Name: " DeepSeek ", BaseURL: "https://custom.example/v1"},
			model:    "custom-model",
			want:     true,
		},
		{
			name:     "auth compat name",
			provider: "custom",
			auth:     &cliproxyauth.Auth{Provider: "openai-compatibility", Attributes: map[string]string{"compat_name": "deepseek", "base_url": "https://custom.example/v1"}},
			model:    "custom-model",
			want:     true,
		},
		{
			name:     "allowlisted base url",
			provider: "custom",
			baseURL:  "https://api.deepseek.com/v1",
			model:    "custom-model",
			want:     true,
		},
		{
			name:     "nil inputs are disabled",
			provider: "",
			model:    "deepseek-chat",
			want:     false,
		},
		{
			name:     "model name only does not activate",
			provider: "openrouter",
			baseURL:  "https://openrouter.ai/api/v1",
			model:    "deepseek-chat",
			want:     false,
		},
		{
			name:     "vague provider string does not activate",
			provider: "my-deepseek-relay",
			baseURL:  "https://relay.example/v1",
			model:    "plain-model",
			want:     false,
		},
		{
			name:     "disabled by lookalike host",
			provider: "custom",
			baseURL:  "https://deepseek.com.evil.test/v1",
			model:    "plain-model",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newDeepSeekCompatIdentity(tt.provider, tt.auth, tt.compat, tt.baseURL, tt.model, cliproxyexecutor.Options{}).Enabled
			if got != tt.want {
				t.Fatalf("Enabled = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeepSeekDetectHostAllowlistRejectsSpoofing(t *testing.T) {
	allowedHosts := make([]string, 0, len(deepSeekAllowedHosts))
	for host := range deepSeekAllowedHosts {
		allowedHosts = append(allowedHosts, host)
	}
	if len(allowedHosts) == 0 {
		t.Fatal("deepSeekAllowedHosts is empty")
	}
	for _, host := range allowedHosts {
		t.Run("allow "+host, func(t *testing.T) {
			if !isDeepSeekAllowedBaseURL("https://" + strings.ToUpper(host) + "./v1") {
				t.Fatalf("expected allowlisted host %q to enable DeepSeek detection", host)
			}
		})
	}

	rejected := []string{
		"",
		"api.deepseek.com",
		"://api.deepseek.com",
		"https://token@api.deepseek.com/v1",
		"https://deepseek.com.evil.test/v1",
		"https://api-deepseek.com/v1",
		"https://notdeepseek.com/v1",
		"https://127.0.0.1/v1/api.deepseek.com",
		"https://example.test/api.deepseek.com",
		"https://example.test?next=api.deepseek.com",
		"https://example.test#api.deepseek.com",
	}
	for _, raw := range rejected {
		t.Run(raw, func(t *testing.T) {
			if isDeepSeekAllowedBaseURL(raw) {
				t.Fatalf("isDeepSeekAllowedBaseURL(%q) = true, want false", raw)
			}
		})
	}
}

func TestDeepSeekNonDeepSeekReplayBlockedWithMatchingCache(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 4)
	turn := deepSeekReasoningTurn{Reasoning: "synthetic reasoning", ToolCallIDs: []string{"call-1"}}
	deepseekIdentity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	if !cache.Put(deepseekIdentity, turn) {
		t.Fatal("expected DeepSeek cache put to succeed")
	}
	nonDeepSeekIdentity := deepSeekCompatIdentity{Enabled: false, Provider: "openrouter", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	if got, ok := cache.Get(nonDeepSeekIdentity, turn); ok {
		t.Fatalf("non-DeepSeek replay returned %q, want blocked", got)
	}
}

func TestDeepSeekCacheKeyIsolatesProviderAuthSessionModelAndToolCalls(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 8)
	base := deepSeekCompatIdentity{
		Enabled:      true,
		Provider:     "deepseek",
		AuthScope:    "id:auth-a",
		Model:        "deepseek-chat",
		SessionScope: "session-a",
	}
	turn := deepSeekReasoningTurn{Reasoning: "synthetic reasoning", ToolCallIDs: []string{"call-b", "call-a"}}
	if !cache.Put(base, turn) {
		t.Fatal("expected cache put to succeed")
	}
	if got, ok := cache.Get(base, deepSeekReasoningTurn{ToolCallIDs: []string{"call-a", "call-b"}}); !ok || got != turn.Reasoning {
		t.Fatalf("reordered tool-call lookup = %q, %v; want cached reasoning", got, ok)
	}

	tests := []struct {
		name     string
		identity deepSeekCompatIdentity
		turn     deepSeekReasoningTurn
	}{
		{"provider", deepSeekCompatIdentity{Enabled: true, Provider: "other", AuthScope: base.AuthScope, Model: base.Model, SessionScope: base.SessionScope}, deepSeekReasoningTurn{ToolCallIDs: []string{"call-a", "call-b"}}},
		{"auth", deepSeekCompatIdentity{Enabled: true, Provider: base.Provider, AuthScope: "id:auth-b", Model: base.Model, SessionScope: base.SessionScope}, deepSeekReasoningTurn{ToolCallIDs: []string{"call-a", "call-b"}}},
		{"session", deepSeekCompatIdentity{Enabled: true, Provider: base.Provider, AuthScope: base.AuthScope, Model: base.Model, SessionScope: "session-b"}, deepSeekReasoningTurn{ToolCallIDs: []string{"call-a", "call-b"}}},
		{"model", deepSeekCompatIdentity{Enabled: true, Provider: base.Provider, AuthScope: base.AuthScope, Model: "deepseek-reasoner", SessionScope: base.SessionScope}, deepSeekReasoningTurn{ToolCallIDs: []string{"call-a", "call-b"}}},
		{"tool call", base, deepSeekReasoningTurn{ToolCallIDs: []string{"call-a", "call-c"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := cache.Get(tt.identity, tt.turn); ok {
				t.Fatalf("mismatched %s returned %q, want miss", tt.name, got)
			}
		})
	}
}

func TestDeepSeekCacheNoIDFallbackRequiresStableSessionScope(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 4)
	noSession := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	fallbackTurn := deepSeekReasoningTurn{Reasoning: "synthetic reasoning", AssistantTurnHash: hashDeepSeekParts("assistant-turn")}
	if cache.Put(noSession, fallbackTurn) {
		t.Fatal("fallback put without session scope succeeded, want disabled")
	}

	withSession := noSession
	withSession.SessionScope = "session-a"
	if !cache.Put(withSession, fallbackTurn) {
		t.Fatal("fallback put with session scope failed")
	}
	if got, ok := cache.Get(withSession, deepSeekReasoningTurn{AssistantTurnHash: fallbackTurn.AssistantTurnHash}); !ok || got != fallbackTurn.Reasoning {
		t.Fatalf("fallback lookup with session = %q, %v; want cached reasoning", got, ok)
	}
	otherSession := withSession
	otherSession.SessionScope = "session-b"
	if got, ok := cache.Get(otherSession, deepSeekReasoningTurn{AssistantTurnHash: fallbackTurn.AssistantTurnHash}); ok {
		t.Fatalf("fallback lookup crossed session and returned %q", got)
	}
}

func TestDeepSeekCacheUsesFinalModelIdentity(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 4)
	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "final-upstream-model"}
	turn := deepSeekReasoningTurn{Reasoning: "synthetic reasoning", ToolCallIDs: []string{"call-1"}}
	if !cache.Put(identity, turn) {
		t.Fatal("expected cache put to succeed")
	}
	aliasIdentity := identity
	aliasIdentity.Model = "client-alias"
	if got, ok := cache.Get(aliasIdentity, deepSeekReasoningTurn{ToolCallIDs: []string{"call-1"}}); ok {
		t.Fatalf("lookup with client alias returned %q, want final-model isolation", got)
	}
	if got, ok := cache.Get(identity, deepSeekReasoningTurn{ToolCallIDs: []string{"call-1"}}); !ok || got != turn.Reasoning {
		t.Fatalf("lookup with final model = %q, %v; want cached reasoning", got, ok)
	}
}

func TestDeepSeekCacheTTLSizeAndEmptyReasoning(t *testing.T) {
	now := time.Unix(100, 0)
	cache := newDeepSeekReasoningCache(10*time.Millisecond, 2)
	cache.now = func() time.Time { return now }
	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}

	if cache.Put(identity, deepSeekReasoningTurn{Reasoning: "", ToolCallIDs: []string{"empty"}}) {
		t.Fatal("empty reasoning was cached")
	}
	if !cache.Put(identity, deepSeekReasoningTurn{Reasoning: "one", ToolCallIDs: []string{"one"}}) {
		t.Fatal("put one failed")
	}
	if !cache.Put(identity, deepSeekReasoningTurn{Reasoning: "two", ToolCallIDs: []string{"two"}}) {
		t.Fatal("put two failed")
	}
	now = now.Add(time.Millisecond)
	if !cache.Put(identity, deepSeekReasoningTurn{Reasoning: "three", ToolCallIDs: []string{"three"}}) {
		t.Fatal("put three failed")
	}
	if got := cache.Len(); got != 2 {
		t.Fatalf("cache Len = %d, want max size 2", got)
	}
	if got, ok := cache.Get(identity, deepSeekReasoningTurn{ToolCallIDs: []string{"one"}}); ok {
		t.Fatalf("oldest entry returned %q after size eviction", got)
	}
	now = now.Add(20 * time.Millisecond)
	if got, ok := cache.Get(identity, deepSeekReasoningTurn{ToolCallIDs: []string{"three"}}); ok {
		t.Fatalf("expired entry returned %q", got)
	}
	if got := cache.Len(); got != 0 {
		t.Fatalf("cache Len after TTL expiration = %d, want 0", got)
	}
}

func TestDeepSeekCacheConcurrent(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 64)
	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	var wg sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				id := fmt.Sprintf("call-%d-%d", worker, i)
				turn := deepSeekReasoningTurn{Reasoning: "synthetic reasoning", ToolCallIDs: []string{id}}
				cache.Put(identity, turn)
				cache.Get(identity, deepSeekReasoningTurn{ToolCallIDs: []string{id}})
			}
		}()
	}
	wg.Wait()
	if got := cache.Len(); got > 64 {
		t.Fatalf("cache Len = %d, want <= 64", got)
	}
}

func TestDeepSeekKeySecretMinimal(t *testing.T) {
	auth := &cliproxyauth.Auth{
		ID:       "auth-id",
		Provider: "deepseek",
		Attributes: map[string]string{
			"api_key":  "sk-secret-sentinel",
			"base_url": "https://api.deepseek.com/v1",
		},
		Metadata: map[string]any{"access_token": "bearer-secret-sentinel"},
	}
	identity := newDeepSeekCompatIdentity("deepseek", auth, nil, "", "final-model", cliproxyexecutor.Options{
		Metadata: map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "session-a"},
	})
	key, ok := deepSeekReasoningKey(identity, deepSeekReasoningTurn{ToolCallIDs: []string{"call-1"}})
	if !ok {
		t.Fatal("expected key construction to succeed")
	}
	keyText := fmt.Sprintf("%#v", key)
	for _, forbidden := range []string{"sk-secret-sentinel", "bearer-secret-sentinel", "Authorization", "full request", "full response"} {
		if strings.Contains(keyText, forbidden) {
			t.Fatalf("cache key contains forbidden value %q: %s", forbidden, keyText)
		}
	}
	if !strings.Contains(keyText, "final-model") || !strings.Contains(keyText, "session-a") || !strings.Contains(keyText, "auth-id") {
		t.Fatalf("cache key missing required non-secret scope: %s", keyText)
	}
}

func TestDeepSeekReasoningTurnFromAssistantMessage(t *testing.T) {
	raw := []byte(`{"role":"assistant","content":null,"reasoning_content":"synthetic reasoning","tool_calls":[{"id":"call-2","type":"function","function":{"name":"edit","arguments":"{\"x\":1}"}},{"id":"call-1","type":"function","function":{"name":"read","arguments":"{}"}}],"unknown":true}`)
	turn, ok := deepSeekReasoningTurnFromAssistantMessage(raw)
	if !ok {
		t.Fatal("expected assistant tool-call reasoning turn")
	}
	if turn.Reasoning != "synthetic reasoning" {
		t.Fatalf("Reasoning = %q", turn.Reasoning)
	}
	if got, want := strings.Join(normalizedToolCallIDs(turn.ToolCallIDs), ","), "call-1,call-2"; got != want {
		t.Fatalf("ToolCallIDs = %s, want %s", got, want)
	}

	rejected := [][]byte{
		[]byte(`{"role":"user","reasoning_content":"synthetic reasoning","tool_calls":[{"id":"call-1"}]}`),
		[]byte(`{"role":"assistant","reasoning_content":"","tool_calls":[{"id":"call-1"}]}`),
		[]byte(`{"role":"assistant","reasoning_content":"synthetic reasoning","tool_calls":[]}`),
		[]byte(`not-json`),
	}
	for _, rawRejected := range rejected {
		if _, ok := deepSeekReasoningTurnFromAssistantMessage(rawRejected); ok {
			t.Fatalf("unexpected turn from %s", rawRejected)
		}
	}
}

func TestDeepSeekPatchRequestPayloadPatchesOnlyEligibleMissingReasoning(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 8)
	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	if !cache.Put(identity, deepSeekReasoningTurn{Reasoning: "cached reasoning", ToolCallIDs: []string{"call-1"}}) {
		t.Fatal("expected cache put")
	}
	if !cache.Put(identity, deepSeekReasoningTurn{Reasoning: "conflicting reasoning", ToolCallIDs: []string{"call-2"}}) {
		t.Fatal("expected cache put for conflicting entry")
	}

	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"system","content":"s"},{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{\"x\":1}"}}],"name":"kept","x-extra":{"ok":true}},{"role":"assistant","content":null,"reasoning_content":"","tool_calls":[{"id":"call-2","type":"function","function":{"name":"kept","arguments":"{}"}}]},{"role":"assistant","content":"no tools"},{"role":"user","content":"hi","tool_calls":[{"id":"call-1"}]}]}`)
	patched := deepSeekPatchRequestPayload(cache, identity, payload)

	first := gjson.GetBytes(patched, "messages.1")
	if got := gjson.Get(first.Raw, "reasoning_content").String(); got != "cached reasoning" {
		t.Fatalf("patched reasoning = %q, want cached reasoning; payload=%s", got, patched)
	}
	if got := gjson.Get(first.Raw, "name").String(); got != "kept" {
		t.Fatalf("name field changed to %q", got)
	}
	if got := gjson.Get(first.Raw, "tool_calls.0.function.arguments").String(); got != `{"x":1}` {
		t.Fatalf("arguments field changed to %q", got)
	}
	if !gjson.Get(first.Raw, "x-extra.ok").Bool() {
		t.Fatalf("unknown extension field was not preserved: %s", first.Raw)
	}
	if got := gjson.GetBytes(patched, "messages.2.reasoning_content"); !got.Exists() || got.String() != "" {
		t.Fatalf("existing empty reasoning not preserved: exists=%v value=%q", got.Exists(), got.String())
	}
	if gjson.GetBytes(patched, "messages.3.reasoning_content").Exists() {
		t.Fatalf("assistant without tool calls was patched: %s", patched)
	}
	if gjson.GetBytes(patched, "messages.4.reasoning_content").Exists() {
		t.Fatalf("user message was patched: %s", patched)
	}
}

func TestDeepSeekPatchRequestPayloadCacheMissAndNonDeepSeekUnchanged(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 4)
	deepSeekIdentity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	nonDeepSeekIdentity := deepSeekCompatIdentity{Enabled: false, Provider: "openrouter", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call-1"}]}]}`)
	if got := deepSeekPatchRequestPayload(cache, deepSeekIdentity, payload); string(got) != string(payload) {
		t.Fatalf("cache miss changed payload: got=%s want=%s", got, payload)
	}
	if !cache.Put(deepSeekIdentity, deepSeekReasoningTurn{Reasoning: "cached reasoning", ToolCallIDs: []string{"call-1"}}) {
		t.Fatal("expected cache put")
	}
	if got := deepSeekPatchRequestPayload(cache, nonDeepSeekIdentity, payload); string(got) != string(payload) {
		t.Fatalf("non-DeepSeek changed payload despite matching cache: got=%s want=%s", got, payload)
	}
}

func TestDeepSeekNonStreamCaptureStoresOnlyEligibleAssistantToolCallReasoning(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 8)
	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	body := []byte(`{"id":"chatcmpl_1","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"synthetic reasoning","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]}},{"index":1,"message":{"role":"assistant","content":"no tools","reasoning_content":"ignored"}},{"index":2,"message":{"role":"assistant","content":null,"reasoning_content":"","tool_calls":[{"id":"call-empty"}]}},{"index":3,"message":{"role":"user","reasoning_content":"ignored","tool_calls":[{"id":"call-user"}]}}]}`)
	deepSeekCaptureNonStreamResponse(cache, identity, body)
	if got, ok := cache.Get(identity, deepSeekReasoningTurn{ToolCallIDs: []string{"call-1"}}); !ok || got != "synthetic reasoning" {
		t.Fatalf("eligible capture lookup = %q, %v; want synthetic reasoning", got, ok)
	}
	for _, id := range []string{"call-empty", "call-user"} {
		if got, ok := cache.Get(identity, deepSeekReasoningTurn{ToolCallIDs: []string{id}}); ok {
			t.Fatalf("ineligible id %s captured reasoning %q", id, got)
		}
	}
}

func TestDeepSeekNonStreamCaptureIgnoresMalformedFailedAndNonDeepSeek(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 8)
	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	nonDeepSeekIdentity := deepSeekCompatIdentity{Enabled: false, Provider: "openrouter", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	deepSeekCaptureNonStreamResponse(cache, identity, []byte(`not-json`))
	deepSeekCaptureNonStreamResponse(cache, identity, []byte(`{"choices":[{"message":{"role":"assistant","reasoning_content":42,"tool_calls":[{"id":"call-1"}]}}]}`))
	deepSeekCaptureNonStreamResponse(cache, nonDeepSeekIdentity, []byte(`{"choices":[{"message":{"role":"assistant","reasoning_content":"synthetic reasoning","tool_calls":[{"id":"call-1"}]}}]}`))
	if got := cache.Len(); got != 0 {
		t.Fatalf("cache Len = %d, want 0 after malformed/ineligible captures", got)
	}
}

func TestDeepSeekAssistantTurnHashIgnoresReasoningContent(t *testing.T) {
	without := []byte(`{"role":"assistant","content":null,"tool_calls":[{"function":{"arguments":"{}","name":"edit"},"id":"call-1","type":"function"}]}`)
	with := []byte(`{"role":"assistant","content":null,"reasoning_content":"synthetic reasoning","tool_calls":[{"function":{"arguments":"{}","name":"edit"},"id":"call-1","type":"function"}]}`)
	if deepSeekAssistantTurnHash(without) == "" || deepSeekAssistantTurnHash(without) != deepSeekAssistantTurnHash(with) {
		t.Fatalf("assistant turn hash should be non-empty and ignore reasoning_content")
	}
}

func TestDeepSeekStreamCaptureReconstructsChoicesAndToolIndexes(t *testing.T) {
	cache := newDeepSeekReasoningCache(time.Minute, 8)
	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	capture := newDeepSeekStreamCapture(cache, identity)

	lines := [][]byte{
		[]byte(`: keepalive`),
		[]byte(`event: completion`),
		[]byte(`data: {"id":"chunk-1","choices":[{"index":0,"delta":{"reasoning_content":"choice0-","tool_calls":[{"index":1,"id":"call-b","type":"function","function":{"name":"wr","arguments":"arg-b-"}},{"index":0,"id":"call-a","type":"function","function":{"name":"ed","arguments":"arg-a-"}}]}},{"index":1,"delta":{"reasoning_content":"choice1-","tool_calls":[{"index":0,"id":"call-c","type":"function","function":{"name":"re","arguments":"arg-c-"}}]}}]}`),
		[]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"reason","content":"visible","tool_calls":[{"index":1,"function":{"name":"ite","arguments":"done"}},{"index":0,"function":{"name":"it","arguments":"done"}}]}},{"index":1,"delta":{"reasoning_content":"reason","tool_calls":[{"index":0,"function":{"name":"ad","arguments":"done"}}]}}]}`),
		[]byte(`data: {"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`),
		[]byte(`data: [DONE]`),
	}
	for _, line := range lines {
		capture.ObserveSSELine(line)
	}

	if got, ok := cache.Get(identity, deepSeekReasoningTurn{ToolCallIDs: []string{"call-a", "call-b"}}); !ok || got != "choice0-reason" {
		t.Fatalf("choice 0 cache lookup = %q, %v; want choice0-reason", got, ok)
	}
	if got, ok := cache.Get(identity, deepSeekReasoningTurn{ToolCallIDs: []string{"call-c"}}); !ok || got != "choice1-reason" {
		t.Fatalf("choice 1 cache lookup = %q, %v; want choice1-reason", got, ok)
	}
}

func TestDeepSeekStreamCaptureIgnoresNoiseAndRejectsIncompleteOrMalformed(t *testing.T) {
	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}

	incompleteCache := newDeepSeekReasoningCache(time.Minute, 8)
	incomplete := newDeepSeekStreamCapture(incompleteCache, identity)
	incomplete.ObserveSSELine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"synthetic","tool_calls":[{"index":0,"id":"call-incomplete","function":{"arguments":"{}"}}]}}]}`))
	incomplete.Commit()
	if got := incompleteCache.Len(); got != 0 {
		t.Fatalf("incomplete tool call populated cache, Len=%d", got)
	}

	malformedCache := newDeepSeekReasoningCache(time.Minute, 8)
	malformed := newDeepSeekStreamCapture(malformedCache, identity)
	malformed.ObserveSSELine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"synthetic","tool_calls":[{"index":0,"id":"call-malformed","function":{"name":"edit","arguments":"{}"}}]}}]}`))
	malformed.ObserveSSELine([]byte(`data: {"choices":[`))
	malformed.ObserveSSELine([]byte(`data: [DONE]`))
	if got := malformedCache.Len(); got != 0 {
		t.Fatalf("malformed stream populated cache, Len=%d", got)
	}

	nonStringReasoningCache := newDeepSeekReasoningCache(time.Minute, 8)
	nonStringReasoning := newDeepSeekStreamCapture(nonStringReasoningCache, identity)
	nonStringReasoning.ObserveSSELine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":["not-string"],"tool_calls":[{"index":0,"id":"call-non-string","function":{"name":"edit","arguments":"{}"}}]}}]}`))
	nonStringReasoning.Commit()
	if got := nonStringReasoningCache.Len(); got != 0 {
		t.Fatalf("non-string reasoning populated cache, Len=%d", got)
	}

	nonDeepSeekCache := newDeepSeekReasoningCache(time.Minute, 8)
	nonDeepSeek := newDeepSeekStreamCapture(nonDeepSeekCache, deepSeekCompatIdentity{Enabled: false, Provider: "openrouter", AuthScope: "id:auth-a", Model: "deepseek-chat"})
	nonDeepSeek.ObserveSSELine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"synthetic","tool_calls":[{"index":0,"id":"call-1","function":{"name":"edit","arguments":"{}"}}]}}]}`))
	nonDeepSeek.Commit()
	if got := nonDeepSeekCache.Len(); got != 0 {
		t.Fatalf("non-DeepSeek stream populated cache, Len=%d", got)
	}
}
