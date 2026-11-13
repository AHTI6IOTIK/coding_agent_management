---
last_mapped_commit: f0a723bf3eb5e105f3b2d82a030d28485e7e13a8
last_mapped_at: 2026-11-13
---
<!-- refreshed: 2026-11-13 -->
# Architecture

**Analysis Date:** 2026-11-13

## System Overview

```text
┌──────────────────────────────────────────────────────────────────┐
│                        Process Entry                              │
│                     `cmd/server/main.go`                          │
│   config load → sqlite open → 3 http.Server → graceful shutdown   │
├──────────────────┬──────────────────────┬────────────────────────┤
│   Config         │   Transport (Echo)   │   Sidecar Handlers     │
│  `internal/config`│  `internal/httpserver`│  MetricsHandler /     │
│                  │                      │  PprofHandler          │
│  `config.Load()` │  `NewApp(version)`   │  (`server.go:68-81`)   │
├──────────────────┴──────────────────────┴────────────────────────┤
│                       Storage (database/sql)                      │
│            sqlite driver `modernc.org/sqlite` opened in           │
│            `cmd/server/main.go:36`, DB file from `DB_PATH`        │
└──────────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| `main` / `run` | Process lifecycle: load config, open SQLite, start 3 servers, wait for SIGINT/SIGTERM, shut down with 10s timeout | `cmd/server/main.go` |
| `config` | Read and validate environment variables (`APP_PORT`, `DB_PATH`, `METRICS_PORT`, `PPROF_PORT`, `APP_REGION`), return immutable `Config` value | `internal/config/config.go` |
| `httpserver` (app) | Public HTTP API: routes `/`, `/calculator`, `/health`, `/version`, `POST /echo`; request binding + validation; calculator logic | `internal/httpserver/server.go` |
| `httpserver.MetricsHandler` | Prometheus `/metrics` endpoint on a separate loopback-only server | `internal/httpserver/server.go:68-70` |
| `httpserver.PprofHandler` | net/http/pprof debug endpoints on a separate loopback-only server | `internal/httpserver/server.go:72-81` |
| `calculator.html` | Embedded single-page calculator UI; posts expressions to `POST /echo` | `internal/httpserver/calculator.html`, embedded via `//go:embed` at `internal/httpserver/server.go:17` |

## Pattern Overview

**Overall:** Layered Go service, single binary, transport-only HTTP layer with sidecar admin servers.

**Key Characteristics:**
- Standard Go project layout: `cmd/` for the binary, `internal/` for everything not importable outside the module.
- Dependency direction follows `.agents/rules/architecture.md`: `cmd` → transport (`internal/httpserver`) → config; no transport code depends on storage.
- Three independent `http.Server` instances share one process: public app, metrics, pprof. Metrics and pprof are deliberately bound to `127.0.0.1` only (`internal/config/config.go:64-65`).
- Config is environment-only, loaded once at startup, validated (distinct ports, non-empty region), and passed down as a plain value struct — never re-read `os.Getenv` outside `internal/config/config.go`.
- UI is embedded into the binary with `//go:embed` (`internal/httpserver/server.go:17`), no external static files at runtime.

## Layers

**Entry / Composition root:**
- Purpose: wire everything, own lifecycle and shutdown.
- Location: `cmd/server/main.go`
- Contains: `main()`, `run()`, `serve()` helper; SQLite driver blank import `_ "modernc.org/sqlite"` (`cmd/server/main.go:17`).
- Depends on: `internal/config`, `internal/httpserver`, `database/sql`.
- Used by: nothing — this is the top.

**Configuration:**
- Purpose: translate environment into a validated `Config` value struct.
- Location: `internal/config/config.go`
- Contains: `Config` type (`config.go:19-26`), `Load()` (`config.go:28`), helpers `port()`, `value()`.
- Depends on: standard library only.
- Used by: `cmd/server/main.go` and `NewApp(version)` (version string only).

**Transport / HTTP:**
- Purpose: HTTP routes, binding, validation, JSON responses, calculation logic.
- Location: `internal/httpserver/server.go`
- Contains: `NewApp(version) *echo.Echo` (`server.go:38`), named handlers (`calculatorHandler`, `versionHandler`, `echoMessage`), DTOs `echoRequest`/`echoResponse` (`server.go:28-36`), `customValidator` (`server.go:20-26`), `calculate()` (`server.go:101`).
- Depends on: Echo, go-playground/validator, promhttp, embedded HTML.
- Used by: `cmd/server/main.go`.
- Per-package rules in `internal/httpserver/AGENTS.md`: every route gets a separate named handler function preceded by a Go-doc comment stating method, path, and purpose.

