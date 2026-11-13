# Architecture Research

**Domain:** Go 1.26 HTTP service (Echo + SQLite via `modernc.org/sqlite`) — repository layer for image storage in an existing layered gallery service
**Researched:** 2026-11-13
**Confidence:** HIGH (all repo facts verified against source; driver behavior verified against `gitlab_cznic/sqlite` docs)

## Recommended Architecture

```text
┌────────────────────────────────────────────────────────────────────┐
│                   Composition Root: cmd/server/main.go              │
│  config.Load() → sql.Open("sqlite", DSN) → SetMaxOpenConns /        │
│  SetMaxIdleConns → migrations (storage.Migrate) →                   │
│  storage.NewImageRepository(db) → httpserver.NewApp(version, repo)  │
├────────────────────────────────────────────────────────────────────┤
│                     Transport: internal/httpserver                   │
│  interface ImageRepository (declared HERE — consumer side)          │
│  named handlers + handler factories:                                │
│    uploadImageHandler(repo) / listImagesHandler(repo) /             │
│    viewImageHandler(repo) — pattern per versionHandler (server.go)  │
├────────────────────────────────────────────────────────────────────┤
│                     Storage: internal/storage (NEW)                 │
│  ImageRepository implementation over *sql.DB                        │
│  schema/*.sql via //go:embed — versioned migrations                 │
├────────────────────────────────────────────────────────────────────┤
│                    SQLite (modernc.org/sqlite)                       │
│  WAL mode + busy_timeout via DSN pragmas; images table, BLOB column │
└────────────────────────────────────────────────────────────────────┘
```

The existing codebase map (`.planning/codebase/ARCHITECTURE.md`) records the gap precisely: the `*sql.DB` handle is opened, pinged and closed in `cmd/server/main.go:36-50` but **not injected anywhere**. This recommendation fills that gap without disturbing the three-server layout, loopback-only sidecars, or the `Config` flow.

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| `cmd/server/main.go` (composition root) | Open SQLite, configure connection pool, run versioned migrations, construct the repository, inject it into `NewApp`, own shutdown (`database.Close()` already deferred at main.go:40-44) | `config`, `httpserver`, `storage` |
| `internal/httpserver` | HTTP routes, DTO binding/validation, mapping DTO ↔ internal types; **declares** the `ImageRepository` interface it consumes | `storage` (only via its own interface) |
| `internal/storage` (new) | SQL statements, transactions, migrations, row ↔ entity mapping for `images` | `*sql.DB` only |
| `internal/config` | Unchanged; no new env vars required (all gallery settings are defaults, see Open Questions) | — |

### Interface Location: consumer side (mandated)

`.agents/rules/architecture.md` line 5: *«Определяйте интерфейсы у потребителя зависимости»*. The consumer of the repository is the transport layer, therefore:

```go
// internal/httpserver/server.go (or image.go in the same package)

// ImageRepository persists and retrieves gallery images. Declared at the
// consumer per .agents/rules/architecture.md; implemented in internal/storage.
type ImageRepository interface {
	Save(ctx context.Context, image Image) (int64, error)
	List(ctx context.Context) ([]ImageMeta, error)
	Get(ctx context.Context, id int64) (Image, error)
}

// Image is an internal domain type — NOT a transport DTO
// (anti-pattern "Transport DTOs as domain entities", codebase map).
type Image struct {
	ID          int64
	Filename    string
	ContentType string
	Data        []byte
}
```

**Do NOT** put the interface in `internal/storage` and import it from handlers — that inverts the dependency and contradicts the rule. The transport package never imports `internal/storage`; only `cmd/server/main.go` does.

### Recommended Project Structure

```text
cmd/
└── server/
    └── main.go                  # + pool tuning, migrations call, repo wiring
internal/
├── config/
│   └── config.go                # unchanged
├── httpserver/
│   ├── AGENTS.md                # rules: named handlers + Go-doc comments
│   ├── server.go                # + ImageRepository interface, NewApp signature
│   ├── image.go                 # new: upload/list/view DTOs + named handlers
│   └── image_test.go            # unit tests next to code (root AGENTS.md)
└── storage/
    ├── AGENTS.md                # optional: package rules mirroring httpserver's
    ├── image_repository.go      # *ImageRepository over *sql.DB
    ├── image_repository_test.go # DB tests assert data changes, not just status
    └── schema/
        ├── 001_images.sql       # versioned migration, //go:embed'ed
        └── migrations.go        # Migrate(ctx, db) — CREATE TABLE IF applied
```

