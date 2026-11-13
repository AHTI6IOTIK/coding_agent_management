# Project Research Summary

**Project:** Image Gallery Service (upload/list/view/delete images, persisted in SQLite)
**Domain:** Brownfield Go HTTP milestone — multipart binary upload + BLOB storage on an existing Echo v4 + modernc.org/sqlite service
**Researched:** 2026-11-13
**Confidence:** HIGH

## Executive Summary

This is a minimal educational, single-user, no-auth image gallery added as a brownfield milestone onto an existing Go 1.26 service (Echo v4.16.3, modernc.org/sqlite v1.61.1, embedded static UI). Experts build exactly this shape of service with stdlib multipart parsing (`c.Request().ParseMultipartForm` + `FormFile`), magic-byte content sniffing on the server, an explicit body-size limit at the transport edge, and image bytes stored directly as SQLite BLOBs served via `http.ServeContent`. All four research files converge on the same conclusion: **zero new dependencies** — `mimetype` and `google/uuid` are already in the module graph as indirect requires and only need promotion to direct; image-processing libraries are explicitly out of scope.

The main new work is not the HTTP handlers — it is the data layer that does not exist yet. The `*sql.DB` handle is opened in `cmd/server/main.go` but never injected anywhere; there is no schema, no migrations, no repository. Research recommends a new `internal/storage` package with a versioned, `//go:embed`-ed migration, a consumer-side `ImageRepository` interface declared in `internal/httpserver` (mandated by `.agents/rules/architecture.md`), and handler factories following the existing `versionHandler` pattern. `NewApp` gains a second parameter, which is a deliberate, flagged deviation that requires updating all existing call sites in the same change.

The key risks are silent SQLite concurrency bugs and upload-endpoint abuse. The single most likely production bug in this feature: per-connection PRAGMAs set outside the DSN never reach pooled connections, and the e2e suite runs on `:memory:` so it cannot catch any of it. Mitigations are cheap if done at DB bootstrap: all pragmas (`busy_timeout`, WAL, synchronous) plus `_txlock=immediate` in the DSN, `SetMaxOpenConns(1)`, and file-backed testcontainers integration tests in `tests/`. On the upload side: `middleware.BodyLimit` + handler-level `LimitReader`, magic-byte sniffing with a strict image whitelist, and `nosniff` on serve — all must land with the endpoint, not as later hardening.

## Key Findings

### Recommended Stack

Zero new dependencies is achievable and recommended: the entire gallery milestone builds on stdlib multipart/serving plus modules already resolved in `go.mod` (Echo v4.16.3, modernc.org/sqlite v1.61.1, mimetype v1.4.16 and google/uuid v1.6.0 as indirects to promote to direct). Echo v4 (not v5) APIs must be used throughout — `BodyLimit("10M")` takes a size string in v4; v5 signatures are incompatible. See `.planning/research/STACK.md` for full rationale.

**Core technologies:**
- stdlib `mime/multipart` via `c.Request().ParseMultipartForm(...)` + `FormFile` — canonical Echo v4 upload pattern; avoid `c.FormFile()`'s implicit 32 MB `maxMemory`
- `middleware.BodyLimit` (Echo v4, already a dependency) — rejects oversized bodies at the edge with 413 before parsing
- `net/http.ServeContent` with `bytes.NewReader` — serves BLOB bytes with Range/conditional-request/HEAD support for free
- `gabriel-vasile/mimetype` (promote indirect → direct) — magic-byte sniffing; stricter than stdlib `http.DetectContentType` (pick **one**, never both)
- `google/uuid` (promote indirect → direct) — opaque public image IDs instead of enumerable integers
- `modernc.org/sqlite` + `database/sql` BLOB via `[]byte` — unchanged; no CGO drivers (`mattn/go-sqlite3`) under any circumstances

### Expected Features