**Storage:**
- Purpose: SQLite persistence via `database/sql`; driver registered by blank import.
- Location: opened in `cmd/server/main.go:36-50`; no repository layer exists yet.
- Contains: `sql.Open("sqlite", configuration.DatabasePath)` + startup `PingContext`.
- Note: the DB handle is opened, pinged and closed in `main`, but is NOT currently injected into any HTTP handler. Any new persistence feature must add a repository abstraction and pass the handle down — do not re-open the DB in handlers.

## Data Flow

### Primary Request Path (POST /echo)

1. `main` builds the app once at startup: `httpserver.NewApp(configuration.Version)` (`cmd/server/main.go:55`, `internal/httpserver/server.go:38`).
2. Echo routes the request to named handler `echoMessage` (`internal/httpserver/server.go:51`, registration `server.go:51`; handler body `server.go:83`).
3. `context.Bind(request)` decodes JSON into `echoRequest`; failure → `echo.NewHTTPError(400, "invalid request")` (`server.go:85-87`).
4. `context.Validate(request)` runs go-playground/validator tags (`validate:"required,max=1024"` — `server.go:29`) through `customValidator` (`server.go:24`); failure → same 400 error.
5. If `calc` is present, `calculate()` sums `+`-separated non-negative integers with `big.Int`; any parse failure, negative or non-integer operand yields the literal string `"NaN"` (`server.go:101-122`).
6. Handler returns `echoResponse` as JSON 200 (`server.go:98`).

### Calculator Browser Flow

1. `GET /calculator` returns the embedded page (`internal/httpserver/server.go:46,57`).
2. Browser JS posts the expression to `POST /echo` with the `calc` field and displays `result`.
3. E2E coverage: `tests/e2e/calculator.spec.ts` drives Playwright against this flow and asserts zero console/page errors.

### Sidecar Server Flow

1. `main` starts 3 servers on `configuration.AppAddress`, `MetricsAddress`, `PprofAddress` (`cmd/server/main.go:52-73`); failures funnel into the `serverErrors` channel (`server.go:70-73,95-99`).
2. `/health` on the app server is the readiness probe used by Playwright's `webServer` in `playwright.config.ts`.
3. Metrics served by `promhttp.Handler()`, pprof by a dedicated `http.ServeMux` (`internal/httpserver/server.go:68-81`).

### Shutdown Flow

1. `signal.NotifyContext(SIGINT, SIGTERM)` fires or a server errors (`cmd/server/main.go:75-82`).
2. All 3 servers shut down with a shared 10-second context (`cmd/server/main.go:84-90`).
3. Deferred `database.Close()` runs with error logging (`cmd/server/main.go:40-44`).

**State Management:**
- No shared mutable state. All state is per-request inside handlers; the only process-level values are the immutable `Config` and the `*sql.DB` handle in `main`.

## Key Abstractions

**`Config` value struct:**
- Purpose: the single, validated representation of process configuration.
- Example: `internal/config/config.go:19-26`.
- Pattern: plain value struct returned by value from `Load()`; callers never mutate it.

**Named Echo handler / handler factory:**
- Purpose: each HTTP contract is a testable standalone function; closures are banned by `internal/httpserver/AGENTS.md`.
- Examples: `calculatorHandler` (`internal/httpserver/server.go:57`), `versionHandler(version) echo.HandlerFunc` (`server.go:62`) — the factory is the pattern for capturing startup values.
- Pattern: `func xxxHandler(context echo.Context) error`, Go-doc comment directly above.

**DTO structs with validator tags:**
- Purpose: define the JSON contract and its validation in one place.
- Examples: `echoRequest`, `echoResponse` (`internal/httpserver/server.go:28-36`).
- Pattern: `xxxRequest` for input, `xxxResponse` for output; validation only via struct tags through `customValidator`.

**Validator adapter:**
- Purpose: bridge go-playground/validator to Echo's `Validator` interface.
- Example: `customValidator` (`internal/httpserver/server.go:20-26`).

**Embedded asset:**
- Purpose: ship UI inside the binary.
- Example: `calculator.html` via `//go:embed` (`internal/httpserver/server.go:17`).

## Entry Points

**`cmd/server`:**
- Location: `cmd/server/main.go`
- Triggers: `go run ./cmd/server` (also launched by Playwright `webServer` in `playwright.config.ts` with e2e env: `APP_PORT=18080`, `DB_PATH=:memory:`).
- Responsibilities: whole process lifecycle described above.

**HTTP routes (app server, `APP_PORT`):**
- `GET /` — hello-world JSON (`internal/httpserver/server.go:43`)
- `GET /calculator` — embedded UI (`server.go:46`)
- `GET /health` — status probe (`server.go:47`)
- `GET /version` — build version (`server.go:50`)
- `POST /echo` — echo + optional calculation (`server.go:51`)