**Structure rationale:**

- **`internal/storage`:** one package per data source matches the existing `internal/` granularity (config, httpserver). Nothing outside `cmd` imports it.
- **`image.go` in `httpserver`:** gallery handlers grouped next to `server.go` rather than growing `server.go`; keeps `internal/httpserver/AGENTS.md` naming/Go-doc conventions localized.
- **`schema/`:** `.agents/rules/architecture.md` line 11 requires *«версионируйте миграции»*. There is no migration tooling today — this is the minimal compliant mechanism (embedded numbered SQL files + an applied-migrations table). No third-party migration library is needed or recommended for this milestone.

### Handler Wiring Without Closures

`internal/httpserver/AGENTS.md` bans inline closures as handlers; the established pattern for capturing startup values is the factory (`versionHandler(version)` at `internal/httpserver/server.go:62-66`). Apply it to the repository:

```go
func NewApp(version string, images ImageRepository) *echo.Echo {
	// ...
	app.POST("/images", uploadImageHandler(images))
	app.GET("/images", listImagesHandler(images))
	app.GET("/images/:id", viewImageHandler(images))
	// ...
}

// uploadImageHandler handles POST /images and stores a new gallery image.
func uploadImageHandler(images ImageRepository) echo.HandlerFunc {
	return func(context echo.Context) error { /* named, testable */ }
}
```

Each handler is still a named function with a Go-doc comment stating method, path, and purpose, exactly as `calculatorHandler`/`versionHandler` do. **Deviation from current code, flagged:** `NewApp` is already called by tests with one argument; every test site and `cmd/server/main.go:55` must be updated in the same change (test changes are part of the approved package, not an afterthought). Alternative rejected: a dependencies struct parameter — overkill while there is a single repository.

## Pattern 1: Repository receiving `*sql.DB`

**What:** concrete `*ImageRepository` struct holding `*sql.DB`; interface declared by the consumer.
**When to use:** every persistence feature in this service; the codebase map explicitly requires "a repository abstraction … pass the handle down — do not re-open the DB in handlers".
**Trade-offs:** one extra indirection layer; in exchange handlers become unit-testable with a fake, and SQLite stays replaceable.

```go
// internal/storage/image_repository.go
type ImageRepository struct{ database *sql.DB }

func NewImageRepository(database *sql.DB) *ImageRepository {
	return &ImageRepository{database: database}
}

func (repository *ImageRepository) Get(ctx context.Context, id int64) (httpserver.Image, error) {
	// SELECT ... FROM images WHERE id = ? — ctx first param per rules
}
```

**Cycle note:** if `internal/storage` importing `internal/httpserver` for the `Image`/`ImageRepository` types feels wrong, hoist the interface and types into a small `internal/gallery` (domain) package that both import — still consumer-agnostic, no cycles. For this milestone, the two-package version above is the simplest compliant shape; `storage`→`httpserver` import is safe because the codebase map confirms no `internal` cross-imports exist today except through `cmd`. **Flag for planner: pick one at plan time and keep it fixed** — the mapping functions differ.

## Pattern 2: BLOB storage in SQLite (recommended over filesystem+metadata)

**What:** image bytes stored as a `BLOB` column; no files on disk.
**When to use:** here — always, for this service.
**Trade-offs:** SQLite BLOBs are fine up to tens of MB per row; filesystem+metadata is the classic recipe for large media libraries, but **not** here.

**Why BLOB wins in this repo — verified facts:**

1. `playwright.config.ts` boots the server for e2e with `DB_PATH=:memory:` (also recorded in the codebase map, Entry Points section). A filesystem-backed store would either break e2e or need a second config knob.
2. The service is a single binary with everything else embedded (`//go:embed calculator.html`, `server.go:17-18`); adding an on-disk media directory introduces an undocumented runtime filesystem dependency that `.agents/rules/architecture.md` line 8 («Получайте конфигурацию извне») would force into `Config`.
3. `.planning/codebase/CONCERNS.md` already flags storage concerns; keeping all state behind `DB_PATH` preserves the one-knob deployment.

