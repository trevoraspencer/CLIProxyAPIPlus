# Environment

Environment variables, external dependencies, and setup notes for the DeepSeek Factory Droid compatibility mission.

**What belongs here:** Required env vars, local toolchain quirks, external credential availability, validation constraints.
**What does NOT belong here:** Service ports/commands (use `.factory/services.yaml`).

---

- Go 1.26.x is required; planning found Go 1.26.3 on darwin/arm64.
- Docker is not installed and is not required.
- No real DeepSeek API key is available to workers. The user will add it to personal config after implementation.
- Do not read, print, edit, or depend on `~/.cli-proxy-api/config.yaml`, `~/.factory/settings.json`, real auth files, `.env`, or personal credential stores.
- `127.0.0.1:8318` is a user-owned proxy. Workers may only perform unauthenticated `GET /v1/models` readiness probes expecting `401`; do not authenticate, restart, reload, stop, or reconfigure it.
- Automated tests must use synthetic config/auth data, temp directories, pure fixtures, and `httptest.Server`.
- Do not call live DeepSeek or other upstream provider endpoints from tests.
