# CLIProxyAPI Plus

This is the Plus version of [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI), adding support for third-party providers on top of the mainline project.

All third-party provider support is maintained by community contributors; CLIProxyAPI does not provide technical support. Please contact the corresponding community maintainer if you need assistance.

So you can use local or multi-account CLI access with OpenAI(include Responses)/Gemini/Claude-compatible clients and SDKs.

## Overview

- OpenAI/Gemini/Claude compatible API endpoints for CLI models
- OpenAI Codex support (GPT models) via OAuth login
- Claude Code support via OAuth login
- x.ai Grok support via OAuth login
- Amp CLI and IDE extensions support with provider routing
- Streaming, non-streaming, and WebSocket responses where supported
- Function calling/tools support
- Multimodal input support (text and images)
- Multiple accounts with round-robin load balancing (Gemini, OpenAI, Claude)
- Simple CLI authentication flows (Gemini, OpenAI, Claude)
- Generative Language API Key support
- AI Studio Build multi-account load balancing
- Gemini CLI multi-account load balancing
- Claude Code multi-account load balancing
- OpenAI Codex multi-account load balancing
- OpenAI-compatible upstream providers via config (e.g., OpenRouter)
- Reusable Go SDK for embedding the proxy (see `docs/sdk-usage.md`)

## Getting Started

