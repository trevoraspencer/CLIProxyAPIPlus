# Provider Research Notes

Raw planning research summary captured for this mission.

## Z.AI

Docs searched from `https://docs.z.ai/llms.txt`, quick start, devpack overview/quick-start, OpenCode/Droid scenario pages, API reference, streaming, function-calling, thinking, and tools pages. Z.AI Coding Plan uses API-key auth with `Authorization: Bearer <key>`, Coding Plan base URL `https://api.z.ai/api/coding/paas/v4`, and OpenAI-compatible chat completions at `/chat/completions`. Thinking fields use `thinking.type` and response `reasoning_content`.

## VibeProxy

`automazeio/vibeproxy` uses bundled CLIProxyAPIPlus and Swift-side config/key storage. Useful pattern: Z.AI is API-key backed with auth-file/config composition, not OAuth.

## OpenCode Zen/Go

OpenCode docs and models.dev identify Zen base URL `https://opencode.ai/zen/v1` and Go base URL `https://opencode.ai/zen/go/v1`, both using `OPENCODE_API_KEY` and OpenAI-compatible chat completions. This mission intentionally documents/configures them through existing `openai-compatibility` only.
