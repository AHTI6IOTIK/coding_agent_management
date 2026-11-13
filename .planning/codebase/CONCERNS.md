---
last_mapped_commit: f0a723bf3eb5e105f3b2d82a030d28485e7e13a8
last_mapped_at: 2026-11-13
---
# Codebase Concerns

**Analysis Date:** 2026-11-13

## Tech Debt

**Database layer is a stub:**
- Issue: `DB_PATH` is loaded and validated, the SQLite database is opened and pinged, but no schema, no queries, no repositories, and no handlers use it. The product name says "gallery", but there are no gallery tables or endpoints.
- Files: `cmd/server/main.go:36-50`, `internal/config/config.go:48`
- Impact: The `sql.DB` is pure overhead at runtime; any feature touching data must build the whole persistence layer from scratch with no foundation.
- Fix approach: Decide whether the gallery feature is real. If yes, add a migration step plus a repository package (e.g. `internal/gallery`) with table-driven tests; if no, drop `DB_PATH`, the `modernc.org/sqlite` blank import in `cmd/server/main.go:17`, and the `DatabasePath` config field.

**Error context is discarded in the shutdown path:**
- Issue: Root-cause errors are replaced with generic `errors.New(...)` strings instead of wrapping with `%w`, contrary to the project rule in `AGENTS.md` ("при передаче причины выше оборачивайте её через `%w`").
- Files: `cmd/server/main.go:38`, `cmd/server/main.go:49`, `cmd/server/main.go:81`, `cmd/server/main.go:88`
- Impact: `database startup failed` / `http server failed` / `http server shutdown failed` hide which server or which underlying error fired; production debugging requires reproducing locally.
- Fix approach: Replace with `fmt.Errorf("database startup failed: %w", err)` and include the failing server's address: `fmt.Errorf("http server on %s failed: %w", server.Addr, err)`.

**Version is hard-coded, not injected at build time:**
- Issue: `applicationVersion = "v1.0.0"` is a compile-time constant inside `config`, so `/version` (`internal/httpserver/server.go:62-66`) always reports v1.0.0 regardless of the actual build.
- Files: `internal/config/config.go:16`
- Impact: Deployments cannot be traced back to a build; version drift is invisible.
- Fix approach: Inject via `-ldflags "-X ...applicationVersion=$VERSION"` or an `APP_VERSION` env override in `internal/config/config.go`.

**No HTTP middleware stack:**
- Issue: `NewApp` registers routes directly with no request logging, no panic recovery (`echo.New()` in Echo v4 does not install `middleware.Recover()` by default), and no request ID.
- Files: `internal/httpserver/server.go:38-54`
- Impact: A panic in a handler terminates the connection with an opaque log line from `net/http`; there is no per-request observability beyond the metrics endpoint.
- Fix approach: Add `middleware.Recover()` and a structured slog request logger when building the `echo.Echo` instance in `internal/httpserver/server.go:38`.

**Sequential server shutdown shares one timeout budget:**
- Issue: All three servers (`app`, metrics, pprof) are shut down one-by-one against a single 10s `context.Context`; a slow first drain starves the rest.
- Files: `cmd/server/main.go:84-90`
- Impact: In-flight requests on later servers can be cut off after an earlier server exhausts the budget.
- Fix approach: Shut down all servers concurrently in goroutines and join with `errgroup` under the same context.

**Repository mixes product code with learning/evidence artifacts:**
- Issue: The git tree contains agent homework evidence (`agent_work_verification/`, `.playwright-mcp/`, `test-results/`), Russian-named screenshot files with non-breaking spaces in filenames, and `docs/about.md` describing them as project documentation.
- Files: `agent_work_verification/`, `.playwright-mcp/`, `test-results/calculator-calculator-adds-ff953-gers-without-console-errors/calculator.png`, `docs/about.md`
- Impact: Noise in every clone; non-ASCII filenames with NBSP characters are fragile in scripts and CI; unclear what is "product" vs "evidence".
- Fix approach: Move evidence out of the repo or into a clearly ignored `evidence/` directory; keep only `docs/` for durable documentation.

## Known Bugs

