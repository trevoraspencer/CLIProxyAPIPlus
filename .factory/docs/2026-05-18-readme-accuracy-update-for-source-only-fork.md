## README Update for Personal Fork

**Goal:** Make README accurate for this fork where users must build from source (no Docker Hub/GHCR images published).

### Changes
1. **"Run with Docker" section** (lines 29-52): Replace the Docker pull instructions with a source-build workflow. Keep `docker-compose.yml` usage but document that it builds locally (remove references to `kaitranntt/cli-proxy-api-plus` images and Docker Hub/GHCR links). Show the clone → build → run pattern or note that `docker compose up` builds automatically.

2. **Docker Compose note**: Update the compose example to clarify it builds from the local Dockerfile (remove the `pull_policy: always` implication and image pull references).

3. **Any other Docker Hub / GHCR mentions**: Strip or qualify them so they don't promise pre-built images that don't exist.

### Non-changes
- Keep all other sections (Management API, Local Proxy Client Credentials, Usage Statistics, Amp, xAI, Z.AI, OpenCode, SDK, Contributing, License) as-is unless they contain fork-specific Docker references.
- Do not modify `docker-compose.yml` or `Dockerfile` — only the README.

### Verification
After spec approval, run `gofmt -w .` (no-op), then `go build -o test-output ./cmd/server && rm test-output` to confirm the repo still compiles. No functional code changes are made.