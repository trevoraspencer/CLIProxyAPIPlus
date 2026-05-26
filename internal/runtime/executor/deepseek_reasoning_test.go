package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestDeepSeekDetectConservativeProviderAndBaseURL(t *testing.T) {
	cfg := &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: " deepseek "}}}
	exec := NewOpenAICompatExecutor("openai-compatibility", cfg)
	if !exec.deepSeekReasoningEnabled(&cliproxyauth.Auth{Provider: "openai-compatibility", Attributes: map[string]string{"compat_name": "DeepSeek"}}, "http://127.0.0.1:1/v1") {
		t.Fatalf("expected DeepSeek config identity to enable reasoning shim")
	}
	if !NewOpenAICompatExecutor("custom", nil).deepSeekReasoningEnabled(&cliproxyauth.Auth{}, "https://api.deepseek.com/v1") {
		t.Fatalf("expected allowed DeepSeek host to enable reasoning shim")
	}
	if NewOpenAICompatExecutor("openrouter", nil).deepSeekReasoningEnabled(&cliproxyauth.Auth{Provider: "openrouter"}, "https://openrouter.ai/api/v1") {
		t.Fatalf("non-DeepSeek provider/base URL must not enable reasoning shim")
	}
}

func TestDeepSeekDetectBaseURLHostAllowlistRejectsLookalikes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "allowed", raw: "https://api.deepseek.com/v1", want: true},
		{name: "allowed trailing dot", raw: "https://api.deepseek.com./v1", want: true},
		{name: "allowed with port", raw: "https://api.deepseek.com:443/v1", want: true},
		{name: "credentials rejected", raw: "https://user:pass@api.deepseek.com/v1", want: false},
		{name: "lookalike suffix", raw: "https://api.deepseek.com.evil.test/v1", want: false},
		{name: "lookalike hyphen", raw: "https://api-deepseek.com/v1", want: false},
		{name: "lookalike prefix", raw: "https://notdeepseek.com/v1", want: false},
		{name: "path text ignored", raw: "https://example.com/deepseek/api.deepseek.com", want: false},
		{name: "query text ignored", raw: "https://example.com/v1?next=api.deepseek.com", want: false},
		{name: "fragment text ignored", raw: "https://example.com/v1#api.deepseek.com", want: false},
		{name: "ip rejected", raw: "https://127.0.0.1/v1", want: false},
		{name: "malformed rejected", raw: "://api.deepseek.com", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deepSeekBaseURL(tc.raw); got != tc.want {
				t.Fatalf("deepSeekBaseURL(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestDeepSeekCacheKeyAndCacheTTLBoundConcurrent(t *testing.T) {
	cache := newDeepSeekReasoningCache(2, time.Minute)
	now := time.Unix(100, 0)
	cache.now = func() time.Time { return now }
	scope := deepSeekReasoningScope{Provider: "openai-compatibility", Auth: "auth-a", Model: "deepseek-chat", Session: "session-a"}
	keyA := deepSeekReasoningKey{deepSeekReasoningScope: scope, ToolCallIDs: "call-a"}
	keyB := deepSeekReasoningKey{deepSeekReasoningScope: scope, ToolCallIDs: "call-b"}
	keyC := deepSeekReasoningKey{deepSeekReasoningScope: scope, ToolCallIDs: "call-c"}

	cache.Store(keyA, "reason-a")
	now = now.Add(time.Second)
	cache.Store(keyB, "reason-b")
	now = now.Add(time.Second)
	cache.Store(keyC, "reason-c")
	if cache.Len() != 2 {
		t.Fatalf("cache len = %d, want bounded len 2", cache.Len())
	}
	if _, ok := cache.Lookup(keyA); ok {
		t.Fatalf("oldest entry was not evicted")
	}
	if got, ok := cache.Lookup(keyC); !ok || got != "reason-c" {
		t.Fatalf("lookup keyC = %q/%v, want reason-c/true", got, ok)
	}

	now = now.Add(2 * time.Minute)
	if _, ok := cache.Lookup(keyC); ok {
		t.Fatalf("expired entry should not replay")
	}

	concurrentCache := newDeepSeekReasoningCache(128, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := deepSeekReasoningKey{deepSeekReasoningScope: scope, ToolCallIDs: fmt.Sprintf("call-%d", i)}
			concurrentCache.Store(key, "reason")
			_, _ = concurrentCache.Lookup(key)
		}(i)
	}
	wg.Wait()
}

func TestDeepSeekKeyToolCallIDsAreOrderIndependentAndScoped(t *testing.T) {
	scope := deepSeekReasoningScope{Provider: "openai-compatibility", Auth: "auth-a", Model: "deepseek-chat", Session: "session-a"}
	first := deepSeekReasoningKeyForMessage(scope, mustJSONMap(t, `{"role":"assistant","tool_calls":[{"id":"b","type":"function","function":{"name":"b","arguments":"{}"}},{"id":"a","type":"function","function":{"name":"a","arguments":"{}"}}]}`))
	second := deepSeekReasoningKeyForMessage(scope, mustJSONMap(t, `{"role":"assistant","tool_calls":[{"id":"a","type":"function","function":{"name":"a","arguments":"{}"}},{"id":"b","type":"function","function":{"name":"b","arguments":"{}"}}]}`))
	if first.ToolCallIDs != second.ToolCallIDs {
		t.Fatalf("tool IDs should match regardless of order: %q vs %q", first.ToolCallIDs, second.ToolCallIDs)
	}
	cache := newDeepSeekReasoningCache(10, time.Minute)
	cache.Store(first, "secret synthetic reasoning")
	mismatched := second
	mismatched.Auth = "auth-b"
	if _, ok := cache.Lookup(mismatched); ok {
		t.Fatalf("cache replay leaked across auth scope")
	}
}

func TestDeepSeekPatchEligibleAssistantToolCallsOnly(t *testing.T) {
	cache := newDeepSeekReasoningCache(10, time.Minute)
	scope := deepSeekReasoningScope{Provider: "openai-compatibility", Auth: "auth-a", Model: "deepseek-chat", Session: "session-a"}
	cache.Store(deepSeekReasoningKeyForMessage(scope, mustJSONMap(t, `{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{\"path\":\"a\"}"}}]}`)), "cached reasoning")

	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"system","content":"s"},{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{\"path\":\"a\"}"}}],"extra":{"kept":true}},{"role":"assistant","content":null,"reasoning_content":"","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]},{"role":"assistant","content":"no tools"},{"role":"tool","tool_call_id":"call-1","content":"done"}]}`)
	patched := deepSeekPatchRequestReasoning(payload, scope, cache)
	if got := gjson.GetBytes(patched, "messages.1.reasoning_content").String(); got != "cached reasoning" {
		t.Fatalf("patched reasoning = %q, want cached reasoning; body=%s", got, string(patched))
	}
	if got := gjson.GetBytes(patched, "messages.1.extra.kept").Bool(); !got {
		t.Fatalf("unknown target fields were not preserved: %s", string(patched))
	}
	if got := gjson.GetBytes(patched, "messages.2.reasoning_content").String(); got != "" || !gjson.GetBytes(patched, "messages.2.reasoning_content").Exists() {
		t.Fatalf("explicit empty reasoning_content must be preserved: %s", string(patched))
	}
	if gjson.GetBytes(patched, "messages.3.reasoning_content").Exists() || gjson.GetBytes(patched, "messages.4.reasoning_content").Exists() {
		t.Fatalf("ineligible messages were patched: %s", string(patched))
	}
}

