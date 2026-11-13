---
last_mapped_commit: f0a723bf3eb5e105f3b2d82a030d28485e7e13a8
last_mapped_at: 2026-11-13
---
# Codebase Structure

**Analysis Date:** 2026-11-13

## Directory Layout

```text
coding_agent_management/
├── cmd/
│   └── server/            # Binary entry point (only cmd package)
│       └── main.go
├── internal/              # Product Go code (not importable externally)
│   ├── config/            # Env config loading + validation
│   │   ├── config.go
│   │   └── config_test.go
│   └── httpserver/        # Echo transport layer
│       ├── AGENTS.md      # Package-level agent rules (handler naming/Go-doc)
│       ├── server.go
│       ├── server_test.go
│       └── calculator.html
├── tests/
│   └── e2e/               # Browser E2E (Playwright)
│       └── calculator.spec.ts
├── docs/                  # Project documentation
│   └── about.md
├── .agents/               # Agent rules and skills
│   ├── hooks/             # Empty placeholder
│   ├── rules/             # Change-boundary + engineering rules (md)
│   └── skills/write-tests/
├── .opencode/             # OpenCode agent runtime config (committed)
│   ├── agent/             # User-defined agent persona (single file)
│   ├── agents/            # GSD subagent personas (compact/full pairs)
│   ├── command/           # User commands (review-pr.md)
│   ├── commands/          # GSD slash commands (gsd-*.md)
│   ├── skills/            # GSD skill bundles (gsd-*/)
│   ├── plugin/            # Project plugins: go-quality.ts, protect-files.ts
│   ├── plugins/           # Installed plugins: gsd-core.js, gsd-statusline
│   ├── hooks/             # GSD lifecycle hooks (js/sh)
│   ├── scripts/           # GSD maintenance scripts
│   ├── test/              # Plugin tests (go-quality.test.ts, protect-files.test.ts)
│   ├── gsd-core/          # Installed GSD core (workflows, templates, bin)
│   ├── gsd-file-manifest.json
│   ├── gsd-install-state.json
│   └── opencode.json      # Main agent config (permissions, MCP, plugins)
├── .planning/
│   └── codebase/          # GSD codebase map (CONVENTIONS.md, STACK.md, ARCHITECTURE.md, STRUCTURE.md)
├── agent_work_verification/  # Evidence artifacts per task (screenshots, logs)
├── test-results/          # Playwright output (ignored)
├── .playwright-mcp/       # MCP browser artifacts (ignored)
├── AGENTS.md              # Root agent instructions (Go conventions, rules)
├── README.md
├── go.mod / go.sum        # Go module `coding_agent_management`, Go 1.26
├── package.json           # Node: only Playwright e2e tooling
├── playwright.config.ts   # E2E config, boots the Go server itself
└── .golangci.yaml         # Linter config (v2)
```

## Directory Purposes

**`cmd/server/`:**
- Purpose: the single executable's composition root.
- Contains: process lifecycle only — no business logic belongs here.
- Key files: `cmd/server/main.go`

**`internal/config/`:**
- Purpose: the only place that reads environment variables.
- Contains: `Config` struct, `Load()`, `port()`/`value()` helpers.
- Key files: `internal/config/config.go`

**`internal/httpserver/`:**
- Purpose: all HTTP behavior: routes, handlers, DTOs, validation, calculation, embedded UI.
- Contains: Echo app, named handlers, embedded `calculator.html`, package rules in `internal/httpserver/AGENTS.md`.
- Key files: `internal/httpserver/server.go`

**`tests/e2e/`:**
- Purpose: Playwright browser tests against the real server (config boots it via `webServer` in `playwright.config.ts`).
- Contains: `tests/e2e/calculator.spec.ts`.
- Per root `AGENTS.md`: heavier integration tests belong in `tests/` (testcontainers); only the e2e spec lives there today.

**`.agents/rules/`:**
- Purpose: mandatory change-boundary and engineering rules (`architecture.md`, `coding.md`, `security.md`, `testing.md`, `change-planning.md`); read `.agents/rules/README.md` before editing product files.

**`.opencode/`:**
- Purpose: agent runtime configuration. Mixed-origin: project-authored files (`opencode.json`, `plugin/`, `command/`, `agent/`, `test/`) plus installed GSD tooling (`commands/`, `skills/`, `hooks/`, `agents/`, `gsd-core/`, manifests).

**`docs/`, `.planning/codebase/`, `agent_work_verification/`:** documentation, GSD planning artifacts, and task evidence respectively.

## Key File Locations

**Entry Points:**
- `cmd/server/main.go`: binary entry; config → DB → 3 servers → graceful shutdown.

**Configuration:**
- `internal/config/config.go`: env → validated `Config` (defaults: `APP_PORT=8080`, `DB_PATH=gallery.db`, `METRICS_PORT=9090`, `PPROF_PORT=6060`; `APP_REGION` required).
- `.golangci.yaml`: lint/formatter setup.
- `playwright.config.ts`: e2e server boot env (`APP_PORT=18080`, `DB_PATH=:memory:`).
- `.opencode/opencode.json`: agent permissions, MCP servers, plugins.