The milestone scope is a complete upload→see loop: upload with validation, list page, image view, delete — all behind a schema + repository layer that must be built first because the current DB handle is dead weight. See `.planning/research/FEATURES.md` for the full prioritization matrix.

**Must have (table stakes):**
- Schema migration + repository layer (`internal/storage`) — unblocks everything; nothing else is possible without DB-handle injection
- Image upload with validation (multipart, sniffed type + size cap) — the core write path; server-side validation required by `.agents/rules/security.md`
- Gallery list (newest first, no pagination needed for v1) — closes the upload→see loop
- Image view (`GET /images/{id}` with correct `Content-Type`) — makes images actually displayable
- Delete image — prevents permanent garbage; completes basic CRUD

**Should have (competitive):** pick at most one if time allows — pagination (`LIMIT/OFFSET`), ETag caching on view, or search by filename. All LOW complexity, all LOW user value at single-user scale.

**Defer (v2+):**
- Thumbnails — first image-processing dependency (`disintegration/imaging`); hijacks the milestone from its persistence-layering learning goal
- Albums/tags/auth/multi-user — explicit anti-features for this milestone

### Architecture Approach

A three-layer composition-root wiring: `cmd/server/main.go` opens SQLite with a fully-pragma'd DSN and pool cap, runs versioned migrations, constructs `*ImageRepository`, and injects it into `httpserver.NewApp(version, images ImageRepository)`. The `ImageRepository` interface is declared in the consumer (`internal/httpserver`), implemented in the new `internal/storage`; the transport package never imports storage. Image bytes live in a `BLOB NOT NULL` column — deliberately chosen over filesystem+metadata because e2e boots with `DB_PATH=:memory:` and the service is single-binary with everything embedded. See `.planning/research/ARCHITECTURE.md`.

**Major components:**
1. `internal/storage` (new) — SQL statements, versioned embedded migrations (`schema/001_images.sql` + applied-migrations table), row↔entity mapping; importable only by `cmd`
2. `internal/httpserver/image.go` (new) — named handler factories (`uploadImageHandler(images)`, etc.), DTO↔domain mapping, sniffing/limit validation at the transport edge
3. `cmd/server/main.go` — composition root: DSN pragmas + `SetMaxOpenConns(1)`, `storage.Migrate(ctx, db)`, repository construction and injection

**Accepted deviations (flagged):** no service/use-case layer (pass-through for CRUD); BLOB-in-SQLite instead of filesystem; hand-rolled migration runner instead of a library. The `NewApp` signature change is the one deviation touching existing code and tests.

### Critical Pitfalls

1. **Per-connection PRAGMAs never reach the pool** (`SQLITE_BUSY` under load, instant, only in parallel tests) — every pragma must go in the DSN via modernc `_pragma=name(value)` syntax; `db.Exec("PRAGMA ...")` after open configures only one pooled connection. Fix at DB bootstrap, before the first write path exists.
2. **`SQLITE_BUSY_SNAPSHOT` (517) on deferred-transaction upgrade** — `_txlock=immediate` in the DSN so `BeginTx` takes the write lock up front; busy_timeout cannot help this failure mode.
3. **`ParseMultipartForm` 32 MB default ≈ 100 MB transient RAM per upload** (measured in PicoShare) — explicit small `maxMemory` (2 MB) + `io.LimitReader` + app-level size cap; keep one INSERT per upload, never hold a transaction across I/O.
4. **No body limit → OOM DoS on an unauthenticated endpoint** — Echo installs no limits by default; `middleware.BodyLimit` + handler re-check with the same shared constant; `413` at cap+1 must be a test.
5. **Content-Type spoofing → stored XSS** — sniff bytes, whitelist only `image/png|jpeg|gif|webp`, store the *sniffed* type, serve with `X-Content-Type-Options: nosniff`; never serve `image/svg+xml`. A test must prove HTML-as-`.png` gets `415`.
6. **`:memory:` e2e hides every SQLite-file pitfall** — integration tests in `tests/` via testcontainers against a **file**-backed DB (concurrent uploads assert zero BUSY + exact row count; pragma readback from a late pool connection); e2e stays `:memory:` as UI-smoke only.

