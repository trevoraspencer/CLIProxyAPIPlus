package executor

import (
	"bytes"
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
	log "github.com/sirupsen/logrus"
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

func TestDeepSeekOpenAICompatStreamCaptureThenPatchDroidReplay(t *testing.T) {
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
		if attempt == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(": keepalive\n\nevent: completion\nid: 1\nretry: 1000\n"))
			_, _ = w.Write([]byte(`data: {"id":"chatcmpl_stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"synthetic ","tool_calls":[{"index":0,"id":"call-stream-1","type":"function","function":{"name":"ed","arguments":"arg-"}}]},"finish_reason":null}]}` + "\n"))
			_, _ = w.Write([]byte(`data: {"id":"chatcmpl_stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"stream reasoning","content":"visible","tool_calls":[{"index":0,"function":{"name":"it","arguments":"done"}}]},"finish_reason":null}]}` + "\n"))
			_, _ = w.Write([]byte(`data: {"id":"chatcmpl_stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n"))
			_, _ = w.Write([]byte(`data: {"id":"chatcmpl_stream_1","object":"chat.completion.chunk","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}` + "\n"))
			_, _ = w.Write([]byte("data: [DONE]\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if got := gjson.GetBytes(body, "messages.1.reasoning_content").String(); got != "synthetic stream reasoning" {
			http.Error(w, "missing patched stream reasoning", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl_2","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{{Name: "deepseek", BaseURL: server.URL + "/v1"}},
	})
	auth := &cliproxyauth.Auth{ID: "auth-stream", Provider: "openai-compatibility", Attributes: map[string]string{
		"compat_name": "deepseek",
		"base_url":    server.URL + "/v1",
		"api_key":     "synthetic-key",
	}}
	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       true,
		Metadata:     map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "session-stream"},
	}
	streamPayload := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"edit file"}],"stream":true}`)
	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: streamPayload}, opts)
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	var emitted strings.Builder
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("unexpected stream error: %v", chunk.Err)
		}
		emitted.Write(chunk.Payload)
	}
	if !strings.Contains(emitted.String(), "synthetic ") || !strings.Contains(emitted.String(), "stream reasoning") || !strings.Contains(emitted.String(), `"usage":{"prompt_tokens":1`) {
		t.Fatalf("stream output did not preserve upstream chunks: %s", emitted.String())
	}

	replayPayload := []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"edit file"},{"role":"assistant","content":"visible","tool_calls":[{"id":"call-stream-1","type":"function","function":{"name":"edit","arguments":"arg-done"}}]},{"role":"tool","tool_call_id":"call-stream-1","content":"ok"},{"role":"user","content":"continue"}]}`)
	if _, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: replayPayload}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Metadata:     map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "session-stream"},
	}); err != nil {
		t.Fatalf("replay Execute error: %v", err)
	}

	mu.Lock()
	if len(upstreamBodies) != 2 {
		t.Fatalf("upstream request count = %d, want 2", len(upstreamBodies))
	}
	secondBody := append([]byte(nil), upstreamBodies[1]...)
	mu.Unlock()
	if got := gjson.GetBytes(secondBody, "messages.1.reasoning_content").String(); got != "synthetic stream reasoning" {
		t.Fatalf("sent upstream reasoning = %q, want captured stream reasoning; body=%s", got, secondBody)
	}
}

