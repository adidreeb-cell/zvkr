# Документация по Backend

## Содержание

- [Обзор архитектуры](#обзор-архитектуры)
- [Технологический стек](#технологический-стек)
- [Структура проекта](#структура-проекта)
- [Точка входа: cmd/gate/main.go](#точка-входа-cmdgatemain-go)
- [Конфигурация: internal/config/config.go](#конфигурация-internalconfigconfig-go)
- [База данных: internal/database/connect.go](#база-данных-internaldatabaseconnect-go)
- [Модели базы данных: internal/models/](#модели-базы-данных-internalmodels)
- [Маршрутизатор: internal/routes/routes.go](#маршрутизатор-internalroutesroutes-go)
- [Обработчики: internal/handlers/](#обработчики-internalhandlers)
- [Промежуточное ПО: internal/middleware/](#промежуточное-по-internalmiddleware)
- [Сервисы: internal/services/](#сервисы-internalservices)
- [Интеграции: internal/integrations/](#интеграции-internalintegrations)
- [API Endpoints](#api-endpoints)
- [Аутентификация](#аутентификация)
- [Интеграция с LLM](#интеграция-с-llm)
- [Python-песочница](#python-песочница)
- [Планировщик](#планировщик)
- [Конфигурация](#конфигурация)

---

## Обзор архитектуры

Backend построен на языке Go с использованием фреймворка Fiber. Предоставляет REST API для аналитики данных, интегрируется с несколькими провайдерами LLM и выполняет код Python в изолированной среде.

---

## Технологический стек

- **Фреймворк**: Fiber v2
- **База данных**: SQLite через GORM + BoltDB для кэширования
- **ORM**: GORM
- **LLM провайдеры**: Ollama, YandexGPT, GigaChat, OpenRouter
- **Песочница**: Wazero (WebAssembly Python runtime)
- **Планировщик**: robfig/cron v3
- **Аутентификация**: JWT (HS256) + bcrypt

---

## Структура проекта

```
internal/
├── config/         # Управление конфигурацией
├── database/       # Подключение к базе данных
├── handlers/       # Обработчики HTTP запросов
│   ├── auth.go
│   ├── analytics.go
│   ├── dataset.go
│   ├── export.go
│   ├── news.go
│   └── settings.go
├── middleware/     # Аутентификация и роли
│   ├── auth_middleware.go
│   └── role_middleware.go
├── models/         # Модели базы данных
│   ├── chat.go
│   ├── dataset.go
│   ├── education.go
│   ├── metrics.go
│   ├── migrate.go
│   ├── settings.go
│   └── user.go
├── routes/        # Настройка API маршрутов
│   └── routes.go
└── services/       # Бизнес-логика
    ├── ai_pipeline.go
    ├── analytics/  # Расчёт метрик
    │   └── metrics.go
    ├── auth/       # JWT аутентификация
    │   └── jwt.go
    ├── email/      # IMAP интеграция
    │   └── imap.go
    ├── emulator/   # Генерация данных
    │   └── emulator.go
    ├── erp/        # ERP интеграция
    │   └── erp.go
    ├── excel/      # Парсинг Excel
    │   └── parser.go
    ├── export/     # Экспорт в Excel/PDF
    │   ├── excel.go
    │   └── pdf.go
    ├── llm/        # Интерфейсы и реализации LLM
    │   ├── llm.go
    │   ├── openrouter.go
    │   ├── gigachat/gigachat.go
    │   ├── ollama/ollama.go
    │   └── yandex/yandex.go
    ├── moodle/     # Moodle интеграция
    │   └── moodle.go
    ├── sandbox/    # Выполнение Python (Wazero)
    │   ├── sandbox.go
    │   └── wazero.go
    └── scheduler/  # Cron задачи
        └── cron.go
cmd/gate/main.go    # Точка входа в приложение
integrations/
└── news.go         # RSS-парсер новостей
```

---

## Точка входа: cmd/gate/main.go

**Назначение:** Главная функция приложения. Инициализирует все компоненты, запускает HTTP-сервер и управляет его жизненным циклом.

### Функции

#### `buildLLMService(cfg *config.Config) llm.LLMService`

Фабрика LLM-провайдеров. Читает поле `cfg.LLMProvider` и возвращает нужную реализацию.

- **Принимает:** объект конфигурации `*config.Config`
- **Возвращает:** интерфейс `llm.LLMService` (любой провайдер реализует его)
- **Зачем нужна:** позволяет менять провайдера через настройки, не меняя остальной код

#### `main()`

Основная функция. Работает в бесконечном цикле (`for {}`), что позволяет **мягко перезапускать** сервер при изменении настроек без остановки процесса.

**Шаги выполнения:**
1. Открывает SQLite (`doctor.db`) и BoltDB (`cache.db`).
2. Создаёт бакет кэша в BoltDB.
3. Запускает миграцию таблиц БД (`models.Migrate`).
4. Инициализирует Python WebAssembly-рантайм.
5. Входит в цикл перезапуска:
   - Читает актуальную конфигурацию (из файла `.env` + из БД).
   - Создаёт все сервисы и обработчики.
   - Запускает HTTP-сервер Fiber в отдельной горутине.
   - Ждёт сигнала остановки (`Ctrl+C`) или перезагрузки (канал `reloadCh`).
   - При перезагрузке: останавливает cron и Fiber, выполняет новый круг цикла.
   - При остановке: завершает программу.

---

## Конфигурация: internal/config/config.go

**Назначение:** Управление настройками приложения — сначала читает переменные окружения (файл `.env`), затем позволяет перезаписать их значениями из базы данных.

### Структура `Config`

```go
type Config struct {
    Port      string    // Порт сервера (по умолчанию: 6331)
    DBPath    string    // Путь к SQLite базе (по умолчанию: doctor.db)
    JWTSecret []byte    // Секрет для подписи JWT токенов

    // LLM настройки
    LLMProvider      string  // "openrouter" | "ollama" | "yandex" | "gigachat"
    OpenrouterApiKey string
    OpenrouterModel  string  // По умолчанию: "google/gemma-3-27b-it"
    OllamaURL        string  // По умолчанию: "http://localhost:11434"
    OllamaModel      string  // По умолчанию: "llama3"
    YandexFolderID   string
    YandexAPIKey     string
    YandexModel      string
    GigaChatClientID string
    GigaChatSecret   string

    // Флаги включения модулей
    EnableIMAP     bool
    EnableMoodle   bool
    EnableUnivDB   bool
    EnableEmulator bool
}
```

### Функции

#### `Load() *Config`

- **Принимает:** ничего (читает переменные окружения)
- **Возвращает:** заполненный объект конфигурации
- **Зачем нужна:** централизованная загрузка настроек при старте

#### `(cfg *Config) UpdateFromDB(s models.SystemSetting)`

Перезаписывает поля конфига значениями из базы данных. Вызывается при каждом перезапуске цикла в `main.go`.

- **Принимает:** объект настроек из БД
- **Зачем нужна:** позволяет менять провайдеров LLM и другие параметры «на лету» через интерфейс, без редактирования файлов

---

## База данных: internal/database/connect.go

**Назначение:** Установка соединения с SQLite базой данных.

#### `Connect() *gorm.DB`

- **Принимает:** ничего (путь захардкожен как `"doctor.db"`)
- **Возвращает:** объект подключения `*gorm.DB`
- **Зачем нужна:** единая точка создания подключения, используется в `main.go`

> **Для нетехнических читателей:** GORM — это «переводчик» между Go-кодом и SQL. Вы пишете `db.Find(&users)`, а GORM сам формирует запрос `SELECT * FROM users`.

---

## Модели базы данных: internal/models/

Каждый файл описывает одну или несколько таблиц в базе данных.

### `User` (internal/models/user.go)

Таблица пользователей системы.

| Поле | Тип | Описание |
|------|-----|---------|
| `ID` | uint | Уникальный номер (первичный ключ) |
| `Username` | string | Логин (уникальный, обязательный) |
| `PasswordHash` | string | bcrypt-хэш пароля (никогда не попадает в JSON) |
| `Role` | RoleType | Роль: `"admin"`, `"analyst"`, `"user"` |
| `CreatedAt`, `UpdatedAt` | time.Time | Временные метки |

### `Dataset` (internal/models/dataset.go)

Хранит загруженный файл данных.

| Поле | Тип | Описание |
|------|-----|---------|
| `ID` | uint | Первичный ключ |
| `Name` | string | Имя файла |
| `Source` | string | Источник: `"upload"`, `"email"`, `"moodle"`, `"erp_database"`, `"emulator"` |
| `Headers` | JSON | Массив названий колонок |
| `Data` | JSON | Массив строк данных (каждая строка — объект) |
| `Summary` | string | Текстовая сводка, сгенерированная после обработки |
| `IsProcessed` | bool | Флаг: прошёл ли датасет ИИ-анализ |
| `ColumnMapping` | JSON | Кэшированный маппинг колонок от LLM |

### `GlobalMetric` (internal/models/metrics.go)

Агрегированные метрики по всем датасетам. В таблице всегда одна запись (ID=1).

| Поле | Тип | Описание |
|------|-----|---------|
| `TotalStudents` | int64 | Общее число студентов |
| `ActiveStudents` | int64 | Число активно обучающихся |
| `AverageScore` | float64 | Средний балл |
| `StatusBreakdown` | JSON | Разбивка по статусам (словарь: статус → количество) |
| `TimeSeries` | JSON | Массив данных по периодам (годам) |
| `Trends` | JSON | Тренды от LLM (массив строк) |
| `Anomalies` | JSON | Аномалии от LLM (массив строк) |

### `ProcessedPeriod` (internal/models/metrics.go)

Таблица для предотвращения конфликта при мерже: каждый год может принадлежать только одному датасету.

| Поле | Описание |
|------|---------|
| `Period` | Год (уникальный индекс, например "2022") |
| `DatasetID` | ID датасета-владельца этого года |

### `ChatMessage` (internal/models/chat.go)

История переписки с ИИ в контексте датасета.

| Поле | Описание |
|------|---------|
| `DatasetID` | К какому датасету относится сообщение |
| `Role` | `"user"` или `"bot"` |
| `Content` | Текст сообщения (Markdown) |
| `CodeOutput` | JSON-результат выполнения Python |
| `SourceCode` | Сгенерированный Python-код |
| `IsError` | Флаг ошибки |

### `SystemSetting` (internal/models/settings.go)

Настройки системы (одна запись, ID=1). Хранит ключи API, URL сервисов, параметры cron.

### `Student`, `Grade` (internal/models/education.go)

Вспомогательные модели для ручного хранения данных о студентах и оценках. В текущей версии используются опосредованно — данные хранятся в `Dataset.Data` как JSON.

### `migrate.go`

#### `Migrate(db *gorm.DB) error`

Автоматически создаёт или обновляет все таблицы в БД на основе Go-структур. Вызывается при каждом старте приложения.

- **Принимает:** объект подключения к БД
- **Возвращает:** ошибку (если миграция не удалась)

---

## Маршрутизатор: internal/routes/routes.go

**Назначение:** Регистрация всех HTTP-маршрутов и встраивание фронтенда.

### Ключевые функции

#### `SetupRoutes(...)`

Регистрирует все маршруты и запускает первоначальную инициализацию:

1. Вызывает `authHandler.InitAdminUser()` — создаёт admin-пользователя если его нет.
2. Открывает браузер (`/setup` или `/`) в зависимости от того, первый ли запуск.
3. Регистрирует публичные маршруты (`/api/v1/login`, `/api/v1/news`, ...).
4. Регистрирует защищённые маршруты (под `middleware.Protected`).
5. Встраивает фронтенд из `dist/` с SPA-fallback.

#### `openBrowser(url string) error`

Кроссплатформенное открытие браузера (Windows: `rundll32`, macOS: `open`, Linux: `xdg-open`).

---

## Обработчики: internal/handlers/

### AuthHandler (auth.go)

Управляет пользователями и аутентификацией.

#### `InitAdminUser() (string, bool, error)`
Создаёт первого admin-пользователя с случайным 32-символьным паролем. Если admin уже существует — ничего не делает.
- **Возвращает:** (сгенерированный пароль, флаг «был создан», ошибка)

#### `GetSetupInfo(c *fiber.Ctx) error`
Возвращает логин и пароль для первого входа. Работает только пока `SetupPassword` не пуст.
- **Маршрут:** `GET /api/v1/system/setup`

#### `CompleteSetup(c *fiber.Ctx) error`
Стирает пароль из памяти — теперь его нельзя получить через API.
- **Маршрут:** `POST /api/v1/system/setup/complete`

#### `Login(c *fiber.Ctx) error`
Авторизация: принимает `{username, password}`, проверяет bcrypt-хэш, возвращает JWT-токен.
- **Маршрут:** `POST /api/v1/login`
- **Возвращает:** `{token, role}`

#### `Register(c *fiber.Ctx) error`
Регистрация нового пользователя (публичный маршрут, но роль по умолчанию — `"user"`).
- **Маршрут:** `POST /api/v1/register`

#### `GetUsers`, `CreateUser`, `DeleteUser`
Управление пользователями (только для администратора).

### DatasetHandler (dataset.go)

Ключевой обработчик — управляет датасетами и чатом с ИИ.

#### `Upload(c *fiber.Ctx) error`
Принимает загруженный файл (multipart), парсит его через `ExcelService`, сохраняет в БД.
- **Маршрут:** `POST /api/v1/upload`
- **Возвращает:** `{id, rows_count, filename}`

#### `ListDatasets(c *fiber.Ctx) error`
Возвращает список всех датасетов (только метаданные, без данных).
- **Маршрут:** `GET /api/v1/datasets`

#### `GetDataset(c *fiber.Ctx) error`
Возвращает полный датасет включая все строки данных.
- **Маршрут:** `GET /api/v1/datasets/:id`

#### `Chat(c *fiber.Ctx) error`
Основная функция чата. Принимает `{message, use_code, use_news}` и выбирает стратегию анализа:
- `use_code=true` → Code Interpreter (Python)
- `use_code=false` → Map-Reduce текстовый анализ

При `use_news=true` добавляет последние новости образования в контекст промпта.

- **Маршрут:** `POST /api/v1/datasets/:id/chat`

#### `GetChatHistory(c *fiber.Ctx) error`
Возвращает историю сообщений для датасета.
- **Маршрут:** `GET /api/v1/datasets/:id/chat`

#### `RunPython(c *fiber.Ctx) error`
Выполняет произвольный Python-код в песочнице.
- **Маршрут:** `POST /api/v1/python/run`

#### `handleCodeAnalysis(...)` (приватная)
Реализует стратегию Code Interpreter с итеративными попытками:

1. Строит промпт с описанием структуры данных.
2. Запрашивает Python-код у LLM.
3. Очищает (`sanitizeCode`) и валидирует код.
4. Добавляет «охранный блок» (sandbox guard).
5. Выполняет через Wazero.
6. Проверяет контракт вывода (JSON с meta, summary, charts).
7. При ошибке — повторяет до 3 раз.
8. Генерирует текстовый отчёт по JSON-результату.

#### `handleTextAnalysisSmart(...)` (приватная)
Реализует Map-Reduce текстовый анализ:

- До 50 строк: прямой запрос к LLM.
- Больше 50 строк: разбивает на части (до 5 чанков по 50 строк), анализирует параллельно, объединяет результаты финальным LLM-запросом.

#### `buildAnalyticalPythonPrompt(...)` (приватная)
Формирует детальный промпт для генерации Python-кода, включая:
- Схему данных (типы и примеры значений полей).
- Строгие правила (нельзя pandas/numpy, нельзя redefine dataset, обязательный контракт вывода).
- Шаблон функции для копирования.
- Описание предыдущей ошибки (если это повторная попытка).

### AnalyticsHandler (analytics.go)

#### `GetBasicMetrics(c *fiber.Ctx) error`
Возвращает метрики только за **последний** доступный период (год).
- **Маршрут:** `GET /api/v1/analytics/metrics`

#### `GetAdvancedAnalytics(c *fiber.Ctx) error`
Возвращает полную аналитику с возможностью фильтрации по дате.
- **Маршрут:** `GET /api/v1/analytics/advanced`
- **Параметры:** `?from=2020&to=2023`

#### `GetSyncStatus(c *fiber.Ctx) error`
Возвращает статус обработки: сколько датасетов ожидают обработки.
- **Маршрут:** `GET /api/v1/analytics/status`

#### `ForceSync(c *fiber.Ctx) error`
Запускает немедленную обработку всех необработанных датасетов.
- **Маршрут:** `GET /api/v1/analytics/sync`

### ExportHandler (export.go)

#### `ExportDatasetExcel(c *fiber.Ctx) error`
Конвертирует датасет в XLSX и отдаёт как скачиваемый файл.
- **Маршрут:** `GET /api/v1/export/excel/:id`

#### `ExportDatasetPDF(c *fiber.Ctx) error`
Конвертирует датасет в PDF (таблица) и отдаёт как скачиваемый файл.
- **Маршрут:** `GET /api/v1/export/pdf/:id`

### SettingsHandler (settings.go)

#### `GetSettings(c *fiber.Ctx) error`
Возвращает текущие системные настройки. Секретные поля (пароли, API-ключи) скрываются маской `"********"`.
- **Маршрут:** `GET /api/v1/settings`

#### `UpdateSettings(c *fiber.Ctx) error`
Сохраняет настройки в БД, применяет их к конфигу в памяти и через 5 секунд перезапускает сервер.
- **Маршрут:** `POST /api/v1/settings`
- Поле не перезаписывается, если пришла маска или пустая строка (защита от случайного удаления ключей).

---

## Промежуточное ПО: internal/middleware/

### auth_middleware.go

#### `Protected(secret []byte) fiber.Handler`

Проверяет JWT-токен в заголовке `Authorization: Bearer <token>`.
- Извлекает из токена `user_id` и `role`, кладёт в контекст запроса через `c.Locals`.
- При невалидном токене — возвращает `401 Unauthorized`.

#### `RoleCheck(allowedRoles ...string) fiber.Handler`

Проверяет, есть ли у пользователя нужная роль.
- `admin` имеет доступ ко всему автоматически.
- При недостаточных правах — возвращает `403 Forbidden`.

---

## Сервисы: internal/services/

### AIPipeline (ai_pipeline.go)

Оркестратор ИИ-анализа. Содержит два независимых алгоритма.

#### `AnalyzeWithCode(ctx, query, data) (map, error)`
Code Interpreter: просит LLM написать Python, выполняет через Wazero, возвращает JSON + отчёт.
- До 3 попыток при ошибках.

#### `AnalyzeWithMapReduce(ctx, query, data) (map, error)`
Map-Reduce: для больших датасетов (>50 строк) разбивает на чанки, анализирует параллельно.

### AnalyticsService (analytics/metrics.go)

Ключевой сервис системы. Обрабатывает датасеты и строит глобальные метрики.

#### `ProcessUnprocessedDatasets(ctx) error`
Находит все необработанные датасеты и запускает полный пайплайн:
1. Маппинг колонок (через LLM).
2. Детерминированный подсчёт метрик.
3. Мердж в глобальную таблицу.
4. Генерация текстовых инсайтов (через LLM).

#### `resolveColumnMapping(ctx, ds, data)` (приватная)
Если маппинг уже сохранён в `Dataset.ColumnMapping` — использует его.
Иначе запрашивает у LLM с кэшированием результата.

#### `detectColumnMapping(ctx, data)` (приватная)
Строит промпт с 5 примерами строк и уникальными значениями каждой колонки. LLM возвращает JSON с сопоставлением: `{"student_id": "ФИО", "status": "Статус", ...}`.

До 3 попыток при ошибках разбора JSON.

#### `computeMetricsDeterministic(data, mapping)` (приватная)
**Не использует LLM** — чистая математика на Go:

1. Дедупликация строк по ключу `(studentID + period)`.
2. Для каждой строки: определяет период (год поступления), статус, балл.
3. Аккумулирует данные по периодам и глобально.
4. Считает средний балл как `sum(баллов) / count(баллов)`.

#### `mergeIntoGlobalMetrics(ctx, newData, datasetID)` (приватная)
В транзакции обновляет глобальную запись метрик. Использует механизм **ownership периодов**: каждый год может принадлежать только одному датасету (первому, кто его занял). Это предотвращает двойной счёт студентов.

#### `GetAdvancedMetrics(startDate, endDate)` (публичная)
Читает GlobalMetric из БД и фильтрует TimeSeries по диапазону дат. Пересчитывает агрегированные метрики по отфильтрованному набору.

#### `recalcMetricsFromTimeSeries(timeSeries)` (приватная)
Суммирует данные по всем периодам в один объект MetricsData. Корректно считает средний балл через накопленные `ScoreSum` / `ScoreCount`.

### AuthService (auth/jwt.go)

#### `HashPassword(password) (string, error)`
Хэширует пароль с bcrypt (cost=14). Результат нельзя обратно декодировать.

#### `CheckPassword(password, hash) bool`
Проверяет пароль против сохранённого хэша.

#### `GenerateToken(userID, role, secret) (string, error)`
Создаёт JWT-токен со сроком жизни 72 часа.

### ExcelService (excel/parser.go)

#### `Parse(reader, filename) ([]string, []map, error)`
Универсальный парсер: автоматически определяет формат по расширению файла.

- `.csv` → `parseCSV()` через стандартную библиотеку Go
- `.xlsx`, `.xls` → `parseExcel()` через библиотеку Excelize

**Возвращает:**
- `[]string` — список заголовков колонок
- `[]map[string]interface{}` — строки данных (каждая строка — словарь `{колонка: значение}`)

### WasmRunner (sandbox/wazero.go)

Python WebAssembly рантайм.

#### `NewWasmRunner(ctx) (*WasmRunner, error)`
Инициализирует Wazero рантайм и компилирует встроенный `python.wasm`. Вызывается один раз при старте приложения. Компиляция занимает несколько секунд.

#### `Execute(ctx, code, data) (string, error)`
Выполняет Python-код с переданными данными:

1. `prepareScript()` — добавляет преамбулу: загружает `data` в переменную `dataset` через `json.loads()`.
2. Передаёт скрипт через stdin (`python -`).
3. Собирает stdout как результат, stderr как лог ошибок.
4. Интерпретирует `sys.exit(0)` как успешное завершение.

Ограничения: 128 МБ памяти, 20 секунд, нет ФС, нет сети.

### ExportService (export/excel.go, export/pdf.go)

#### `GenerateExcel(rawData, sheetName) (*bytes.Buffer, error)`
Создаёт XLSX-файл в памяти из JSON-данных. Первая строка — заголовки, далее данные.

#### `GeneratePDF(rawData, title) (*bytes.Buffer, error)`
Создаёт PDF-таблицу в горизонтальной ориентации (Landscape A4). Шрифт Arial встроен прямо в бинарник через `embed`. Длинные строки обрезаются до 20 символов.

### IMAPService (email/imap.go)

#### `FetchUnreadExcel()`
Подключается к почтовому серверу через IMAP (TLS), ищет непрочитанные письма с вложениями `.xlsx`/`.xls`/`.csv`, парсит их и сохраняет как датасеты. Помечает обработанные письма как прочитанные.

### ERPService (erp/erp.go)

#### `SyncFromDB()`
Подключается к внешней базе данных университета (PostgreSQL, MySQL или SQL Server) и делает снапшот таблицы `students` (первые 1000 строк). Сохраняет результат как датасет.

### MoodleService (moodle/moodle.go)

#### `FetchData()`
Обращается к Moodle REST API (`core_course_get_courses`) и загружает список курсов как датасет.

### EmulatorService (emulator/emulator.go)

#### `GenerateMockDrift()`
Генерирует синтетический датасет ~1500 студентов со случайными распределением по факультетам, статусам и баллам. Используется для тестирования системы без реальных данных.

### Scheduler (scheduler/cron.go)

#### `Start(db, boltDB, analyticsSvc) *cron.Cron`
Регистрирует все фоновые задачи. Расписание берётся из настроек БД (или использует значения по умолчанию).

| Задача | Расписание по умолчанию |
|--------|------------------------|
| Новости (RSS) | каждые 3 часа |
| Почта (IMAP) | каждые 30 минут |
| Аналитика (LLM) | каждый час |
| Moodle | каждые 6 часов |
| ERP | каждые 24 часа |

При старте сервера новости и аналитика запускаются **немедленно** в горутине (не ждут первого срабатывания cron).

---

## Интеграции: internal/integrations/

### OfficialNewsModule (news.go)

#### `NewOfficialNewsModule(db *bbolt.DB) *OfficialNewsModule`
Создаёт парсер с двумя источниками по умолчанию: Минпросвещения РФ и Рособрнадзор.

#### `Run(ctx, gormDB) error`
Получает список RSS-источников из БД (или использует дефолтные). Загружает последние 10 новостей от каждого источника, сортирует по дате, сохраняет как JSON в BoltDB (`DashboardCache` → `official_news`). HTML-теги очищаются библиотекой bluemonday.

---

## API Endpoints

### Публичные маршруты

| Метод | Endpoint | Описание |
|--------|----------|-------------|
| GET | `/api/v1/system/setup` | Получить пароль настройки (первый запуск) |
| POST | `/api/v1/system/setup/complete` | Завершить начальную настройку |
| POST | `/api/v1/register` | Регистрация нового пользователя |
| POST | `/api/v1/login` | Вход пользователя |
| GET | `/api/v1/news` | Получить кэшированные новости |

### Защищённые маршруты (требуется JWT)

#### Админ
| Метод | Endpoint | Описание |
|--------|----------|-------------|
| GET | `/api/v1/users` | Список пользователей |
| POST | `/api/v1/users/add` | Создать пользователя |
| POST | `/api/v1/users/remove` | Удалить пользователя |
| GET | `/api/v1/settings` | Получить настройки системы |
| POST | `/api/v1/settings` | Обновить настройки |

#### Датасеты
| Метод | Endpoint | Описание |
|--------|----------|-------------|
| POST | `/api/v1/upload` | Загрузить Excel/CSV файл |
| GET | `/api/v1/datasets` | Список датасетов |
| GET | `/api/v1/datasets/:id` | Детали датасета |

#### Аналитика
| Метод | Endpoint | Описание |
|--------|----------|-------------|
| GET | `/api/v1/analytics/sync` | Принудительная обработка датасетов |
| GET | `/api/v1/analytics/metrics` | Базовые метрики (последний период) |
| GET | `/api/v1/analytics/advanced` | Расширенная аналитика с фильтрацией по датам |
| GET | `/api/v1/analytics/status` | Статус синхронизации |

#### Чат/ИИ
| Метод | Endpoint | Описание |
|--------|----------|-------------|
| GET | `/api/v1/datasets/:id/chat` | История чата |
| POST | `/api/v1/datasets/:id/chat` | Отправить сообщение ИИ |

#### Python-песочница
| Метод | Endpoint | Описание |
|--------|----------|-------------|
| POST | `/api/v1/python/run` | Выполнить код Python |

#### Экспорт
| Метод | Endpoint | Описание |
|--------|----------|-------------|
| GET | `/api/v1/export/excel/:id` | Экспорт датасета в Excel |
| GET | `/api/v1/export/pdf/:id` | Экспорт датасета в PDF |

---

## Аутентификация

JWT токены используются для аутентификации. Роли:
- `admin` — Полный доступ
- `analyst` — Загрузка, аналитика, чат
- `user` — Только чтение

---

## Интеграция с LLM

### Интерфейс LLMService (llm/llm.go)

```go
type LLMService interface {
    AnalyzeData(ctx context.Context, question string, dataPreview interface{}) (string, error)
}
```

Все четыре провайдера реализуют этот интерфейс. Переключение провайдера происходит в `cmd/gate/main.go` без изменения кода сервисов.

### Поддерживаемые провайдеры

**1. OpenRouter** (`llm/openrouter.go`) — по умолчанию

Использует совместимый с OpenAI API протокол. Добавляет заголовки `HTTP-Referer` и `X-Title` для идентификации на платформе.

- Модель по умолчанию: `google/gemma-3-27b-it`
- Температура: 0.1 (детерминированные ответы)
- Базовый URL: `https://openrouter.ai/api/v1`

**2. Ollama** (`llm/ollama/ollama.go`) — локальный

Обращается к локальному Ollama-серверу. Не требует API-ключа.

- URL по умолчанию: `http://localhost:11434`
- Модель по умолчанию: `llama3`

**3. YandexGPT** (`llm/yandex/yandex.go`)

Использует библиотеку `neuron-nexus/yandexgpt`. Требует FolderID и IAM/API-ключ.

- Температура: 0.1

**4. GigaChat** (`llm/gigachat/gigachat.go`)

Российский языковой провайдер. Требует ClientID и ClientSecret (OAuth).

### AI конвейер

Система поддерживает два режима анализа:

**Режим кода** (Code Interpreter):
- Генерирует Python-код для анализа данных.
- Код выполняется в Wazero-песочнице.
- Проверяется на безопасность (без pandas/numpy, без файловых операций).
- Возвращает JSON с метриками и графиками.

**Текстовый режим** (Map-Reduce):
- Прямой анализ LLM.
- Использует Map-Reduce для больших датасетов.
- Разбивает данные на чанки (50 строк на чанк).
- Объединяет сводки в финальный ответ.

---

## Python-песочница

Использует Wazero WebAssembly runtime для безопасного выполнения кода Python:
- Встроенный Python WASM бинарник (`python.wasm`)
- Ограничение памяти: 128 МБ
- Нет доступа к файловой системе
- Нет сетевого доступа
- Таймаут: 20 секунд на выполнение

### Контракт выполнения

Сгенерированный код должен выводить валидный JSON:
```json
{
  "meta": {
    "scanned_rows": 100,
    "used_rows": 95,
    "total_rows_expected": 100
  },
  "summary": { },
  "charts": [ ]
}
```

---

## Планировщик

Cron задачи для фоновых процессов:
- **Новости**: Каждые 3 часа (получение RSS)
- **IMAP**: Каждые 30 минут (проверка почты)
- **Аналитика**: Каждый час (обработка датасетов)
- **Moodle**: Каждые 6 часов
- **ERP**: Каждые 24 часа

---

## Конфигурация

Переменные окружения (также хранятся в БД):

| Переменная | По умолчанию | Описание |
|-----------|-------------|---------|
| `PORT` | `6331` | Порт сервера |
| `DB_PATH` | `doctor.db` | Путь к SQLite базе |
| `JWT_SECRET` | `super-secret-key` | Ключ для подписи JWT |
| `LLM_PROVIDER` | `openrouter` | Выбранный LLM провайдер |
| `OPENROUTER_API_KEY` | — | API-ключ OpenRouter |
| `OPENROUTER_MODEL` | `google/gemma-3-27b-it` | Модель OpenRouter |
| `OLLAMA_URL` | `http://localhost:11434` | URL Ollama |
| `OLLAMA_MODEL` | `llama3` | Модель Ollama |

## Интеграция с Frontend

Backend обслуживает frontend как встроенные статические файлы и предоставляет REST API. Все API эндпоинты возвращают JSON ответы.
