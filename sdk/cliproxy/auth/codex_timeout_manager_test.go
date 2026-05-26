package auth

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

type codexManagerTimeoutError struct{}

func (codexManagerTimeoutError) Error() string   { return "codex timeout" }
func (codexManagerTimeoutError) Timeout() bool   { return true }
func (codexManagerTimeoutError) Temporary() bool { return true }

type codexTimeoutManagerExecutor struct {
	provider     string
	timeoutAuths map[string]bool

	mu    sync.Mutex
	calls []string
}

func (e *codexTimeoutManagerExecutor) Identifier() string {
	if e.provider != "" {
		return e.provider
	}
	return "codex"
}

func (e *codexTimeoutManagerExecutor) Execute(_ context.Context, auth *Auth, _ cliproxyexecutor.Request, _ cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	e.mu.Lock()
	e.calls = append(e.calls, auth.ID)
	shouldTimeout := e.timeoutAuths[auth.ID]
	e.mu.Unlock()
	if shouldTimeout {
		return cliproxyexecutor.Response{}, codexManagerTimeoutError{}
	}
	return cliproxyexecutor.Response{Payload: []byte(auth.ID)}, nil
}

func (e *codexTimeoutManagerExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	return nil, nil
}

func (e *codexTimeoutManagerExecutor) Refresh(context.Context, *Auth) (*Auth, error) {
	return nil, nil
}

func (e *codexTimeoutManagerExecutor) CountTokens(ctx context.Context, auth *Auth, req cliproxyexecutor.Request, opts cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return e.Execute(ctx, auth, req, opts)
}

func (e *codexTimeoutManagerExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

func (e *codexTimeoutManagerExecutor) snapshotCalls() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.calls...)
}

func newCodexTimeoutManager(t *testing.T, cfg *internalconfig.Config, executor *codexTimeoutManagerExecutor, model string) (*Manager, *Auth, *Auth) {
	t.Helper()

	manager := NewManager(nil, &FillFirstSelector{}, nil)
	manager.SetRetryConfig(0, 0, 0)
	manager.SetConfig(cfg)
	manager.RegisterExecutor(executor)

	authA := &Auth{ID: t.Name() + "-auth-a", Provider: "codex"}
	authB := &Auth{ID: t.Name() + "-auth-b", Provider: "codex"}
	registerSchedulerModels(t, "codex", model, authA.ID, authB.ID)
	if _, err := manager.Register(WithSkipPersist(context.Background()), authA); err != nil {
		t.Fatalf("register auth A: %v", err)
	}
	if _, err := manager.Register(WithSkipPersist(context.Background()), authB); err != nil {
		t.Fatalf("register auth B: %v", err)
	}
	manager.RefreshSchedulerEntry(authA.ID)
	manager.RefreshSchedulerEntry(authB.ID)
	return manager, authA, authB
}

