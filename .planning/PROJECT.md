# Image Gallery Service

## What This Is

HTTP-сервис на Go (Echo + SQLite): встроенный калькулятор, health/version, Prometheus-метрики и pprof на loopback-серверах. Целевой продукт — галерея изображений: загрузка и просмотр картинок с сохранением в SQLite (`gallery.db`), как описано в `AGENTS.md` и `docs/about.md`.

## Core Value

Работающий HTTP-сервис галереи: пользователь загружает изображение и видит его в списке, данные сохраняются в SQLite между перезапусками.

## Requirements

### Validated

- ✓ HTTP-калькулятор: `GET /calculator` + `POST /echo` с валидацией и вычислением суммы (`internal/httpserver/server.go`)
- ✓ Health/version-эндпоинты для probes (`internal/httpserver/server.go`)
- ✓ Prometheus-метрики и pprof на loopback-серверах (`internal/httpserver/server.go:68-81`)
- ✓ Environment-only конфигурация с валидацией портов и региона (`internal/config/config.go`)
- ✓ Graceful shutdown трёх серверов и SQLite-соединения (`cmd/server/main.go`)
- ✓ e2e-покрытие калькулятора через Playwright (`tests/e2e/calculator.spec.ts`)

### Active

- [ ] Репозиторий-слой поверх SQLite с внедрением `*sql.DB` в приложение (сейчас хэндл открывается и закрывается в `main`, в хэндлеры не передаётся)
- [ ] Загрузка изображения пользователем через HTTP с сохранением в SQLite
- [ ] Список загруженных изображений и просмотр отдельного изображения
- [ ] Интеграционный тест репозитория через testcontainers (`tests/`)

### Out of Scope

- Аутентификация и многопользовательность — сервис учебный, без auth по дизайну (`.planning/codebase/ARCHITECTURE.md`)
- Облачное хранилище файлов (S3 и т.п.) — файлы хранятся в SQLite/локально
- Переход на OpenCode 2.x рантайм — касается окружения агентов, не продукта

## Context

- Brownfield: карта кодовой базы в `.planning/codebase/` (7 документов, 2026-11-13).
- Правила кода зафиксированы в `AGENTS.md`, `.agents/rules/*`, package-level `internal/httpserver/AGENTS.md`: named handlers с Go-doc, env-only конфиг, `%w`-оборачивание ошибок, table-driven тесты.
- Go 1.26 (`go.mod`), Echo, go-playground/validator, prometheus, modernc.org/sqlite (pure Go).
- История работ и доказательства: `agent_work_verification/`, `docs/about.md`.

## Constraints

- **Tech stack**: Go 1.26, Echo, SQLite через `modernc.org/sqlite` — менять стек нельзя (учебные требования модулей).
- **Architecture**: transport не зависит от storage; домен не зависит от HTTP (`.agents/rules/architecture.md`); новый persistence — только через репозиторий-абстракцию.
- **Security**: без auth, но без секретов в ответах/логах (`.agents/rules/security.md`); метрики/pprof остаются на `127.0.0.1`.
- **Testing**: `go test --race ./...`, линтер `golangci-lint run ./...` зелёные перед завершением любой фазы.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Проект для GSD-планирования = Go-сервис, не инфраструктура агентов | Решение пользователя 2026-11-13 | — Pending |
| SQLite как единственное хранилище изображений | Уже подключено, без внешних зависимостей | — Pending |
| Внедрение БД через репозиторий-слой, не в хэндлеры | Правило архитектуры: transport не зависит от storage | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-11-13 after initialization*
