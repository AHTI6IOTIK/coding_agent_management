# Pitfalls Research: Binary Image Upload/Storage/Retrieval on SQLite in Go

**Domain:** Go 1.26 HTTP service (Echo v4.16.3, modernc.org/sqlite v1.61.1) — image gallery: upload + list + view images persisted as BLOBs in SQLite. Brownfield milestone on an existing service; educational, no auth.
**Researched:** 2026-11-13
**Confidence:** HIGH (SQLite locking/concurrency findings cross-verified against driver docs and multiple independent post-mortems; multipart findings verified against `net/http` source and the PicoShare post-mortems, which is a directly comparable "files in SQLite in Go" project)

---

## Critical Pitfalls

### Pitfall 1: `database is locked (5) (SQLITE_BUSY)` because PRAGMAs are per-connection, not per-pool

**What goes wrong:**
Uploads work in single-request tests, then fail intermittently under concurrent load with `SQLITE_BUSY` immediately (no wait). Root cause: `busy_timeout` (like most PRAGMAs in modernc.org/sqlite) applies **per connection**. The codebase opens the DB as `sql.Open("sqlite", configuration.DatabasePath)` with a bare DSN (`cmd/server/main.go:36`), so every pooled connection gets the default `busy_timeout=0`: any contention between a concurrent upload INSERT and any other write fails instantly. Running `db.Exec("PRAGMA busy_timeout=5000")` once at startup configures only whichever connection happened to run it — every connection the pool opens later is unconfigured. This exact bug is documented in multiple real post-mortems (e.g. a project with `SetMaxOpenConns(4)` where 3 of 4 connections kept `busy_timeout=0`).

**Why it happens:**
`database/sql` opens connections on demand; per-connection PRAGMAs set outside the DSN do not propagate. Tests pass because they run sequentially (no contention).

**How to avoid:**
Put every connection-level PRAGMA in the DSN using modernc's `_pragma=` URI syntax so it is applied on **every** connection the pool opens:
`file:gallery.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)`
Note the syntax is modernc-specific (`_pragma=name(value)`), not the cgo driver's `_busy_timeout=5000`. Verify by reading `PRAGMA busy_timeout` back from a *later* pool connection, not the first one.

**Warning signs:**
Errors appear only under parallel load or in `--race` test runs that fire concurrent requests; error surfaces instantly (0s wait) instead of after a timeout.

**Phase to address:**
Phase topic: DB layer bootstrap (schema + connection setup). Must be fixed **before** the first write path exists — retrofitting later means chasing flaky failures.

---

### Pitfall 2: Deferred-transaction upgrade fails instantly with `SQLITE_BUSY_SNAPSHOT` (517) — `busy_timeout` cannot help

**What goes wrong:**
A transaction starts with `BEGIN` (deferred, no lock), does a SELECT (acquires a read snapshot), then tries to INSERT/UPDATE. If another connection committed a write in between, SQLite cannot upgrade the lock and returns `database is locked (517)` **immediately**, regardless of `busy_timeout` — waiting would deadlock. This is the failure mode that survives Pitfall 1's fix and confuses everyone who meets it.

**Why it happens:**
Go's `database/sql` defaults to `BEGIN DEFERRED`. A read-then-write transaction in deferred mode can never be safely upgraded under concurrency.

**How to avoid:**
In modernc.org/sqlite, add `_txlock=immediate` to the DSN so every `BeginTx` issues `BEGIN IMMEDIATE`, acquiring the write lock up front where `busy_timeout` can do its work. For the gallery, the write pattern is small (insert image row + BLOB, insert/delete metadata), so making all transactions immediate is safe and simple. Alternative/adjunct: give the write path a pool with `SetMaxOpenConns(1)` so writes queue in Go instead of colliding in SQLite — see Pitfall 3.

**Warning signs:**
`SQLITE_BUSY` with code 517 in logs, or busy errors on transactions that contain both a SELECT and a write, failing at the write statement rather than at `Begin`.

**Phase to address:**
Phase topic: DB layer bootstrap — same DSN/transaction-design decision as Pitfall 1; lock both in one place.

---

### Pitfall 3: Default journal mode serializes readers behind the single writer

