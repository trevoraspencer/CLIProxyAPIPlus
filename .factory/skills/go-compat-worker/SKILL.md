---
name: go-compat-worker
description: Implements and verifies Go compatibility behavior with synthetic tests and strict secret boundaries.
---

# Go Compatibility Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## When to Use This Skill

Use for features in this mission that touch Go auth/model-alias baseline repair, OpenAI-compatible executor DeepSeek compatibility, config-driven model listing, regression tests, validation evidence, or final non-secret smoke handoff guidance.

## Work Procedure

1. Read `mission.md`, mission `AGENTS.md`, `validation-contract.md`, `.factory/services.yaml`, `.factory/library/*.md`, and the assigned feature before editing.
2. Inspect current `git status --short` and `git diff --name-only`. Identify which files are pre-existing/unrelated and do not claim or commit unrelated changes.
3. For behavior changes, practice TDD:
   - Add or tighten focused tests first.
   - When practical, run the focused test and observe the expected failure.
   - Implement the minimal code to pass.
4. Use only synthetic fixtures, `t.TempDir()`, in-memory config/auth data, and `httptest.Server`. Never call DeepSeek or any live provider.
5. Respect hard boundaries:
   - Do not read, edit, print, or depend on `~/.cli-proxy-api/config.yaml`, `~/.factory/settings.json`, `.env`, or real auth files.
   - Do not authenticate to, stop, restart, reload, kill, or reconfigure `127.0.0.1:8318`.
   - Optional unauthenticated `/v1/models` 401 probe only if assigned.
6. For DeepSeek detection/cache work:
   - Use provider/config/base URL identity only; never model-name-only.
   - Use parsed host allowlist for DeepSeek URLs.
   - Treat request `reasoning_content: ""` as present and preserve it.
   - Do not cache empty captured reasoning.
   - Do not store raw API keys, Authorization headers, bearer tokens, auth JSON, personal config, or full request/response bodies in cache keys/values.
   - Include non-secret execution session scope when present; disable no-ID fallback without stable session scope.
7. For streaming work, capture before translation and never alter emitted chunks, terminal markers, clean-EOF behavior, or existing error semantics.
8. For model-listing work, prove config-driven behavior through config synthesis/auth registration/registry/API. Do not hardcode DeepSeek defaults.
9. Run `gofmt -w` on changed Go files, then targeted tests for touched packages. Use `.factory/services.yaml` command names when possible.
10. Before committing, run `git diff --check`, review `git diff` for secrets/reasoning leaks, remove transient artifacts, and commit only files in your feature scope.
11. If full verification is too expensive for a non-final feature, run targeted tests and state exactly what remains for the final validation feature.

## Example Handoff

```json
{
  "salientSummary": "Implemented DeepSeek detection/cache helpers with conservative provider/base-URL gating and bounded session-scoped cache behavior; targeted helper tests and gofmt passed.",
  "whatWasImplemented": "Added table-driven tests for DeepSeek provider/config/base URL detection, model-name-only rejection, cache TTL/eviction, execution-session scoping, no-ID fallback disabling, and secret-minimal key construction. Implemented helper code under internal/runtime/executor without touching translator packages or personal config.",
  "whatWasLeftUndone": "Non-stream executor wiring remains for the next feature.",
  "verification": {
    "commandsRun": [
      {"command": "gofmt -w internal/runtime/executor/deepseek_compat.go internal/runtime/executor/deepseek_compat_test.go", "exitCode": 0, "observation": "Formatted changed Go files."},
      {"command": "go test ./internal/runtime/executor -run 'TestDeepSeek.*Detect|TestDeepSeek.*Cache|TestDeepSeek.*Key' -count=1", "exitCode": 0, "observation": "Detection and cache helper tests passed."},
      {"command": "git diff --check", "exitCode": 0, "observation": "No whitespace errors."}
    ],
    "interactiveChecks": [
      {"action": "Reviewed git diff for secrets and personal config access", "observed": "Only synthetic keys and inert fixture URLs were present; no personal config paths were read."}
    ]
  },
  "tests.added": [
    {"file": "internal/runtime/executor/deepseek_compat_test.go", "cases": [
      {"name": "TestIsDeepSeekCompatProviderRejectsModelNameOnly", "verifies": "OpenRouter with deepseek-looking model does not enable the shim."},
      {"name": "TestDeepSeekReasoningCacheTTLAndScope", "verifies": "Cache entries expire and do not cross auth/model/session boundaries."}
    ]}
  ],
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- Requirements require real DeepSeek credentials, authenticated local proxy calls, or personal config edits.
- A translator-only change appears necessary.
- Existing unrelated worktree changes block safe commits or cannot be isolated.
- Full repository validation fails for an unrelated pre-existing issue that cannot be confidently scoped.
- Mission boundaries would need to change to complete the feature.