func TestDeepSeekOpenAICompatStreamCaptureObservationOnlyAndErrorsDoNotCommit(t *testing.T) {
	oldCache := defaultDeepSeekReasoningCache
	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(time.Minute, 16)
	t.Cleanup(func() { defaultDeepSeekReasoningCache = oldCache })

	sse := []byte(`data: {"id":"chatcmpl_stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"synthetic","tool_calls":[{"index":0,"id":"call-observe","function":{"name":"edit","arguments":"{}"}}]},"finish_reason":null}]}` + "\n" +
		`data: {"id":"chatcmpl_stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n")

	run := func(t *testing.T, provider string, auth *cliproxyauth.Auth) string {
		t.Helper()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write(sse)
		}))
		defer server.Close()
		auth.Attributes["base_url"] = server.URL + "/v1"
		executor := NewOpenAICompatExecutor(provider, &config.Config{
			OpenAICompatibility: []config.OpenAICompatibility{{Name: "deepseek", BaseURL: server.URL + "/v1"}},
		})
		result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
			Model:   "deepseek-chat",
			Payload: []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"stream":true}`),
		}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Stream: true})
		if err != nil {
			t.Fatalf("ExecuteStream error: %v", err)
		}
		var got strings.Builder
		for chunk := range result.Chunks {
			if chunk.Err != nil {
				t.Fatalf("unexpected stream error: %v", chunk.Err)
			}
			got.Write(chunk.Payload)
		}
		return got.String()
	}

	deepSeekOut := run(t, "openai-compatibility", &cliproxyauth.Auth{ID: "auth-a", Provider: "openai-compatibility", Attributes: map[string]string{
		"compat_name": "deepseek",
		"api_key":     "synthetic-key",
	}})
	nonDeepSeekOut := run(t, "openai-compatibility", &cliproxyauth.Auth{ID: "auth-b", Provider: "openai-compatibility", Attributes: map[string]string{
		"compat_name": "openrouter",
		"api_key":     "synthetic-key",
	}})
	if deepSeekOut != nonDeepSeekOut {
		t.Fatalf("capture changed emitted stream chunks\nDeepSeek: %s\nNonDeepSeek: %s", deepSeekOut, nonDeepSeekOut)
	}
	if !strings.Contains(deepSeekOut, "call-observe") {
		t.Fatalf("clean EOF stream output missing upstream tool call chunk: %s", deepSeekOut)
	}

	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(time.Minute, 16)
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(sse)
		_, _ = w.Write([]byte(`{"error":{"message":"upstream failed"}}` + "\n"))
	}))
	defer errorServer.Close()
	executor := NewOpenAICompatExecutor("deepseek", &config.Config{})
	auth := &cliproxyauth.Auth{ID: "auth-error", Provider: "deepseek", Attributes: map[string]string{
		"base_url": errorServer.URL + "/v1",
		"api_key":  "synthetic-key",
	}}
	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "deepseek-chat",
		Payload: []byte(`{"model":"deepseek-chat","messages":[{"role":"user","content":"hi"}],"stream":true}`),
	}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Stream: true})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	var gotErr error
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			gotErr = chunk.Err
			break
		}
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "upstream failed") {
		t.Fatalf("stream error = %v, want upstream failed", gotErr)
	}
	if got := defaultDeepSeekReasoningCache.Len(); got != 0 {
		t.Fatalf("error stream populated cache, Len=%d", got)
	}
}

