# clor — Plan улучшений

## Текущее состояние

clor — визуальный оркестратор мульти-агентных Claude Code пайплайнов. Single Go binary, Drawflow.js фронт, JSON-хранилище. Активная разработка: интерактивный режим, partial runs, snap alignment.

---

## 1. Server-Sent Events вместо polling

**Проблема**: UI опрашивает `/api/run/{id}/status` каждую секунду — лишний трафик, задержка до 1с, 60+ запросов в минуту на один запуск.

**Решение**: SSE endpoint `GET /api/run/{id}/events` — сервер пушит обновления статуса при каждом изменении. Фронт подписывается через `EventSource`.

**Файлы**:
- `server.go` — новый handler `handleRunEvents`, использующий `http.Flusher`
- `orchestrator.go` — broadcast-канал для рассылки событий подписчикам
- `web/index.html` — заменить `setInterval` + `fetch` на `new EventSource()`

**Этапы**:
1. Добавить `eventSubscribers map[string][]chan RunStatus` в Server
2. Handler `handleRunEvents`: `Content-Type: text/event-stream`, keep-alive, слушать канал
3. Orchestrator при каждом `saveStatus()` отправляет в broadcast
4. Фронт: `EventSource` с fallback на polling для старых браузеров
5. Удалить `pollTimer` логику

**Приоритет**: Высокий — улучшает отзывчивость UI и снижает нагрузку.

---

## 2. Стриминг вывода агентов

**Проблема**: Пока агент работает (до 10 минут), UI показывает только "running" без прогресса. Пользователь не знает, что происходит внутри.

**Решение**: Перенаправить stdout агента через `io.TeeReader` — писать в лог-файл потоково и пушить дельты клиенту.

**Файлы**:
- `agent.go` — `cmd.Stdout` → `io.MultiWriter(logFile, streamBuffer)`
- `server.go` — SSE поток для логов конкретной ноды `GET /api/run/{id}/logs/{nodeId}/stream`
- `web/index.html` — log viewer подключается к потоку при выборе running-ноды

**Этапы**:
1. В `runAgent()` использовать `cmd.StdoutPipe()` + горутину чтения
2. Писать чанки в файл и в ring-buffer
3. SSE endpoint для стриминга лога (tail -f семантика)
4. Фронт: переключение между SSE-стримом (running) и полным логом (done)

**Приоритет**: Высокий — критично для UX при длительных задачах.

---

## 3. Retry & Error Recovery

**Проблема**: Ошибка одной ноды останавливает весь пайплайн. Нет возможности повторить упавшую ноду.

**Решение**: Настраиваемая политика повторов + ручной retry из UI.

**Файлы**:
- `types.go` — добавить `MaxRetries int`, `RetryDelay int` в `NodeDetail`
- `orchestrator.go` — retry-loop вокруг `runAgent()`, экспоненциальный backoff
- `server.go` — `POST /api/run/{id}/retry/{nodeId}` для ручного retry
- `web/index.html` — кнопка "Retry" на failed-нодах, настройка retries в конфиг-панели

**Этапы**:
1. Расширить `NodeDetail` полями retry-конфига
2. В orchestrator обернуть `runAgent` в retry-loop с backoff
3. При исчерпании попыток — пометить ноду как `failed`, но продолжить независимые ветки
4. API endpoint для manual retry (перезапуск одной ноды, обновление статуса)
5. UI: кнопка retry, счётчик попыток в статусе ноды

**Приоритет**: Высокий — основная жалоба на надёжность.

---

## 4. Валидация пайплайна и Dry Run

**Проблема**: Невалидные конфигурации (отсутствующий проект, цикл в графе, пустой prompt) обнаруживаются только при запуске. Цикл в графе ломает пайплайн молча.

**Решение**: Предварительная валидация перед запуском + режим dry run.

