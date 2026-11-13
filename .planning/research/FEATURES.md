# Feature Research

**Domain:** Minimal educational image gallery HTTP service (single user, no auth) — brownfield milestone on an existing Go 1.26 / Echo / SQLite service
**Researched:** 2026-11-13
**Confidence:** HIGH (standard web-gallery feature set; well-understood domain, no novel technology risk)

## Existing Codebase Baseline (what v1 already gets for free)

| Provided by existing code | Where | Relevance to gallery features |
|---------------------------|-------|-------------------------------|
| Echo app factory, named-handler + Go-doc pattern, DTO + validator tags | `internal/httpserver/server.go`, `internal/httpserver/AGENTS.md` | New gallery handlers/DTOs follow this pattern directly — no design work needed |
| `go-playground/validator` wired via `customValidator` | `internal/httpserver/server.go:20-26` | Request validation for upload params is already plumbed; multipart validation needs custom checks |
| `*sql.DB` handle opened, pinged, held in `main` but **not injected anywhere**; no schema, no repository layer, no migrations | `cmd/server/main.go:36-50`, CONCERNS.md "Database layer is a stub" | Every gallery feature must build the repository layer + schema from zero — this is the main new-work area |
| Embedded static UI via `//go:embed` (`calculator.html`) | `internal/httpserver/server.go:17` | Gallery list/view pages follow the same embed pattern |
| Prometheus metrics + pprof sidecars (loopback-only), `/health`, `/version` | `internal/httpserver/server.go:68-81` | Observability exists; gallery adds no infra work |
| Table-driven HTTP tests + Playwright e2e harness (boots real server, in-memory SQLite) | `internal/httpserver/server_test.go`, `playwright.config.ts` | Test patterns for gallery features are established; e2e can drive gallery flows |
| No auth, no middleware stack (no Recover, no request logger) | CONCERNS.md | Upload endpoint especially needs panic-recovery consideration before shipping file handling |

## Feature Landscape

### Table Stakes (Users Expect These)

Features users assume a gallery has. Missing = milestone feels incomplete.

| Feature | Why Expected | Complexity | Research Notes |
|---------|--------------|------------|----------------|
| Image upload (multipart POST, metadata stored in SQLite) | A gallery you cannot add images to is broken; this is the core write path | MEDIUM | Echo v4 `c.FormFile()` handles multipart; binary goes to a SQLite BLOB column (simplest, single-file persistence matches `DB_PATH` design) or disk path — pick one and document. Validation: content-type + size limit via existing validator/DTO pattern |
| Upload validation (allowed types, size limit, reject corrupt/multipart errors) | Users expect clear errors instead of garbage rows; security rule (`.agents/rules/security.md`) requires server-side checks | MEDIUM | Check magic bytes, not just `Content-Type` header (client-supplied, spoofable); enforce a size cap (e.g. 5 MB) at handler level; map failures to Echo 400/413 with opaque messages per existing error strategy |
| Gallery list (page showing all images, newest first) | Second half of the core loop: upload → see what you uploaded; missing = product feels incomplete | LOW | Simple `SELECT id, name, content_type, created_at ORDER BY id DESC`; render via a new embedded HTML page; reuses embedded-UI + named-handler patterns; no pagination needed for v1 |
| Image view (serve image binary by ID with correct `Content-Type`) | Browsers can only display images served with proper MIME; the list page is useless without clickable detail view | LOW | `GET /images/{id}` handler streams from SQLite BLOB; set `Content-Type` from stored metadata and cache headers; standard `database/sql` blob read, rows always closed (`sqlclosecheck` linter) |
| Delete image (remove record + blob) | Users expect to remove mistakes; without delete, bad uploads are permanent | LOW | `DELETE /images/{id}` handler (or form button on detail page); single-row delete + verification the row was affected; also worth the repository layer's first write-besides-insert test |
| Schema/migrations + repository layer (`internal/gallery`) | Prerequisite plumbing: DB handle currently opened but never injected; architecture rules require consumer-side interfaces and no re-opening DB in handlers | MEDIUM | First real milestone task: `CREATE TABLE` migration at startup (no migration tooling exists yet — a simple versioned migration runner is enough), repository interface defined at consumer side, wired from `cmd/server/main.go` |

### Differentiators (Competitive Advantage)

Not required, but valuable if time allows. For an educational project, pick at most one.

| Feature | Value Proposition | Complexity | Research Notes |
|---------|-------------------|------------|----------------|
| Thumbnails (small preview variant stored on upload) | Grid pages with full-size images feel heavy; thumbnails make the list page feel "real" | HIGH | Requires image decoding/resizing — adds a third-party dependency (`github.com/disintegration/imaging` is the de-facto pure-Go choice); big scope jump for an educational milestone; recommend deferring |
| Pagination on the gallery list | Scales beyond a screenful; teaches limit/offset SQL patterns | LOW | `LIMIT ? OFFSET ?` + page param validation — genuinely cheap, but unnecessary for single-user educational scale; add only if list feels unwieldy in demos |
| Search by filename | Find one image among many without scrolling | LOW | `WHERE name LIKE ?` — trivial SQL, marginal value for a single user with dozens of images; defer |
| ETag/conditional caching on image view | Teaches HTTP caching semantics (ETag, If-None-Match, 304) | LOW | Educational value is real (nice HTTP lesson), but invisible to users; only worth it if HTTP-caching is a learning goal |