func TestDeepSeekPatchCacheMissUnchanged(t *testing.T) {
	scope := deepSeekReasoningScope{Provider: "openai-compatibility", Auth: "auth-a", Model: "deepseek-chat", Session: "session-a"}
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","tool_calls":[{"id":"missing","type":"function","function":{"name":"edit","arguments":"{}"}}]}]}`)
	patched := deepSeekPatchRequestReasoning(payload, scope, newDeepSeekReasoningCache(10, time.Minute))
	if string(patched) != string(payload) {
		t.Fatalf("cache miss changed payload: got %s want %s", string(patched), string(payload))
	}
}

func TestDeepSeekNonStreamCaptureStoresEligibleReasoningOnly(t *testing.T) {
	cache := newDeepSeekReasoningCache(10, time.Minute)
	scope := deepSeekReasoningScope{Provider: "openai-compatibility", Auth: "auth-a", Model: "deepseek-chat", Session: "session-a"}
	body := []byte(`{"choices":[{"message":{"role":"assistant","reasoning_content":"","tool_calls":[{"id":"empty","type":"function","function":{"name":"x","arguments":"{}"}}]}},{"message":{"role":"assistant","reasoning_content":"store me","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]}},{"message":{"role":"assistant","reasoning_content":"no tools"}}]}`)
	deepSeekCaptureNonStreamReasoning(body, scope, cache)
	if cache.Len() != 1 {
		t.Fatalf("cache len = %d, want exactly one eligible capture", cache.Len())
	}
	key := deepSeekReasoningKeyForMessage(scope, mustJSONMap(t, `{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]}`))
	if got, ok := cache.Lookup(key); !ok || got != "store me" {
		t.Fatalf("captured reasoning = %q/%v, want store me/true", got, ok)
	}
	deepSeekCaptureNonStreamReasoning([]byte(`{"choices":[{"message":{"reasoning_content":123,"tool_calls":[{"id":"bad"}]}}]}`), scope, cache)
	deepSeekCaptureNonStreamReasoning([]byte(`not-json`), scope, cache)
	if cache.Len() != 1 {
		t.Fatalf("malformed/ineligible responses should not populate cache; len=%d", cache.Len())
	}
}