## Implications for Roadmap

Based on research, suggested phase structure:

### Phase 1: DB layer bootstrap (schema + repository + connection config)
**Rationale:** every feature depends on the repository; all SQLite concurrency pitfalls (1–3, 7, 11) must be fixed here — retrofitting after the write path exists means chasing flaky failures.
**Delivers:** `internal/storage` with `001_images.sql` migration, `*ImageRepository`, pragma'd DSN + `SetMaxOpenConns(1)` in `main.go`, wired `ImageRepository` interface and updated `NewApp` call sites.
**Addresses:** schema/repository table stake; unblocks all other features.
**Avoids:** Pitfalls 1–3 (DSN pragmas, immediate tx, WAL), 7 (size column + cap decision), 11 (driver pinning, one-INSERT design).
**Verification:** pragma readback test + concurrent-writers integration test in `tests/` (testcontainers, file-backed).

### Phase 2: Upload endpoint with validation
**Rationale:** the write path creates the data everything else displays; validation pitfalls must land *with* the endpoint, not as later hardening.
**Delivers:** `POST /images` — explicit small `ParseMultipartForm`, `io.LimitReader`, magic-byte sniffing with image whitelist, `middleware.BodyLimit` with a shared cap constant, one short INSERT per upload.
**Uses:** stdlib multipart + `mimetype` (+ `middleware.BodyLimit`, `google/uuid` if UUID IDs are chosen here).
**Avoids:** Pitfalls 4–6 (multipart memory, body limit, spoofing), 8 (short transactions).

### Phase 3: Gallery list + image view (read path + UI)
**Rationale:** after uploads exist, both read features are LOW complexity and can ship together; they share the embedded-UI and named-handler patterns.
**Delivers:** `GET /images` (list page, no BLOB selected, `LIMIT 1` discipline on view), `GET /images/{id}` via `http.ServeContent` with stored sniffed Content-Type, `Content-Length` from size column, `nosniff`.
**Addresses:** gallery list + image view table stakes.
**Avoids:** Pitfall 7 read-memory trap (BLOB selected alone, cap enforced).

### Phase 4: Delete + hardening tests (e2e coverage, "looks done" checklist)
**Rationale:** delete is trivial once the repository exists; the final phase closes the verification gaps the research explicitly flagged as "often missing".
**Delivers:** `DELETE /images/{id}`; rejected-upload e2e case; spoofing test (HTML-as-`.png` → 415); body-limit boundary test; upload→list→view→assert-bytes e2e chain.
**Addresses:** delete table stake; completes CRUD.
**Avoids:** Pitfall 9 (`:memory:` blind spot) via the file-backed integration suite.

### Phase Ordering Rationale

- **Dependency-driven:** repository → upload → list/view → delete follows the feature-dependency graph exactly; nothing can be reordered without dead work.
- **Risk-driven grouping:** all SQLite concurrency decisions are isolated in Phase 1 so the flaky-failure class is fixed before any write path exists; all abuse/validation pitfalls are isolated in Phase 2 where the endpoint is born.
- **Architecture-pattern reuse:** each phase after the first reuses the handler-factory, named-handler, and DTO-mapping conventions established in the previous one, keeping diffs small and reviewable.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 1:** DSN/pragma syntax conflict between research files (see Gaps) — planner must verify modernc `_pragma=busy_timeout(value)` vs `_busy_timeout=` against the pinned driver version before writing the DSN. Also decide interface/type placement (storage imports httpserver vs hoisted `internal/gallery` domain package) and pin it.
- **Phase 2:** explicit `ParseMultipartForm` vs `MultipartReader` streaming choice; UUID vs integer IDs (affects schema, URLs, and delete/view handlers).

