# Providers

Provider-specific facts for the DeepSeek Factory Droid compatibility mission.

---

## DeepSeek via OpenAI-Compatible Config

- DeepSeek must remain configured through `openai-compatibility`; do not add hardcoded built-in DeepSeek catalog defaults.
- Typical base URL for user config is `https://api.deepseek.com` or `https://api.deepseek.com/v1` depending on existing config conventions. URL detection must parse the host and use an explicit allowlist.
- Droid should call the local proxy (`http://127.0.0.1:8318/v1`) with a local proxy API key, not DeepSeek directly.
- The upstream DeepSeek API key belongs only in personal proxy config `openai-compatibility[].api-key-entries[]` or auth storage, never in repo files.
- DeepSeek thinking/tool-call compatibility requires replaying prior assistant tool-call messages with their original `reasoning_content` when Droid omits it.

## Non-DeepSeek OpenAI-Compatible Providers

- Providers such as OpenRouter/custom aggregators must not receive DeepSeek reasoning replay solely because a model name contains `deepseek`.
- Pass-through behavior for requests, responses, streaming chunks, token counting, image/compact paths, logging, usage, and errors must remain unchanged.
