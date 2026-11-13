# Stack Research

**Domain:** Go 1.26 HTTP service — image gallery milestone (multipart upload + validation + serving image bytes from SQLite) on an existing Echo v4 + modernc.org/sqlite service
**Researched:** 2026-11-13
**Confidence:** HIGH (all recommendations verified against official docs; go.mod state verified directly)

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `net/http` + `mime/multipart` (stdlib) | Go 1.26 | Multipart form parsing: `Request.ParseMultipartForm(maxMemory)` and `Request.FormFile("image")` | The standard, zero-dependency way to handle multipart uploads in Go. Echo v4 has no dedicated upload helper — `c.Request().FormFile()` is the canonical pattern documented by Echo itself. Already in go.mod (stdlib). |
| `github.com/labstack/echo/v4/middleware` (`middleware.BodyLimit`) | v4.16.3 (in go.mod) | Coarse request-body size gate before any parsing (e.g. `middleware.BodyLimit("10M")` on the upload route) | Rejects oversized bodies at the edge with a 413, cheap and route-scoped. Already a direct dependency — **no new dependency**. Note: v4 signature takes a size string (`"10M"`); the int64-bytes variant is v5-only. |
| `net/http.ServeContent` (stdlib) | Go 1.26 | Serving image bytes from a `[]byte` BLOB: `http.ServeContent(w, r, name, modTime, bytes.NewReader(data))` | Handles `Content-Type`, `Content-Length`, `Range` requests (progressive image loading), `HEAD`, and conditional requests (ETag/If-Modified-Since) for free. The standard answer for serving bytes from a DB or memory. Already in go.mod (stdlib). |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/gabriel-vasile/mimetype` | v1.4.16 (already in go.mod as **indirect**, via go-playground/validator) | Reliable image sniffing by magic bytes (`mimetype.DetectReader` / `mimetype.Detect`), not trusting the client's `Content-Type` header | Use it as the authoritative upload validator: read first N bytes (default `readLimit` = 3072/4096 bytes is enough for JPEG/PNG/GIF/WebP magic numbers), require `mtype.Is("image/…")`. Promote from indirect to direct with one `go get` — **no new dependency is introduced**, only a direct reference to an existing module. |
| `net/http.DetectContentType` (stdlib) | Go 1.26 | Zero-dependency alternative sniffer (first 512 bytes), recognizes `image/jpeg`, `image/png`, `image/gif`, `image/webp` | Sufficient if the team prefers strictly stdlib and only these four image formats. Weaker than `mimetype` (no WebP pre-Go-1.20 issues today, but fewer formats, coarser hierarchy). Pick **either** this or `mimetype` — do not layer both. |
| `github.com/google/uuid` | v1.6.0 (already in go.mod as **indirect**) | Generating opaque public image IDs (stored in DB, used in `/images/{id}` URLs) | Avoids enumerable integer IDs in URLs. Promote from indirect to direct — **no new dependency**. |
| `modernc.org/sqlite` + `database/sql` | v1.61.1 (in go.mod) | Persisting image bytes as SQLite `BLOB` columns via `[]byte` | Matches the existing data layer and the milestone requirement "images persisted in SQLite". No changes needed. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `golangci-lint` (existing, `.golangci.yaml`) | Gates the new code | Already enabled `bodyclose`, `sqlclosecheck`, `gosec`, `errorlint` — they will police the new repository layer; no config changes expected |
| `go test --race ./...` + Playwright e2e (existing) | Verification | New repository handlers should get table-driven unit tests beside code (project convention), plus an e2e upload/list/view spec in `tests/e2e/` |

## Installation

```bash
# No NEW dependencies. Promote two already-present indirect modules to direct:
go get github.com/gabriel-vasile/mimetype@v1.4.16
go get github.com/google/uuid@v1.6.0
go mod tidy
```

Everything else (Echo v4.16.3, modernc.org/sqlite v1.61.1, validator, Prometheus) stays untouched.

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| stdlib `mime/multipart` via `c.Request().FormFile()` | `echo.BindBody` (dispatches multipart into a struct) | Only if you want form *fields* bound to a struct; file bytes still need `FormFile`. Extra abstraction buys nothing here. |
| `net/http.DetectContentType` (stdlib) | `gabriel-vasile/mimetype` | Prefer stdlib-only policy and a fixed set of 4 image formats. `mimetype` is better for stricter hierarchical sniffing and `mtype.Is()` semantics; both cost zero new deps since it's already in the module graph. |
| `middleware.BodyLimit` (Echo) | `http.MaxBytesReader` wrapping `c.Request().Body` before parsing | Use `MaxBytesReader` additionally if you want a hard cap independent of Echo middleware (defense in depth); `BodyLimit` alone is the simple route-scoped option. |
| `http.ServeContent` with `bytes.NewReader` | `echo.Context.Blob(code, contentType, data)` | `Blob` is fine for one-shot full-body responses, but lacks `Range`/conditional-request handling — matters for image viewing in browsers. |
| SQLite `BLOB` column | Filesystem storage + DB stores path | Only if images were large (MBs+) and count was high; for a gallery-scale service BLOB-in-SQLite keeps the "single binary + one DB file" deployment property. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| Trusting the multipart part's `Content-Type` header | Client-controlled; trivially spoofed (e.g. an HTML file sent as `image/png`) | Sniff magic bytes: `mimetype.DetectReader` or `http.DetectContentType` |
| `disintegration/imaging`, `nfnt/resize`, EXIF-stripping libs | Adds image *processing* — out of scope for upload/list/view milestone; new dependency with no requirement behind it | Skip entirely; add a dedicated phase only if resizing/exif becomes a requirement |
| `mattn/go-sqlite3` or other CGO SQLite drivers | Requires CGO toolchain; the project deliberately runs CGO-free pure-Go `modernc.org/sqlite` | Keep `modernc.org/sqlite` v1.61.1 (in go.mod) |
| `gofrs/uuid` / `rs/xid` / `oklog/ulid` for IDs | New dependency when `github.com/google/uuid@v1.6.0` is already in the module graph | `github.com/google/uuid` (promote to direct) |
| Writing uploaded files to `/tmp` before insert | Unnecessary I/O and cleanup burden; `multipart.File` is already an in-memory-or-tmp managed reader | Read `multipart.FileHeader.Size`, sniff, then `io.ReadAll` (bounded by BodyLimit/Size check) and insert the `[]byte` |
| Per-route custom size checks without `middleware.BodyLimit` | Parsing a 500 MB body into memory before you can reject it defeats the check | Apply `BodyLimit` first, then validate `FileHeader.Size` against the same limit |

## Stack Patterns by Variant

**If strict-stdlib policy is preferred:**
- Use `net/http.DetectContentType` (first 512 bytes) + `crypto/rand`-derived IDs
- Because zero module-graph changes; accepts weaker sniffing and hand-rolled ID encoding

**If stricter validation is acceptable (recommended):**
- Use `mimetype.DetectReader` + `google/uuid` (both already indirect in go.mod, promoted to direct)
- Because `mtype.Is("image/jpeg")`-style checks are precise and IDs are opaque by construction

**If image sizes could grow beyond a few MB:**
- Keep `middleware.BodyLimit` but set a sane `ParseMultipartForm(maxMemory)` (e.g. 8 MB) so large files spill to managed temp storage rather than RAM
- Because stdlib multipart already implements this spill behavior; no extra code needed

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| echo v4.16.3 | Go 1.26.0 | Current line; `middleware.BodyLimit(string)` signature. v5 (`/labstack/echox`) changes signatures (`*echo.Context`, `BodyLimit(int64)`) — do not mix v5 imports into the v4 codebase. |
| mimetype v1.4.16 | Go ≥ 1.20 | Already resolved in this module's go.sum (indirect); promoting is a no-op for the build. |
| modernc.org/sqlite v1.61.1 | `database/sql`, Go 1.26 | Existing pinned driver; BLOB read/write via `[]byte` is standard `database/sql` behavior. |
| google/uuid v1.6.0 | — | Already resolved in this module's go.sum (indirect); promoting is a no-op for the build. |

## Sources

- Official Echo docs (via Context7 `/labstack/echo`, mirrors labstack/echo master) — middleware catalog incl. `BodyLimit`; binder `BindBody` multipart dispatch; note that Echo v5 API changes move `BodyLimit` to bytes. Confidence: HIGH.
- Official mimetype docs (via Context7 `/gabriel-vasile/mimetype`, mirrors gabriel-vasile/mimetype master: README, api-reference, configuration) — `Detect`, `DetectReader`, `SetLimit` (default 4096-byte read block), `MIME.Is/String/Extension`. Confidence: HIGH.
- Go stdlib references (pkg.go.dev): `net/http.ServeContent` — Range/conditional-request serving; `net/http.DetectContentType` — 512-byte sniffing; `mime/multipart.FileReader`/`ParseMultipartForm`/`FileHeader.Size`. Confidence: HIGH (stable, long-standing APIs; exact snippet quotes not re-fetched — flagged for planner verification if any behavior nuance matters).
- `.planning/codebase/STACK.md` (mapped 2026-11-13, commit f0a723b) — existing stack versions and go.mod state. Confidence: HIGH.

---
*Stack research for: image gallery milestone (multipart upload + SQLite-backed image serving) on Go 1.26 + Echo v4*
*Researched: 2026-11-13*