func TestManagerExecute_CodexTimeoutCooldownSkipsAuthAfterManagerMarkResult(t *testing.T) {
	model := "codex-timeout-manager-model"
	executor := &codexTimeoutManagerExecutor{timeoutAuths: map[string]bool{
		t.Name() + "-auth-a": true,
	}}
	manager, authA, authB := newCodexTimeoutManager(t, &internalconfig.Config{
		SDKConfig: internalconfig.SDKConfig{
			CodexTimeoutRetries:         2,
			CodexTimeoutCooldownSeconds: 60,
		},
	}, executor, model)

	resp, err := manager.Execute(context.Background(), []string{"codex"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(resp.Payload) != authB.ID {
		t.Fatalf("Execute() payload = %q, want %q", string(resp.Payload), authB.ID)
	}
	if got := executor.snapshotCalls(); len(got) != 2 || got[0] != authA.ID || got[1] != authB.ID {
		t.Fatalf("executor calls = %v, want [%s %s]", got, authA.ID, authB.ID)
	}

	updatedA, ok := manager.GetByID(authA.ID)
	if !ok {
		t.Fatalf("missing auth %q", authA.ID)
	}
	state := updatedA.ModelStates[model]
	if state == nil {
		t.Fatalf("missing model state for %q", model)
	}
	if !state.Unavailable {
		t.Fatalf("timed-out auth state Unavailable = false, want true")
	}
	if state.NextRetryAfter.IsZero() || !state.NextRetryAfter.After(time.Now()) {
		t.Fatalf("timed-out auth NextRetryAfter = %v, want future cooldown", state.NextRetryAfter)
	}

	resp, err = manager.Execute(context.Background(), []string{"codex"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if string(resp.Payload) != authB.ID {
		t.Fatalf("second Execute() payload = %q, want %q", string(resp.Payload), authB.ID)
	}
	if got := executor.snapshotCalls(); len(got) != 3 || got[2] != authB.ID {
		t.Fatalf("second Execute() should skip cooled auth; calls = %v, want third call %s", got, authB.ID)
	}
}

func TestManagerExecute_CodexTimeoutCooldownZeroDisablesCooldown(t *testing.T) {
	model := "codex-timeout-cooldown-zero-model"
	executor := &codexTimeoutManagerExecutor{timeoutAuths: map[string]bool{
		t.Name() + "-auth-a": true,
	}}
	manager, authA, _ := newCodexTimeoutManager(t, &internalconfig.Config{
		SDKConfig: internalconfig.SDKConfig{
			CodexTimeoutRetries:         2,
			CodexTimeoutCooldownSeconds: 0,
		},
	}, executor, model)

	if _, err := manager.Execute(context.Background(), []string{"codex"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	updatedA, ok := manager.GetByID(authA.ID)
	if !ok {
		t.Fatalf("missing auth %q", authA.ID)
	}
	state := updatedA.ModelStates[model]
	if state == nil {
		t.Fatalf("missing model state for %q", model)
	}
	if !state.NextRetryAfter.IsZero() {
		t.Fatalf("NextRetryAfter = %v, want zero when codex timeout cooldown is disabled", state.NextRetryAfter)
	}
}

func TestManagerExecute_CodexTimeoutRetriesZeroDisablesFailover(t *testing.T) {
	model := "codex-timeout-retries-zero-model"
	executor := &codexTimeoutManagerExecutor{timeoutAuths: map[string]bool{
		t.Name() + "-auth-a": true,
	}}
	manager, authA, _ := newCodexTimeoutManager(t, &internalconfig.Config{
		SDKConfig: internalconfig.SDKConfig{
			CodexTimeoutRetries:         0,
			CodexTimeoutCooldownSeconds: 60,
		},
	}, executor, model)

	if _, err := manager.Execute(context.Background(), []string{"codex"}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{}); err == nil {
		t.Fatal("Execute() succeeded, want timeout error")
	}
	if got := executor.snapshotCalls(); len(got) != 1 || got[0] != authA.ID {
		t.Fatalf("executor calls = %v, want only %s when timeout retries are disabled", got, authA.ID)
	}
}

func TestManagerExecute_NonCodexTimeoutIgnoresCodexRetryDisable(t *testing.T) {
	model := "non-codex-timeout-model"
	provider := "gemini"
	executor := &codexTimeoutManagerExecutor{
		provider: provider,
		timeoutAuths: map[string]bool{
			t.Name() + "-auth-a": true,
		},
	}

	manager := NewManager(nil, &FillFirstSelector{}, nil)
	manager.SetRetryConfig(0, 0, 0)
	manager.SetConfig(&internalconfig.Config{
		SDKConfig: internalconfig.SDKConfig{
			CodexTimeoutRetries:         0,
			CodexTimeoutCooldownSeconds: 60,
		},
	})
	manager.RegisterExecutor(executor)

	authA := &Auth{ID: t.Name() + "-auth-a", Provider: provider}
	authB := &Auth{ID: t.Name() + "-auth-b", Provider: provider}
	registerSchedulerModels(t, provider, model, authA.ID, authB.ID)
	if _, err := manager.Register(WithSkipPersist(context.Background()), authA); err != nil {
		t.Fatalf("register auth A: %v", err)
	}
	if _, err := manager.Register(WithSkipPersist(context.Background()), authB); err != nil {
		t.Fatalf("register auth B: %v", err)
	}
	manager.RefreshSchedulerEntry(authA.ID)
	manager.RefreshSchedulerEntry(authB.ID)

	resp, err := manager.Execute(context.Background(), []string{provider}, cliproxyexecutor.Request{Model: model}, cliproxyexecutor.Options{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(resp.Payload) != authB.ID {
		t.Fatalf("Execute() payload = %q, want %q", string(resp.Payload), authB.ID)
	}
	if got := executor.snapshotCalls(); len(got) != 2 || got[0] != authA.ID || got[1] != authB.ID {
		t.Fatalf("executor calls = %v, want [%s %s]", got, authA.ID, authB.ID)
	}

	updatedA, ok := manager.GetByID(authA.ID)
	if !ok {
		t.Fatalf("missing auth %q", authA.ID)
	}
	if state := updatedA.ModelStates[model]; state != nil && !state.NextRetryAfter.IsZero() {
		t.Fatalf("non-Codex timeout cooldown = %v, want zero", state.NextRetryAfter)
	}
}
