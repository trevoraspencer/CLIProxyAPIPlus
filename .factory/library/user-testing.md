# User Testing

Testing surface, resource cost classification, and validation gotchas for the DeepSeek Factory Droid compatibility mission.

## Validation Surface

- Primary automated surface: Go unit/integration tests with synthetic JSON/SSE fixtures and `httptest.Server` upstreams.
- Model-listing surface: in-memory/temp config/auth/registry/API tests, plus optional unauthenticated readiness probe of the user-owned local proxy.
- Real user surface: Droid configured with `generic-chat-completion-api` against `http://127.0.0.1:8318/v1`; this is user-run after the mission because workers must not edit personal config or use real DeepSeek keys.
- No browser UI validation is required.

## Validation Concurrency

- Max concurrent validators for full Go tests: 2.
- Use `go test ./... -p 2 -parallel=4 -count=1 -timeout 10m` for the final full gate.
- Targeted package tests may run normally unless resource usage spikes.
- Rationale: planning observed 8 logical CPUs, 16 GiB RAM, and active Droid/Cursor load; use conservative parallelism.

## Flow Validator Guidance: Automated

- Use only local/mocked validation: Go unit tests, synthetic fixtures, temp configs, static review, and `httptest.Server`.
- Do not start persistent services, bind fixed ports, authenticate to `127.0.0.1:8318`, or call inference endpoints on it.
- Do not call live DeepSeek or other provider endpoints.
- Do not read or write `~/.cli-proxy-api/config.yaml`, `~/.factory/settings.json`, `.env`, or real auth files.
- Optional local proxy readiness probe is unauthenticated only: `GET http://127.0.0.1:8318/v1/models`, expected `401` if running.

## Deferred User Smoke Checklist Requirements

Final handoff must tell the user to:

1. Add a placeholder-shaped DeepSeek `openai-compatibility` provider to personal proxy config, keeping `<LOCAL_PROXY_API_KEY>` separate from `<DEEPSEEK_API_KEY>`.
2. Restart or reload their own proxy on `127.0.0.1:8318`.
3. Verify authenticated local `/v1/models` lists the configured alias such as `deepseek-v4`.
4. Configure Droid `generic-chat-completion-api` with base URL `http://127.0.0.1:8318/v1`, local proxy API key, and alias.
5. Run a simple prompt.
6. Run a file-edit/tool-call prompt in a disposable workspace.
7. Continue the same session after tool results so Droid replays prior assistant tool-call history.
8. Confirm no DeepSeek 400 caused by missing `reasoning_content` appears in proxy logs.
9. Confirm DeepSeek Platform usage increments.
10. Redact keys, auth headers, request/response bodies, hidden reasoning content, and personal config paths if sharing evidence.
