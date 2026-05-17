package executor

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
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
			"api_key":  "zai-key",
			"base_url": srv.URL,
		},
	}
	req := cliproxyexecutor.Request{
		Model:   "glm-5.1",
		Payload: []byte(`{"model":"glm-5.1","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"high"}`),
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
	if got := gjson.GetBytes(gotBody, "thinking.type").String(); got != "enabled" {
		t.Fatalf("thinking.type = %q; body=%s", got, gotBody)
	}
	if gjson.GetBytes(gotBody, "reasoning_effort").Exists() {
		t.Fatalf("expected reasoning_effort removed, body=%s", gotBody)
	}
}
