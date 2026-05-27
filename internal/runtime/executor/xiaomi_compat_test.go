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

func TestXiaomiDetectProviderAndBaseURL(t *testing.T) {
	cfg := &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: " xiaomi "}}}
	exec := NewOpenAICompatExecutor("openai-compatibility", cfg)
	if !exec.xiaomiEnabled(&cliproxyauth.Auth{Provider: "openai-compatibility", Attributes: map[string]string{"compat_name": "Xiaomi"}}, "http://127.0.0.1:1/v1") {
		t.Fatalf("expected Xiaomi config identity to enable Xiaomi traits")
	}
	if !NewOpenAICompatExecutor("custom", nil).xiaomiEnabled(&cliproxyauth.Auth{}, "https://api.xiaomimimo.com/v1") {
		t.Fatalf("expected official Xiaomi host to enable Xiaomi traits")
	}
	if NewOpenAICompatExecutor("openrouter", nil).xiaomiEnabled(&cliproxyauth.Auth{Provider: "openrouter"}, "https://api.xiaomimimo.com.evil.test/v1") {
		t.Fatalf("lookalike Xiaomi host must not enable Xiaomi traits")
	}
}

func TestXiaomiExecuteAppliesThinkingAndNormalizesTools(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		body, errRead := io.ReadAll(r.Body)
		if errRead != nil {
			t.Fatalf("read request body: %v", errRead)
		}
		gotBody = body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-xiaomi","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	cfg := &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{
		Name:             "xiaomi",
		BaseURL:          srv.URL + "/v1",
		WebSearchEnabled: true,
	}}}
	exec := NewOpenAICompatExecutor("xiaomi", cfg)
	auth := &cliproxyauth.Auth{ID: "auth-xiaomi", Provider: "xiaomi", Attributes: map[string]string{
		"compat_name":        "xiaomi",
		"base_url":           srv.URL + "/v1",
		"api_key":            "synthetic-mimo-key",
		"web_search_enabled": "true",
	}}
	payload := []byte(`{
		"model":"mimo-v2.5-pro",
		"messages":[{"role":"user","content":"hi"}],
		"reasoning_effort":"high",
		"reasoning":{"effort":"high"},
		"output_config":{"effort":"high"},
		"tools":[
			{"type":"function","function":{"name":"lookup","parameters":{"type":"object"},"strict":null}},
			{"type":"function","function":{"name":"lookup","parameters":{"type":"object"}}},
			{"type":"web_search","max_keyword":3,"search_context_size":"high"}
		],
		"tool_choice":{"type":"function","function":{"name":"lookup"}}
	}`)

	resp, err := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "mimo-v2.5-pro", Payload: payload}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(resp.Payload) == 0 {
		t.Fatal("expected response payload")
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer synthetic-mimo-key" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if got := gjson.GetBytes(gotBody, "thinking.type").String(); got != "enabled" {
		t.Fatalf("thinking.type = %q, want enabled; body=%s", got, gotBody)
	}
	for _, path := range []string{"reasoning_effort", "reasoning", "output_config"} {
		if gjson.GetBytes(gotBody, path).Exists() {
			t.Fatalf("%s should be removed; body=%s", path, gotBody)
		}
	}
	if got := gjson.GetBytes(gotBody, "tools.#").Int(); got != 2 {
		t.Fatalf("tools count = %d, want 2; body=%s", got, gotBody)
	}
	if gjson.GetBytes(gotBody, "tools.0.function.strict").Exists() {
		t.Fatalf("strict:null should be omitted; body=%s", gotBody)
	}
	if got := gjson.GetBytes(gotBody, "tools.1.type").String(); got != "web_search" {
		t.Fatalf("tools.1.type = %q, want web_search; body=%s", got, gotBody)
	}
	if got := gjson.GetBytes(gotBody, "tools.1.max_keyword").Int(); got != 3 {
		t.Fatalf("web_search max_keyword = %d, want 3; body=%s", got, gotBody)
	}
	if gjson.GetBytes(gotBody, "tools.1.search_context_size").Exists() {
		t.Fatalf("unsupported web_search field should be stripped; body=%s", gotBody)
	}
	if got := gjson.GetBytes(gotBody, "tool_choice").String(); got != "auto" {
		t.Fatalf("tool_choice = %q, want auto; body=%s", got, gotBody)
	}
}

