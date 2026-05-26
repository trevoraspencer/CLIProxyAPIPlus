# Final Validation Evidence and User-Run Smoke Handoff

Feature: `final-validation-smoke-handoff-and-docs`
Milestone: `deepseek-droid-compat`
Date: 2026-05-26

## Scope and Boundaries

- Automated validation used local Go tests, synthetic fixtures, and `httptest` executor coverage only.
- No worker edited, printed, reloaded, stopped, restarted, or authenticated to personal proxy/config state.
- The only user-owned proxy interaction was the allowed unauthenticated readiness probe against `127.0.0.1:8318/v1/models`, which returned `401`.
- Real DeepSeek + Droid smoke remains user-run because workers do not have authority to edit personal config or use live DeepSeek credentials.
- A pre-existing dirty `internal/runtime/executor/codex_executor.go` change reappeared and caused initial executor/model validation builds to fail with `undefined: cliproxyauth.IsAuthBlockedForModel`. It was isolated into local stash `stash@{0}` named `pre-existing-codex-executor-worktree-change-blocking-final-validation` before final validation; this stash is not claimed as mission work.

## Scope Correction: `cleanup-final-validation-artifact-scope-and-rerun`

The original final-validation evidence commit (`b96b0550`) also added historical Factory documentation and validation artifacts for unrelated xAI, Z.AI, OpenCode, auth-docs, and docs-cleanup work. Those files are not DeepSeek final-validation evidence and are not claimed by this milestone.

The cleanup feature intentionally removes these unrelated tracked artifact sets from the current DeepSeek evidence scope:

- `.factory/docs/`
- `.factory/validation/auth-docs-refresh/`
- `.factory/validation/docs-cleanup/`
- `.factory/validation/opencode-go/`
- `.factory/validation/opencode-zen/`
- `.factory/validation/zai-coding-plan/`

The retained `.factory/validation/deepseek-droid-compat/` scrutiny reports are current milestone audit artifacts and must stay available for the scrutiny rerun. The retained `.factory/init.sh`, `.factory/services.yaml`, `.factory/skills/`, `.factory/research/`, and `.factory/library/` files are mission infrastructure or DeepSeek milestone handoff context, not unrelated historical validation evidence.

This file now explicitly records the cleanup so final evidence describes every changed file set in this correction. The completion matrix below remains the final-validation handoff evidence for the DeepSeek milestone; the prior round-1 scrutiny report remains a failing audit record until scrutiny is rerun after this cleanup and the required fixes.

## Exact Validation Commands

| Evidence | Command | Exit | Observation |
|---|---|---:|---|
| E0 | `/Users/trevor/code/CLIProxyAPIPlus/.factory/init.sh` | 0 | Ran mission init; it completed after `go mod download`. |
| E1 | `go test ./... -p 2 -parallel=4 -count=1 -timeout 10m` | 0 | Baseline and final full-repository validation passed across all packages, including `internal/runtime/executor`, `internal/api`, `internal/registry`, `sdk/cliproxy`, and `sdk/cliproxy/auth`. |
| E2 | `go test ./internal/runtime/executor -run 'TestDeepSeek|TestOpenAICompat' -count=1` | 0 | Executor validation passed, including DeepSeek detection/cache/patch/non-stream/stream, non-DeepSeek pass-through, logging, token counting, and OpenAI-compatible regression tests. |
| E3 | `go test ./internal/watcher/synthesizer ./sdk/cliproxy ./internal/registry ./internal/api ./sdk/cliproxy/auth -run 'OpenAICompat|DeepSeek|Model' -count=1` | 0 | Config synthesis, SDK, registry, API `/v1/models`, and auth model-listing tests passed. |
| E4 | `go test ./sdk/cliproxy/auth -run 'TestFillFirstSelectorPick|TestSchedulerPick_FillFirst|TestSchedulerPick_PromotesExpiredCooldownBeforePick|TestManager_SchedulerTracksMarkResultCooldownAndRecovery|TestSessionAffinitySelector|OpenAICompatAliasPool|ResolveModelAliasPoolFromConfigModels|ModelAliasSession|APIKeyModelAlias|OAuthModelAlias|ResolveOAuthUpstreamModel|LookupAPIKeyUpstreamModel|RequestedModelAlias|TimeoutCooldown|CodexTimeout|CodexResponseHeaderTimeout' -count=1` | 0 | Auth selector, scheduler, session-affinity, alias, and timeout cooldown regressions passed. |
| E5 | `go test ./sdk/cliproxy/auth ./sdk/cliproxy ./internal/config -run 'APIKeyModelAlias|OAuthModelAlias|ResolveOAuthUpstreamModel|LookupAPIKeyUpstreamModel|RequestedModelAlias' -count=1` | 0 | API-key/OAuth alias lookup and requested-model alias behavior passed in auth, SDK, and config packages. |
| E6 | `go test -race ./internal/runtime/executor -run 'TestDeepSeek.*Cache|TestDeepSeek.*Concurrent' -count=1` | 0 | Race-enabled DeepSeek cache/concurrency validation passed. |
| E7 | `go test ./internal/runtime/executor -list 'DeepSeek|OpenAICompat'` | 0 | Listed actual `TestDeepSeek...` executor tests, including non-stream Droid replay, stream replay, non-DeepSeek pass-through, CountTokens, and log-sentinel coverage. |
| E8 | `go build -o test-output ./cmd/server && rm test-output` | 0 | Server binary built successfully and the temporary `test-output` artifact was removed. |
| E9 | `curl -sS -o /dev/null -w 'HTTP_STATUS:%{http_code}\n' --max-time 5 http://127.0.0.1:8318/v1/models` | 0 | Allowed unauthenticated local readiness probe returned `HTTP_STATUS:401`, consistent with a running authenticated proxy surface. |
| E10 | `git diff --check` | 0 | Final diff had no whitespace errors. |