**What goes wrong:**
Without WAL, a long-running write (a multi-MB BLOB INSERT is exactly such a write) blocks **all** readers: concurrent image listing/viewing requests stall or time out while an upload commits. With the default `journal_mode=DELETE`, even reads queue behind the writer.

**Why it happens:**
SQLite defaults are tuned for embedded use, not a concurrent HTTP service.

**How to avoid:**
1. `journal_mode(WAL)` + `synchronous(NORMAL)` in the DSN (WAL is persistent in the file; the others are per-connection — all belong in the DSN anyway per Pitfall 1).
2. Consider the canonical Go-on-SQLite layout for this service: one writer pool with `SetMaxOpenConns(1)` (writes serialize in Go — which SQLite does anyway — and `SQLITE_BUSY` becomes unreachable) and, if needed, a read-only pool (`mode=ro`) with several connections that proceed concurrently with the writer in WAL mode. For this milestone's scale, a single pool with `SetMaxOpenConns(1)` is the simplest correct option; note its caveat: an open transaction monopolizes the one connection, so **keep write transactions short** (single-digit ms) — never hold a transaction across an HTTP round-trip or a slow client read.

**Warning signs:**
Upload endpoint latency correlated with image-list latency; request timeouts during uploads.

**Phase to address:**
Phase topic: DB layer bootstrap (pool configuration); verified via a concurrency integration test in `tests/` (see "Looks Done But Isn't").

---

### Pitfall 4: Multipart upload memory blow-up — `ParseMultipartForm`'s 32 MB default is not the whole story

