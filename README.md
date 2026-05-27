# CLIProxyAPI Plus

This is the Plus version of [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI), adding support for third-party providers on top of the mainline project.

A proxy server that provides OpenAI/Gemini/Claude/Codex/Grok compatible API interfaces for CLI.

It now also supports OpenAI Codex (GPT models) and Claude Code via OAuth.

So you can use local or multi-account CLI access with OpenAI(include Responses)/Gemini/Claude-compatible clients and SDKs.

## Sponsor

[![https://www.packyapi.com/register?aff=cliproxyapi](./assets/packycode-en.png)](https://www.packyapi.com/register?aff=cliproxyapi)

Thanks to PackyCode for sponsoring this project!

PackyCode is a reliable and efficient API relay service provider, offering relay services for Claude Code, Codex, Gemini, and more.

PackyCode provides special discounts for our software users: register using <a href="https://www.packyapi.com/register?aff=cliproxyapi">this link</a> and enter the "cliproxyapi" promo code during recharge to get 10% off.

---

<table>
<tbody>
<tr>
<td width="180"><a href="https://www.aicodemirror.com/register?invitecode=TJNAIF"><img src="./assets/aicodemirror.png" alt="AICodeMirror" width="150"></a></td>
<td>Thanks to AICodeMirror for sponsoring this project! AICodeMirror provides official high-stability relay services for Claude Code / Codex / Gemini CLI, with enterprise-grade concurrency, fast invoicing, and 24/7 dedicated technical support. Claude Code / Codex / Gemini official channels at 38% / 2% / 9% of original price, with extra discounts on top-ups! AICodeMirror offers special benefits for CLIProxyAPI users: register via <a href="https://www.aicodemirror.com/register?invitecode=TJNAIF">this link</a> to enjoy 20% off your first top-up, and enterprise customers can get up to 25% off!</td>
</tr>
<tr>
<td width="180"><a href="https://shop.bmoplus.com/?utm_source=github"><img src="./assets/bmoplus.png" alt="BmoPlus" width="150"></a></td>
<td>Huge thanks to BmoPlus for sponsoring this project! BmoPlus is a highly reliable AI account provider built strictly for heavy AI users and developers. They offer rock-solid, ready-to-use accounts and official top-up services for ChatGPT Plus / ChatGPT Pro (Full Warranty) / Claude Pro / Super Grok / Gemini Pro. By registering and ordering through <a href="https://shop.bmoplus.com/?utm_source=github">BmoPlus - Premium AI Accounts & Top-ups</a>, users can unlock the mind-blowing rate of <b>10% of the official GPT subscription price (90% OFF)</b>!</td>
</tr>
<tr>
<td width="180"><a href="https://coder.visioncoder.cn"><img src="./assets/visioncoder.png" alt="VisionCoder" width="150"></a></td>
<td>Thanks to <b>VisionCoder</b> for supporting this project. <a href="https://coder.visioncoder.cn" target="_blank">VisionCoder Developer Platform</a> is a reliable and efficient API relay service provider, offering access to mainstream AI models such as Claude Code, Codex, and Gemini. It helps developers and teams integrate AI capabilities more easily and improve productivity.
<p></p>
VisionCoder is also offering our users a limited-time <a href="https://coder.visioncoder.cn" target="_blank">Token Plan</a> promotion: <b>buy 1 month and get 1 month free</b>.</td>
</tr>
<tr>
<td width="180"><a href="https://apikey.fun/register?aff=CLIProxyAPI"><img src="./assets/apikey.png" alt="APIKEY.FUN" width="150"></a></td>
<td>Thanks to APIKEY.FUN for sponsoring this project! APIKEY.FUN is a professional enterprise-grade AI relay platform dedicated to providing stable, efficient, and low-cost AI model API access for enterprises and individual developers. The platform supports popular mainstream models such as Claude, OpenAI, and Gemini, with prices as low as 7% of the official price. Register through this project's <a href="https://apikey.fun/register?aff=CLIProxyAPI">exclusive link</a> to enjoy a special <b>permanent 5% top-up discount</b>.</td>
</tr>
</tbody>
</table>

## Overview

- OpenAI/Gemini/Claude/Grok compatible API endpoints for CLI models
- OpenAI Codex support (GPT models) via OAuth login
- Claude Code support via OAuth login
- Grok Build support via OAuth login
- Amp CLI and IDE extensions support with provider routing
- Streaming, non-streaming, and WebSocket responses where supported
- Function calling/tools support
- Multimodal input support (text and images)
- Multiple accounts with round-robin load balancing (Gemini, OpenAI, Claude, Grok)
- Simple CLI authentication flows (Gemini, OpenAI, Claude, Grok)
- Generative Language API Key support
- AI Studio Build multi-account load balancing
- Gemini CLI multi-account load balancing
- Claude Code multi-account load balancing
- OpenAI Codex multi-account load balancing
- Grok Build multi-account load balancing
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

## DeepSeek with Factory Droid

DeepSeek can be configured as an `openai-compatibility` provider and used by Factory Droid through the local proxy's generic OpenAI-compatible endpoint. Keep the top-level `api-keys` value for Droid/local clients, and put the upstream DeepSeek key only under the DeepSeek provider's `api-key-entries`.

```yaml
api-keys:
  - "<LOCAL_PROXY_API_KEY>"

openai-compatibility:
  - name: "deepseek"
    base-url: "https://api.deepseek.com"
    api-key-entries:
      - api-key: "<DEEPSEEK_API_KEY>"
    models:
      - name: "<DEEPSEEK_MODEL_ID>"
        alias: "deepseek-v4"
```

Point Droid at the local proxy, for example `http://127.0.0.1:8318/v1`, with model `deepseek-v4` and the local proxy API key. After reloading the proxy, verify the alias with `GET /v1/models`; do not commit real keys or personal config contents.

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

CLIProxyAPI supports x.ai Grok accounts through OAuth, not through an x.ai API-key configuration. Set `CLIPROXY_XAI_OAUTH_CLIENT_ID` to the public Grok CLI OAuth client ID (this is not a secret). Plus does not ship the value in-repo; copy it from upstream [`internal/auth/xai/types.go`](https://github.com/router-for-me/CLIProxyAPI/blob/main/internal/auth/xai/types.go) (`ClientID` constant). You can export it in your shell or add it to `.env` (see `.env.example`).

Start the login flow with:

```sh
export CLIPROXY_XAI_OAUTH_CLIENT_ID="<grok-cli-oauth-client-id>"
cli-proxy-api --xai-login
```

The login flow opens a browser and stores the resulting OAuth credential under the normal auth storage location. For headless or remote shells, add `--no-browser` and open the printed URL manually. If the default callback port is unavailable, set a provider-specific callback listener with `--oauth-callback-port <port>`, for example:

```sh
cli-proxy-api --xai-login --no-browser --oauth-callback-port 56121
```

After login, run the proxy normally and point OpenAI-compatible clients at the local proxy endpoint, for example `http://127.0.0.1:8317/v1/chat/completions`, using one of the local proxy `api-keys` from your `config.yaml`. The upstream x.ai OAuth tokens stay in auth storage; do not put x.ai OAuth tokens in `api-keys` or commit them to the repository.

**Migration:** Existing `auths/*.xai-oauth.json` files from older Plus builds are no longer recognized. Run `--xai-login` again to create credentials under the `xai` provider.

The x.ai OAuth provider/channel name is `xai`. The registry keeps the full upstream xAI catalog; `config.example.yaml` ships a default `oauth-excluded-models` whitelist that exposes only `grok-build-0.1` and `grok-4.3` until you remove entries. Use `oauth-model-alias` when you want a different client-visible name, and adjust `oauth-excluded-models` when you want more models in listing/routing:

```yaml
oauth-model-alias:
  xai:
    - name: "grok-4.3"
      alias: "grok"

oauth-excluded-models:
  xai:
    - grok-4.20-0309-reasoning
    - grok-4.20-0309-non-reasoning
    - grok-4.20-multi-agent-0309
    - grok-3-mini
    - grok-3-mini-fast
    - grok-imagine-image
    - grok-imagine-image-quality
    - grok-imagine-video
```

Plus preserves two behaviors on top of upstream xAI: an aggressive tool sanitizer for Factory/Droid clients (unsupported tool types and `custom_tool_call` input items are stripped after upstream normalization), and immediate 401 refresh-and-retry in the auth conductor.

Aliases and exclusions apply to OAuth channels such as `xai`; they do not configure upstream API-key credentials.

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

## Who is with us?

Those projects are based on CLIProxyAPI:

### [vibeproxy](https://github.com/automazeio/vibeproxy)

Native macOS menu bar app to use your Claude Code & ChatGPT subscriptions with AI coding tools - no API keys needed

### [Subtitle Translator](https://github.com/VjayC/SRT-Subtitle-Translator-Validator)

A cross-platform desktop and web app to translate and validate SRT subtitles using your existing LLM subscriptions (Gemini, ChatGPT, Claude, etc.) via CLIProxyAPI - no API keys needed.

### [CCS (Claude Code Switch)](https://github.com/kaitranntt/ccs)

CLI wrapper for instant switching between multiple Claude accounts and alternative models (Gemini, Codex, Antigravity) via CLIProxyAPI OAuth - no API keys needed

### [Quotio](https://github.com/nguyenphutrong/quotio)

Native macOS menu bar app that unifies Claude, Gemini, OpenAI, and Antigravity subscriptions with real-time quota tracking and smart auto-failover for AI coding tools like Claude Code, OpenCode, and Droid - no API keys needed.

### [CodMate](https://github.com/loocor/CodMate)

Native macOS SwiftUI app for managing CLI AI sessions (Codex, Claude Code, Gemini CLI) with unified provider management, Git review, project organization, global search, and terminal integration. Integrates CLIProxyAPI to provide OAuth authentication for Codex, Claude, Gemini, and Antigravity, with built-in and third-party provider rerouting through a single proxy endpoint - no API keys needed for OAuth providers.

### [ProxyPilot](https://github.com/Finesssee/ProxyPilot)

Windows-native CLIProxyAPI fork with TUI, system tray, and multi-provider OAuth for AI coding tools - no API keys needed.

### [Claude Proxy VSCode](https://github.com/uzhao/claude-proxy-vscode)

VSCode extension for quick switching between Claude Code models, featuring integrated CLIProxyAPI as its backend with automatic background lifecycle management.

### [ZeroLimit](https://github.com/0xtbug/zero-limit)

Windows desktop app built with Tauri + React for monitoring AI coding assistant quotas via CLIProxyAPI. Track usage across Gemini, Claude, OpenAI Codex, and Antigravity accounts with real-time dashboard, system tray integration, and one-click proxy control - no API keys needed.

### [CPA-XXX Panel](https://github.com/ferretgeek/CPA-X)

A lightweight web admin panel for CLIProxyAPI with health checks, resource monitoring, real-time logs, auto-update, request statistics and pricing display. Supports one-click installation and systemd service.

### [CLIProxyAPI Tray](https://github.com/kitephp/CLIProxyAPI_Tray)

A Windows tray application implemented using PowerShell scripts, without relying on any third-party libraries. The main features include: automatic creation of shortcuts, silent running, password management, channel switching (Main / Plus), and automatic downloading and updating.

### [霖君](https://github.com/wangdabaoqq/LinJun)

霖君 is a cross-platform desktop application for managing AI programming assistants, supporting macOS, Windows, and Linux systems. Unified management of Claude Code, Gemini CLI, OpenAI Codex, and other AI coding tools, with local proxy for multi-account quota tracking and one-click configuration.

### [CLIProxyAPI Dashboard](https://github.com/itsmylife44/cliproxyapi-dashboard)

A modern web-based management dashboard for CLIProxyAPI built with Next.js, React, and PostgreSQL. Features real-time log streaming, structured configuration editing, API key management, OAuth provider integration for Claude/Gemini/Codex, usage analytics, container management, and config sync with OpenCode via companion plugin - no manual YAML editing needed.

### [All API Hub](https://github.com/qixing-jk/all-api-hub)

Browser extension for one-stop management of New API-compatible relay site accounts, featuring balance and usage dashboards, auto check-in, one-click key export to common apps, in-page API availability testing, and channel/model sync and redirection. It integrates with CLIProxyAPI through the Management API for one-click provider import and config sync.

### [Shadow AI](https://github.com/HEUDavid/shadow-ai)

Shadow AI is an AI assistant tool designed specifically for restricted environments. It provides a stealthy operation
mode without windows or traces, and enables cross-device AI Q&A interaction and control via the local area network (
LAN). Essentially, it is an automated collaboration layer of "screen/audio capture + AI inference + low-friction delivery",
helping users to immersively use AI assistants across applications on controlled devices or in restricted environments.

### [ProxyPal](https://github.com/buddingnewinsights/proxypal)

Cross-platform desktop app (macOS, Windows, Linux) wrapping CLIProxyAPI with a native GUI. Connects Claude, ChatGPT, Gemini, GitHub Copilot, and custom OpenAI-compatible endpoints with usage analytics, request monitoring, and auto-configuration for popular coding tools - no API keys needed.

### [CLIProxyAPI Quota Inspector](https://github.com/AllenReder/CLIProxyAPI-Quota-Inspector)

Ready-to-use cross-platform quota inspector for CLIProxyAPI, supporting per-account codex 5h/7d quota windows, plan-based sorting, status coloring, and multi-account summary analytics.

### [CodexCliPlus](https://github.com/C4AL/CodexCliPlus)

Windows-focused, local-first desktop management platform for Codex CLI built on CLIProxyAPI, focused on simplifying local setup, account and runtime management, and providing a more complete Codex CLI experience for local users.

### [CLIProxy Pool Watch](https://github.com/murasame612/CLIProxyPoolWidget)

Native macOS SwiftUI app for monitoring ChatGPT/Codex account quotas in CLIProxyAPI pools. Displays account availability, Plus-base capacity, 5-hour and weekly quota bars, plan weights, and restore forecasts through the Management API.

> [!NOTE]  
> If you developed a project based on CLIProxyAPI, please open a PR to add it to this list.

## More choices

Those projects are ports of CLIProxyAPI or inspired by it:

### [9Router](https://github.com/decolua/9router)

A Next.js implementation inspired by CLIProxyAPI, easy to install and use, built from scratch with format translation (OpenAI/Claude/Gemini/Ollama), combo system with auto-fallback, multi-account management with exponential backoff, a Next.js web dashboard, and support for CLI tools (Cursor, Claude Code, Cline, RooCode) - no API keys needed.

### [OmniRoute](https://github.com/diegosouzapw/OmniRoute)

Never stop coding. Smart routing to FREE & low-cost AI models with automatic fallback.

OmniRoute is an AI gateway for multi-provider LLMs: an OpenAI-compatible endpoint with smart routing, load balancing, retries, and fallbacks. Add policies, rate limits, caching, and observability for reliable, cost-aware inference.

### [Playful Proxy API Panel (PPAP)](https://github.com/daishuge/playful-proxy-api-panel)

A public CLIProxyAPI-compatible fork and bundled management panel. It keeps upstream-style usage while restoring built-in usage statistics, adding cache hit rate, first-byte latency, TPS tracking, and Docker-oriented self-hosted installation docs.

### [Codex Switch](https://github.com/9ycrooked/CodexSwitch)

This is a tool built with Tauri 2 + Vue 3 for managing multiple OpenAI Codex desktop accounts. Switch between saved ChatGPT/Codex certification profiles, check 5-hour and weekly quota usage in real time, verify token health, view active account details, and import or save auth.json files without manual copying.

> [!NOTE]  
> If you have developed a port of CLIProxyAPI or a project inspired by it, please open a PR to add it to this list.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
