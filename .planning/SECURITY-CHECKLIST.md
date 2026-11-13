# Security Checklist — Image Gallery Service

**Создан:** 2026-11-13
**Источники:** `.agents/rules/security.md`, `.planning/research/PITFALLS.md`, `.planning/codebase/CONCERNS.md`
**Per-phase threat-контракт GSD:** `.planning/phases/<N>-*/SECURITY.md` (создаётся и аудируется командой `/gsd-secure-phase <N>`)

## Чек-лист (проектный уровень)

### Валидация входа

- [ ] Валидация только на транспортной границе через DTO + validator-теги (`customValidator`, `internal/httpserver/server.go:20-26`)
- [ ] Тип файла определяется по magic bytes, не по клиентскому Content-Type (подделываем) — 400/415
- [ ] Жёсткий лимит размера загрузки (5 МБ) — `middleware.BodyLimit` + маленький `ParseMultipartForm` — 413
- [ ] Сообщения об ошибках непрозрачны: без внутренних деталей (`echo.NewHTTPError(400, "invalid request")` — образец)

### Хранение и доступ

- [ ] Секреты и персональные данные не попадают в ответы, логи и ошибки
- [ ] Ошибки оборачиваются через `%w`, без утечки содержимого БД/файлов в тексте ошибки
- [ ] Метрики и pprof остаются только на `127.0.0.1` (`internal/config/config.go:64-65`) — не менять без явного согласования
- [ ] SQLite: DSN-параметры (`_pragma=busy_timeout`, `_txlock=immediate`) вместо per-connection PRAGMA вне пула

### Эксплуатация

- [ ] Нет auth — осознанное решение; сервис не должен добавлять секреты в конфиг/ответы
- [ ] Перед завершением фазы: `go test --race ./...` и `golangci-lint run ./...` зелёные
- [ ] Новые угрозы фазы фиксируются в threat register фазы (`/gsd-secure-phase`), disposition у каждой: mitigate/accept/transfer

## Принятые риски

| Риск | Обоснование |
|------|-------------|
| Отсутствие аутентификации | Учебный однопользовательский сервис, без auth по дизайну |

---
*Last updated: 2026-11-13 after project initialization*