**What goes wrong:**
`c.FormFile(...)` (Echo's shorthand) calls `r.ParseMultipartForm(32 << 20)` under the hood. Two surprises, both measured in the PicoShare project (files-in-SQLite in Go — directly comparable):
- Buffer reallocation while reading multipart data means reading "32 MB in memory" can transiently allocate **~100 MB**; several concurrent uploads multiply this.
- On older Go HTTP/2 paths, multipart temp resources were not freed automatically unless `r.MultipartForm.RemoveAll()` was called — a per-upload leak.

**Why it happens:**
The default is inherited silently; documentation does not surface the reallocation factor or the cleanup contract.

**How to avoid:**
1. Call `c.Request.ParseMultipartForm(2 << 20)` (2 MB `maxMemory`) explicitly instead of relying on `c.FormFile`'s 32 MB default — measured reduction from ~100 MB to ~10 MB per upload in PicoShare.
2. Since Go 1.21+ the HTTP/1 and HTTP/2 cleanup behavior is consistent, but explicitly `defer`-ing cleanup of the multipart form (or using `MultipartReader` streaming, which never buffers whole parts) removes the whole class of problem.
3. For an educational gallery, prefer the simplest robust route: explicit `ParseMultipartForm` with a small `maxMemory`, read the part with `io.ReadAll(io.LimitReader(file, maxImageBytes+1))`, and enforce the app-level size cap (see Pitfall 6).

**Warning signs:**
RSS grows with the number of uploads, not just their size; memory spikes ~3× the configured `maxMemory` under concurrent uploads.

**Phase to address:**
Phase topic: upload endpoint implementation.

---

### Pitfall 5: No request body limit → a single request can OOM the process

**What goes wrong:**
Echo does **not** install a body-size limit by default (`middleware.BodyLimit` must be added explicitly). The codebase currently validates `Message` with `max=1024` but leaves `Calc` unbounded (`internal/httpserver/server.go:29-31` — already flagged in CONCERNS.md as an unbounded-input risk). Repeating that pattern for uploads means a client can stream gigabytes into a handler before validation runs. Without a limit, `io.ReadAll` accumulates the entire body in RAM; with multipart spillover, oversized parts also land as temp files on disk.

**Why it happens:**
"Add the endpoint first, harden later" — but for a binary-upload endpoint, the limit *is* part of the contract.

**How to avoid:**
1. Add `middleware.BodyLimit("<max-image-size + multipart overhead>")` to the Echo app (or `middleware.BodyLimitWithConfig` scoped per-route if different routes need different limits — note `BodyLimit` on the global stack also caps non-upload routes).
2. Enforce the same cap again in the handler with `io.LimitReader` so the defense does not depend on middleware alone.
3. Return `413 Request Entity Too Large` with a clear message.

**Warning signs:**
Handler accepts a 500 MB "image"; e2e suite has no oversized-upload case.

**Phase to address:**
Phase topic: upload endpoint implementation (middleware + handler limits together).

---

### Pitfall 6: Content-Type spoofing — client-declared MIME type is attacker-controlled

**What goes wrong:**
Storing the `Content-Type` header from the upload and serving it back verbatim lets a client upload an HTML/SVG file that the browser renders as a web page (stored XSS) or a polyglot that sniffs differently downstream. "It's just an image gallery, no auth" makes this *more* dangerous, not less: anyone can upload.

**Why it happens:**
The client header is the most visible type signal, so it gets stored by default.

**How to avoid:**
1. Sniff the actual bytes with `http.DetectContentType` (stdlib, images + a few others) or the more thorough `gabriel-vasile/mimetype` — note this library is **already an indirect dependency** in `go.mod:16`, so a direct `require` costs nothing new.
2. Whitelist only real image types (e.g. `image/png`, `image/jpeg`, `image/gif`, `image/webp`); reject everything else with `415 Unsupported Media Type`.
3. Store the *sniffed* type, never the client's.
4. When serving: set the stored sniffed Content-Type **and** `X-Content-Type-Options: nosniff` so the browser cannot second-guess; do not serve `image/svg+xml` at all (it can carry scripts).

**Warning signs:**
Upload handler reads any MIME value from the multipart part header; no test uploads an HTML file renamed to `.png`.

**Phase to address:**
Phase topic: upload endpoint (validation) — must land with the endpoint, not as a later hardening pass.

---

## Moderate Pitfalls

### Pitfall 7: SQLite BLOB size ceiling and per-row BLOB memory on read

**What goes wrong:** SQLite's default `SQLITE_MAX_LENGTH` is ~1 GB per BLOB (configurable at build time only downward at runtime). More practically, `SELECT data FROM images WHERE id=?` materializes the whole BLOB in Go memory before the handler can stream it — a 50 MB image costs 50 MB of RAM per concurrent viewer, and the multipart-write side similarly inflates the transaction's page cache/WAL usage.

**Prevention:** Enforce a modest app-level cap for this milestone (e.g. 5–10 MB per image) — it sidesteps both the 1 GB ceiling and most read-memory pressure. Always `LIMIT 1`, select the BLOB column alone in its own query (not joined with metadata), and set `Content-Length` from the stored size column so you don't need the BLOB's length separately. For this scale, do **not** build a chunked-BLOB abstraction — that is over-engineering the milestone; document the cap as a product decision instead.

### Pitfall 8: Long transactions monopolize the single write connection

**What goes wrong:** With the recommended `SetMaxOpenConns(1)` write pool, any transaction holds the *only* connection until commit. A transaction that wraps a slow client upload stream (e.g. `INSERT` per chunk inside `tx` where chunks are buffered in memory until commit — the PicoShare issue #284 pattern) starves all other writers and inflates RAM by file size.

**Prevention:** Read the upload fully (bounded by the cap) *first*, then open a short transaction for metadata + BLOB insert. One INSERT, one commit, done. Never pass a `*sql.Tx` into a streaming writer.

### Pitfall 9: `:memory:` e2e database hides every SQLite-file pitfall

**What goes wrong:** The Playwright config boots the server with an in-memory SQLite (`playwright.config.ts:10`). `:memory:` never exercises WAL, `busy_timeout`, file locking, or multi-connection behavior — the exact things Pitfalls 1–3 are about. The e2e suite will stay green while the file-backed production path is broken.

**Prevention:** Per `AGENTS.md`, DB-behavior guarantees live in **integration tests in `tests/` via testcontainers** against a temp **file**-backed DB: (a) N concurrent upload+list requests assert zero `SQLITE_BUSY` errors and correct final row count; (b) pragma readback test (WAL/busy_timeout from a late pool connection). Keep e2e on `:memory:` for speed; treat it as UI-smoke only.

### Pitfall 10: `--race` test runs with concurrent BLOB traffic are slow and memory-hungry

**What goes wrong:** The race detector adds 5–10× memory overhead and 2–20× slowdown. A concurrency test that uploads several MB images across goroutines under `go test --race ./...` (mandated by `AGENTS.md`) can balloon CI/dev memory and runtime, tempting developers to shrink the concurrency test until it proves nothing.

**Prevention:** In concurrency tests use tiny payloads (a few hundred bytes of synthetic bytes, not real JPEGs) — `SQLITE_BUSY` reproduction depends on concurrency and transaction shape, not payload size. If a race appears, the likely real bug is sharing `[]byte` returned from `rows.Scan` across goroutines or retaining it after rows are closed — scan into a fresh slice per request and never cache the returned slice.

### Pitfall 11: modernc.org/sqlite is slower on writes than cgo drivers — and the family upgrades break

**What goes wrong:** Pure-Go SQLite (WASM/translated C) has measurably slower insert throughput than `mattn/go-sqlite3`; plus `modernc.org/*` upgrades have historically carried breaking changes (already noted in CONCERNS.md). A design that issues many small INSERTs per upload amplifies both.

**Prevention:** Keep the write path to **one INSERT per upload** (single row with metadata + BLOB). Pin the version (`v1.61.1` is already exact in `go.mod`). Do not accept this as a reason to switch drivers mid-milestone — the pure-Go driver was chosen to avoid cgo; note the trade-off instead.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Trusting client `Content-Type` instead of sniffing | 5 lines less code | Stored-XSS vector; wrong rendering forever for stored rows | Never — sniffing is one stdlib call |
| Skipping `middleware.BodyLimit`, validating size only after full read | Endpoint "just works" in dev | OOM DoS on first oversized request | Never |
| No DSN pragmas (bare `sql.Open` as today) | Zero config effort | Guaranteed `SQLITE_BUSY` flakiness under load; retrofit is a debugging session | Never — do it at DB bootstrap |
| Storing images as files on disk with paths in SQLite instead of BLOBs | Better for large files | Diverges from the stated product decision (persisted *in* SQLite); backup/consistency story split | Out of scope unless the size cap decision changes (>50 MB images) |
| Single global `*sql.DB` with no write/read split | Simplest wiring | Fine at this scale; only matters when read latency under upload load is observed | Acceptable for this milestone — document the scaling path |
| Hand-rolled MIME sniffing via file extension | Trivial | Extension is spoofable by definition | Never |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Echo v4 multipart | `c.FormFile()` with its implicit 32 MB `maxMemory` | Explicit `c.Request.ParseMultipartForm(smallMaxMemory)` + `io.LimitReader` |
| Echo middleware | Assuming `echo.New()` gives you Recover/limits (it gives neither) | Add `middleware.Recover()` (already flagged missing in CONCERNS.md) and `middleware.BodyLimit` when adding the upload route |
| modernc.org/sqlite DSN | Using `mattn`-style params (`_busy_timeout=5000`, `_journal_mode=WAL`) — silently ignored | modernc syntax: `_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)`, plus `_txlock=immediate` |
| `database/sql` pool | `db.Exec("PRAGMA ...")` after open, expecting it to stick | PRAGMAs are per-connection; only DSN params reach every pooled connection |
| SQLite locking | Treating `SQLITE_BUSY` as a bug to suppress with retries-only | Retry-on-busy is the last resort; busy_timeout + WAL + single writer + immediate tx are the design |
| testcontainers integration tests | Testing against `:memory:` and calling it DB coverage | Temp file-backed DB, real WAL/locking; `:memory:` is UI-smoke only |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Whole-BLOB SELECT materialized per view | RSS ∝ concurrent viewers × image size | Cap image size; select BLOB alone; set `Content-Length` from metadata column | ~10+ concurrent viewers of multi-MB images |
| Multipart `maxMemory=32MB` default | ~100 MB transient RSS per upload | Explicit small `maxMemory` (2 MB) | ~3–5 concurrent uploads |
| Transaction held across slow operations | All uploads queue behind one; possible timeouts | Read/upload fully → then short single-INSERT tx | First slow/keep-alive-holding upload |
| No WAL, default DELETE journal | List/view latency spikes during uploads | `journal_mode(WAL)` in DSN | First concurrent upload + view |
| Many small INSERTs per upload (chunked rows) | Slow uploads; WAL growth; driver slowness amplified | One row, one INSERT | Immediately, but invisible until several uploads |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Serving user-uploaded bytes with client-supplied Content-Type | Stored XSS (HTML/SVG served inline), content sniffing abuse | Sniff bytes; whitelist image types only; `X-Content-Type-Options: nosniff`; never serve `image/svg+xml` |
| No upload size cap | Trivial OOM/disk-fill DoS on an unauthenticated endpoint | `middleware.BodyLimit` + handler `LimitReader` + app-level cap stored in one constant |
| Using client filename for storage or URLs | Path traversal if ever persisted to disk; header injection via Content-Disposition | Generate server-side IDs (UUID — `google/uuid` already in the dep tree); never echo client filename into headers |
| Reflecting upload errors with user input embedded | Reflected XSS in error pages | Echo JSON errors only; fixed message strings |
| Unauthenticated write endpoint bound to all interfaces | Anyone can fill the DB (already the documented posture for `/echo`, CONCERNS.md §Security) | Document the loopback/dev-only assumption in config comments; size cap is the practical mitigation for this milestone |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| No client-side file-size/type pre-check | Browser uploads 200 MB, then gets 413 after full transfer | Pre-check size/type in the form JS before submitting; server remains the authority |
| Generic `400` on rejected upload ("invalid request" — the existing calculator pattern) | User can't tell why (too big? wrong type?) | Distinct status + message per rejection reason: `413` size, `415` type, `400` missing file |
| Double-submit on slow uploads | Duplicate images in the gallery | Disable submit while in flight; idempotency is not required for this milestone |
| Upload success but image never verified viewable | Broken page discovered later | e2e case: upload → list → GET image → assert bytes/status match |

## "Looks Done But Isn't" Checklist

- [ ] **Upload endpoint:** Often missing a concurrency integration test — verify N parallel uploads against a *file*-backed testcontainers DB produce zero `SQLITE_BUSY` and the exact expected row count (per `AGENTS.md`: DB tests must assert data changes, not just HTTP statuses).
- [ ] **DSN configuration:** Often missing pragma readback — verify `journal_mode`, `busy_timeout` read from a pool connection opened *after* startup, not the first one.
- [ ] **Content-type validation:** Often missing the spoofing test — verify uploading HTML bytes named `x.png` is rejected with `415`.
- [ ] **Size limit:** Often missing the boundary test — verify `cap` bytes passes and `cap+1` bytes gets `413`, at both middleware and handler levels.
- [ ] **Serve endpoint:** Often missing `Content-Length` + `nosniff` headers — verify response headers in a handler test.
- [ ] **List endpoint:** Often missing the "BLOB not selected" guard — verify the list query does not fetch the `data` column (inspect SQL in a unit test).
- [ ] **e2e:** Often limited to the happy path (current suite has exactly one case) — verify at least one rejected-upload e2e case exists.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Bare DSN shipped (Pitfall 1–3 discovered in prod) | LOW | Add DSN params; WAL persists in the file; redeploy. No data migration needed |
| Spoofed-type rows already stored | LOW | Re-sniff stored bytes in a one-off pass or accept mixed rows; fix handler first so no new bad rows |
| Oversized rows already stored | MEDIUM | Delete offending rows; if DB bloated, `VACUUM INTO` (never copy a live WAL-mode file) |
| Multipart temp-file litter (leaked forms) | LOW | `RemoveAll()` fix; temp files self-clean on restart |
| `SQLITE_BUSY_SNAPSHOT` failures in flight | LOW | Retry whole transaction once; restructure to `BEGIN IMMEDIATE` (design fix, not data fix) |

## Pitfall-to-Phase Mapping

Roadmap phases for this milestone are not yet numbered; mapped to the anticipated phase topics the planner will define.

| Pitfall | Prevention Phase Topic | Verification |
|---------|------------------------|--------------|
| 1–3 (DSN pragmas, WAL, immediate tx, writer pool) | DB layer bootstrap (schema + connection config) | Pragma readback test + concurrent-writers integration test in `tests/` (testcontainers, file-backed) |
| 4–5 (multipart memory, body limit) | Upload endpoint implementation | Body-limit boundary test (`413` at cap+1); memory behavior reviewed in code review |
| 6 (content-type spoofing) | Upload endpoint (validation) | Unit test: HTML-as-`.png` → `415`; serve headers test (`nosniff`) |
| 7 (BLOB size/read memory) | DB schema design (size column, cap decision) | Schema test asserts size column; serve test asserts `Content-Length` |
| 8 (short transactions) | Upload endpoint implementation | Code review checklist item: no `tx` held across I/O |
| 9 (`:memory:` blind spot) | Testing infrastructure | Integration test exists in `tests/` per AGENTS.md; e2e stays `:memory:` |
| 10 (`--race` cost) | Testing infrastructure | Concurrency tests use tiny payloads; `go test --race ./...` runtime stays bounded |
| 11 (driver perf/pinning) | DB layer bootstrap | `go.mod` exact pin; one-INSERT-per-upload design in review |

## Planner Checklist (distilled for plan pre-emption)

1. DSN at `cmd/server/main.go:36` must become: `file:<path>?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)&_txlock=immediate` — not `db.Exec` PRAGMAs.
2. Decide single pool (`SetMaxOpenConns(1)`) vs read/write split — single pool is sufficient for this milestone; document the scaling path.
3. Upload handler: explicit small `ParseMultipartForm` → `io.LimitReader` → sniff bytes (`gabriel-vasile/mimetype` or stdlib) → whitelist image types → short tx, one INSERT (metadata + BLOB + size).
4. `middleware.BodyLimit` + one shared `maxImageBytes` constant reused by middleware, handler, and tests.
5. Serve handler: `SELECT data ... LIMIT 1` alone, `Content-Type` = stored sniffed type, `Content-Length` from size column, `X-Content-Type-Options: nosniff`.
6. Tests: DB tests in `tests/` via testcontainers, file-backed DB, assert data changes; concurrency test with tiny payloads under `--race`; e2e adds a rejected-upload case alongside the existing happy path.

## Sources

- gohacks.dev: "SQLite Database Is Locked in Go and How to Fix It Properly" — per-connection pragmas, `_pragma`/`_txlock` DSN syntax for modernc.org/sqlite, `SQLITE_BUSY_SNAPSHOT` mechanics, writer-pool layout (HIGH confidence; includes runnable experiment output)
- gohacks.dev: "SQLite WAL Mode in Go…" and "Pure Go SQLite With modernc…" — WAL + synchronous + busy_timeout checklist, modernc vs cgo performance note (HIGH)
- River Queue docs, "Using with SQLite" — official recommendation of `SetMaxOpenConns(1)`, DSN pragmas, deferred-tx upgrade trap, short-transaction caveat (HIGH, vendor docs)
- Real-world post-mortems: github.com/klabast/audiotheque PR#16 and Healarr PR#321 — production `SQLITE_BUSY` bugs caused by per-connection PRAGMA misconfiguration under modernc.org/sqlite (HIGH)
- mtlynch.io PicoShare notes + picoshare issue #284 — measured multipart memory blow-up (~100 MB for 32 MB `maxMemory`), `RemoveAll` cleanup contract, whole-BLOB-read memory cost, in-transaction buffering inflating RAM/WAL (HIGH — directly comparable project: files stored in SQLite in Go)
- Go stdlib `net/http/request.go` source — `ParseMultipartForm` semantics and default `maxMemory` (HIGH)
- ncruces/go-sqlite3 issue #148 — SQLite BLOB max length (~1 GB default) and large-BLOB I/O characteristics (MEDIUM — different driver, but the SQLite limit itself is engine-level)
- Local codebase map: `.planning/codebase/CONCERNS.md` (unbounded `Calc` input, missing middleware, `:memory:` e2e, SQLite scaling notes), `.planning/codebase/ARCHITECTURE.md` (bare `sql.Open` DSN), `go.mod` (mimetype and uuid already in dependency tree)

---
*Pitfalls research for: image gallery on SQLite in Go (Echo v4 + modernc.org/sqlite), brownfield milestone*
*Researched: 2026-11-13*
