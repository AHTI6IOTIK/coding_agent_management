---
last_mapped_commit: f0a723bf3eb5e105f3b2d82a030d28485e7e13a8
last_mapped_at: 2026-11-13
---
# Testing Patterns

**Analysis Date:** 2026-11-13

## Test Framework

**Runner:**
- Go standard `testing` package for unit tests (Go 1.26.0, `go.mod:3`).
- Playwright for E2E tests: `@playwright/test` 1.64.1 (`package.json:7`).
- Config: `.golangci.yaml` (lint), `playwright.config.ts` (E2E). No test-specific Go config file.

**Assertion Library:**
- Plain `if` + `t.Errorf`/`t.Fatalf` — no assertion framework. Go 1.26 stdlib only; do not add testify/gomega.
- Playwright: `expect(...)` from `@playwright/test` (`tests/e2e/calculator.spec.ts:17,19`).

**Run Commands:**
```bash
go test --race ./...                        # Run all unit tests (mandatory baseline command)
go test --race ./internal/httpserver        # Run HTTP package tests only (per internal/httpserver/AGENTS.md)
npm run test:e2e                            # Run Playwright E2E suite
golangci-lint run ./...                     # Lint (run with tests before finishing a change)
```

## Test File Organization

**Location:**
- Unit tests are co-located with the code under test in the same package directory: `internal/config/config_test.go`, `internal/httpserver/server_test.go`.
- Functional and integration tests go into `tests/`; integration tests use testcontainers (root `AGENTS.md`, «Проверки»).
- E2E specs live in `tests/e2e/` (`playwright.config.ts:4`, `testDir: "./tests/e2e"`).

**Naming:**
- `Test<Subject><Behavior>` — `TestAppRoutes`, `TestNewAppLogsVersion`, `TestCalculatorPage`, `TestMetricsHandler` (`internal/httpserver/server_test.go`); `TestLoadDefaults`, `TestLoad`, `TestLoadRejectsDuplicatePorts`, `TestLoadRequiresAppRegion`, `TestLoadRejectsEmptyAppRegion` (`internal/config/config_test.go`).
- Table-case subtests get descriptive lowercase names via `t.Run(testCase.name, ...)`.

**Structure:**
```
internal/<package>/
├── <file>.go        # production code
└── <file>_test.go   # same-package tests (white-box allowed)
tests/
└── e2e/
    └── <feature>.spec.ts
```

## Test Structure

**Suite Organization:**
- Table-driven tests are the standard for multi-scenario coverage (root `AGENTS.md`, «Для наборов сценариев используйте table-driven тесты»). Canonical example from `internal/httpserver/server_test.go:13-56`:

```go
func TestAppRoutes(t *testing.T) {
	app := NewApp("v1.0.0")

	testCases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "hello", method: http.MethodGet, path: "/", wantStatus: http.StatusOK, wantBody: `{"message":"hello world"}`},
		// ... one row per scenario, including happy path, validation errors, edge cases
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(context.Background(), testCase.method, testCase.path, strings.NewReader(testCase.body))
			if testCase.body != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()

			app.ServeHTTP(response, request)

			if response.Code != testCase.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, testCase.wantStatus)
			}
			if response.Body.String() != testCase.wantBody+"\n" {
				t.Errorf("body = %q, want %q", response.Body.String(), testCase.wantBody+"\n")
			}
		})
	}
}
```

- HTTP tests exercise the full `*echo.Echo` app through `httptest.NewRequestWithContext` + `httptest.NewRecorder` + `app.ServeHTTP(...)` — no live listener, no network.
- Config tests use `t.Setenv` for env setup (`internal/config/config_test.go:26-30`) and a custom `unsetEnv(t, name)` helper (with `t.Helper()` and `t.Cleanup` restore) to guarantee variables are absent (`internal/config/config_test.go:83-97`).

**Patterns:**
- Setup: construct app/config per test function (`NewApp("v1.0.0")`), not per subtest.
- Teardown: `t.Cleanup(func() { ... })` to restore global state (default logger, env vars) — `internal/httpserver/server_test.go:62`, `internal/config/config_test.go:90-96`.
- Assertions: `got != want` comparisons with `t.Errorf`; `t.Fatalf` only when the test cannot continue (e.g., setup error — `internal/config/config_test.go:17`).
- Assertion message format: `"got = %v, want %v"` style with the actual values, e.g. `t.Errorf("status = %d, want %d", ...)`.
- DB-touching tests MUST assert expected data changes explicitly, not just HTTP status or absence of error (root `AGENTS.md`, «Тесты с БД должны явно проверять ожидаемые изменения данных»).
- Per `.agents/rules/testing.md`: capture a `go test --race ./...` baseline before product changes and repeat it after; a failing test stops work for root-cause analysis — never weaken or delete a test to pass.

## Mocking

**Framework:** None. No mock library (no testify/mock, no mockery). The codebase relies on the app abstraction boundary instead of mocks.

**Patterns:**
- HTTP layer: no mocking — handlers are tested against the real Echo app with an injected `httptest` request/recorder pair (`internal/httpserver/server_test.go:40-46`).
- Globals: swap real dependencies (default slog logger) with test doubles and restore via `t.Cleanup`:

```go
var output bytes.Buffer
previousLogger := slog.Default()
slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
t.Cleanup(func() { slog.SetDefault(previousLogger) })
```
(`internal/httpserver/server_test.go:59-62`)