## Changed Package to Validation Mapping

The final validation handoff itself is this non-secret `.factory/library/` evidence/handoff file. The later cleanup feature changes this file and removes unrelated historical `.factory/docs/` plus non-DeepSeek `.factory/validation/` artifact directories listed in the scope-correction section above; it does not change Go packages. The Go implementation under validation was already present in the final tree and is covered by:

- `./internal/runtime/executor`: E2, E6, E7, E1.
- `./internal/watcher/synthesizer`, `./sdk/cliproxy`, `./internal/registry`, `./internal/api`, `./sdk/cliproxy/auth`: E3 and E1.
- `./sdk/cliproxy/auth`, `./internal/config`: E4, E5, and E1.
- Server build path `./cmd/server`: E8.

## Assertion-by-Assertion Status Matrix

| Assertion | Status | Evidence |
|---|---|---|
| VAL-BASE-001 | PASS | E4, E5, E1; baseline auth package compiles and auth/alias behavior is revalidated in the final tree. |
| VAL-BASE-002 | PASS | E4 covers fill-first selector, scheduler, cooldown promotion/recovery, and session-affinity selectors. |
| VAL-BASE-003 | PASS | E5 covers API-key/OAuth aliases, upstream lookup, requested model alias behavior, SDK, and config paths. |
| VAL-BASE-004 | PASS | E4 covers OpenAI-compatible alias pool and model alias session behavior. |
| VAL-BASE-005 | PASS | Package mapping above ties relevant Go packages to E1-E8 validation commands. |
| VAL-BASE-006 | PASS | E1 passed with approved `-p 2 -parallel=4` concurrency. |
| VAL-BASE-007 | PASS | E8 built the server and removed `test-output`. |
| VAL-BASE-008 | PASS | Exact commands, exits, and observations are recorded in this file. |
| VAL-BASE-009 | PASS | Pre-existing dirty Codex executor work was isolated in `stash@{0}` and not claimed as mission work. |
| VAL-BASE-010 | PASS | E4/E5/E1 validate current auth helper use; no untracked helper files remain. |
| VAL-DS-001 | PASS | E2/E7 validate DeepSeek behavior localized to OpenAI-compatible executor tests and regressions. |
| VAL-DS-002 | PASS | E2/E7 list and pass conservative provider/base-URL detection tests. |
| VAL-DS-003 | PASS | E2/E7 list and pass host allowlist/lookalike rejection tests. |
| VAL-DS-004 | PASS | E2/E7 include non-DeepSeek model-name-only no-patch coverage. |
| VAL-DS-005 | PASS | E2/E7 include non-DeepSeek prepopulated-cache pass-through coverage. |
| VAL-DS-006 | PASS | E2 includes integrated executor patching after request preparation and before send/log observation. |
| VAL-DS-007 | PASS | E2 includes eligible assistant tool-call-only patching tests. |
| VAL-DS-008 | PASS | E2 covers patch preserving existing target message fields except inserted reasoning. |
| VAL-DS-009 | PASS | E2 covers explicit empty `reasoning_content` preservation and empty captured reasoning exclusion. |
| VAL-DS-010 | PASS | E2 covers cache-miss unchanged behavior. |
| VAL-DS-011 | PASS | E2/E6 cover provider/auth/model/session/tool-call key scoping and concurrency. |
| VAL-DS-012 | PASS | E2 covers final model identity usage in capture/replay keying paths. |
| VAL-DS-013 | PASS | E6 race-enabled cache/concurrency tests passed. |
| VAL-DS-014 | PASS | Code/test review plus E2/E6 show cache stores scoped non-secret keys and replay-required reasoning only. |
| VAL-DS-015 | PASS | E2/E7 include non-stream capture stores eligible assistant tool-call reasoning only. |
| VAL-DS-016 | PASS | E2 includes malformed/ineligible non-stream fixture coverage. |
| VAL-DS-017 | PASS | E2 validates observation-only non-stream capture through executor replay tests. |
| VAL-DS-018 | PASS | E2/E7 include streaming fragment reconstruction and replay patch coverage. |
| VAL-DS-019 | PASS | E2 covers choice/tool-call indexed streaming reconstruction. |
| VAL-DS-020 | PASS | E2 covers clean EOF, malformed data, and SSE termination handling. |
| VAL-DS-021 | PASS | E2 validates terminal stream behavior remains available to the translator path. |
| VAL-DS-022 | PASS | E2 covers streaming output/error pass-through regressions. |
| VAL-DS-023 | PASS | E2/E7 include non-DeepSeek pass-through with DeepSeek-looking model names and prepopulated cache. |
| VAL-DS-024 | PASS | E2/E7 include `TestDeepSeekCountTokensDoesNotPatchOrCache`. |
| VAL-DS-025 | PASS | E2/E7 include default log sentinel test proving no shim-specific reasoning/secret leakage. |
| VAL-MODEL-001 | PASS | E3 validates configured DeepSeek-style alias registration through synthetic active auth. |
| VAL-MODEL-002 | PASS | E3 validates parsed API `/v1/models` response assertions for configured aliases. |
| VAL-MODEL-003 | PASS | E3 validates prefix and force-prefix OpenAI-compatible alias behavior. |
| VAL-MODEL-004 | PASS | E3 validates disabled/empty/credentialless/removed config negatives. |
| VAL-MODEL-005 | PASS | E3 and diff review confirm no hardcoded DeepSeek default alias is required. |
| VAL-MODEL-006 | PASS | E3 validates non-DeepSeek providers with DeepSeek-looking upstream names do not synthesize DeepSeek aliases. |
| VAL-MODEL-007 | PASS | E3 uses synthetic test state; E9 is the only allowed unauthenticated probe to the user-owned proxy. |
| VAL-MODEL-008 | PASS | E3 validates isolated model-listing paths through temp/in-memory synthetic state. |
| VAL-SMOKE-001 | PASS | Placeholder-only local DeepSeek proxy config handoff is included below and separates local proxy from upstream keys. |
| VAL-SMOKE-002 | PASS | User-owned reload and authenticated local `/v1/models` alias check are included below. |
| VAL-SMOKE-003 | PASS | Droid `generic-chat-completion-api` setup against the local proxy is included below. |
| VAL-SMOKE-004 | PASS | Simple prompt, tool/file-edit loop, and same-session follow-up turn are included below. |
| VAL-SMOKE-005 | PASS | Missing-reasoning 400 absence, DeepSeek usage increment, and privacy/redaction checks are included below. |
| VAL-SMOKE-006 | DEFERRED-USER-SMOKE | No live DeepSeek/Droid evidence was supplied; execution remains user-run and is not claimed as automated success. |
| VAL-CROSS-001 | PASS | Matrix covers baseline, DeepSeek capture/patch/streaming, model listing, non-DeepSeek regressions, logging/security, automated tests, build, and smoke handoff. |
| VAL-CROSS-002 | PASS | E4/E5 baseline auth/alias validations and E1 full validation passed after final DeepSeek integration. |
| VAL-CROSS-003 | PASS | Automated DeepSeek validation used synthetic fixtures and `httptest` executor tests only; no live DeepSeek calls were made. |
| VAL-CROSS-004 | PASS | E2/E3 tests are synthetic/local and do not depend on personal config, remote model updates, or authenticated port `8318` state. |
| VAL-CROSS-005 | PASS | E2/E7 include non-streaming and streaming Droid-shaped capture-to-replay executor tests. |
| VAL-CROSS-006 | PASS | E2/E7 include integrated mismatch/non-DeepSeek replay isolation coverage. |
| VAL-CROSS-007 | PASS | E2 validates patched payload logging/sending equivalence and E2/E7 include default-log leak checks. |
| VAL-CROSS-008 | PASS | E2/E7 include paired DeepSeek patch and non-DeepSeek no-patch regression coverage. |
| VAL-CROSS-009 | PASS | This matrix lists every `VAL-*` assertion with status and evidence. |
| VAL-CROSS-010 | PASS | E2, E3, E1, E8, and the checklist below satisfy automated command and deferred smoke gate requirements. |
| VAL-DOC-001 | PASS | This handoff is scoped to non-secret DeepSeek local-proxy/Droid setup and uses placeholders only. |