func TestOpenAICompatNonDeepSeekDeepSeekModelPassThroughWithPrepopulatedCache(t *testing.T) {
	oldCache := defaultDeepSeekReasoningCache
	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(time.Minute, 16)
	t.Cleanup(func() { defaultDeepSeekReasoningCache = oldCache })

	deepSeekIdentity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	if !defaultDeepSeekReasoningCache.Put(deepSeekIdentity, deepSeekReasoningTurn{Reasoning: "cached reasoning must not leak", ToolCallIDs: []string{"call-nondeepseek"}}) {
		t.Fatal("expected prepopulated DeepSeek cache entry")
	}

	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = append([]byte(nil), body...)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"upstream aggregator reasoning is preserved","tool_calls":[{"id":"call-response","type":"function","function":{"name":"edit","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctxWithGin := context.WithValue(context.Background(), "gin", ginCtx)
	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{
		SDKConfig:           config.SDKConfig{RequestLog: true},
		OpenAICompatibility: []config.OpenAICompatibility{{Name: "openrouter", BaseURL: server.URL + "/v1"}},
	})
	auth := &cliproxyauth.Auth{ID: "auth-a", Provider: "openai-compatibility", Attributes: map[string]string{
		"compat_name": "openrouter",
		"base_url":    server.URL + "/v1",
		"api_key":     "synthetic-key",
	}}
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call-nondeepseek","type":"function","function":{"name":"edit","arguments":"{}"}}]},{"role":"user","content":"continue"}]}`)
	resp, err := executor.Execute(ctxWithGin, auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: payload}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gjson.GetBytes(gotBody, "messages.0.reasoning_content").Exists() {
		t.Fatalf("non-DeepSeek request was patched: %s", gotBody)
	}
	if !strings.Contains(string(resp.Payload), "upstream aggregator reasoning is preserved") {
		t.Fatalf("non-DeepSeek response reasoning was not passed through: %s", resp.Payload)
	}
	logValue, ok := ginCtx.Get("API_REQUEST")
	if !ok {
		t.Fatal("expected request log body")
	}
	if strings.Contains(string(logValue.([]byte)), "cached reasoning must not leak") {
		t.Fatalf("non-DeepSeek request log leaked cached reasoning: %s", logValue.([]byte))
	}
	if got := defaultDeepSeekReasoningCache.Len(); got != 1 {
		t.Fatalf("non-DeepSeek response capture changed cache Len=%d, want prepopulated entry only", got)
	}
}

func TestOpenAICompatNonDeepSeekStreamPassThroughWithPrepopulatedCache(t *testing.T) {
	oldCache := defaultDeepSeekReasoningCache
	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(time.Minute, 16)
	t.Cleanup(func() { defaultDeepSeekReasoningCache = oldCache })

	deepSeekIdentity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	if !defaultDeepSeekReasoningCache.Put(deepSeekIdentity, deepSeekReasoningTurn{Reasoning: "cached stream reasoning must not leak", ToolCallIDs: []string{"call-stream-nondeepseek"}}) {
		t.Fatal("expected prepopulated DeepSeek cache entry")
	}

	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = append([]byte(nil), body...)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"id":"chatcmpl_stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"nondeepseek stream reasoning","tool_calls":[{"index":0,"id":"call-upstream","function":{"name":"edit","arguments":"{}"}}]},"finish_reason":null}]}` + "\n"))
		_, _ = w.Write([]byte("data: [DONE]\n"))
	}))
	defer server.Close()

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{
		OpenAICompatibility: []config.OpenAICompatibility{{Name: "openrouter", BaseURL: server.URL + "/v1"}},
	})
	auth := &cliproxyauth.Auth{ID: "auth-a", Provider: "openai-compatibility", Attributes: map[string]string{
		"compat_name": "openrouter",
		"base_url":    server.URL + "/v1",
		"api_key":     "synthetic-key",
	}}
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call-stream-nondeepseek","type":"function","function":{"name":"edit","arguments":"{}"}}]},{"role":"user","content":"continue"}],"stream":true}`)
	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: payload}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Stream: true})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	var emitted strings.Builder
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		emitted.Write(chunk.Payload)
	}
	if gjson.GetBytes(gotBody, "messages.0.reasoning_content").Exists() {
		t.Fatalf("non-DeepSeek stream request was patched: %s", gotBody)
	}
	if !strings.Contains(emitted.String(), "nondeepseek stream reasoning") {
		t.Fatalf("non-DeepSeek stream output was not passed through: %s", emitted.String())
	}
	if got := defaultDeepSeekReasoningCache.Len(); got != 1 {
		t.Fatalf("non-DeepSeek stream capture changed cache Len=%d, want prepopulated entry only", got)
	}
}

