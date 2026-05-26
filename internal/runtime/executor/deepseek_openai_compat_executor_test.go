package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestDeepSeekOpenAICompatNonStreamCaptureThenPatchDroidReplay(t *testing.T) {
	oldCache := defaultDeepSeekReasoningCache
	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(time.Minute, 16)
	t.Cleanup(func() { defaultDeepSeekReasoningCache = oldCache })

	var mu sync.Mutex
	var upstreamBodies [][]byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		upstreamBodies = append(upstreamBodies, append([]byte(nil), body...))
		attempt := len(upstreamBodies)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if attempt == 1 {
			_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"synthetic captured reasoning","tool_calls":[{"id":"call-edit-1","type":"function","function":{"name":"edit","arguments":"{\"path\":\"file.txt\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			return
		}
		if got := gjson.GetBytes(body, "messages.1.reasoning_content").String(); got != "synthetic captured reasoning" {
			http.Error(w, "missing patched reasoning", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl_2","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{
		SDKConfig:           config.SDKConfig{RequestLog: true},
		OpenAICompatibility: []config.OpenAICompatibility{{Name: "deepseek", BaseURL: server.URL + "/v1"}},
	})
	auth := &cliproxyauth.Auth{ID: "auth-a", Provider: "openai-compatibility", Attributes: map[string]string{
		"compat_name": "deepseek",
		"base_url":    server.URL + "/v1",
		"api_key":     "synthetic-key",
	}}
	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Metadata:     map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "session-a"},
	}

	firstPayload := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"edit file"}]}`)
	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: firstPayload}, opts)
	if err != nil {
		t.Fatalf("first Execute error: %v", err)
	}
	if !gjson.GetBytes(resp.Payload, "choices.0.message.reasoning_content").Exists() {
		t.Fatalf("raw non-stream response did not preserve reasoning_content: %s", resp.Payload)
	}

	gin.SetMode(gin.TestMode)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctxWithGin := context.WithValue(context.Background(), "gin", ginCtx)
	replayPayload := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"edit file"},{"role":"assistant","content":null,"tool_calls":[{"id":"call-edit-1","type":"function","function":{"name":"edit","arguments":"{\"path\":\"file.txt\"}"}}]},{"role":"tool","tool_call_id":"call-edit-1","content":"ok"},{"role":"user","content":"continue"}]}`)
	if _, err := executor.Execute(ctxWithGin, auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: replayPayload}, opts); err != nil {
		t.Fatalf("second Execute error: %v", err)
	}

	mu.Lock()
	if len(upstreamBodies) != 2 {
		t.Fatalf("upstream request count = %d, want 2", len(upstreamBodies))
	}
	secondBody := append([]byte(nil), upstreamBodies[1]...)
	mu.Unlock()
	if got := gjson.GetBytes(secondBody, "messages.1.reasoning_content").String(); got != "synthetic captured reasoning" {
		t.Fatalf("sent upstream reasoning = %q, want captured reasoning; body=%s", got, secondBody)
	}
	logValue, ok := ginCtx.Get("API_REQUEST")
	if !ok {
		t.Fatal("expected request log body")
	}
	logText := string(logValue.([]byte))
	if !strings.Contains(logText, `"reasoning_content":"synthetic captured reasoning"`) {
		t.Fatalf("request log did not contain patched body: %s", logText)
	}
}

func TestDeepSeekOpenAICompatCacheMissLeavesUpstreamErrorVisible(t *testing.T) {
	oldCache := defaultDeepSeekReasoningCache
	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(time.Minute, 16)
	t.Cleanup(func() { defaultDeepSeekReasoningCache = oldCache })

	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"missing reasoning_content"}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("deepseek", &config.Config{})
	auth := &cliproxyauth.Auth{ID: "auth-a", Provider: "deepseek", Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "synthetic-key",
	}}
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call-miss","type":"function","function":{"name":"edit","arguments":"{}"}}]}]}`)
	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: payload}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")})
	if err == nil {
		t.Fatal("expected upstream bad request error")
	}
	if !strings.Contains(err.Error(), "missing reasoning_content") {
		t.Fatalf("upstream error not visible: %v", err)
	}
	if gjson.GetBytes(gotBody, "messages.0.reasoning_content").Exists() {
		t.Fatalf("cache miss invented reasoning_content: %s", gotBody)
	}
}

func TestDeepSeekOpenAICompatNonStreamCaptureIgnoresFailedAndMalformedResponses(t *testing.T) {
	oldCache := defaultDeepSeekReasoningCache
	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(time.Minute, 16)
	t.Cleanup(func() { defaultDeepSeekReasoningCache = oldCache })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","reasoning_content":"must not cache","tool_calls":[{"id":"call-failed"}]}}]}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("deepseek", &config.Config{})
	auth := &cliproxyauth.Auth{ID: "auth-a", Provider: "deepseek", Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  "synthetic-key",
	}}
	_, _ = executor.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}]}`)}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")})
	if got := defaultDeepSeekReasoningCache.Len(); got != 0 {
		t.Fatalf("failed response populated cache, Len=%d", got)
	}

	deepSeekCaptureNonStreamResponse(defaultDeepSeekReasoningCache, deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}, []byte(`{"choices":[{"message":`))
	if got := defaultDeepSeekReasoningCache.Len(); got != 0 {
		t.Fatalf("malformed response populated cache, Len=%d", got)
	}
}
