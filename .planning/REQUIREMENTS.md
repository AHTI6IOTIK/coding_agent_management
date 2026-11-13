# Requirements: Image Gallery Service

**Defined:** 2026-11-13
**Core Value:** Пользователь загружает изображение и видит его в галерее; данные сохраняются в SQLite между перезапусками.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Storage

- [ ] **STOR-01**: Схема таблицы изображений создаётся при старте сервиса автоматической миграцией (id, имя, content-type, BLOB, время создания)
- [ ] **STOR-02**: Репозиторий-слой в `internal/gallery` с интерфейсом на стороне потребителя; `*sql.DB` внедряется из `cmd/server/main.go`, хэндлеры БД не открывают

### Upload

- [ ] **UPLD-01**: Пользователь может загрузить изображение multipart-запросом через браузерную форму
- [ ] **UPLD-02**: Сервер отклоняет не-изображения и файлы больше лимита (5 МБ): проверка magic bytes, а не только Content-Type; ошибки — 400/413 с непрозрачным сообщением

### Gallery

- [ ] **GAL-01**: Пользователь видит список загруженных изображений (новые сверху) на отдельной HTML-странице
- [ ] **GAL-02**: Пользователь может открыть изображение по ID — бинарник отдаётся с корректным `Content-Type`
- [ ] **GAL-03**: Пользователь может удалить изображение из галереи

### Quality

- [ ] **QUAL-01**: Репозиторий покрыт интеграционными тестами на файловой БД через testcontainers в `tests/`, проверяющими реальные изменения данных
- [ ] **QUAL-02**: HTTP-сценарии загрузки/валидации покрыты table-driven тестами в `internal/httpserver`; `go test --race ./...` и `golangci-lint run ./...` зелёные

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Gallery enhancements

- **GAL-04**: Пагинация списка галереи (LIMIT/OFFSET)
- **GAL-05**: ETag/условное кэширование на просмотре изображения (If-None-Match → 304)
- **GAL-06**: Поиск по имени файла

## Out of Scope

| Feature | Reason |
|---------|--------|
| Thumbnails / ресайз изображений | Требует внешнюю image-processing зависимость; уводит милстоун от цели «persistence layering» |
| Аутентификация, аккаунты, альбомы, теги | Сервис учебный, однопользовательский, без auth по дизайну |
| S3/облачное хранилище | Внешние зависимости противоречат однофайловому образовательному сервису; SQLite BLOB — осознанный компромисс |
| Обработка изображений при загрузке (EXIF, антивирус) | Большая поверхность зависимостей; достаточно лимита размера и проверки magic bytes |
| Real-time обновления галереи (WebSocket) | Новый транспорт и тесты без ценности для одного пользователя |

## Traceability

| Requirement | Phase | Status |
|-------------|-------|--------|
| STOR-01 | Phase 1 | Pending |
| STOR-02 | Phase 1 | Pending |
| UPLD-01 | Phase 2 | Pending |
| UPLD-02 | Phase 2 | Pending |
| QUAL-02 | Phase 2 | Pending |
| GAL-01 | Phase 3 | Pending |
| GAL-02 | Phase 3 | Pending |
| GAL-03 | Phase 3 | Pending |
| QUAL-01 | Phase 1 | Pending |

**Coverage:**
- v1 requirements: 9 total
- Mapped to phases: 9
- Unmapped: 0 ✓

---
*Requirements defined: 2026-11-13*
*Last updated: 2026-11-13 after initial definition*