- Environment: tests mutate the real process environment with `t.Setenv` / restore-on-cleanup helpers rather than mocking `os` calls (`internal/config/config_test.go`).
- Future DB tests should use a real database (SQLite in-memory for E2E per `playwright.config.ts:10`; testcontainers for integration tests per root `AGENTS.md`) — do not mock the repository layer unless a consumer-defined interface already exists (`.agents/rules/architecture.md`: interfaces are defined at the consumer).

**What to Mock:**
- External side effects the test cannot control: default loggers, process env, time/signal sources if they enter the code under test.
- Integration boundaries beyond the process (network services) — via testcontainers, not hand mocks.

**What NOT to Mock:**
- The Echo framework, validator, or Prometheus handler — always exercise them for real (`TestMetricsHandler` hits the real `promhttp.Handler()` and asserts `go_goroutines` is present — `internal/httpserver/server_test.go:87-99`).
- Pure functions: `calculate()` behavior is fully covered through HTTP-level table cases (NaN for empty/fractional/negative input — `internal/httpserver/server_test.go:29-31`).

## Fixtures and Factories

**Test Data:**
- Inline literals in table rows; raw JSON bodies as backtick strings with expected response bodies exactly matched (including Echo's trailing `\n` — `internal/httpserver/server_test.go:51-53`).
- No builder/factory pattern; construct request structs/JSON inline.

**Location:**
- No `testdata/` or fixture directories. Static content used by handlers is embedded from the package itself (`internal/httpserver/calculator.html`), so tests assert against it via the served response (`TestCalculatorPage` checks for `id="expression"`, `Calculate`, `role="status"` fragments — `internal/httpserver/server_test.go:71-85`).

## Coverage

**Requirements:** None enforced (no coverage thresholds configured in `.golangci.yaml`, `playwright.config.ts`, or CI config).

**View Coverage:**
```bash
go test --race -cover ./...
```

## Test Types

**Unit Tests:**
- Same-package `*_test.go` next to the code; cover happy path, invalid input, boundary conditions (e.g., 1025-char message exceeding `max=1024` — `internal/httpserver/server_test.go:34`), and config validation errors (`internal/config/config_test.go:54-81`).
- Always run with `--race` (root `AGENTS.md`).

**Integration Tests:**
- Belong in `tests/` and use testcontainers per root `AGENTS.md`. Not yet present in the repo — when adding the first one, follow `.agents/rules/testing.md` baseline/TDD cycle and assert actual data changes in the DB.

**E2E Tests:**
- Playwright, spec per feature in `tests/e2e/`. The server is started automatically by `playwright.config.ts` `webServer` with dedicated env (`APP_PORT=18080`, `APP_REGION=e2e`, `DB_PATH=:memory:`) and health-checked via `/health`. `reuseExistingServer: !process.env.CI`.
- E2E pattern (from `tests/e2e/calculator.spec.ts:3-21`): collect `console`/`pageerror` events, interact via accessible-role locators (`getByLabel`, `getByRole`), assert with `expect(...)`, capture a screenshot into `testInfo.outputPath(...)`, and finally assert zero console/page errors:

```typescript
const consoleErrors: string[] = [];
const consoleErrorsGuard = page.on("console", (message) => {
  if (message.type() === "error") consoleErrors.push(message.text());
});
...
expect(consoleErrors).toEqual([]);
expect(pageErrors).toEqual([]);
```

## Common Patterns

**Async Testing:**
```go
// Go tests are synchronous; concurrency safety is checked by the runner flag:
// go test --race ./...
```
For E2E, Playwright auto-waits; use `expect(locator).toHaveText(...)` rather than sleeps (`tests/e2e/calculator.spec.ts:17`).

**Error Testing:**
```go
// Validation/parse failures return one fixed body — assert both status and body:
{name: "invalid echo", method: http.MethodPost, path: "/echo", body: `{}`, wantStatus: http.StatusBadRequest, wantBody: `{"message":"invalid request"}`},
```
(`internal/httpserver/server_test.go:32-34`). For config errors:

```go
_, err := Load()
if err == nil {
	t.Fatal("Load() error = nil, want error for duplicate ports")
}
```
(`internal/config/config_test.go:59-62`)

**Security-in-tests requirement (`.agents/rules/security.md`):** when writing or changing tests, cover scenarios of unauthorized access and cross-client data access whenever such data or access exists in the contract; verify sensitive data never reaches API responses or logs.

**Mandatory minimum (`.agents/rules/coding.md`):** any non-trivial logic (branching, loops, parsing, data transformation, external access, money, security) MUST have at least one test proving the new behavior; bug fixes MUST include a test reproducing the defect when it can be reproduced stably. External input requires valid, invalid, and boundary cases.

## TDD Process (from `.agents/rules/testing.md`)

For new behavior and bug fixes, follow the short Red–Green–Refactor cycle:
1. **Red** — add the failing test and confirm it fails for the expected reason.
2. **Green** — make the minimal correct change until the test passes.
3. **Refactor** — stabilize the code without changing behavior; re-run tests after each step.

Record the actual commands executed, pre-change baseline failures, post-change results, and unverified areas in the report. Never mask failures by weakening or deleting tests.

---

*Testing analysis: 2026-11-13*
