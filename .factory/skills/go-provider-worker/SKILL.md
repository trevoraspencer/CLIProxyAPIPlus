---
name: go-provider-worker
description: Implements and verifies Go provider/config/runtime behavior with mocked tests.
---

# Go Provider Worker

NOTE: Startup and cleanup are handled by `worker-base`. This skill defines the WORK PROCEDURE.

## Work Procedure

1. Read `mission.md`, mission `AGENTS.md`, `.factory/services.yaml`, `.factory/library/`, and your assigned feature.
2. Preserve mission boundaries: mocked/local validation only, no live provider calls, no fixed ports, no real credentials.
3. Use TDD for behavior changes: add or tighten failing tests first, then implement. For docs/config-only changes, inspect existing patterns first and keep examples placeholder-only.
4. Run targeted checks for touched areas, then run broader checks required by the feature.
5. Before handoff, review `git diff` for secrets/stale references and ensure no generated artifacts remain.
6. If ending successfully after making a commit, the `EndFeatureRun` tool call must include the feature commit hash in the top-level `commitId` field, not inside the handoff text.
7. Do not loop on repeated file reads. If you have read the same file range more than twice without making progress, switch to a wider `Read`, targeted `Grep`, or make the edit. If you still cannot proceed after a few focused inspections, return to orchestrator instead of continuing analysis.

## When to Use This Skill

Use for Go implementation and test features touching provider config, auth synthesis, registry, executors, thinking, SDK routing, or model aliases.


## Mission-Specific Quality Requirements

- Before handoff, verify and record that the feature commit is an ancestor of the active branch (`git merge-base --is-ancestor <commit> HEAD`) unless no new commit was intentionally created; if no new commit exists, state that explicitly and do not report an unrelated previous HEAD as the feature commit.
- For changes touching `sdk/cliproxy/auth` Manager execution loops, cooldowns, retry/failover semantics, or selector interactions, add Manager-level integration tests in addition to helper/config tests.
- For final-validation/evidence features, keep `.factory/` artifacts scoped to the current milestone. Do not commit or claim unrelated historical `.factory/docs` or `.factory/validation` artifacts as part of the current feature.
- When reviewing DeepSeek executor helpers, enforce capture eligibility strictly: assistant-role messages only for non-stream capture, failed stream state for SSE error objects, and no cache commit for incomplete reconstructed tool calls.

## Verification Requirements

- Add tests before implementation for behavior changes.
- Use `httptest` or pure unit tests; never call live Z.AI/OpenCode endpoints.
- Run `gofmt -w` on changed Go files.
- Run targeted `go test -count=1` packages for touched areas.

## Example Handoff

```json
{
  "salientSummary": "Added mocked Z.AI streaming executor coverage and fixed missing default auth-file base URL handling; targeted executor and synthesizer tests pass.",
  "whatWasImplemented": "Implemented local-only tests for Z.AI SSE handling, custom headers, and blank API-key rejection, then adjusted auth-file synthesis to skip unusable Z.AI entries.",
  "whatWasLeftUndone": "",
  "verification": {
    "commandsRun": [
      {"command": "gofmt -w internal/runtime/executor/zai_executor_test.go internal/watcher/synthesizer/file.go", "exitCode": 0, "observation": "Formatted changed Go files."},
      {"command": "go test -count=1 ./internal/runtime/executor ./internal/watcher/synthesizer", "exitCode": 0, "observation": "All targeted tests passed."}
    ],
    "interactiveChecks": [
      {"action": "Reviewed git diff for live endpoints/secrets", "observed": "Only placeholder test keys and static URLs were present."}
    ]
  },
  "tests.added": [
    {"file": "internal/runtime/executor/zai_executor_test.go", "cases": [{"name": "TestZAIExecutorExecuteStream", "verifies": "SSE stream uses local mock, bearer auth, and /chat/completions."}]}
  ],
  "discoveredIssues": []
}
```

## When to Return to Orchestrator

- A requirement needs real provider credentials or live upstream validation.
- Completing the feature requires changing OpenCode from config/docs-only into first-class provider code.
- Existing unrelated test failures block verification and cannot be isolated.