**HTTP routes (sidecar servers):** `/metrics` (promhttp) and `/debug/pprof/*` (`internal/httpserver/server.go:68-81`).

## Architectural Constraints

- **Threading:** Go runtime; each `http.Server` runs in its own goroutine (`cmd/server/main.go:71-73`); handlers are stateless and safe for concurrent requests. SQLite (`modernc.org/sqlite`, pure Go) is opened with default single-connection semantics — see `.agents/rules/architecture.md` guidance on SQLite locking before adding concurrent writes.
- **Global state:** none beyond `slog.Default()`; tests swap it locally (`internal/httpserver/server_test.go:66-69`).
- **Environment-only config:** no config files, no `.env` loading in Go code; `.env` exists for local tooling but is not read by the service. All required vars: `APP_REGION` (mandatory), `APP_PORT`, `DB_PATH`, `METRICS_PORT`, `PPROF_PORT` (optional with defaults) — `internal/config/config.go:28-68`.
- **Loopback-only sidecars:** `MetricsAddress` and `PprofAddress` are always `net.JoinHostPort("127.0.0.1", ...)` (`internal/config/config.go:64-65`). Do not change this without explicit agreement.
- **Ports must be distinct:** validated at startup (`internal/config/config.go:44-46`).
- **Versioning:** application version is a compile-time constant `applicationVersion` in `internal/config/config.go:16`, threaded through `NewApp(version)`.
- **Circular imports:** none; `internal` packages do not import each other except through `cmd`.

## Anti-Patterns

### Inline closures as handlers

**What happens:** registering `app.GET("/x", func(c echo.Context) error {...})` directly in `NewApp`.
**Why it's wrong:** violates `internal/httpserver/AGENTS.md`; untestable in isolation, no Go-doc, no clear HTTP contract.
**Do this instead:** named function + Go-doc comment, like `calculatorHandler` (`internal/httpserver/server.go:56-59`); use a factory when capturing startup values (`versionHandler`, `server.go:61-66`).

### Reading environment outside `internal/config`

**What happens:** calling `os.Getenv` from handlers or new packages.
**Why it's wrong:** breaks the single validated-config-source rule and `.agents/rules/architecture.md` ("получайте конфигурацию извне и проверяйте её при запуске"); untestable.
**Do this instead:** add the variable to `Config` via the `value()`/`port()` helpers in `internal/config/config.go` and pass it down.

### Transport DTOs as domain entities

**What happens:** reusing `echoRequest`/`echoResponse` or echoing Echo types into future service/repository layers.
**Why it's wrong:** violates `.agents/rules/architecture.md` — domain logic must not depend on HTTP.
**Do this instead:** define plain internal types at the consumer layer; map at the transport boundary.

### Silently losing errors

**What happens:** `_ = err`, `fmt.Errorf("failed")` without `%w`, or `==` error comparison.
**Why it's wrong:** `errorlint`, `contextcheck` fail; root causes are lost.
**Do this instead:** wrap with `%w`, check with `errors.Is`/`errors.As` (example: `cmd/server/main.go:96`); never include sensitive data in errors or logs (`.agents/rules/security.md`).

## Error Handling

**Strategy:** fail fast at startup; convert to HTTP 400 at the transport edge; wrap with `%w` upward; structured logging of failures.

**Patterns:**
- Startup: return error up from `run()`; `main` logs via `slog.Error` and exits 1 (`cmd/server/main.go:22-27`).
- Transport: `echo.NewHTTPError(http.StatusBadRequest, "invalid request")` for bind/validate failures (`internal/httpserver/server.go:86,89`) — opaque message, no internals leaked.
- Domain edge: `calculate()` degrades invalid input to the `"NaN"` result string instead of an error (`internal/httpserver/server.go:108-118`) — keep this contract in tests when touching it (`internal/httpserver/server_test.go:22-24`).
- Shutdown: `errors.Is(err, http.ErrServerClosed)` filter in `serve()` (`cmd/server/main.go:96`).

## Cross-Cutting Concerns

**Logging:** stdlib `log/slog`, key-value pairs, no third-party logger; structured level tested in `internal/httpserver/server_test.go:65-75`. Follow `loggercheck` rules for even key-value pairs.

**Validation:** go-playground/validator struct tags + `customValidator`; per-request, at the transport boundary only.

**Authentication:** none — the service is unauthenticated by design; do not add secrets to responses.

**Observability:** Prometheus metrics (`github.com/prometheus/client_golang`) and pprof, both loopback-only.

---

*Architecture analysis: 2026-11-13*