func TestOpenAICompatCountTokensDoesNotUseDeepSeekCacheOrPatchPayload(t *testing.T) {
	oldCache := defaultDeepSeekReasoningCache
	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(time.Minute, 16)
	t.Cleanup(func() { defaultDeepSeekReasoningCache = oldCache })

	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-a", Model: "deepseek-chat"}
	if !defaultDeepSeekReasoningCache.Put(identity, deepSeekReasoningTurn{Reasoning: "count token cached reasoning must not be used", ToolCallIDs: []string{"call-count"}}) {
		t.Fatal("expected prepopulated DeepSeek cache entry")
	}

	executor := NewOpenAICompatExecutor("openai-compatibility", &config.Config{})
	auth := &cliproxyauth.Auth{ID: "auth-a", Provider: "openai-compatibility", Attributes: map[string]string{
		"compat_name": "openrouter",
		"base_url":    "https://openrouter.ai/api/v1",
		"api_key":     "synthetic-key",
	}}
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call-count","type":"function","function":{"name":"edit","arguments":"{}"}}]},{"role":"user","content":"count this"}]}`)
	resp, err := executor.CountTokens(context.Background(), auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: payload}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")})
	if err != nil {
		t.Fatalf("CountTokens error: %v", err)
	}
	if !gjson.GetBytes(resp.Payload, "usage.prompt_tokens").Exists() {
		t.Fatalf("CountTokens payload missing usage: %s", resp.Payload)
	}
	if strings.Contains(string(resp.Payload), "count token cached reasoning must not be used") || strings.Contains(string(resp.Payload), "reasoning_content") {
		t.Fatalf("CountTokens response exposed cached reasoning or patched payload: %s", resp.Payload)
	}
	if got := defaultDeepSeekReasoningCache.Len(); got != 1 {
		t.Fatalf("CountTokens changed cache Len=%d, want prepopulated entry only", got)
	}
}

func TestOpenAICompatDeepSeekPatchedReasoningAbsentFromDefaultLogs(t *testing.T) {
	oldCache := defaultDeepSeekReasoningCache
	defaultDeepSeekReasoningCache = newDeepSeekReasoningCache(time.Minute, 16)
	t.Cleanup(func() { defaultDeepSeekReasoningCache = oldCache })

	secretReasoning := "synthetic-secret-reasoning-sentinel"
	secretAPIKey := "sk-synthetic-secret-sentinel"
	identity := deepSeekCompatIdentity{Enabled: true, Provider: "deepseek", AuthScope: "id:auth-secret", Model: "deepseek-chat"}
	if !defaultDeepSeekReasoningCache.Put(identity, deepSeekReasoningTurn{Reasoning: secretReasoning, ToolCallIDs: []string{"call-secret"}}) {
		t.Fatal("expected prepopulated DeepSeek cache entry")
	}

	var logBuffer bytes.Buffer
	oldOut := log.StandardLogger().Out
	oldLevel := log.GetLevel()
	log.SetOutput(&logBuffer)
	log.SetLevel(log.DebugLevel)
	t.Cleanup(func() {
		log.SetOutput(oldOut)
		log.SetLevel(oldLevel)
	})

	var sentBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sentBody = append([]byte(nil), body...)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctxWithGin := context.WithValue(context.Background(), "gin", ginCtx)
	executor := NewOpenAICompatExecutor("deepseek", &config.Config{SDKConfig: config.SDKConfig{RequestLog: false}})
	auth := &cliproxyauth.Auth{ID: "auth-secret", Provider: "deepseek", Attributes: map[string]string{
		"base_url": server.URL + "/v1",
		"api_key":  secretAPIKey,
	}}
	payload := []byte(`{"model":"deepseek-chat","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call-secret","type":"function","function":{"name":"edit","arguments":"{}"}}]},{"role":"user","content":"continue"}]}`)
	if _, err := executor.Execute(ctxWithGin, auth, cliproxyexecutor.Request{Model: "deepseek-chat", Payload: payload}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")}); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := gjson.GetBytes(sentBody, "messages.0.reasoning_content").String(); got != secretReasoning {
		t.Fatalf("expected sent upstream body to be patched, got %q body=%s", got, sentBody)
	}
	if _, ok := ginCtx.Get("API_REQUEST"); ok {
		t.Fatal("default request logging disabled but API_REQUEST was stored")
	}
	logText := logBuffer.String()
	for _, forbidden := range []string{secretReasoning, secretAPIKey, "Authorization"} {
		if strings.Contains(logText, forbidden) {
			t.Fatalf("default logs exposed forbidden sentinel %q: %s", forbidden, logText)
		}
	}
}