```sql
-- internal/storage/schema/001_images.sql
CREATE TABLE IF NOT EXISTS images (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    filename     TEXT    NOT NULL,
    content_type TEXT    NOT NULL,
    size_bytes   INTEGER NOT NULL,
    data         BLOB    NOT NULL,
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
```

**Guard rail (required by `.agents/rules/security.md` data-isolation mindset):** enforce a maximum upload size at the transport edge (`echo` body limit / `http.MaxBytesReader`), because a 30-byte DB check after the body was read still lets an attacker exhaust memory.

## Pattern 3: SQLite concurrency configuration at the composition root

**What:** the only place SQLite concurrency is configured is `cmd/server/main.go`, via DSN pragmas and pool settings. Handlers and the repository never tune anything.
**When to use:** at startup, before constructing the repository.

**Verified driver facts** (`gitlab_cznic/sqlite`, driver `v1.61.1` per `go.mod` — check exact pin):

- DSN parameters are applied in a fixed order; `_busy_timeout` first, then `_pragma` values. Example DSN: `file:gallery.db?_busy_timeout=5000&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)`.
- `*sql.DB` is a pool; physical connections preserve per-connection PRAGMA state across borrowers — so per-connection pragmas via DSN are the correct way (not one-time `Exec` after open, which would hit only one pooled connection).
- Since v1.61.0 the busy handler honors `context` cancellation — long lock waits will not outlive the request context.
- The driver's own Performance documentation explicitly recommends bounding the pool with `SetMaxOpenConns`.

```go
// cmd/server/main.go — after config.Load()
database, err := sql.Open("sqlite", dsn(configuration.DatabasePath))
database.SetMaxOpenConns(1) // see below
database.SetMaxIdleConns(1)
```

**Recommended policy: `SetMaxOpenConns(1)` for this milestone.** Rationale: writes serialize at the SQLite level anyway; one connection removes `SQLITE_BUSY` between pooled connections entirely, keeps ordering simple for upload/list, and is mandatory for `:memory:` (see Anti-Pattern 3). WAL + `busy_timeout` remain set for correctness if the limit is ever raised. Multiple connections would only buy read parallelism, which is not the bottleneck for a personal gallery.

`.agents/rules/architecture.md` line 12 («При работе с SQLite учитывайте ограничения конкурентного доступа и режимы блокировок») is satisfied by: single connection (writer serialization), `busy_timeout` (bounded lock waits), WAL (readers never block the writer once the pool grows), and `context` timeouts on all repository calls (line 6).

## Data Flow

### Request Flow (upload example)

```text
POST /images (multipart form)
    ↓
Echo router → uploadImageHandler(repo)     [named handler, Go-doc]
    ↓ bind + validate + size limit          [transport edge only]
ImageDTO → httpserver.Image                [map DTO → internal type]
    ↓
repo.Save(ctx, image)                      [ctx first, %w wrapping]
    ↓
INSERT INTO images ... in *ImageRepository
    ↓
ID → imageResponse → JSON 201              [map back at transport boundary]
```

**Key data flows:**

1. **Save:** single `INSERT`; no transaction needed for one statement, but any future multi-statement write goes in `sqlx`-free `database/sql` transactions (`.agents/rules/architecture.md` line 11: *«Выполняйте изменения данных в транзакциях»*).
2. **List:** `SELECT id, filename, content_type, size_bytes, created_at FROM images ORDER BY id DESC` — never selects `data` (N+1/bandwidth avoidance, rule line 11: *«исключайте N+1 запросы»*).
3. **View:** `SELECT ... data FROM images WHERE id = ?`; 404 on `sql.ErrNoRows` mapped at the transport edge, never leaking SQL details (`.agents/rules/security.md`: no internals in responses).
4. **Startup:** `main` runs `storage.Migrate(ctx, db)` after `PingContext` (main.go:46-50) and before building servers.

## Anti-Patterns

### Anti-Pattern 1: Inline closures as handlers

**What people do:** `app.POST("/images", func(c echo.Context) error { return images.Save(...) })`.
**Why it's wrong:** banned by `internal/httpserver/AGENTS.md`; no Go-doc contract, not testable in isolation.
**Do this instead:** named function + factory, as in `versionHandler` (`server.go:62-66`).

