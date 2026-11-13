---
last_mapped_commit: f0a723bf3eb5e105f3b2d82a030d28485e7e13a8
last_mapped_at: 2026-11-13
---
# Coding Conventions

**Analysis Date:** 2026-11-13

## Naming Patterns

**Files:**
- Go source files: lowercase, single-word or snake_case matching package purpose — `internal/config/config.go`, `internal/httpserver/server.go`, `cmd/server/main.go`.
- Tests are co-located with the code they test and carry the `_test.go` suffix — `internal/config/config_test.go`, `internal/httpserver/server_test.go`.
- Static assets embedded into a package live inside that package — `internal/httpserver/calculator.html` is embedded via `//go:embed` in `internal/httpserver/server.go:17`.
- Package-level agent rules live as `AGENTS.md` next to the package — root `AGENTS.md`, `internal/httpserver/AGENTS.md`.
- TypeScript: only E2E specs, under `tests/e2e/` with the `.spec.ts` suffix — `tests/e2e/calculator.spec.ts`.

**Functions:**
- Exported Go identifiers: PascalCase verbs/nouns — `config.Load()` (`internal/config/config.go:28`), `httpserver.NewApp()` (`internal/httpserver/server.go:38`), `httpserver.MetricsHandler()`, `httpserver.PprofHandler()`.
- Unexported identifiers: lowerCamelCase — `run()` (`cmd/server/main.go:29`), `echoMessage()` (`internal/httpserver/server.go:83`), `calculate()` (`internal/httpserver/server.go:101`).
- Test helper functions: plain descriptive names with `t.Helper()` as first statement — `unsetEnv(t, name)` (`internal/config/config_test.go:83`).
- HTTP handlers are standalone named functions (or handler factories returning `echo.HandlerFunc`), never inline closures in route registration — required by `internal/httpserver/AGENTS.md`. Example: `calculatorHandler` and `versionHandler(version)` (`internal/httpserver/server.go:57-66`).
- Config helper constructors take `(name, defaultValue)` and are named after what they return: `port()`, `value()` (`internal/config/config.go:71,85`).

**Variables:**
- lowerCamelCase for locals; short names for loop variables (`testCase`, `request`, `response`) — see the table-driven loop in `internal/httpserver/server_test.go:38-55`.
- Local variable that shadows a type name is the idiomatic lowercase of that type: `configuration, err := config.Load()` (`cmd/server/main.go:30`).

**Types:**
- PascalCase structs; DTO structs end in `Request`/`Response` — `echoRequest`, `echoResponse` (`internal/httpserver/server.go:28-36`).
- Config struct is a single aggregate returned by value — `Config` (`internal/config/config.go:19-26`).
- Constants are grouped in a single `const (...)` block at the top of the file with lowerCamelCase names — `defaultAppPort`, `defaultDatabasePath`, `applicationVersion` (`internal/config/config.go:11-17`).
- Unexported struct holding a dependency is a lowerCamelCase noun + suffix — `customValidator` wrapping `*validator.Validate` (`internal/httpserver/server.go:20-22`).

## Code Style

**Formatting:**
- `gofmt` + `goimports` are enforced via formatters in `.golangci.yaml` (formatters section). Do not hand-format; run `gofmt` before finishing any change.
- No config for line length; standard Go style (tabs, 100-col soft limit) applies.

**Linting:**
- `golangci-lint` v2 config in `.golangci.yaml`: `default: standard` plus enabled linters `bodyclose`, `contextcheck`, `errorlint`, `gosec`, `loggercheck`, `noctx`, `sqlclosecheck`.
- Practical consequences you MUST follow:
  - `errorlint`: compare/unwrap errors with `errors.Is`/`errors.As`, not `==` on arbitrary errors — pattern used in `cmd/server/main.go:96` (`errors.Is(err, http.ErrServerClosed)`).
  - `noctx`/`contextcheck`: never make HTTP/DB calls without a context that carries cancellation.
  - `loggercheck`: slog key-value pairs must be even and correctly ordered (`slog.Info("service starting", "region", configuration.Region)` — `cmd/server/main.go:34`).
  - `gosec`: do not disable lint rules with `//nolint` unless absolutely necessary and agreed.
