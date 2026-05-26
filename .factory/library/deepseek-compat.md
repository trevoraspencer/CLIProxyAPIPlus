# DeepSeek Compatibility Notes

Worker-facing notes for implementing the DeepSeek reasoning replay shim.

---

## Detection

Enable only when provider/config/base URL clearly identifies DeepSeek. Do not enable solely from model name. Use parsed URL host allowlist; reject lookalikes and path/query/credential matches.

## Request Patching

Patch after translation, thinking, and payload config application, and before request logging/sending. Patch only assistant messages with tool calls and missing `reasoning_content`. Preserve existing `reasoning_content`, including empty string.

## Capture

Non-streaming capture observes raw response bytes before translation and stores only assistant messages that have both non-empty string `reasoning_content` and valid tool calls.

Streaming capture observes raw SSE `data:` JSON before stream translation. Reconstruct by choice index and tool-call index; ignore noise and do not alter emitted chunks or terminal events.

## Cache

Cache is bounded, TTL-based, concurrent-safe, and secret-minimal. Key by DeepSeek provider/auth/final upstream model, tool-call IDs or session-scoped fallback hash, and non-secret execution session scope when present. Disable no-ID fallback if no stable session scope exists.
