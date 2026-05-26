package auth

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type xaiRetryStore struct {
	mu    sync.Mutex
	saved []*Auth
}

func (s *xaiRetryStore) List(context.Context) ([]*Auth, error) { return nil, nil }

func (s *xaiRetryStore) Save(_ context.Context, auth *Auth) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, auth.Clone())
	return "", nil
}

func (s *xaiRetryStore) Delete(context.Context, string) error { return nil }

func (s *xaiRetryStore) lastAccessToken() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.saved) == 0 {
		return ""
	}
	return xaiAccessToken(s.saved[len(s.saved)-1])
}

type xaiRetryExecutor struct {
	refreshErr error

	mu            sync.Mutex
	executeCalls  int
	streamCalls   int
	refreshCalls  int
	executeTokens []string
	streamTokens  []string
}

func (e *xaiRetryExecutor) Identifier() string { return "xai" }

func (e *xaiRetryExecutor) Execute(_ context.Context, auth *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.executeCalls++
	e.executeTokens = append(e.executeTokens, xaiAccessToken(auth))
	if e.executeCalls == 1 {
		return cliproxyexecutor.Response{}, &Error{HTTPStatus: http.StatusUnauthorized, Message: "expired token"}
	}
	return cliproxyexecutor.Response{Payload: []byte(`{"ok":true}`)}, nil
}

func (e *xaiRetryExecutor) ExecuteStream(_ context.Context, auth *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.streamCalls++
	e.streamTokens = append(e.streamTokens, xaiAccessToken(auth))
	if e.streamCalls == 1 {
		return nil, &Error{HTTPStatus: http.StatusUnauthorized, Message: "expired token"}
	}
	ch := make(chan cliproxyexecutor.StreamChunk, 1)
	ch <- cliproxyexecutor.StreamChunk{Payload: []byte("ok")}
	close(ch)
	return &cliproxyexecutor.StreamResult{Chunks: ch}, nil
}

func (e *xaiRetryExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	e.mu.Lock()
	e.refreshCalls++
	refreshErr := e.refreshErr
	e.mu.Unlock()
	if refreshErr != nil {
		return nil, refreshErr
	}
	updated := auth.Clone()
	if updated.Metadata == nil {
		updated.Metadata = make(map[string]any)
	}
	updated.Metadata["access_token"] = "fresh-token"
	if xaiRefreshToken(updated) == "" {
		updated.Metadata["refresh_token"] = "refresh-token"
	}
	return updated, nil
}