- Run before finishing any change: `go vet` and `golangci-lint run ./...` (root `AGENTS.md`, «Конвенции Go»).
- Validation is done with `go-playground/validator` tags on request DTOs (`validate:"required,max=1024"` — `internal/httpserver/server.go:29`), wired through a `customValidator` satisfying Echo's `Validate(any) error`.

## Import Organization

**Order:**
1. Standard library (`context`, `errors`, `net/http`, `log/slog`, ...).
2. Blank import for driver registration last in its own group — `_ "modernc.org/sqlite"` (`cmd/server/main.go:17`), `_ "embed"` (`internal/httpserver/server.go:4`).
3. Third-party packages.
4. Internal packages of this module — `coding_agent_management/internal/config`, `coding_agent_management/internal/httpserver` (`cmd/server/main.go:14-15`).

Groups are separated by a blank line, exactly as `goimports` produces.

**Path Aliases:**
- Module path is `coding_agent_management` (`go.mod:1`); internal imports use the full module path, no aliases.

## Error Handling

**Patterns:**
- **Wrap with `%w` and add message context**: use `fmt.Errorf("...: %w", err)` when propagating; root `AGENTS.md` («Обрабатывайте каждую ошибку; ... оборачивайте её через `%w`»), enforced by `errorlint`.
- **Sentinel-downgrade for user-facing failures**: startup errors are converted to fixed, information-poor messages to avoid leaking internals — `errors.New("database startup failed")` (`cmd/server/main.go:38,49`), `errors.New("http server failed")` (`cmd/server/main.go:81`). Use this pattern at trust boundaries; never pass raw driver errors to clients.
- **HTTP errors via `echo.NewHTTPError` with a fixed message**: bind and validation failures both return `echo.NewHTTPError(http.StatusBadRequest, "invalid request")` — the same generic message for both cases so validation details never leak (`internal/httpserver/server.go:86,89`).
- **Early return per step**: each config field is read and its error returned immediately — `internal/config/config.go:29-59`. Do not accumulate errors into a list.
- **Domain-level "failure" encoded as a value, not an error**: `calculate()` returns the string `"NaN"` for invalid input instead of an error (`internal/httpserver/server.go:101-121`). Follow this for arithmetic-like pure functions where failure is an expected outcome.
- **Deferred cleanup checks its own error**: `defer func() { if err := database.Close(); err != nil { slog.Error(...) } }()` (`cmd/server/main.go:40-44`). Never `_ = f.Close()` silently for resources that matter; `_ = os.Unsetenv(...)` style discard is acceptable only in test helpers (`internal/config/config_test.go:92-95`).
- **Context with timeout for every external call**: `context.WithTimeout(context.Background(), shutdownTimeout)` for DB ping and server shutdown (`cmd/server/main.go:46-48,84-85`). `context.Context` is always the first parameter of any function that needs it.
- **Validation of config at startup, fail fast**: distinct-port check and non-empty `APP_REGION` enforced in `config.Load()` (`internal/config/config.go:44-46,57-59`). Add new config validation here, not in callers.
- Per `.agents/rules/coding.md` («Изменение кода»): add a comment only for reasons not visible in code; format known temporary limitations as `shortcut: <ограничение>, <условие пересмотра>`.
- Per `.agents/rules/security.md`: never include sensitive data in errors, API responses, or logs; authorization checks happen server-side, and ambiguous authorization means deny.

## Logging

**Framework:** standard library `log/slog` (structured key-value logging). No third-party logger.

