package executor

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestXAIOAuthExecutorExecuteSendsBearerResponsesRequest(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotAuth string
	var gotOriginator string
	var gotAccountID string
	var gotUserAgent string
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotOriginator = r.Header.Get("Originator")
		gotAccountID = r.Header.Get("Chatgpt-Account-Id")
		gotUserAgent = r.Header.Get("User-Agent")
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1775555723,\"status\":\"completed\",\"model\":\"grok-4.3\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n"))
	}))
	defer server.Close()

	executor := NewXAIOAuthExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		ID:       "xai-test",
		Provider: "xai-oauth",
		Metadata: map[string]any{
			"access_token": "token-1",
			"base_url":     server.URL,
			"type":         "xai-oauth",
		},
	}

	resp, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "grok-4.3",
		Payload: []byte(`{"model":"grok-4.3","messages":[{"role":"user","content":"Say hello"}]}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai"),
		Stream:       false,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if gotPath != "/responses" {
		t.Fatalf("path = %q, want /responses", gotPath)
	}
	if gotAuth != "Bearer token-1" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotOriginator != "" {
		t.Fatalf("Originator header should not be sent, got %q", gotOriginator)
	}
	if gotAccountID != "" {
		t.Fatalf("Chatgpt-Account-Id header should not be sent, got %q", gotAccountID)
	}
	if gotUserAgent != xaiOAuthUserAgent {
		t.Fatalf("User-Agent = %q, want %q", gotUserAgent, xaiOAuthUserAgent)
	}
	if !gjson.GetBytes(gotBody, "stream").Bool() {
		t.Fatalf("stream = false, want true; body=%s", string(gotBody))
	}
	if gjson.GetBytes(gotBody, "include").Exists() {
		t.Fatalf("include should be stripped for xAI OAuth; body=%s", string(gotBody))
	}
	if gjson.GetBytes(gotBody, "reasoning.effort").Exists() {
		t.Fatalf("reasoning.effort should be stripped for non-reasoning xAI model; body=%s", string(gotBody))
	}

	gotContent := gjson.GetBytes(resp.Payload, "choices.0.message.content").String()
	if gotContent != "hello" {
		t.Fatalf("choices.0.message.content = %q, want hello; payload=%s", gotContent, string(resp.Payload))
	}
}

func TestPrepareXAIOAuthResponsesBodyFiltersUnsupportedCustomTools(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "grok-4.3",
		"stream": false,
		"tools": [
			{"type": "function", "name": "Read", "parameters": {"type": "object"}},
			{"type": "custom", "name": "ApplyPatch", "format": {"type": "grammar", "syntax": "lark", "definition": "start: /a/"}}
		],
		"tool_choice": {
			"type": "allowed_tools",
			"tools": [
				{"type": "function", "name": "Read"},
				{"type": "custom", "name": "ApplyPatch"}
			]
		},
		"input": [
			{"type": "custom_tool_call", "call_id": "call-custom", "name": "ApplyPatch", "input": "*** Begin Patch"},
			{"type": "custom_tool_call_output", "call_id": "call-custom", "output": "ok"},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "hello"}]}
		]
	}`)

	out := prepareXAIOAuthResponsesBody(body, "grok-4.3", true)

	if got := gjson.GetBytes(out, "tools.#").Int(); got != 1 {
		t.Fatalf("tools count = %d, want 1; body=%s", got, string(out))
	}
	if got := gjson.GetBytes(out, "tools.0.type").String(); got != "function" {
		t.Fatalf("tools.0.type = %q, want function; body=%s", got, string(out))
	}
	if gjson.GetBytes(out, `tools.#(type=="custom")`).Exists() {
		t.Fatalf("custom tool should be filtered; body=%s", string(out))
	}
	if got := gjson.GetBytes(out, "tool_choice.tools.#").Int(); got != 1 {
		t.Fatalf("tool_choice.tools count = %d, want 1; body=%s", got, string(out))
	}
	if got := gjson.GetBytes(out, "tool_choice.tools.0.type").String(); got != "function" {
		t.Fatalf("tool_choice.tools.0.type = %q, want function; body=%s", got, string(out))
	}
	if got := gjson.GetBytes(out, "input.#").Int(); got != 1 {
		t.Fatalf("input count = %d, want 1; body=%s", got, string(out))
	}
	if got := gjson.GetBytes(out, "input.0.type").String(); got != "message" {
		t.Fatalf("input.0.type = %q, want message; body=%s", got, string(out))
	}
}

func TestPrepareXAIOAuthResponsesBodyDeletesOnlyUnsupportedToolChoice(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"model": "grok-4.3",
		"tools": [
			{"type": "custom", "name": "ApplyPatch", "format": {"type": "grammar", "syntax": "lark", "definition": "start: /a/"}}
		],
		"tool_choice": {"type": "custom", "name": "ApplyPatch"},
		"input": "hello"
	}`)

	out := prepareXAIOAuthResponsesBody(body, "grok-4.3", false)

	if gjson.GetBytes(out, "tools").Exists() {
		t.Fatalf("tools should be deleted when all entries are unsupported; body=%s", string(out))
	}
	if gjson.GetBytes(out, "tool_choice").Exists() {
		t.Fatalf("tool_choice should be deleted when it targets an unsupported tool; body=%s", string(out))
	}
}

func TestXAIOAuthExecutorExecuteStreamTranslatesSSE(t *testing.T) {
	t.Parallel()

	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]},\"output_index\":0}\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":1775555723,\"status\":\"completed\",\"model\":\"grok-4.3\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1,\"total_tokens\":2}}}\n\n"))
	}))
	defer server.Close()

	executor := NewXAIOAuthExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "xai-oauth",
		Metadata: map[string]any{
			"access_token": "token-2",
			"base_url":     server.URL,
		},
	}

	result, err := executor.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "grok-4.3",
		Payload: []byte(`{"model":"grok-4.3","input":"Say ok"}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       true,
	})
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	if gotAuth != "Bearer token-2" {
		t.Fatalf("Authorization = %q", gotAuth)
	}

	var completed []byte
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error: %v", chunk.Err)
		}
		payload := bytes.TrimSpace(chunk.Payload)
		if !bytes.HasPrefix(payload, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(payload[5:])
		if gjson.GetBytes(data, "type").String() == "response.completed" {
			completed = append([]byte(nil), data...)
		}
	}
	if len(completed) == 0 {
		t.Fatal("missing response.completed chunk")
	}
	if got := gjson.GetBytes(completed, "response.output.0.content.0.text").String(); got != "ok" {
		t.Fatalf("completed output text = %q, want ok; completed=%s", got, string(completed))
	}
}

func TestXAIOAuthExecutorExecuteReturnsUnauthorizedStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"expired token"}}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	executor := NewXAIOAuthExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{
		Provider: "xai-oauth",
		Metadata: map[string]any{
			"access_token": "expired",
			"base_url":     server.URL,
		},
	}

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "grok-4.3",
		Payload: []byte(`{"model":"grok-4.3","input":"hi"}`),
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
	})
	if err == nil {
		t.Fatal("Execute succeeded, want error")
	}
	statusErr, ok := err.(interface{ StatusCode() int })
	if !ok {
		t.Fatalf("error does not expose StatusCode: %T", err)
	}
	if got := statusErr.StatusCode(); got != http.StatusUnauthorized {
		t.Fatalf("StatusCode = %d, want %d", got, http.StatusUnauthorized)
	}
}