func TestXiaomiNormalizeStripsWebSearchWhenDisabled(t *testing.T) {
	body := normalizeXiaomiChatPayload([]byte(`{"model":"mimo-v2.5-pro","messages":[],"tools":[{"type":"function","function":{"name":"lookup"}},{"type":"web_search","max_keyword":2}],"tool_choice":"auto"}`), nil, false)
	if got := gjson.GetBytes(body, "tools.#").Int(); got != 1 {
		t.Fatalf("tools count = %d, want 1; body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "tools.0.function.name").String(); got != "lookup" {
		t.Fatalf("remaining tool = %q, want lookup; body=%s", got, body)
	}
}

func TestXiaomiNonStreamReasoningCaptureThenReplay(t *testing.T) {
	restore := replaceDefaultXiaomiCacheForTest()
	defer restore()

	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		if len(bodies) == 1 {
			_, _ = w.Write([]byte(`{"id":"chatcmpl_1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":null,"reasoning_content":"full hidden reasoning","tool_calls":[{"id":"call-edit-1","type":"function","function":{"name":"edit_file","arguments":"{\"path\":\"tmp.txt\"}"}}]},"finish_reason":"tool_calls"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"chatcmpl_2","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	exec := NewOpenAICompatExecutor("xiaomi", &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "xiaomi", BaseURL: srv.URL + "/v1"}}})
	auth := &cliproxyauth.Auth{ID: "auth-xiaomi-replay", Provider: "xiaomi", Attributes: map[string]string{"compat_name": "xiaomi", "base_url": srv.URL + "/v1", "api_key": "synthetic-key"}}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Metadata: map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "xiaomi-session-1"}}

	if _, err := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "mimo-v2.5-pro", Payload: []byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"edit a file"}]}`)}, opts); err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if defaultXiaomiReasoningCache.Len() != 1 {
		t.Fatalf("first response did not populate Xiaomi reasoning cache; len=%d", defaultXiaomiReasoningCache.Len())
	}

	secondPayload := []byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"edit a file"},{"role":"assistant","content":null,"reasoning_content":"","tool_calls":[{"id":"call-edit-1","type":"function","function":{"name":"edit_file","arguments":"{\"path\":\"tmp.txt\"}"}}]},{"role":"tool","tool_call_id":"call-edit-1","content":"ok"},{"role":"user","content":"continue"}]}`)
	if _, err := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "mimo-v2.5-pro", Payload: secondPayload}, opts); err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	if got := gjson.GetBytes(bodies[1], "messages.1.reasoning_content").String(); got != "full hidden reasoning" {
		t.Fatalf("second upstream body missing replayed reasoning: got %q body=%s", got, bodies[1])
	}
}

func TestXiaomiStreamReasoningCaptureThenReplay(t *testing.T) {
	restore := replaceDefaultXiaomiCacheForTest()
	defer restore()

	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(bodies) == 1 {
			_, _ = w.Write([]byte(`data: {"id":"stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"stream "}}]}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"id":"stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"reasoning_content":"reasoning","tool_calls":[{"index":0,"id":"call-","type":"function","function":{"name":"edit_","arguments":"{}"}}]},"finish_reason":null}]}` + "\n\n"))
			_, _ = w.Write([]byte(`data: {"id":"stream_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"edit-1","function":{"name":"file","arguments":""}}]},"finish_reason":"tool_calls"}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		if got := gjson.GetBytes(body, "messages.1.reasoning_content").String(); got != "stream reasoning" {
			http.Error(w, "missing replayed reasoning", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`data: {"id":"stream_2","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"done"},"finish_reason":"stop"}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	exec := NewOpenAICompatExecutor("xiaomi", &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "xiaomi", BaseURL: srv.URL + "/v1"}}})
	auth := &cliproxyauth.Auth{ID: "auth-xiaomi-stream", Provider: "xiaomi", Attributes: map[string]string{"compat_name": "xiaomi", "base_url": srv.URL + "/v1", "api_key": "synthetic-key"}}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai"), Metadata: map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "xiaomi-stream-session"}}

	first, err := exec.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{Model: "mimo-v2.5-pro", Payload: []byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"edit"}],"stream":true}`)}, opts)
	if err != nil {
		t.Fatalf("first ExecuteStream returned error: %v", err)
	}
	if joined := strings.Join(collectDeepSeekStreamPayloads(t, first), ""); !strings.Contains(joined, "stream ") || !strings.Contains(joined, "reasoning") {
		t.Fatalf("first stream output changed unexpectedly: %q", joined)
	}
	if defaultXiaomiReasoningCache.Len() != 1 {
		t.Fatalf("stream did not populate Xiaomi reasoning cache; len=%d", defaultXiaomiReasoningCache.Len())
	}

	secondPayload := []byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"edit"},{"role":"assistant","content":null,"tool_calls":[{"id":"call-edit-1","type":"function","function":{"name":"edit_file","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call-edit-1","content":"ok"},{"role":"user","content":"continue"}],"stream":true}`)
	second, err := exec.ExecuteStream(context.Background(), auth, cliproxyexecutor.Request{Model: "mimo-v2.5-pro", Payload: secondPayload}, opts)
	if err != nil {
		t.Fatalf("second ExecuteStream returned error: %v", err)
	}
	if joined := strings.Join(collectDeepSeekStreamPayloads(t, second), ""); !strings.Contains(joined, "done") {
		t.Fatalf("second stream output changed unexpectedly: %q", joined)
	}
}

func TestXiaomiRejectsImageInputForProModel(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	exec := NewOpenAICompatExecutor("xiaomi", &config.Config{OpenAICompatibility: []config.OpenAICompatibility{{Name: "xiaomi", BaseURL: srv.URL + "/v1"}}})
	auth := &cliproxyauth.Auth{ID: "auth-xiaomi-image", Provider: "xiaomi", Attributes: map[string]string{"compat_name": "xiaomi", "base_url": srv.URL + "/v1", "api_key": "synthetic-key"}}
	payload := []byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AAAA"}}]}]}`)

	_, err := exec.Execute(context.Background(), auth, cliproxyexecutor.Request{Model: "mimo-v2.5-pro", Payload: payload}, cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai")})
	if err == nil {
		t.Fatal("expected image input to fail for mimo-v2.5-pro")
	}
	var status interface{ StatusCode() int }
	if !errors.As(err, &status) || status.StatusCode() != http.StatusBadRequest {
		t.Fatalf("error status = %v, want 400; err=%v", status, err)
	}
	if called {
		t.Fatalf("upstream should not be called for rejected Xiaomi multimodal payload")
	}
}
