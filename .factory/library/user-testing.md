# User Testing

Real DeepSeek/Droid smoke is user-run and deferred; automated validation uses synthetic fixtures and httptest.

## Validation Concurrency

- Surface: `go-test-api-cli`
  - Max concurrent validators: 1 for commands that run `go test ./...`, server builds, or other broad package compilation.
  - Rationale: the repo's approved full-suite command already constrains concurrency with `-p 2 -parallel=4`; running broad validators in parallel would duplicate compile/test load on a 16 GiB workstation.
- Surface: `evidence-review`
  - Max concurrent validators: 2 for read-heavy validation of existing artifacts, test lists, and final smoke handoff content.
  - Rationale: evidence review is mostly file reads plus narrow commands and can safely run alongside one other light validator without shared-state interference.

## Flow Validator Guidance: go-test-api-cli

- Use `/Users/trevor/code/CLIProxyAPIPlus` as the repository root.
- Do not read, print, or edit personal configs such as `~/.cli-proxy-api/config.yaml`, `~/.factory/settings.json`, auth files, `.env`, or credential stores.
- Do not call DeepSeek or any live upstream provider. Automated validation must use existing synthetic fixtures, `httptest.Server`, temp directories, and in-memory setup only.
- Do not stop, restart, reload, authenticate to, or reconfigure the user-owned proxy on `127.0.0.1:8318`. The only allowed probe is unauthenticated `GET /v1/models`, expecting `401` when the proxy is running.
- Use approved commands from `.factory/services.yaml` and mission `AGENTS.md`; preserve exact commands and exit codes in flow reports.
- Keep `go test ./... -p 2 -parallel=4 -count=1 -timeout 10m` and `go build -o test-output ./cmd/server && rm test-output` serial.
- Confirm `test-output` is removed after build checks.

## Flow Validator Guidance: evidence-review

- Treat `.factory/library/final-validation-smoke-handoff.md`, `.factory/validation/deepseek-droid-compat/scrutiny/synthesis.json`, mission `validation-contract.md`, and test-list output as the primary evidence bundle.
- Verify every assigned `VAL-*` assertion has evidence or, for live Droid/DeepSeek smoke assertions, is explicitly labeled `DEFERRED-USER-SMOKE`.
- Do not create or claim unrelated historical Factory artifacts as DeepSeek validation evidence.
- Flow reports must include `assertions`, `commands`, `frictions`, `blockers`, and `toolsUsed` fields so synthesis can update `validation-state.json`.