### Anti-Features (Commonly Requested, Often Problematic)

Features to explicitly NOT build in this milestone.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Auth / multi-user | "Real apps have login" | Explicitly out of scope by design (single user, no auth per architecture rules); auth would touch every endpoint, middleware, and schema and explode scope | Keep unauthenticated; note it in docs |
| User accounts / albums / tags / permissions | Social-gallery features look impressive | Each implies auth + schema + UI growth far beyond one milestone; gallery already has zero middleware stack | Single flat gallery; tags/albums as future milestones if the project continues |
| S3/cloud storage | "Never store blobs in DB" dogma | Adds external dependency, config, and failure modes to an educational single-binary service with zero outbound calls today | SQLite BLOB (or local `data/` dir) — deliberate, documented tradeoff for this scale |
| Image processing (resize/EXIF strip/virus scan) on upload | "Handle any image safely" | Large dependency surface, CPU cost, edge-case matrix; hijacks the milestone from persistence layering (the actual learning goal) | Fixed size cap + magic-byte content-type check |
| Real-time updates (WebSocket refresh on new uploads) | "Gallery should update live" | New transport paradigm, new tests, no value for a single user | Reload the page / re-render on navigation |

## Feature Dependencies

```
[Schema + repository layer (internal/gallery)]
    └──requires──> [DB handle injection from main]  (exists, unused — small wiring task)

[Image upload + validation]
    └──requires──> [Schema + repository layer]

[Gallery list] ──requires──> [Image upload]          (nothing to list without uploads)
[Image view]   ──requires──> [Image upload]          (nothing to view without uploads)

[Delete]       ──requires──> [Image view]            (delete button lives on detail/list page)

[Thumbnails]   ──requires──> [Image upload] + external image-processing dependency
[Thumbnails]   ──enhances──> [Gallery list]

[Pagination]   ──enhances──> [Gallery list]
[Search]       ──enhances──> [Gallery list]
[ETag caching] ──enhances──> [Image view]
```

### Dependency Notes

- **Every gallery feature requires the repository layer:** the `*sql.DB` handle exists but is not injected into any handler (`cmd/server/main.go:36-50`); schema and repository are the foundation phase.
- **List and view both require upload:** they can be built in either order afterward, but upload-with-validation is the natural first feature since it creates the data.
- **Thumbnails conflict with minimal scope:** introduces the project's first image-processing dependency and CPU work on the write path — deliberately deferred to keep the educational focus on HTTP + persistence layering.

## MVP Definition

### Launch With (v1)

- [ ] Schema migration + repository layer (`internal/gallery`) — unblocks everything; DB handle currently dead weight
- [ ] Image upload with validation (multipart, type + size limits) — the core write path
- [ ] Gallery list page — closes the upload→see loop
- [ ] Image view endpoint (`GET /images/{id}` with correct Content-Type) — makes images actually displayable
- [ ] Delete image — prevents permanent garbage uploads; completes basic CRUD

### Add After Validation (v1.x)

- [ ] Pagination on list — add when demo data exceeds one screenful
- [ ] ETag caching on image view — add if HTTP-caching is a chosen learning objective
- [ ] Search by filename — add if list grows and manual scrolling hurts

### Future Consideration (v2+)

- [ ] Thumbnails — real differentiator but requires image-processing dependency; defer until CRUD + tests are solid
- [ ] Albums/tags — implies scope growth beyond single-user educational goals

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Schema + repository layer | HIGH | MEDIUM | P1 |
| Upload + validation | HIGH | MEDIUM | P1 |
| Gallery list | HIGH | LOW | P1 |
| Image view | HIGH | LOW | P1 |
| Delete | MEDIUM | LOW | P1 |
| Pagination | LOW | LOW | P2 |
| ETag caching | LOW | LOW | P2/P3 |
| Search by filename | LOW | LOW | P3 |
| Thumbnails | MEDIUM | HIGH | P3 |

**Priority key:**
- P1: Must have for launch
- P2: Should have, add when possible
- P3: Nice to have, future consideration

## Sources

- Codebase map: `.planning/codebase/ARCHITECTURE.md`, `STRUCTURE.md`, `INTEGRATIONS.md`, `CONCERNS.md` (DB stub, missing middleware, security rules)
- Project rules: `.agents/rules/architecture.md` (repository interfaces, no DB in handlers), `.agents/rules/security.md` (server-side validation, no sensitive data in responses/logs)
- Domain knowledge: standard minimal web-gallery feature set (upload/validate/list/view/delete CRUD), Echo v4 multipart handling, `modernc.org/sqlite` BLOB support — all HIGH-confidence, well-established patterns

---
*Feature research for: minimal educational image gallery service (brownfield milestone)*
*Researched: 2026-11-13*