**Файлы**:
- `validate.go` (новый) — функции валидации: граф, проекты, промпты, модели
- `graph.go` — `ComputeWaves()` возвращает ошибку при обнаружении цикла
- `server.go` — `POST /api/validate` endpoint, вызов валидации перед `POST /api/run`
- `web/index.html` — визуальная подсветка ошибок на нодах (красная рамка, tooltip)

**Проверки**:
- Цикл в графе → ошибка с указанием нод
- Нода без проекта (agent/report) → предупреждение
- Пустой prompt (agent/report) → ошибка
- Проект path не существует → предупреждение
- `{read:alias:file}` — alias не найден → предупреждение
- Isolated nodes (без связей) → info

**Этапы**:
1. Создать `validate.go` с функцией `ValidatePipeline(cfg PipelineConfig, projects []Project) []ValidationError`
2. Расширить `ComputeWaves()` — возвращать `([][]string, error)` при цикле
3. API endpoint `/api/validate` — возвращает массив ошибок/предупреждений
4. Вызывать валидацию автоматически при Save и перед Run
5. UI: иконки ошибок на нодах, блокировка Run при критических ошибках
6. Dry Run: валидация + expand промптов без запуска агентов, показать итоговые промпты

**Приоритет**: Средний — предотвращает потерю времени на невалидных пайплайнах.

---

## 5. Review Loop (завершение запланированной фичи)

**Проблема**: В `types.go` есть упоминание reviewer нод и `max_review_retries`, в `prompt_expand.go` есть `{review_issues}`, но сам review loop не реализован. `review.go` отсутствует.

**Решение**: Реализовать полный цикл review — reviewer парсит PASS/FAIL, при FAIL отправляет замечания обратно coder-ноде.

**Файлы**:
- `review.go` (новый) — парсинг вывода reviewer: статус PASS/FAIL, список issues
- `orchestrator.go` — после reviewer: если FAIL, повторить coder-ноду с `{review_issues}`, до `max_review_retries`
- `types.go` — поле `ReviewerFor string` в NodeDetail (ссылка на target ноду)
- `web/index.html` — конфиг reviewer-ноды: выбор target, max retries; визуализация review-цикла

**Протокол вывода reviewer**:
```
## Review: PASS
или
## Review: FAIL
## Issues
1. Описание проблемы...
2. ...
```

**Этапы**:
1. `review.go`: парсинг `## Review: PASS|FAIL` + извлечение issues
2. Orchestrator: после завершения reviewer → парсить вывод → при FAIL записать issues в `review.md` target-проекта → перезапустить target-ноду → повторить reviewer
3. Добавить счётчик итераций, ограничить `max_review_retries`
4. Статус ноды: "review round 2/3"
5. UI: визуальная индикация review-итераций (badge на ноде)

**Приоритет**: Средний — ключевая фича для code quality workflows.

---

## 6. Pipeline Templates & Variables

**Проблема**: Пайплайны жёстко привязаны к конкретным проектам и промптам. Нельзя переиспользовать пайплайн с другими параметрами.

**Решение**: Параметризованные пайплайны — переменные определяются при создании, значения задаются при запуске.

**Файлы**:
- `types.go` — `Variables []Variable` в `PipelineConfig`, `Variable{Name, Default, Description}`
- `prompt_expand.go` — поддержка `{var:name}` в промптах
- `server.go` — `POST /api/run` принимает `variables` map
- `web/index.html` — форма ввода переменных перед запуском, UI для определения переменных

**Этапы**:
1. Расширить `PipelineConfig` полем `Variables`
2. Добавить паттерн `{var:name}` в `expandPrompt()`
3. При запуске — если есть variables без значений, вернуть 400 со списком
4. UI: модальное окно перед запуском для ввода значений переменных
5. UI: секция в header/settings для определения переменных пайплайна

**Приоритет**: Средний — повышает переиспользуемость.

---

## 7. Улучшения Canvas UX

**Проблема**: Ряд мелких UX-проблем на canvas: нет undo, нет multi-select, нет copy-paste групп, нет minimap.

