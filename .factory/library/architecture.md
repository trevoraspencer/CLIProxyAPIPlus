# Architecture

DeepSeek reasoning replay belongs in `internal/runtime/executor` OpenAI-compatible executor path; no model-name-only activation.

- `OpenAICompatExecutor.Execute` handles both `/chat/completions` and `/responses/compact`; DeepSeek reasoning replay hooks must remain explicitly gated to the chat-completions endpoint.

- `sdk/cliproxy/auth.Manager` error paths may call `MarkResult` before provider-specific cooldown handling; cooldown changes need Manager-level tests because `MarkResult` mutates model state and `NextRetryAfter`.
