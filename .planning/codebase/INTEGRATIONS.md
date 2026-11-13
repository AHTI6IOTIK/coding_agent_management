---
last_mapped_commit: f0a723bf3eb5e105f3b2d82a030d28485e7e13a8
last_mapped_at: 2026-11-13
---
# External Integrations

**Analysis Date:** 2026-11-13

## APIs & External Services

**None.** The service makes no outbound HTTP calls and imports no third-party SDKs. All client-facing interaction is its own HTTP API:

- `GET /` — JSON hello (`internal/httpserver/server.go:43-45`)
- `GET /health` — liveness JSON for tooling (used as the Playwright readiness probe, `playwright.config.ts`)
- `GET /version` — returns `v1.0.0` (`internal/httpserver/server.go:50,62-66`)
- `GET /calculator` — serves the embedded HTML page `internal/httpserver/calculator.html` (`internal/httpserver/server.go:17-18,57-59`)
- `POST /echo` — bind + validate `echoRequest{message (required,max=1024), calc}`; when `calc` is set, sums `+`-separated non-negative integers via `math/big` and returns `result`, else `NaN` (`internal/httpserver/server.go:83-121`)

## Data Storage

**Databases:**
- SQLite, embedded (no server process)
  - Driver: `modernc.org/sqlite v1.61.1` registered as `"sqlite"` (blank import in `cmd/server/main.go:17`), accessed via `database/sql`
  - Connection: file path from `DB_PATH` env var (default `gallery.db`), opened in `cmd/server/main.go:36`; e2e runs use `DB_PATH=:memory:` (`playwright.config.ts`)
  - Client/ORM: none — raw `database/sql`; note `sqlclosecheck` linter is enabled in `.golangci.yaml`, so always close rows
  - No migrations, no schema files detected; the database is opened and pinged but the current code creates no tables

**File Storage:**
- Local filesystem only (SQLite database file)

**Caching:**
- None

## Authentication & Identity

**Auth Provider:**
- None. All endpoints are unauthenticated; there is no middleware, no session/token handling anywhere in `internal/httpserver/`

## Monitoring & Observability

**Error Tracking:**
- None (no Sentry/external APM)

**Logs:**
- Standard library `log/slog` structured logging (`cmd/server/main.go:24,34,42`, `internal/httpserver/server.go:41`)
- `loggercheck` linter enabled in `.golangci.yaml` — keep key-value pairs well-formed when adding slog calls

**Metrics:**
- Prometheus: `promhttp.Handler()` served by a separate loopback-only server on `127.0.0.1:METRICS_PORT` (default 9090) — `internal/httpserver/server.go:68-70`, address built in `internal/config/config.go:64`

**Profiling:**
- Standard `net/http/pprof` endpoints (`/debug/pprof/*`) on a third loopback-only server at `127.0.0.1:PPROF_PORT` (default 6060) — `internal/httpserver/server.go:72-81`

## CI/CD & Deployment

**Hosting:**
- None configured. Local `go run ./cmd/server` / `go build`; no Dockerfile, no IaC, no deploy manifests detected

**CI Pipeline:**
- None. Required local gates (per `README.md` / `AGENTS.md`): `go test --race ./...`, `golangci-lint run ./...`, `npm run test:e2e`

## Environment Configuration

**Required env vars:**
- `APP_REGION` — **required**; startup fails if empty (`internal/config/config.go:53-59`)
- Optional with defaults: `APP_PORT` (8080), `DB_PATH` (`gallery.db`), `METRICS_PORT` (9090), `PPROF_PORT` (6060)
- Port triple must be distinct (`internal/config/config.go:44-46`)

**Secrets location:**
- `.env` file exists in repo root (16 bytes, contents not read per policy); it is git-ignored (`.gitignore`), and the agent config denies reading `.env*` (`.opencode/opencode.json` permission block)
- No other secret stores detected

## Webhooks & Callbacks

**Incoming:**
- None (the public endpoints above are plain REST, not webhook receivers)

**Outgoing:**
- None

## Development-Tooling Integrations (agent workflow, not runtime)

- Playwright MCP server (`@playwright/mcp`) — browser automation for the coding agent, registered in `.opencode/opencode.json` `mcp.servers.playwright`
- GSD MCP server (`@opengsd/gsd-core` `gsd-mcp-server --no-prompts`) — planning workflow tooling, registered in `.opencode/opencode.json` `mcp.servers.gsd`
- Playwright e2e harness — `playwright.config.ts` spawns the real server (`go run ./cmd/server`) as a web server on `http://127.0.0.1:18080` and waits for `/health`; tests live in `tests/e2e/calculator.spec.ts`

---

*Integration audit: 2026-11-13*