func TestDeepSeekNonStreamCaptureRejectsNonAssistantAndInvalidToolCalls(t *testing.T) {
	scope := deepSeekReasoningScope{Provider: "openai-compatibility", Auth: "auth-a", Model: "deepseek-chat", Session: "session-a"}
	cases := []struct {
		name string
		body string
	}{
		{
			name: "user role",
			body: `{"choices":[{"message":{"role":"user","reasoning_content":"do not cache","tool_calls":[{"id":"call-user","type":"function","function":{"name":"edit","arguments":"{}"}}]}}]}`,
		},
		{
			name: "tool role",
			body: `{"choices":[{"message":{"role":"tool","reasoning_content":"do not cache","tool_calls":[{"id":"call-tool","type":"function","function":{"name":"edit","arguments":"{}"}}]}}]}`,
		},
		{
			name: "missing role",
			body: `{"choices":[{"message":{"reasoning_content":"do not cache","tool_calls":[{"id":"call-missing-role","type":"function","function":{"name":"edit","arguments":"{}"}}]}}]}`,
		},
		{
			name: "missing id",
			body: `{"choices":[{"message":{"role":"assistant","reasoning_content":"do not cache","tool_calls":[{"type":"function","function":{"name":"edit","arguments":"{}"}}]}}]}`,
		},
		{
			name: "missing function name",
			body: `{"choices":[{"message":{"role":"assistant","reasoning_content":"do not cache","tool_calls":[{"id":"call-missing-name","type":"function","function":{"arguments":"{}"}}]}}]}`,
		},
		{
			name: "missing function arguments",
			body: `{"choices":[{"message":{"role":"assistant","reasoning_content":"do not cache","tool_calls":[{"id":"call-missing-args","type":"function","function":{"name":"edit"}}]}}]}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cache := newDeepSeekReasoningCache(10, time.Minute)
			deepSeekCaptureNonStreamReasoning([]byte(tc.body), scope, cache)
			if cache.Len() != 0 {
				t.Fatalf("ineligible non-stream response populated cache; len=%d body=%s", cache.Len(), tc.body)
			}
		})
	}
}

func TestDeepSeekNonStreamExecutorCaptureThenPatchDroidReplay(t *testing.T) {
	restore := replaceDefaultDeepSeekCacheForTest(t)
	defer restore()

	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"synthetic hidden reasoning","tool_calls":[{"id":"call-edit-1","type":"function","function":{"name":"edit_file","arguments":"{\"path\":\"tmp.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl_2","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
	}))
	defer server.Close()

	exec := NewOpenAICompatExecutor("openai-compatibility", &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "deepseek", BaseURL: server.URL + "/v1"}}})
	auth := &cliproxyauth.Auth{ID: "auth-deepseek-a", Provider: "openai-compatibility", Attributes: map[string]string{
		"compat_name": "deepseek",
		"base_url":    server.URL + "/v1",
		"api_key":     "synthetic-key",
	}}
	if !exec.deepSeekReasoningEnabled(auth, server.URL+"/v1") {
		t.Fatalf("test DeepSeek executor fixture did not enable reasoning shim")
	}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Metadata: map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "droid-session-1"}}
	firstPayload := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"edit a file"}]}`)
	firstResp, err := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: firstPayload}, opts)
	if err != nil {
		t.Fatalf("first Execute error: %v", err)
	}
	if !strings.Contains(string(firstResp.Payload), "synthetic hidden reasoning") {
		t.Fatalf("non-stream response payload was not preserved: %s", string(firstResp.Payload))
	}
	if defaultDeepSeekReasoningCache.Len() != 1 {
		t.Fatalf("first response did not populate DeepSeek reasoning cache; len=%d", defaultDeepSeekReasoningCache.Len())
	}

	secondPayload := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"edit a file"},{"role":"assistant","content":null,"tool_calls":[{"id":"call-edit-1","type":"function","function":{"name":"edit_file","arguments":"{\"path\":\"tmp.txt\"}"}}]},{"role":"tool","tool_call_id":"call-edit-1","content":"ok"},{"role":"user","content":"continue"}]}`)
	if _, err = exec.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: secondPayload}, opts); err != nil {
		t.Fatalf("second Execute error: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("upstream request count = %d, want 2", len(bodies))
	}
	if got := gjson.GetBytes(bodies[1], "messages.1.reasoning_content").String(); got != "synthetic hidden reasoning" {
		t.Fatalf("second upstream body missing replayed reasoning: got %q body=%s", got, string(bodies[1]))
	}
}

func TestDeepSeekNonStreamExecutorDoesNotPatchNonDeepSeekModelNameOnly(t *testing.T) {
	restore := replaceDefaultDeepSeekCacheForTest(t)
	defer restore()

	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	scope := deepSeekReasoningScope{Provider: "openrouter", Auth: "auth-openrouter", Model: "deepseek-chat", Session: "droid-session-1"}
	defaultDeepSeekReasoningCache.Store(deepSeekReasoningKeyForMessage(scope, mustJSONMap(t, `{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]}`)), "must not leak")
	exec := NewOpenAICompatExecutor("openrouter", &config.Config{})
	auth := &cliproxyauth.Auth{ID: "auth-openrouter", Provider: "openrouter", Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "synthetic-key",
	}}
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]}]}`)
	_, err := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: payload}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Metadata:     map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "droid-session-1"},
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gjson.GetBytes(gotBody, "messages.0.reasoning_content").Exists() {
		t.Fatalf("non-DeepSeek model-name-only request was patched: %s", string(gotBody))
	}
	if defaultDeepSeekReasoningCache.Len() != 1 {
		t.Fatalf("non-DeepSeek response should not capture cache entries; len=%d", defaultDeepSeekReasoningCache.Len())
	}
}

func mustJSONMap(t *testing.T, raw string) map[string]any {
	t.Helper()
	var out map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return out
}

func replaceDefaultDeepSeekCacheForTest(t *testing.T) func() {
	t.Helper()
	previous := defaultDeepSeekReasoningCache
	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(32, time.Minute)
	return func() {
		defaultDeepSeekReasoningCache = previous
	}
}