**Calculator UI renders "undefined" on server errors:**
- Symptoms: Submitting an invalid expression (e.g. `-1+2`) returns `{"message":"invalid request"}` with HTTP 400; `data.result` is `undefined` and the `<output>` element displays the literal string `undefined`.
- Files: `internal/httpserver/calculator.html:26-27`
- Trigger: Open `/calculator`, enter any expression that `calculate` rejects (negative operand, fraction, empty input passes only if whitespace-only — actually empty string also yields `"NaN"` so it is fine; negative/fractional inputs reproduce it), press Calculate.
- Workaround: None in UI. Note the backend deliberately returns `"NaN"` for whitespace-only input (`internal/httpserver/server.go:108-110`), so only hard 400s (e.g. >1024-char message) hit this path.

**No fetch error handling in the calculator page:**
- Symptoms: If `/echo` is unreachable or returns non-JSON, the promise rejects and the submit handler dies silently; the output element keeps its previous value.
- Files: `internal/httpserver/calculator.html:19-28`
- Trigger: Stop the server after the page loads, then submit.
- Workaround: None; the e2e test in `tests/e2e/calculator.spec.ts` only covers the happy path.

## Security Considerations

**Metrics and pprof exposure is correctly mitigated, app server is not:**
- Risk: `/debug/pprof/` and `/metrics` leak internals if bound to all interfaces; the main app server (`""` host in `internal/config/config.go:62`) binds all interfaces with no authentication on `/echo` and `/version`.
- Files: `internal/config/config.go:62-65`, `internal/httpserver/server.go:68-81`
- Current mitigation: Metrics and pprof are hard-bound to `127.0.0.1` (`internal/config/config.go:64-65`); `gosec` is enabled in `.golangci.yaml:9`.
- Recommendations: If the service is ever deployed beyond localhost, front the app server with an authenticating proxy or add auth middleware; document the current "loopback-only aux ports" assumption in `internal/config/config.go`.

**`.env` file present in working tree:**
- Risk: Local environment configuration exists; contents were not inspected (16 bytes, ignored by `.gitignore:8`).
- Files: `.env`
- Current mitigation: `.gitignore` excludes `.env`/`.env.*` with `!.env.example` negation; however, no `.env.example` template exists, so required vars (`APP_REGION` is mandatory per `internal/config/config.go:57-59`) are undocumented.
- Recommendations: Add a `.env.example` listing all five supported variables without values.

## Performance Bottlenecks

**No measured bottlenecks:**
- Problem: The service has 5 routes and one in-process calculation over `big.Int`; no load-sensitive path exists. `calculate` in `internal/httpserver/server.go:101-122` allocates fresh `big.Int` per operand, which is fine at this scale.
- Files: `internal/httpserver/server.go:101-122`
- Cause: N/A at current size.
- Improvement path: pprof endpoint (`internal/httpserver/server.go:72-81`) is available for profiling when real load exists.

## Fragile Areas

**`calculate` expression parser:**
- Files: `internal/httpserver/server.go:101-122`
- Why fragile: Hand-rolled split-on-`+` parser with implicit contract: whitespace stripped first, each operand must parse as a non-negative decimal integer, `"+"`-only and empty operands return `"NaN"`. Any change (e.g. adding `-`, `*`, unary plus) breaks the implicit operand rules and several table-driven expectations at once.
- Safe modification: Extend `server_test.go:16-36` cases first (new operator cases, `++`, `+2`, `2+`), then modify `calculate`; keep `"NaN"` as the single failure sentinel.
- Test coverage: Good for current behavior (negative, fractional, whitespace cases) but none for oversized operands beyond `big.Int` limits or `calc` values near the 1024-char validation ceiling on `message` (note: `max=1024` validates `message` only — `calc` itself is unbounded, `internal/httpserver/server.go:29-31`).

**Shared error-message string coupling between handler and tests:**
- Files: `internal/httpserver/server.go:86,89`, `internal/httpserver/server_test.go:32-34`
- Why fragile: Tests assert the exact body `{"message":"invalid request"}`; renaming or re-wording the error string in either `Bind` or `Validate` branch breaks three test cases simultaneously.
- Safe modification: Extract the string into a package-level constant used by both handler and tests.

**Playwright webServer with `reuseExistingServer: !CI`:**
- Files: `playwright.config.ts:13`
- Why fragile: Outside CI, the runner reuses whatever is already listening on `http://127.0.0.1:18080` — including a stale server started with different env (`DB_PATH`, `APP_REGION`), producing tests that pass against the wrong build.
- Safe modification: Check the `/health` and `/version` endpoints before reusing, or set `reuseExistingServer: false` for local runs too.