**Core Logic:**
- `internal/httpserver/server.go`: routes (`server.go:38-54`), `echoMessage` (`server.go:83`), `calculate` (`server.go:101`), metrics/pprof handlers (`server.go:68-81`).

**Testing:**
- `internal/httpserver/server_test.go`: table-driven HTTP tests via `httptest` + `app.ServeHTTP`.
- `internal/config/config_test.go`: config validation tests.
- `tests/e2e/calculator.spec.ts`: browser e2e with console-error assertions.

**Docs & Rules:**
- `AGENTS.md` (root), `internal/httpserver/AGENTS.md` (package), `.agents/rules/README.md`, `docs/about.md`, `README.md`.

## Naming Conventions

**Files:**
- Go: lowercase/snake_case matching package purpose — `config.go`, `server.go`, `main.go`; tests co-located as `*_test.go`.
- Embedded assets live inside the package directory — `internal/httpserver/calculator.html`.
- TypeScript specs: kebab/camelCase `.spec.ts` under `tests/e2e/`.
- Agent docs: package-scoped rules as `AGENTS.md` next to the package.

**Directories:**
- Go: `cmd/<binary>`, `internal/<layer>` — single-word lowercase (`config`, `httpserver`).
- GSD: `gsd-<command-name>` directories under `.opencode/skills/` mirroring `gsd-<command-name>.md` under `.opencode/commands/`.

**Packages:**
- Short, idiomatic, singular nouns: `config`, `httpserver`, `main`.
- Module path: `coding_agent_management` (`go.mod:1`) — internal imports use the full path: `coding_agent_management/internal/config`.

## Where to Add New Code

**New HTTP endpoint:**
- Handler: named function + Go-doc (method, path, purpose) in `internal/httpserver/server.go`; register it in `NewApp()` (`server.go:43-51`). Follow `internal/httpserver/AGENTS.md` strictly.
- DTO: `xxxRequest`/`xxxResponse` with validator tags in the same file.
- Unit test: extend the table-driven `TestAppRoutes` in `internal/httpserver/server_test.go` (status + exact body), plus dedicated cases for validation failures.
- E2E (if browser-visible): new spec in `tests/e2e/` following `calculator.spec.ts`.

**New configuration variable:**
- Add to `Config` in `internal/config/config.go` using `value()`/`port()` helpers; add validation in `Load()`; extend `internal/config/config_test.go`.

**New business/persistence logic:**
- New package under `internal/` (e.g., `internal/gallery`); define consumer-side interfaces per `.agents/rules/architecture.md`; wire the `*sql.DB` handle from `cmd/server/main.go:36` through constructors — do not open the DB again inside the package.

**New frontend page:**
- HTML file next to `internal/httpserver/server.go` + `//go:embed` var, route via named handler.

**New agent rules / skills:**
- Rules: `.agents/rules/*.md` + reference from `.agents/rules/README.md`.
- Skills: `.agents/skills/<name>/SKILL.md` (path already wired in `.opencode/opencode.json` → `skills.paths`).
- Agent command: `.opencode/command/<name>.md` (user-level commands; `gsd-*` system commands live in `.opencode/commands/`).
- Plugin: `.opencode/plugin/<name>.ts`, tests in `.opencode/test/<name>.test.ts`.

**Documentation:**
- Project docs: `docs/`; planning artifacts: `.planning/`.

## Special Directories

**`.planning/`:**
- Purpose: GSD planning artifacts including this codebase map.
- Generated: No (agent-authored). Committed: Yes.

**`.opencode/`:**
- Purpose: agent runtime; `.opencode/gsd-core/`, `.opencode/hooks/`, `.opencode/skills/`, `.opencode/commands/`, `.opencode/agents/` are managed by the GSD installer (`gsd-file-manifest.json` records checksums).
- Generated: GSD parts — yes (do not hand-edit; reinstall via gsd-core). Project parts (`opencode.json`, `plugin/`, `command/`, `agent/`, `test/`) — no.
- Committed: Yes (except `.opencode/node_modules/`).

**`agent_work_verification/`:**
- Purpose: evidence artifacts (screenshots, logs) per completed task; referenced by `docs/about.md`.
- Generated: Yes (by agent sessions). Committed: Yes.

**`test-results/`, `.playwright-mcp/`, `node_modules/`, `.DS_Store`, `gallery.db*`:**
- Purpose: ephemeral tooling/runtime output.
- Generated: Yes. Committed: No (ignored by `.gitignore`).

**`.env`:**
- Purpose: local environment values for tooling; NOT read by the Go service.
- Contains secrets — never read, quote, or commit; protected by `.opencode/plugin/protect-files.ts` and `.gitignore`.

---

*Structure analysis: 2026-11-13*