Phases with standard patterns (skip research-phase):
- **Phase 3:** read path + embedded HTML page — well-established `http.ServeContent` and existing embedded-UI patterns.
- **Phase 4:** delete is a single-row DELETE; test additions follow existing table-driven + Playwright patterns.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All recommendations verified against official Echo/mimetype/driver docs and go.mod state; stdlib APIs stable |
| Features | HIGH | Standard web-gallery CRUD set; no novel technology risk; priorities clear |
| Architecture | HIGH | Repo facts verified against source; consumer-side interface mandated by project rules; deviations explicitly flagged |
| Pitfalls | HIGH | Concurrency findings cross-verified against driver docs, vendor docs (River), and real production post-mortems (PicoShare, audiotheque, Healarr) |

**Overall confidence:** HIGH

### Gaps to Address

- **DSN pragma syntax discrepancy:** ARCHITECTURE.md shows `_busy_timeout=5000` in its example DSN, while PITFALLS.md explicitly states mattn-style `_busy_timeout=` params are silently ignored by modernc and mandates `_pragma=busy_timeout(5000)`. Resolve in favor of `_pragma=` syntax during Phase 1 planning; verify with a pragma-readback test on a late pool connection.
- **Interface/type placement (storage → httpserver import vs `internal/gallery` domain package):** both are compliant; the mapping functions differ. Planner must pick one at plan time and keep it fixed.
- **ID scheme (int64 vs UUID):** stack research recommends opaque UUIDs; architecture sketches use int64 AUTOINCREMENT. Decide in Phase 1/2 planning — it affects schema, URLs, and delete semantics.
- **Image size cap value:** research recommends 5–10 MB but leaves the exact number as a product decision; pick one constant and reuse it in middleware, handler, and tests.
- **Exact modernc.org/sqlite pin:** architecture cites v1.61.1 "per go.mod — check exact pin"; confirm during planning (per-connection PRAGMA persistence and context-aware busy handler behavior depend on ≥ v1.61.x).

## Sources

### Primary (HIGH confidence)
- Official Echo docs (Context7 `/labstack/echo`) — middleware catalog, `BodyLimit`, `BindBody` multipart dispatch, v4 vs v5 signature changes
- Official mimetype docs (Context7 `/gabriel-vasile/mimetype`) — `DetectReader`, `SetLimit`, `MIME.Is`
- modernc.org/sqlite / `gitlab_cznic/sqlite` driver docs — DSN pragma order, per-connection PRAGMA persistence, `_txlock=immediate`, context-aware busy handler, `SetMaxOpenConns` recommendation
- Go stdlib pkg.go.dev — `net/http.ServeContent`, `DetectContentType`, `mime/multipart` (`ParseMultipartForm`, `FileHeader.Size`)
- gohacks.dev SQLite-in-Go guides — per-connection pragmas, modernc DSN syntax, `SQLITE_BUSY_SNAPSHOT` mechanics (with runnable experiment output)
- River Queue official docs "Using with SQLite" — writer-pool and DSN recommendations
- Repo sources: `cmd/server/main.go`, `internal/httpserver/server.go` + AGENTS.md, `.agents/rules/architecture.md`, `.agents/rules/security.md`, root `AGENTS.md`, `.planning/codebase/*` map

### Secondary (MEDIUM confidence)
- mtlynch.io PicoShare notes + issue #284 — measured multipart memory blow-up (~100 MB), cleanup contract, in-transaction buffering (directly comparable project)
- ncruces/go-sqlite3 issue #148 — SQLite BLOB ~1 GB ceiling (engine-level, different driver)

### Tertiary (LOW confidence)
- Real-world post-mortems (audiotheque PR#16, Healarr PR#321) — production `SQLITE_BUSY` from per-connection PRAGMA misconfiguration; corroborating, not independently reproduced in this repo

---
*Research completed: 2026-11-13*
*Ready for roadmap: yes*