## Scaling Limits

**SQLite single-writer with no WAL/migration setup:**
- Current capacity: Single process, file-backed SQLite at `gallery.db` (or `:memory:` in e2e, `playwright.config.ts:10`).
- Limit: No schema or write path exists yet; when data access is added, default SQLite settings will serialize writers and cap throughput at one writer connection.
- Scaling path: Enable WAL, set busy timeout, and route writes through a single connection pool; for multi-instance scale-out, replace SQLite with a server database.

## Dependencies at Risk

**`modernc.org/sqlite` (pure-Go SQLite):**
- Risk: Large transitive surface (`modernc.org/libc`, `modernc.org/memory` in `go.mod:37-39`); slower than cgo drivers; currently used only for `sql.Open` + `Ping` in `cmd/server/main.go:36-50`.
- Impact: Upgrades of the `modernc.org/*` family have historically carried breaking changes; right now the dependency earns nothing.
- Migration plan: Either commit to SQLite (add schema, repository, integration tests via testcontainers per `AGENTS.md`) or remove the dependency until a data feature actually lands.

**Go 1.26 requirement:**
- Risk: `go.mod:3` requires `go 1.26.0`; toolchains older than that fail hard, and earlier GSD observations noted a Node version mismatch on this machine for the gsd-core tooling.
- Impact: New contributor onboarding requires an exact-ish toolchain.
- Migration plan: Document the required Go version in `README.md`; consider `toolchain` directive pinning.

## Missing Critical Features

**No CI pipeline:**
- Problem: `go test --race ./...`, `golangci-lint run ./...` (mandated by `AGENTS.md`) and the Playwright suite run only locally; no workflow file exists anywhere in the repo.
- Blocks: Regression detection before merge; the `reuseExistingServer: !process.env.CI` branch in `playwright.config.ts:13` implies CI was planned but never built.

**No integration tests:**
- Problem: `AGENTS.md` requires DB-touching tests in `tests/` via testcontainers; the directory contains only `tests/e2e/calculator.spec.ts` and there are no DB tests at all (consistent with the missing persistence layer).
- Blocks: Any future database feature lacks the test harness pattern to copy.

**No `.env.example`:**
- Problem: `.gitignore:10` anticipates `!.env.example` but the file does not exist; `APP_REGION` is mandatory yet undiscoverable without reading `internal/config/config.go:53-59`.
- Blocks: Fast local setup for new environments.

## Test Coverage Gaps

**Metrics/pprof server wiring in `main`:**
- What's not tested: `cmd/server/main.go` (the whole `run()` flow: three-server startup, port-conflict behavior at runtime, shutdown ordering) has zero tests; only `internal/config` and `internal/httpserver` are covered.
- Files: `cmd/server/main.go`
- Risk: Startup regressions (bad address, failed ping, shutdown deadlock) surface only at deploy time.
- Priority: Medium — can be covered by an integration test that boots the binary against ephemeral ports.

**Pprof handler surface:**
- What's not tested: `PprofHandler()` in `internal/httpserver/server.go:72-81` has no test asserting that `/debug/pprof/` routes actually respond on its own mux (only that it is *not* reachable through the app server, `server_test.go:35`).
- Files: `internal/httpserver/server.go:72-81`
- Risk: A broken pprof mount would go unnoticed until needed for debugging.
- Priority: Low.

**Calculator UI error paths:**
- What's not tested: `tests/e2e/calculator.spec.ts` covers only the happy path; the `undefined`-output bug and network-failure path described under Known Bugs are untested.
- Files: `internal/httpserver/calculator.html`, `tests/e2e/calculator.spec.ts`
- Risk: UI regressions on invalid input ship silently.
- Priority: Medium — fixing the Known Bug should come with an e2e case for a rejected expression.

**`calc` field validation asymmetry:**
- What's not tested: `echoRequest.Calc` is a `*string` with no length limit while `Message` has `max=1024` (`internal/httpserver/server.go:29-31`); no test sends a multi-megabyte `calc` value.
- Files: `internal/httpserver/server.go:29-31`, `internal/httpserver/server.go:101-122`
- Risk: Unbounded input into a per-request string parse; a crafted large `calc` costs memory and CPU per request.
- Priority: Medium — add a `max` tag on `Calc` plus a rejection test.

---

*Concerns audit: 2026-11-13*
