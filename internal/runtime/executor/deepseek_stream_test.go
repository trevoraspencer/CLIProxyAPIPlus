package executor

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

func TestDeepSeekStreamCaptureReconstructsFragmentsAndReplayPatchesStream(t *testing.T) {
	restore := replaceDefaultDeepSeekCacheForTest(t)
	defer restore()

	var bodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(bodies) == 1 {
			_, _ = w.Write([]byte(": keepalive\n\n"))
			_, _ = w.Write([]byte("event: message\n"))
			_, _ = w.Write([]byte(`data: {"id":"stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"synthetic "}}]}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"id":"stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"stream reasoning ","tool_calls":[{"index":0,"id":"call-","type":"function","function":{"name":"edit_","arguments":"{\"path\":"}}]}}]}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"id":"stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"by choice","content":"ignored","tool_calls":[{"index":0,"id":"edit-1","function":{"name":"file","arguments":"\"tmp.txt\"}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"id":"stream_1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		if got := gjson.GetBytes(body, "messages.1.reasoning_content").String(); got != "synthetic stream reasoning by choice" {
			http.Error(w, "missing replayed reasoning", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`data: {"id":"stream_2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	exec := NewOpenAICompatExecutor("openai-compatibility", &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "deepseek", BaseURL: server.URL + "/v1"}}})
	auth := &cliproxyauth.Auth{ID: "auth-deepseek-stream", Provider: "openai-compatibility", Attributes: map[string]string{"compat_name": "deepseek", "base_url": server.URL + "/v1", "api_key": "synthetic-key"}}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Metadata: map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "droid-stream-session"}}

	firstStream, err := exec.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"edit a file"}],"stream":true}`)}, opts)
	if err != nil {
		t.Fatalf("first ExecuteStream error: %v", err)
	}
	firstChunks := collectDeepSeekStreamPayloads(t, firstStream)
	if joined := strings.Join(firstChunks, ""); !strings.Contains(joined, "synthetic ") || !strings.Contains(joined, "stream reasoning ") {
		t.Fatalf("stream output changed unexpectedly: %q", joined)
	}
	if defaultDeepSeekReasoningCache.Len() != 1 {
		t.Fatalf("stream did not commit one eligible reasoning turn; len=%d", defaultDeepSeekReasoningCache.Len())
	}

	secondPayload := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"edit a file"},{"role":"assistant","content":null,"tool_calls":[{"id":"call-edit-1","type":"function","function":{"name":"edit_file","arguments":"{\"path\":\"tmp.txt\"}"}}]},{"role":"tool","tool_call_id":"call-edit-1","content":"ok"},{"role":"user","content":"continue"}],"stream":true}`)
	secondStream, err := exec.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: secondPayload}, opts)
	if err != nil {
		t.Fatalf("second ExecuteStream error: %v", err)
	}
	if joined := strings.Join(collectDeepSeekStreamPayloads(t, secondStream), ""); !strings.Contains(joined, "done") {
		t.Fatalf("second stream output changed unexpectedly: %q", joined)
	}
}

