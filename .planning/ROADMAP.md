# Roadmap: Image Gallery Service

## Overview

Brownfield-милстоун на существующем Go 1.26 + Echo + modernc.org/sqlite сервисе: от мёртвого SQLite-хэндла в `main` до работающей галереи. Путь из трёх фаз следует графу зависимостей фич: сначала фундамент хранения (схема, репозиторий, внедрение `*sql.DB` — без него невозможен ни один путь данных, а все SQLite-ловушки конкурентности фиксируются до появления первой записи), затем путь записи (multipart-загрузка с валидацией magic bytes и лимитом размера — ловушки злоупотребления эндпоинтом закрываются вместе с его рождением), затем путь чтения (список, просмотр, удаление), замыкающий цикл «загрузил → увидел». Фаза завершается, когда пользователь может выполнить своё действие и `go test --race ./...` с `golangci-lint run ./...` зелёные.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Storage Foundation** - Схема изображений, репозиторий-слой и внедрение `*sql.DB` в приложение
- [ ] **Phase 2: Image Upload** - Multipart-загрузка через браузерную форму с проверкой magic bytes и лимитом размера
- [ ] **Phase 3: Gallery** - Список, просмотр и удаление изображений; цикл «загрузил → увидел» замкнут

## Phase Details

### Phase 1: Storage Foundation
**Goal**: Изображения сохраняются в SQLite между перезапусками через репозиторий-слой, хэндлеры не открывают БД напрямую
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: STOR-01, STOR-02, QUAL-01
**Success Criteria** (what must be TRUE):
  1. Сервис стартует на файловой БД и создаёт схему таблицы изображений (id, имя, content-type, BLOB, время создания) автоматической миграцией; повторный старт не падает и не теряет данные.
  2. Репозиторий выполняет запись/чтение/удаление записей изображений в реальной файловой SQLite — подтверждается интеграционными тестами testcontainers в `tests/`, проверяющими фактические изменения данных, а не только отсутствие ошибок.
  3. `*sql.DB` внедряется из `cmd/server/main.go` через интерфейс репозитория, объявленный на стороне потребителя; хэндлеры `internal/httpserver` БД не открывают (transport не зависит от storage).
**Plans**: TBD

### Phase 2: Image Upload
**Goal**: Пользователь загружает изображение через браузерную форму, сервер сохраняет его в SQLite, отклоняя не-изображения и файлы больше лимита
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: UPLD-01, UPLD-02, QUAL-02
**Success Criteria** (what must be TRUE):
  1. Пользователь загружает изображение (PNG/JPEG/GIF/WebP) multipart-запросом через форму — файл сохраняется в SQLite, пользователь получает подтверждение успешной загрузки.
  2. Файл, не являющийся изображением по magic bytes (даже с подделанным Content-Type), отклоняется с ошибкой 400/415 и непрозрачным сообщением.
  3. Файл больше лимита 5 МБ отклоняется с ошибкой 413.
  4. HTTP-сценарии загрузки и валидации покрыты table-driven тестами в `internal/httpserver`; `go test --race ./...` и `golangci-lint run ./...` зелёные.
**Plans**: TBD
**UI hint**: yes

### Phase 3: Gallery
**Goal**: Пользователь видит загруженные изображения в списке, открывает их по ID и удаляет — цикл «загрузил → увидел» замкнут
**Mode:** mvp
**Depends on**: Phase 2
**Requirements**: GAL-01, GAL-02, GAL-03
**Success Criteria** (what must be TRUE):
  1. Пользователь открывает отдельную HTML-страницу галереи и видит список загруженных изображений, новые сверху.
  2. Пользователь открывает изображение по ID — браузер отображает картинку с корректным `Content-Type`.
  3. Пользователь удаляет изображение — оно исчезает из списка и больше не открывается по ID.
  4. Данные галереи сохраняются между перезапусками сервиса на файловой БД.
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Storage Foundation | 0/TBD | Not started | - |
| 2. Image Upload | 0/TBD | Not started | - |
| 3. Gallery | 0/TBD | Not started | - |