**Решение**: Инкрементальные улучшения canvas.

**Файлы**:
- `web/index.html` — вся логика фронта

### 7a. Undo/Redo
- Стек состояний Drawflow (`editor.export()`)
- Ctrl+Z / Ctrl+Shift+Z
- Максимум 50 шагов

### 7b. Multi-select и групповые операции
- Shift+Click для multi-select
- Drag-select (rubber band)
- Групповое перемещение, удаление, дублирование

### 7c. Minimap
- Canvas preview в углу для навигации по большим пайплайнам
- Click-to-navigate

### 7d. Auto-layout
- Кнопка для автоматического расположения нод по волнам (topological order)
- Dagre-like layout: волны = колонки, ноды в волне = строки

**Приоритет**: Низкий (7a, 7b — средний) — quality of life.

---

## 8. Node Output Preview & Artifact Browser

**Проблема**: После завершения пайплайна нет удобного способа просмотреть, что каждая нода произвела. Артефакты доставляются молча.

**Решение**: Панель просмотра артефактов + лог доставки файлов.

**Файлы**:
- `courier.go` — логирование доставки, возврат списка скопированных файлов
- `orchestrator.go` — сохранение delivery log в статус ноды
- `server.go` — `GET /api/run/{id}/artifacts/{nodeId}` — список артефактов
- `web/index.html` — таб "Artifacts" в log viewer, tree view файлов

**Этапы**:
1. `courier.go`: `deliverFile()` возвращает `[]DeliveryResult{Source, Dest, Size, Error}`
2. Сохранять результат доставки в `NodeStatus.Artifacts`
3. API endpoint для получения списка артефактов ноды
4. UI: таб "Artifacts" рядом с "Logs", показывает дерево файлов + статус доставки

**Приоритет**: Низкий — полезно для отладки.

---

## 9. Notifications

**Проблема**: При длительном запуске пользователь не узнает о завершении, если переключился на другую вкладку.

**Решение**: Browser notifications + опциональный webhook.

**Файлы**:
- `web/index.html` — `Notification API` при завершении/ошибке
- `types.go` — `Webhook string` в `PipelineConfig.Settings`
- `orchestrator.go` — HTTP POST на webhook при завершении

**Этапы**:
1. Фронт: запросить разрешение на уведомления при первом запуске
2. При получении статуса `done`/`error` — показать notification
3. Опционально: webhook URL в настройках пайплайна
4. Orchestrator: POST `{pipeline, status, elapsed, errors}` на webhook URL

**Приоритет**: Низкий — nice to have.

---

## 10. Улучшение безопасности

**Проблема**: Path traversal в `{read:...}` и `{files:...}`, нет санитизации имён файлов для pipeline/run ID.

**Решение**: Валидация и санитизация на границах.

**Файлы**:
- `prompt_expand.go` — `filepath.Clean()` + проверка что путь не выходит за workdir
- `server.go` — санитизация pipeline name и run ID (только `[a-zA-Z0-9_-]`)
- `courier.go` — проверка destination path

**Этапы**:
1. `prompt_expand.go`: после `filepath.Join()` проверить `strings.HasPrefix(resolved, baseDir)`
2. `server.go`: regex-валидация для всех path-параметров
3. `courier.go`: аналогичная проверка для destination

**Приоритет**: Высокий — предотвращает path traversal.

---

## Порядок реализации

| Фаза | Задачи | Обоснование |
|-------|--------|-------------|
| **Фаза 1** | #10 Security, #4 Validation | Исправление критичных проблем |
| **Фаза 2** | #1 SSE, #2 Streaming | Основа для всех real-time фич |
| **Фаза 3** | #3 Retry, #5 Review Loop | Надёжность и code quality |
| **Фаза 4** | #6 Templates, #7a Undo | Продуктивность пользователя |
| **Фаза 5** | #7b-d Canvas, #8 Artifacts, #9 Notifications | Polish |
