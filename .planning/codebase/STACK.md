---
last_mapped_commit: f0a723bf3eb5e105f3b2d82a030d28485e7e13a8
last_mapped_at: 2026-11-13
---
# Technology Stack

**Analysis Date:** 2026-11-13

## Languages

**Primary:**
- Go 1.26.0 — application code (`go.mod` declares `go 1.26.0`); all server code lives in `cmd/server/main.go`, `internal/config/`, `internal/httpserver/`

**Secondary:**
- TypeScript — Playwright e2e test config `playwright.config.ts` and specs under `tests/e2e/calculator.spec.ts`
- HTML/JS (vanilla) — the calculator UI is a single embedded page `internal/httpserver/calculator.html` (built into the binary via `go:embed`)
- JSON (declarative) — agent tooling config `.opencode/opencode.json`; npm manifest `package.json`

## Runtime

**Environment:**
- Go toolchain 1.26.0 (CGO not required — SQLite uses the pure-Go driver, see below)
- Node.js (only for e2e testing toolchain; no Node runtime in production code)
- No Dockerfile, no container manifests detected

**Package Manager:**
- Go modules (`go.mod` + `go.sum` present)
- npm (`package-lock.json` v3 present); only dev dependency: `@playwright/test` 1.64.1

## Frameworks

**Core:**
- Echo v4 (`github.com/labstack/echo/v4 v4.16.3`) — HTTP framework and router; app built in `internal/httpserver/server.go` (`NewApp`)
- go-playground/validator v10 (`github.com/go-playground/validator/v10 v10.30.6`) — request struct validation, wired as Echo's custom validator `customValidator` in `internal/httpserver/server.go:20-26,40`
- Prometheus client_golang (`github.com/prometheus/client_golang v1.25.0`) — `/metrics` endpoint via `promhttp.Handler()` in `internal/httpserver/server.go:68-70`
- modernc.org/sqlite v1.61.1 — CGO-free SQLite driver, registered as `"sqlite"` and opened in `cmd/server/main.go:36`

**Testing:**
- Go standard `testing` — unit tests co-located: `internal/httpserver/server_test.go`, `internal/config/config_test.go`
- Playwright (`@playwright/test` 1.64.1) — browser e2e tests in `tests/e2e/`, config `playwright.config.ts`

**Build/Dev:**
- `go run ./cmd/server` — local run (also used as the Playwright web server command in `playwright.config.ts`)
- `golangci-lint` v2 config in `.golangci.yaml`: formatters `gofmt`, `goimports`; linters `bodyclose`, `contextcheck`, `errorlint`, `gosec`, `loggercheck`, `noctx`, `sqlclosecheck` on top of the `standard` set
- No Makefile, no CI pipeline config detected

## Key Dependencies

**Critical:**
- `github.com/labstack/echo/v4 v4.16.3` — every HTTP route and handler is an Echo handler (`internal/httpserver/server.go`)
- `modernc.org/sqlite v1.61.1` — the only data layer; imported as blank import in `cmd/server/main.go:17` to register the driver, used through `database/sql`
- `github.com/prometheus/client_golang v1.25.0` — operational metrics endpoint; do not remove without replacing observability

**Infrastructure:**
- `github.com/go-playground/validator/v10 v10.30.6` — declarative validation tags on request structs (`echoRequest` in `internal/httpserver/server.go:28-31`)
- `@playwright/test 1.64.1` (dev, npm) — e2e verification against a locally spawned server (`playwright.config.ts` `webServer` block)

## Configuration

**Environment:**
- Configuration comes only from OS environment variables, parsed in `internal/config/config.go` (`Load`); there is no config file loader
- `APP_PORT` — application HTTP port, default `8080` (`config.go:12`)
- `DB_PATH` — SQLite file path, default `gallery.db` (`config.go:13`); e2e uses `DB_PATH=:memory:`
- `METRICS_PORT` — metrics port, default `9090`; bound to `127.0.0.1` only (`config.go:64`)
- `PPROF_PORT` — pprof port, default `6060`; bound to `127.0.0.1` only (`config.go:65`)
- `APP_REGION` — **required, no default**; empty value fails startup (`config.go:53-59`)
- Port collisions between the three servers are rejected at load time (`config.go:44-46`)
- Application version is a compile-time constant `v1.0.0` (`config.go:16`), exposed via `GET /version`

**Build:**
- `go.mod` / `go.sum` — Go dependency pins
- `package.json` / `package-lock.json` — e2e tooling only
- `playwright.config.ts` — e2e server lifecycle: starts `go run ./cmd/server` with `APP_PORT=18080 METRICS_PORT=19090 PPROF_PORT=16060 APP_REGION=e2e DB_PATH=:memory:` and waits for `http://127.0.0.1:18080/health`
- `.golangci.yaml` — lint/format gates
- AGENTS.md files (`AGENTS.md`, `internal/httpserver/AGENTS.md`) — agent coding rules: route handlers must be named functions with Go-doc comments

## Platform Requirements

**Development:**
- Go ≥ 1.26
- Node.js + npm for e2e; run `npm install` and `npx playwright install chromium` once
- `golangci-lint` installed locally (install instructions in `README.md`)
- Verification commands: `go test --race ./...`, `golangci-lint run ./...`, `npm run test:e2e`

**Production:**
- Single static Go binary; no external services required except a writable path for the SQLite database (`DB_PATH`)
- App port must be reachable; metrics and pprof are loopback-only by design — keep it that way
- Startup requires `APP_REGION` to be set

---

*Stack analysis: 2026-11-13*
