package executor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestZAIExecutorExecutePostsChatCompletions(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotContentType string
	var gotCustomHeader string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotCustomHeader = r.Header.Get("X-ZAI-Test")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if errClose := r.Body.Close(); errClose != nil {
			t.Fatalf("close request body: %v", errClose)
		}
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-test","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	exec := NewZAIExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID:       "zai-auth",
		Provider: "zai",
		Attributes: map[string]string{
			"api_key":           "zai-key",
			"base_url":          srv.URL,
			"header:X-ZAI-Test": "custom-value",
		},
	}
	req := cliproxyexecutor.Request{
		Model:   "glm-5.1",
		Payload: []byte(`{"model":"glm-5.1","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}}],"tool_choice":{"type":"function","function":{"name":"lookup"}}}`),
	}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")}

	resp, err := exec.Execute(context.Background(), auth, req, opts)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(resp.Payload) == 0 {
		t.Fatal("expected response payload")
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer zai-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Fatalf("Content-Type = %q", gotContentType)
	}
	if gotCustomHeader != "custom-value" {
		t.Fatalf("X-ZAI-Test = %q", gotCustomHeader)
	}
	if got := gjson.GetBytes(gotBody, "thinking.type").String(); got != "enabled" {
		t.Fatalf("thinking.type = %q; body=%s", got, gotBody)
	}
	if gjson.GetBytes(gotBody, "reasoning_effort").Exists() {
		t.Fatalf("expected reasoning_effort removed, body=%s", gotBody)
	}
	if got := gjson.GetBytes(gotBody, "tools.0.function.name").String(); got != "lookup" {
		t.Fatalf("tools were not preserved, got %q; body=%s", got, gotBody)
	}
	if got := gjson.GetBytes(gotBody, "tool_choice.function.name").String(); got != "lookup" {
		t.Fatalf("tool_choice was not preserved, got %q; body=%s", got, gotBody)
	}
}

func TestZAIExecutorExecuteStreamHandlesSSEAndUsage(t *testing.T) {
	var gotPath string
	var gotAccept string
	var gotAuth string
	var gotCustomHeader string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")
		gotCustomHeader = r.Header.Get("X-ZAI-Stream")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-stream\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"ok\"},\"finish_reason\":null}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-stream\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	exec := NewZAIExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Provider: "zai", Attributes: map[string]string{"api_key": "zai-stream-key", "base_url": srv.URL, "header:X-ZAI-Stream": "stream-custom"}}
	req := cliproxyexecutor.Request{Model: "glm-5.1", Payload: []byte(`{"model":"glm-5.1","messages":[{"role":"user","content":"hi"}],"stream":true,"reasoning_effort":"none"}`)}
	result, err := exec.ExecuteStream(context.Background(), auth, req, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Stream: true})
	if err != nil {
		t.Fatalf("ExecuteStream returned error: %v", err)
	}
	var payloads []string
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		payloads = append(payloads, string(chunk.Payload))
	}
	joined := strings.Join(payloads, "\n")
	if !strings.Contains(joined, "ok") {
		t.Fatalf("stream payloads missing content: %s", joined)
	}
	if gotPath != "/chat/completions" || gotAccept != "text/event-stream" || gotAuth != "Bearer zai-stream-key" || gotCustomHeader != "stream-custom" {
		t.Fatalf("unexpected stream request path=%q accept=%q auth=%q custom=%q", gotPath, gotAccept, gotAuth, gotCustomHeader)
	}
	if !gjson.GetBytes(gotBody, "stream_options.include_usage").Bool() {
		t.Fatalf("expected stream_options.include_usage true, body=%s", gotBody)
	}
	if got := gjson.GetBytes(gotBody, "thinking.type").String(); got != "disabled" {
		t.Fatalf("thinking.type = %q; body=%s", got, gotBody)
	}
}

func TestZAIExecutorExecuteStreamPropagatesUpstreamErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()
	exec := NewZAIExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Provider: "zai", Attributes: map[string]string{"api_key": "zai-key", "base_url": srv.URL}}
	_, err := exec.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{Model: "glm-5.1", Payload: []byte(`{"model":"glm-5.1","messages":[]}`)}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Stream: true})
	if err == nil {
		t.Fatal("expected upstream error")
	}
	var status interface{ StatusCode() int }
	if !errors.As(err, &status) || status.StatusCode() != http.StatusTooManyRequests {
		t.Fatalf("expected 429 status error, got %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("expected upstream body in error, got %v", err)
	}
}

func TestZAIExecutorRejectsEmptyAPIKeyBeforeDispatch(t *testing.T) {
	t.Parallel()

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Fatalf("empty Z.AI API key must not dispatch upstream; got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	exec := NewZAIExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Provider: "zai", Attributes: map[string]string{"api_key": "   ", "base_url": srv.URL}}
	req := cliproxyexecutor.Request{Model: "glm-5.1", Payload: []byte(`{"model":"glm-5.1","messages":[]}`)}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")}

	if _, err := exec.Execute(context.Background(), auth, req, opts); err == nil || !strings.Contains(err.Error(), "missing zai api key") {
		t.Fatalf("Execute error = %v, want missing api key", err)
	}
	if _, err := exec.ExecuteStream(context.Background(), auth, req, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Stream: true}); err == nil || !strings.Contains(err.Error(), "missing zai api key") {
		t.Fatalf("ExecuteStream error = %v, want missing api key", err)
	}
	httpReq, err := http.NewRequest(http.MethodPost, srv.URL+"/chat/completions", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if _, err = exec.HttpRequest(context.Background(), auth, httpReq); err == nil || !strings.Contains(err.Error(), "missing zai api key") {
		t.Fatalf("HttpRequest error = %v, want missing api key", err)
	}
	if requests != 0 {
		t.Fatalf("expected zero upstream requests, got %d", requests)
	}
}

func TestZAIExecutorCountTokensIsLocalOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("CountTokens must not call upstream; got %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()
	exec := NewZAIExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Provider: "zai", Attributes: map[string]string{"api_key": "zai-key", "base_url": srv.URL}}
	resp, err := exec.CountTokens(context.Background(), auth, cliproxyexecutor.Request{Model: "glm-5.1", Payload: []byte(`{"model":"glm-5.1","messages":[{"role":"user","content":"count these tokens"}],"reasoning_effort":"high"}`)}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")})
	if err != nil {
		t.Fatalf("CountTokens returned error: %v", err)
	}
	if total := gjson.GetBytes(resp.Payload, "usage.total_tokens").Int(); total <= 0 {
		t.Fatalf("expected positive local token count, payload=%s", resp.Payload)
	}
}