CLIProxyAPI Guides: [https://help.router-for.me/](https://help.router-for.me/)

### Run with Docker

Multi-arch images (`linux/amd64`, `linux/arm64`) are published to Docker Hub and GitHub Container Registry on every release tag.

```sh
# Pull a specific version (recommended)
docker pull kaitranntt/cli-proxy-api-plus:v6.9.45-0

# Or pull the latest published release
docker pull kaitranntt/cli-proxy-api-plus:latest
```

GHCR mirror:

```sh
docker pull ghcr.io/kaitranntt/cli-proxy-api-plus:latest
```

Or use the included `docker-compose.yml` (defaults to the Docker Hub image, builds from source if `CLI_PROXY_IMAGE` is overridden):

```sh
git clone https://github.com/kaitranntt/CLIProxyAPIPlus.git
cd CLIProxyAPIPlus
docker compose up -d
```

Available tags:
- Docker Hub: [`kaitranntt/cli-proxy-api-plus`](https://hub.docker.com/r/kaitranntt/cli-proxy-api-plus)
- GHCR: [`ghcr.io/kaitranntt/cli-proxy-api-plus`](https://github.com/kaitranntt/CLIProxyAPIPlus/pkgs/container/cli-proxy-api-plus)

## Management API

see [MANAGEMENT_API.md](https://help.router-for.me/management/api)

## Local Proxy Client Credentials

CLIProxyAPI separates downstream client authentication from upstream provider credentials:

- Put local proxy client keys in the top-level `api-keys` list in `config.yaml`. Clients use one of these keys when they call the proxy.
- Put upstream provider credentials only in the provider-specific config entry or auth file for that provider.
- Do not put upstream API keys, OAuth tokens, or auth JSON contents in top-level `api-keys`, and do not commit real credentials to the repository.

For OpenAI-compatible clients, use the local proxy `/v1` base URL, for example `http://127.0.0.1:8317/v1`, and send requests to paths such as `/chat/completions` with a local proxy API key.

## Usage Statistics

Since v6.10.0, upstream CLIProxyAPI and [CPAMC](https://github.com/router-for-me/Cli-Proxy-API-Management-Center) no longer ship built-in usage statistics. CLIProxyAPIPlus preserves this workflow with its usage logger and the maintained [CPAMC dashboard fork](https://github.com/kaitranntt/Cli-Proxy-API-Management-Center), which is the default management panel release stream.

If you need a separate external usage service, use:

### [CPA Usage Keeper](https://github.com/Willxup/cpa-usage-keeper)

Standalone persistence and visualization service for CLIProxyAPI, with periodic data sync, SQLite storage, aggregate APIs, and a built-in dashboard for usage and statistics.

### [CLIProxyAPI Usage Dashboard](https://github.com/zhanglunet/cliproxyapi-usage-dashboard)

Local-first usage and quota dashboard for CLIProxyAPI. It collects per-request token usage from the Redis-compatible usage queue into SQLite, visualizes daily and recent-window usage by account and model, and shows Codex 5h/7d quota remaining in a local web UI.

### [CPA-Manager](https://github.com/seakee/CPA-Manager)

Full CLIProxyAPI management center with request-level monitoring and cost estimates. CPA-Manager tracks collected requests by account, model, channel, latency, status, and token usage; estimates cost with editable model prices and one-click LiteLLM price sync; persists events in SQLite; and provides Codex account-pool operations with batch inspection, quota detection, unhealthy account discovery, cleanup suggestions, and one-click execution for day-to-day multi-account maintenance.

## Amp CLI Support

CLIProxyAPI includes integrated support for [Amp CLI](https://ampcode.com) and Amp IDE extensions, enabling you to use your Google/ChatGPT/Claude OAuth subscriptions with Amp's coding tools:

- Provider route aliases for Amp's API patterns (`/api/provider/{provider}/v1...`)
- Management proxy for OAuth authentication and account features
- Smart model fallback with automatic routing
- **Model mapping** to route unavailable models to alternatives (e.g., `claude-opus-4.5` → `claude-sonnet-4`)
- Security-first design with localhost-only management endpoints

When you need the request/response shape of a specific backend family, use the provider-specific paths instead of the merged `/v1/...` endpoints:

- Use `/api/provider/{provider}/v1/messages` for messages-style backends.
- Use `/api/provider/{provider}/v1beta/models/...` for model-scoped generate endpoints.
- Use `/api/provider/{provider}/v1/chat/completions` for chat-completions backends.

These routes help you select the protocol surface, but they do not by themselves guarantee a unique inference executor when the same client-visible model name is reused across multiple backends. Inference routing is still resolved from the request model/alias. For strict backend pinning, use unique aliases, prefixes, or otherwise avoid overlapping client-visible model names.

**→ [Complete Amp CLI Integration Guide](https://help.router-for.me/agent-client/amp-cli.html)**

## x.ai OAuth

CLIProxyAPI supports x.ai Grok accounts through OAuth, not through an x.ai API-key configuration. Start the login flow with:

```sh
cli-proxy-api --xai-oauth-login
```

The login flow opens a browser and stores the resulting OAuth credential under the normal auth storage location. For headless or remote shells, add `--no-browser` and open the printed URL manually. If the default callback port is unavailable, set a provider-specific callback listener with `--oauth-callback-port <port>`, for example:

```sh
cli-proxy-api --xai-oauth-login --no-browser --oauth-callback-port 56121
```

After login, run the proxy normally and point OpenAI-compatible clients at the local proxy endpoint, for example `http://127.0.0.1:8317/v1/chat/completions`, using one of the local proxy `api-keys` from your `config.yaml`. The upstream x.ai OAuth tokens stay in auth storage; do not put x.ai OAuth tokens in `api-keys` or commit them to the repository.

The x.ai OAuth provider/channel name is `xai-oauth`. Built-in x.ai OAuth model metadata includes `grok-4.3`, so clients can request that model directly after an account is available. Use `oauth-model-alias` when you want a different client-visible name, and `oauth-excluded-models` when you want to hide models from listing/routing:

```yaml
oauth-model-alias:
  xai-oauth:
    - name: "grok-4.3"
      alias: "grok"

oauth-excluded-models:
  xai-oauth:
    - "grok-4.20-0309-non-reasoning"
```

Aliases and exclusions apply to OAuth channels such as `xai-oauth`; they do not configure upstream API-key credentials.

## Z.AI GLM Coding Plan API Keys

Z.AI GLM Coding Plan uses first-class API-key auth through `zai-api-key`, not OAuth. Put the Z.AI upstream key in the Z.AI config entry or a Z.AI auth file, keep local proxy client keys in top-level `api-keys`, and never commit real Z.AI keys.

The Coding Plan base URL is `https://api.z.ai/api/coding/paas/v4`. If a Z.AI credential omits `base-url`, CLIProxyAPI uses that endpoint by default. Z.AI requests use the OpenAI chat-completions shape against the Z.AI upstream `/chat/completions` endpoint.

```yaml
api-keys:
  - "local-proxy-client-key"

zai-api-key:
  - api-key: "<ZAI_API_KEY>"
    # base-url may be omitted; this is the default Coding Plan endpoint.
    base-url: "https://api.z.ai/api/coding/paas/v4"
    prefix: "zai"
    models:
      - name: "glm-5.1"
        alias: "glm-5"
```

Clients can call the local proxy with base URL `http://127.0.0.1:8317/v1`, local proxy API key `local-proxy-client-key`, and model `zai/glm-5` when the prefix is enabled. Other GLM Coding Plan model examples include `glm-5.1`, `glm-5`, and `glm-5-turbo`.

Z.AI file-backed auth entries use provider type `zai` and support `api_key` or `api-key`, plus optional `base_url` or `base-url`. The same Coding Plan default is applied when `base-url` is omitted. Use placeholders in examples and keep real auth files outside version control:

```json
{
  "type": "zai",
  "api_key": "<ZAI_API_KEY>",
  "base_url": "https://api.z.ai/api/coding/paas/v4",
  "prefix": "zai",
  "headers": {
    "X-Custom-Header": "example-value"
  },
  "disable-cooling": "false"
}
```

Blank or missing Z.AI API keys are not usable credentials. Optional metadata such as model aliases, excluded models, priority, prefix, proxy URL, custom headers, and disable-cooling settings should match the supported fields in your config or auth-file loader.

## OpenCode Zen and Go via OpenAI Compatibility

OpenCode Zen and OpenCode Go are configured as separate `openai-compatibility` providers, not as built-in OpenCode login or auth-file providers. Put your upstream OpenCode API key only in `api-key-entries`; keep the top-level `api-keys` for clients that call your local proxy.

Use `https://opencode.ai/zen/v1` for Zen and `https://opencode.ai/zen/go/v1` for Go. Both examples use the generic OpenAI-compatible path, which sends bearer-token requests to the upstream `/chat/completions` endpoint; they do not cover OAuth, auth files, `/responses`, WebSocket, or Anthropic `/messages` routes. Keep Go routing distinct from Zen with Go-specific settings such as `name: opencode-go`, `prefix: go`, and aliases like `go-default`.

```yaml
openai-compatibility:
  - name: "opencode-zen"
    prefix: "zen"
    base-url: "https://opencode.ai/zen/v1"
    api-key-entries:
      - api-key: "<OPENCODE_API_KEY>"
    models:
      - name: "zen"
        alias: "zen-default"

  - name: "opencode-go"
    prefix: "go"
    base-url: "https://opencode.ai/zen/go/v1"
    api-key-entries:
      - api-key: "<OPENCODE_API_KEY>"
    models:
      - name: "go"
        alias: "go-default"
```

Clients can then call the local proxy's OpenAI-compatible chat completions endpoint, for example `http://127.0.0.1:8317/v1/chat/completions`, with a local proxy API key from top-level `api-keys` and model `zen/zen-default` or `go/go-default` when the corresponding prefix is enabled.

## SDK Docs

- Usage: [docs/sdk-usage.md](docs/sdk-usage.md)
- Advanced (executors & translators): [docs/sdk-advanced.md](docs/sdk-advanced.md)
- Access: [docs/sdk-access.md](docs/sdk-access.md)
- Watcher: [docs/sdk-watcher.md](docs/sdk-watcher.md)
- Custom Provider Example: `examples/custom-provider`

## Contributing

This project only accepts pull requests that relate to third-party provider support. Any pull requests unrelated to third-party provider support will be rejected.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