**Patterns:**
- Key-value pairs after a short message: `slog.Info("service starting", "region", configuration.Region)` (`cmd/server/main.go:34`), `slog.Info("application version", "version", version)` (`internal/httpserver/server.go:41`).
- Error-level for fatal/terminal conditions: `slog.Error("service stopped", "error", err)` (`cmd/server/main.go:24`).
- In tests, swap the default logger via `slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))` and restore it with `t.Cleanup` (`internal/httpserver/server_test.go:58-62`) — use this pattern to assert log output.
- Per `.agents/rules/security.md`: when adding or changing logging, verify no sensitive data reaches the logs.

## Comments

**When to Comment:**
- Go-doc comment directly above every HTTP handler stating method, path, and purpose — mandatory per `internal/httpserver/AGENTS.md`:
  ```go
  // calculatorHandler handles GET /calculator and returns the calculator page.
  func calculatorHandler(context echo.Context) error {
  ```
  (`internal/httpserver/server.go:56-59`)
- No comment otherwise unless the reason is invisible in the code (`.agents/rules/coding.md`). Temporary limitations are marked `shortcut: <limitation>, <review condition>`.

**JSDoc/TSDoc:**
- Not applicable; Go doc comments are the only documented-comment form. TypeScript E2E specs (`tests/e2e/calculator.spec.ts`) carry no doc comments — keep it that way.

## Function Design

**Size:** functions stay small and single-purpose; the largest function in the codebase is `run()` at ~65 lines orchestrating startup/shutdown (`cmd/server/main.go:29-93`). Keep orchestration code in `run()`-style wrappers and put logic in small pure helpers (`calculate`, `port`, `value`).

**Parameters:** few positional parameters; when a handler needs immutable configuration, use a factory closure capturing it rather than a config struct — `versionHandler(version string) echo.HandlerFunc` (`internal/httpserver/server.go:62`). Any function doing cancellation-capable work takes `context.Context` first.

**Return Values:** 
- Go idiom `(T, error)` everywhere; config `Load() (Config, error)`.
- Handlers return `error` (Echo contract) and produce responses via `context.JSON`/`context.HTML`.
- Constructors return concrete types, not interfaces: `NewApp(version string) *echo.Echo` (`internal/httpserver/server.go:38`), `MetricsHandler() http.Handler`.
- Optional response fields use pointer types + `omitempty` — `Result *string \`json:"result,omitempty"\`` (`internal/httpserver/server.go:35`).

## Module Design

**Exports:**
- Export only the package surface others need: `config` exports `Config` and `Load()` only; `httpserver` exports `NewApp`, `MetricsHandler`, `PprofHandler`. Everything else (validators, DTOs, calculation logic) stays unexported.
- No interfaces declared until a consumer needs one (`.agents/rules/architecture.md`: «Определяйте интерфейсы у потребителя зависимости»).

**Barrel Files:**
- None. No `doc.go`, no re-export files; each package is self-contained. Do not add them.

## Layering and Scope Rules (from `.agents/rules/` and `AGENTS.md`)

- Dependency direction: `cmd` → transport → service/use case → repository; inner layers must not import outer ones; domain logic must not depend on HTTP, SQLite, logger, or config (`.agents/rules/architecture.md`).
- Solution-order preference when implementing (`.agents/rules/coding.md`): (1) don't add the capability, (2) reuse existing code/pattern, (3) standard library, (4) already-installed dependency, (5) minimal new code, (6) new dependency only with explicit approval. Adding unrequested capabilities, wrappers, or "future-proofing" code is ЗАПРЕЩЕНО.
- Change only lines required by the agreed task; report unrelated problems instead of fixing them.
- Branch naming: `feature/<task>`, `fix/<task>`, `chore/<task>`; Conventional Commits; one commit = one logical change package. Never commit/push without explicit user request (root `AGENTS.md`, «Правила работы»).
- Never modify `.env`, `.git`, or lock files without separate approval (root `AGENTS.md`).

---

*Convention analysis: 2026-11-13*