func (e *xaiRetryExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

func (e *xaiRetryExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

func (e *xaiRetryExecutor) snapshot() (executeCalls, streamCalls, refreshCalls int, executeTokens, streamTokens []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.executeCalls, e.streamCalls, e.refreshCalls, append([]string(nil), e.executeTokens...), append([]string(nil), e.streamTokens...)
}

func TestManagerExecute_XAIRefreshesAndRetriesOnceAfter401(t *testing.T) {
	ctx := context.Background()
	store := &xaiRetryStore{}
	exec := &xaiRetryExecutor{}
	manager, auth := newXAIRetryManager(t, store, exec)

	resp, err := manager.Execute(ctx, []string{"xai"}, cliproxyexecutor.Request{
		Model: "grok-4.3",
	}, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(resp.Payload) != `{"ok":true}` {
		t.Fatalf("Execute() payload = %s", string(resp.Payload))
	}

	executeCalls, _, refreshCalls, executeTokens, _ := exec.snapshot()
	if executeCalls != 2 {
		t.Fatalf("Execute calls = %d, want 2", executeCalls)
	}
	if refreshCalls != 1 {
		t.Fatalf("Refresh calls = %d, want 1", refreshCalls)
	}
	if len(executeTokens) != 2 || executeTokens[0] != "expired-token" || executeTokens[1] != "fresh-token" {
		t.Fatalf("Execute tokens = %v, want expired-token then fresh-token", executeTokens)
	}
	updated, ok := manager.GetByID(auth.ID)
	if !ok {
		t.Fatalf("missing auth %q", auth.ID)
	}
	if got := xaiAccessToken(updated); got != "fresh-token" {
		t.Fatalf("manager access token = %q, want fresh-token", got)
	}
	if got := store.lastAccessToken(); got != "fresh-token" {
		t.Fatalf("persisted access token = %q, want fresh-token", got)
	}
}

func TestManagerExecuteStream_XAIRefreshesAndRetriesPreStream401(t *testing.T) {
	ctx := context.Background()
	store := &xaiRetryStore{}
	exec := &xaiRetryExecutor{}
	manager, auth := newXAIRetryManager(t, store, exec)

	result, err := manager.ExecuteStream(ctx, []string{"xai"}, cliproxyexecutor.Request{
		Model: "grok-4.3",
	}, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("ExecuteStream() error = %v", err)
	}
	var payload []byte
	for chunk := range result.Chunks {
		if chunk.Err != nil {
			t.Fatalf("stream chunk error = %v", chunk.Err)
		}
		payload = append(payload, chunk.Payload...)
	}
	if string(payload) != "ok" {
		t.Fatalf("stream payload = %q, want ok", string(payload))
	}

	_, streamCalls, refreshCalls, _, streamTokens := exec.snapshot()
	if streamCalls != 2 {
		t.Fatalf("ExecuteStream calls = %d, want 2", streamCalls)
	}
	if refreshCalls != 1 {
		t.Fatalf("Refresh calls = %d, want 1", refreshCalls)
	}
	if len(streamTokens) != 2 || streamTokens[0] != "expired-token" || streamTokens[1] != "fresh-token" {
		t.Fatalf("ExecuteStream tokens = %v, want expired-token then fresh-token", streamTokens)
	}
	updated, ok := manager.GetByID(auth.ID)
	if !ok {
		t.Fatalf("missing auth %q", auth.ID)
	}
	if got := xaiAccessToken(updated); got != "fresh-token" {
		t.Fatalf("manager access token = %q, want fresh-token", got)
	}
	if got := store.lastAccessToken(); got != "fresh-token" {
		t.Fatalf("persisted access token = %q, want fresh-token", got)
	}
}

func TestManagerExecute_XAIRefreshFailureLeavesOriginal401(t *testing.T) {
	ctx := context.Background()
	exec := &xaiRetryExecutor{refreshErr: errors.New("refresh failed")}
	manager, _ := newXAIRetryManager(t, &xaiRetryStore{}, exec)

	_, err := manager.Execute(ctx, []string{"xai"}, cliproxyexecutor.Request{
		Model: "grok-4.3",
	}, cliproxyexecutor.Options{})
	if err == nil {
		t.Fatal("Execute() succeeded, want original 401 error")
	}
	if got := statusCodeFromError(err); got != http.StatusUnauthorized {
		t.Fatalf("Execute() status = %d, want %d", got, http.StatusUnauthorized)
	}

	executeCalls, _, refreshCalls, _, _ := exec.snapshot()
	if executeCalls != 1 {
		t.Fatalf("Execute calls = %d, want 1", executeCalls)
	}
	if refreshCalls != 1 {
		t.Fatalf("Refresh calls = %d, want 1", refreshCalls)
	}
}

func newXAIRetryManager(t *testing.T, store Store, exec *xaiRetryExecutor) (*Manager, *Auth) {
	t.Helper()

	manager := NewManager(store, &RoundRobinSelector{}, nil)
	manager.RegisterExecutor(exec)
	auth := &Auth{
		ID:       "xai-retry-auth",
		Provider: "xai",
		Metadata: map[string]any{
			"type":          "xai",
			"access_token":  "expired-token",
			"refresh_token": "refresh-token",
		},
	}
	registerSchedulerModels(t, "xai", "grok-4.3", auth.ID)
	if _, err := manager.Register(WithSkipPersist(context.Background()), auth); err != nil {
		t.Fatalf("register auth: %v", err)
	}
	manager.RefreshSchedulerEntry(auth.ID)
	return manager, auth
}
