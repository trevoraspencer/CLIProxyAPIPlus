# Architecture

Architectural decisions, patterns discovered, and implementation notes for the DeepSeek Factory Droid compatibility mission.

---

- The primary implementation path is `internal/runtime/executor/openai_compat_executor.go` with focused helper/test files under `internal/runtime/executor/`.
- Patch DeepSeek requests after `sdktranslator.TranslateRequest(...)`, `thinking.ApplyThinking(...)`, and payload config application, but before `helps.RecordAPIRequest(...)` and upstream send.
- Non-streaming capture must inspect raw upstream response bytes before downstream translation and must preserve those bytes.
- Streaming capture must observe raw SSE `data:` payloads before `sdktranslator.TranslateStream(...)` and must not alter emitted stream chunks or terminal events.
- DeepSeek activation is conservative: configured provider/auth identity or parsed allowlisted DeepSeek base URL only. Model-name-only activation is forbidden.
- URL detection should use a single implementation-owned allowlist constant. At minimum `api.deepseek.com` is allowed; every allowed host must be tested.
- Reasoning cache entries must be bounded, TTL-based, concurrent-safe, and secret-minimal. Include provider/auth/final-upstream-model/tool-call identity and non-secret execution session scope when present.
- If no stable downstream session scope is present, fallback matching without tool-call IDs must be disabled.
- `reasoning_content: ""` in requests is present caller-provided content and must not be overwritten. Empty captured reasoning is not cached.
- `/v1/models` exposure must stay config-driven via `openai-compatibility.models`, config synthesis, auth registration, registry, and OpenAI models handler. Do not hardcode DeepSeek defaults.
- Avoid standalone changes to `internal/translator/`; return to orchestrator if translator-only changes seem necessary.