## User-Run DeepSeek + Droid Smoke Checklist

Use placeholders only. Do not paste real keys, auth headers, hidden reasoning text, request/response bodies, personal config paths, or full config files into shared evidence.

### 1. Add DeepSeek to the Personal Proxy Config

In your personal proxy config, add an OpenAI-compatible DeepSeek provider using separate keys:

```yaml
api-keys:
  - "<LOCAL_PROXY_API_KEY>"

openai-compatibility:
  - name: "deepseek"
    enabled: true
    base-url: "https://api.deepseek.com/v1"
    api-key: "<UPSTREAM_DEEPSEEK_API_KEY>"
    models:
      - name: "deepseek-chat"
        alias: "deepseek-v4"
```

Key boundary:

- `<LOCAL_PROXY_API_KEY>` is only for clients calling the local proxy.
- `<UPSTREAM_DEEPSEEK_API_KEY>` is only for the proxy calling DeepSeek upstream.
- Do not put the upstream DeepSeek key in the top-level `api-keys` list.

### 2. Reload the User-Owned Proxy

Restart or reload your own proxy on `127.0.0.1:8318` after editing personal config. Workers did not and must not perform this step for you.

### 3. Confirm Local `/v1/models` Lists the Alias

Run this against the local proxy, not DeepSeek directly:

```sh
curl -sS \
  -H "Authorization: Bearer <LOCAL_PROXY_API_KEY>" \
  http://127.0.0.1:8318/v1/models
```

Expected parsed JSON shape:

```json
{
  "object": "list",
  "data": [
    {
      "id": "deepseek-v4",
      "object": "model",
      "owned_by": "deepseek"
    }
  ]
}
```

### 4. Configure Droid

Use Droid's `generic-chat-completion-api` provider pointed at the local proxy:

```json
{
  "provider": "generic-chat-completion-api",
  "baseUrl": "http://127.0.0.1:8318/v1",
  "apiKey": "<LOCAL_PROXY_API_KEY>",
  "model": "deepseek-v4"
}
```

Do not configure Droid with the upstream DeepSeek API key and do not point Droid directly at `api.deepseek.com` for this smoke.

### 5. Run the Smoke Flows

1. Simple prompt: ask a basic non-tool question and confirm Droid receives a normal response.
2. Tool/file-edit loop: in a disposable test workspace, ask Droid to create or edit a harmless file, then let Droid call its file/tool workflow.
3. Follow-up turn: continue the same Droid session after tool results and ask for a second small edit or a summary that references the tool result. This forces Droid to resend prior assistant tool-call history and exercises missing-`reasoning_content` replay.

### 6. Confirm Live Outcomes and Privacy

- Confirm no DeepSeek `400` caused by missing `reasoning_content` appears in proxy logs.
- Confirm DeepSeek platform usage increments for the smoke requests.
- If sharing evidence, redact:
  - local proxy API keys,
  - upstream DeepSeek API keys,
  - Authorization headers,
  - request/response bodies,
  - hidden reasoning content,
  - auth JSON contents,
  - personal config paths and full config files.