func TestDeepSeekStreamCaptureCleanEOFCommitsButMalformedDoesNot(t *testing.T) {
	scope := deepSeekReasoningScope{Provider: "openai-compatibility", Auth: "auth-a", Model: "deepseek-chat", Session: "session-a"}
	cache := newDeepSeekReasoningCache(10, time.Minute)
	capture := newDeepSeekStreamCapture(scope, cache)
	capture.ObserveLine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"clean eof reasoning","tool_calls":[{"index":0,"id":"call-clean","type":"function","function":{"name":"edit","arguments":"{}"}}]}}]}`))
	capture.Commit()
	key := deepSeekReasoningKeyForMessage(scope, mustJSONMap(t, `{"role":"assistant","tool_calls":[{"id":"call-clean","type":"function","function":{"name":"edit","arguments":"{}"}}]}`))
	if got, ok := cache.Lookup(key); !ok || got != "clean eof reasoning" {
		t.Fatalf("clean EOF capture = %q/%v, want clean eof reasoning/true", got, ok)
	}

	malformedCache := newDeepSeekReasoningCache(10, time.Minute)
	malformed := newDeepSeekStreamCapture(scope, malformedCache)
	malformed.ObserveLine([]byte(`data: {"choices":[{"index":0,"delta":{"reasoning_content":"partial","tool_calls":[{"index":0,"id":"call-bad"}]}}]}`))
	malformed.ObserveLine([]byte(`data: {not-json}`))
	malformed.Commit()
	if malformedCache.Len() != 0 {
		t.Fatalf("malformed stream populated cache; len=%d", malformedCache.Len())
	}
}

func TestDeepSeekStreamNonDeepSeekPassThroughWithPrepopulatedCache(t *testing.T) {
	restore := replaceDefaultDeepSeekCacheForTest(t)
	defer restore()

	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"id":"stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	scope := deepSeekReasoningScope{Provider: "openrouter", Auth: "auth-openrouter", Model: "deepseek-chat", Session: "droid-stream-session"}
	defaultDeepSeekReasoningCache.Store(deepSeekReasoningKeyForMessage(scope, mustJSONMap(t, `{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]}`)), "must not leak")
	exec := NewOpenAICompatExecutor("openrouter", &config.Config{})
	auth := &cliproxyauth.Auth{ID: "auth-openrouter", Provider: "openrouter", Attributes: map[string]string{"base_url": server.URL + "/v1", "api_key": "synthetic-key"}}
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]}],"stream":true}`)
	result, err := exec.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: payload}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Metadata: map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "droid-stream-session"}})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	if joined := strings.Join(collectDeepSeekStreamPayloads(t, result), ""); !strings.Contains(joined, "ok") {
		t.Fatalf("non-DeepSeek stream output changed unexpectedly: %q", joined)
	}
	if gjson.GetBytes(body, "messages.0.reasoning_content").Exists() || defaultDeepSeekReasoningCache.Len() != 1 {
		t.Fatalf("non-DeepSeek stream patched or captured unexpectedly: body=%s len=%d", string(body), defaultDeepSeekReasoningCache.Len())
	}
}

func TestDeepSeekCountTokensDoesNotPatchOrCache(t *testing.T) {
	restore := replaceDefaultDeepSeekCacheForTest(t)
	defer restore()
	scope := deepSeekReasoningScope{Provider: "openai-compatibility", Auth: "auth-deepseek", Model: "deepseek-chat", Session: "token-session"}
	defaultDeepSeekReasoningCache.Store(deepSeekReasoningKeyForMessage(scope, mustJSONMap(t, `{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]}`)), "cached token reasoning")
	exec := NewOpenAICompatExecutor("openai-compatibility", &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "deepseek", BaseURL: "https://api.deepseek.com/v1"}}})
	resp, err := exec.CountTokens(context.Background(), &cliproxyauth.Auth{ID: "auth-deepseek", Provider: "openai-compatibility", Attributes: map[string]string{"compat_name": "deepseek"}}, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"edit","arguments":"{}"}}]}]}`)}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Metadata: map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "token-session"}})
	if err != nil {
		t.Fatalf("CountTokens error: %v", err)
	}
	if strings.Contains(string(resp.Payload), "cached token reasoning") || defaultDeepSeekReasoningCache.Len() != 1 {
		t.Fatalf("CountTokens used reasoning cache unexpectedly: resp=%s len=%d", string(resp.Payload), defaultDeepSeekReasoningCache.Len())
	}
}

func TestDeepSeekDefaultLogsDoNotExposeReasoningOrSecrets(t *testing.T) {
	var logs bytes.Buffer
	previousOut := log.StandardLogger().Out
	log.SetOutput(&logs)
	defer log.SetOutput(previousOut)
	restore := replaceDefaultDeepSeekCacheForTest(t)
	defer restore()
	scope := deepSeekReasoningScope{Provider: "openai-compatibility", Auth: "auth-a", Model: "deepseek-chat", Session: "session-a"}
	defaultDeepSeekReasoningCache.Store(deepSeekReasoningKeyForMessage(scope, mustJSONMap(t, `{"role":"assistant","tool_calls":[{"id":"call-secret","type":"function","function":{"name":"edit","arguments":"{}"}}]}`)), "SENTINEL_REASONING_DO_NOT_LOG")
	_ = deepSeekPatchRequestReasoning([]byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","tool_calls":[{"id":"call-secret","type":"function","function":{"name":"edit","arguments":"{}"}}]}]}`), scope, defaultDeepSeekReasoningCache)
	if got := logs.String(); strings.Contains(got, "SENTINEL_REASONING_DO_NOT_LOG") || strings.Contains(got, "SENTINEL_SECRET_KEY") {
		t.Fatalf("default logs exposed sentinel values: %q", got)
	}
}

func collectDeepSeekStreamPayloads(t *testing.T, result *cliproxyexecutor.StreamResult) []string {
	t.Helper()
	payloads := make([]string, 0, 8)
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		payloads = append(payloads, string(chunk.Payload))
	}
	return payloads
}