### Anti-Pattern 2: Opening or querying SQLite in handlers

**What people do:** `sql.Open(...)` inside a handler, or `*sql.DB` passed directly as the handler dependency.
**Why it's wrong:** violates the `cmd → transport → repository` direction (`.agents/rules/architecture.md` line 1); the codebase map's Storage note explicitly forbids re-opening; SQL leaks into transport.
**Do this instead:** one `*sql.DB` in `main`, wrapped by `*ImageRepository`, injected as the `ImageRepository` interface.

### Anti-Pattern 3: `:memory:` + connection pool > 1

**What people do:** keep default pool settings while `DB_PATH=:memory:` (what e2e uses today).
**Why it's wrong:** with `database/sql`, each pooled connection to `:memory:` opens a **separate, empty database** — the handler would write to one connection and read from another. Today this is masked because only `PingContext` touches the DB; the gallery makes it fatal.
**Do this instead:** `database.SetMaxOpenConns(1)` (recommended always for this milestone), or switch e2e to a temp file DB. This is the single most likely silent bug in the whole feature — the planner must pin it in the task list.

### Anti-Pattern 4: Transport DTOs as domain entities

**What people do:** passing `uploadImageRequest` (with `validate` tags) or Echo context values into `internal/storage`.
**Why it's wrong:** codebase map anti-pattern; domain/storage must not depend on HTTP.
**Do this instead:** plain `Image`/`ImageMeta` types in the interface's package; map at the handler.

### Anti-Pattern 5: Silently losing SQLite errors

**What people do:** `_ = db.Exec(...)`, `fmt.Errorf("save failed")`, comparing `err == sql.ErrNoRows` with `==`.
**Why it's wrong:** `errorlint`/`contextcheck` fail; `.agents/rules/architecture.md` line 9.
**Do this instead:** wrap with `%w`, match with `errors.Is(err, sql.ErrNoRows)` (pattern already used in `main.go:96`).

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| 0–1k users | Single binary, single SQLite connection, BLOBs in DB — the recommended design, no changes |
| 1k–100k users | Raise pool for read parallelism (WAL already set), add `LIMIT/OFFSET` pagination to `List`, consider streaming BLOB out instead of full read; still one binary |
| 100k+ users | This is the natural ceiling of SQLite-in-BLOB: move to filesystem/object storage + metadata, or a client-server DB — out of scope, do not design for it now |

**First bottleneck:** memory during upload (whole file buffered) — mitigate with request size limit.
**Second bottleneck:** `List` without pagination — add `LIMIT` from day one, it is one SQL clause.

## Deviations from `.agents/rules/architecture.md` (explicit flags)

1. **No service/use-case layer.** The rule names the direction `cmd → transport → service → repository`, but no service layer exists in this codebase (calculator logic lives in the handler). For upload/list/view CRUD, an intermediate service package would be pass-through abstraction. **Flagged as accepted deviation:** handlers call the repository interface directly; the direction invariant "transport does not depend on storage" is preserved because the interface is transport-owned. If the planner judges otherwise, `internal/gallery` is the insertion point.
2. **BLOB in SQLite instead of filesystem+metadata.** Standard advice for media is filesystem; here overridden by the `:memory:` e2e configuration and single-binary design (justified under Pattern 2).
3. **Migration mechanism is hand-rolled** (embedded SQL + applied-migrations table) rather than a library, to honor the "versioned migrations" rule without a new dependency (root AGENTS.md: no dependencies without agreement).

## Sources

- Repo files (verified): `cmd/server/main.go`, `internal/httpserver/server.go`, `internal/httpserver/AGENTS.md`, `internal/config/config.go` (via codebase map), `.agents/rules/architecture.md`, `.agents/rules/security.md`, root `AGENTS.md`, `.planning/codebase/ARCHITECTURE.md`, `.planning/codebase/CONCERNS.md`.
- Driver documentation (HIGH confidence, `gitlab_cznic/sqlite` / pkg.go.dev mirror): DSN pragma application order, per-connection PRAGMA persistence across pool borrowers, v1.61.0 context-aware busy handler, `SetMaxOpenConns` recommendation.

---
*Architecture research for: gallery image repository layer (brownfield milestone)*
*Researched: 2026-11-13*
