# Режим агента

- Агент сначала уточняет цель, границы и риски, затем предлагает план и ждёт подтверждения перед изменениями.
- После подтверждения агент выполняет минимальное полное изменение с проверками.

Этот режим снижает риск несанкционированных и лишних правок.

# Linter

1. Установить линтер `https://golangci-lint.run/docs/welcome/install/local/`
2. Проверить что линтер установлен в системе `golangci-lint --version`
3. Запуск линтера из корня проекта `golangci-lint run ./...`

# Tests

1. Тесты запускаются командой `go test --race ./...`
2. Юнит тесты описываются рядом с тестируемым функционалом в файлах `..._test.go`.
3. Интеграционные и функциональные тесты описываются в корне проекта в директории `tests`. Такие тесты должны использовать `testcontainers`.

# Модуль 2: управление контекстом и фреймворк GSD

## GSD Core

Установлен локально для OpenCode: `npx @opengsd/gsd-core@latest --opencode --local` (v1.21.0).
Команды: `/gsd-help`, `/gsd-new-project`, `/gsd-plan-phase N`, `/gsd-execute-phase N`, полный список — `.opencode/commands/` (72 команды).

GSD-рантайм не хранится в репозитории: после свежего клона запустите установщик той же командой — он развернёт `.opencode/{gsd-core,agents,commands,hooks,plugins,scripts,skills}` и допишет gsd-MCP-сервер и права в `opencode.json`.

## Discovery (инициализация проекта)

Пройден через `/gsd-onboard` → `/gsd-map-codebase` → `/gsd-new-project` (brownfield-пайплайн). Артефакты в `.planning/`:

| Артефакт | Файл |
|----------|------|
| Контекст проекта | `.planning/PROJECT.md` |
| Требования v1/v2 | `.planning/REQUIREMENTS.md` |
| Дорожная карта (3 фазы) | `.planning/ROADMAP.md` |
| Состояние/прогресс | `.planning/STATE.md` |
| Архитектура кодовой базы | `.planning/codebase/ARCHITECTURE.md` (+ ещё 6 документов карты) |
| Research (stack/features/architecture/pitfalls) | `.planning/research/` |
| Security-checklist | `.planning/SECURITY-CHECKLIST.md` (+ per-phase через `/gsd-secure-phase`) |
| Итоги onboarding | `.planning/onboarding/SUMMARY.md` |

Фазы roadmap: 1) Storage Foundation (репозиторий + миграции) → 2) Image Upload → 3) Gallery.

## Память агента

Подключён claude-mem: наблюдения автосохраняются, поиск по памяти (`search`/`get_observations`), чек-пойнты состояния задач (`work_state`). Полное описание доказательств — `agent_work_verification/module2/usage-context-stats.md`: автосжатие удерживает 98–99% контекста (6.7M токенов истории сжимаются до ~120K в full-режиме и ~17K в стандартном).

## Быстрые команды и auto hooks

- Быстрые команды: `/lint` (golangci-lint по проекту), `/test` (`go test --race ./...`), `/review-pr` — `.opencode/commands/`.
- Auto hooks GSD: `.opencode/plugins/gsd-core.js` (мост к `.opencode/hooks/*.js`: context-monitor, config-reload, agent-isolation-guard).
- Плагины проекта: `go-quality.ts` (gofmt/vet/lint после правок .go), `protect-files.ts` (блокировка .env/.git/lock) — `.opencode/plugin/`.

## Управление контекстом

- Автосжатие истории: claude-mem (см. выше).
- Чек-пойнты: `.planning/STATE.md` (GSD) + work-state списки задач.
- Ограничение: OpenCode 1.19.2 объявлен GSD устаревшим — поддержка до 2026-12-31, далее требуется OpenCode 2.x (`opencode upgrade --major`).

## Browser e2e

1. Установить Node-зависимости: `npm install`.
2. Установить Chromium для Playwright: `npx playwright install chromium`.
3. Запустить e2e-проверку калькулятора: `npm run test:e2e`.
