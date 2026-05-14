# RAG Chat Platform — Полное руководство

## Содержание

- [Обзор платформы](#обзор-платформы)
- [Архитектура](#архитектура)
- [Микросервисы](#микросервисы)
- [RAG Pipeline](#rag-pipeline)
- [Реестр моделей и динамические контейнеры](#реестр-моделей-и-динамические-контейнеры)
- [Совместный доступ и роли](#совместный-доступ-и-роли)
- [Автономная работа](#автономная-работа)
- [API Documentation](#api-documentation)
- [База данных](#база-данных)
- [Структура проекта](#структура-проекта)
- [Troubleshooting](#troubleshooting)

---

## Обзор платформы

RAG Chat Platform — микросервисная платформа для интеллектуального чата с документами. Пользователи регистрируются, создают ботов, загружают документы, ведут диалоги с историей и могут делиться ботами с коллегами. Администратор управляет пользователями и ботами на уровне всей платформы. Владелец бота может назначить ему конкретную LLM модель (базовую или собственную дообученную) и развернуть отдельный llama.cpp контейнер под неё.

### Ключевые возможности

- **Multi-user** — регистрация, JWT-аутентификация, RBAC (`user` / `admin`).
- **Управление ботами** — CRUD, поиск, пагинация, выбор LLM модели.
- **Совместный доступ** — приглашение коллабораторов на бота (`viewer` / `editor`).
- **Загрузка документов** — PDF, DOCX, TXT, CSV, JSON, HTML, Markdown, XLSX.
- **Advanced RAG** — Agentic Router, tiered retrieval, hybrid search, cosine + cross-encoder re-ranking, self-correction.
- **Streaming чат** — SSE стрим с остановкой генерации (AbortController на фронте, корректное закрытие апстрима на бэке).
- **История диалогов** — persistent conversations + messages; контекстное окно последних N сообщений передаётся модели.
- **Обратная связь** — thumbs-up / thumbs-down на каждое сообщение ассистента (доступно и в публичном чате).
- **Аналитика** — на бота: число диалогов, сообщений, среднее время ответа, статистика фидбека, активность по дням.
- **Админ-панель** — статистика платформы, управление пользователями и ботами.
- **Реестр моделей** — base (общие) + finetuned (только владельца). Деплой/стоп отдельных llama.cpp контейнеров через Docker API.
- **Публичный чат** — `/public/:bot_id` без авторизации, с возможностью оставить фидбек.
- **Темы оформления** — переключатель тёмной/светлой темы.
- **Полная автономность** — после `docker compose build` интернет не нужен.

### Технологический стек

| Компонент | Технология |
|-----------|------------|
| Frontend | React 18 + Vite + Nginx |
| Backend Gateway | Go 1.24 + Fiber v2 + GORM |
| Container Manager | Docker SDK for Go |
| Document Parser | Go 1.24 |
| Vector DB Service | Go 1.24 + Qdrant gRPC |
| AI Service | Python 3.10 + FastAPI + httpx |
| LLM Server | llama.cpp (Docker, OpenAI API) |
| Vector DB | Qdrant |
| Embeddings | intfloat/multilingual-e5-base (768D) |
| Reranker | cross-encoder/ms-marco-MiniLM-L-6-v2 |
| Database | PostgreSQL 15 |
| LLM | Qwen3.5-4B (GGUF по умолчанию, любая GGUF модель) |

---

## Архитектура

### Диаграмма

```
+-----------------------------------------------------------+
|                  Frontend (React + Nginx)                  |
|                     http://localhost:3000                  |
+-----------------------------+-----------------------------+
                              | HTTP/SSE
                              v
+-----------------------------------------------------------+
|              Backend Gateway (Go + Fiber) :8080            |
|   Auth (JWT), RBAC, Rate Limiting, RAG Orchestration,      |
|   Container Manager (Docker API)                           |
+----+----------------+------------------+------------------+
     |                |                  |
     v                v                  v
+-----------+   +------------+   +-----------------+
| Document  |   | Vector DB  |   |   AI Service    |
| Parser    |   | Service    |   |   (Python)      |
| :8081     |   | :8082      |   |   :8000         |
+-----------+   +-----+------+   +--------+--------+
                      |                   |
                      v                   v
               +------------+    +-----------------+
               |   Qdrant   |    | llama.cpp server|
               | :6333/6334 |    | :8090 (default) |
               +------------+    +-----------------+
                                          ^
                                          | (динамические контейнеры
                                          |  :8100-8199 для finetuned
                                          |  моделей, создаются backend'ом
                                          |  через Docker API)
```

### Разделение ответственности LLM и AI Service

| Контейнер | Роль | Модели |
|-----------|------|--------|
| **llama-cpp** (дефолтный) | LLM inference для base моделей | GGUF из `./models/` |
| **llama-ft-{id}** (динамические) | LLM inference для finetuned моделей | GGUF, созданный fine-tune-сервисом |
| **ai-service** | Embeddings, RAG pipeline, reranking | multilingual-e5-base, ms-marco-MiniLM |

**llama-cpp** — чистый LLM сервер на C++ с OpenAI-совместимым API (`/v1/chat/completions`). Загружает GGUF, выполняет inference. Может работать на CPU или GPU.

**ai-service** — Python сервис, который:
- Создает embeddings для документов и запросов (sentence-transformers).
- Выполняет semantic chunking документов.
- Реализует Advanced RAG pipeline (agentic router, hybrid search, reranking).
- Проксирует запросы генерации к llama-cpp через HTTP (httpx). Принимает `llm_endpoint` в теле запроса — backend подставляет туда `endpoint_url` модели бота, если она finetuned.

### Потоки данных

#### Загрузка документа

```
Пользователь -> Frontend -> Backend -> Document Parser (парсинг)
                                    -> AI Service (semantic chunking + embeddings)
                                    -> Vector DB Service -> Qdrant
                                    -> PostgreSQL (метаданные документа)
```

#### RAG запрос (Advanced)

```
Пользователь -> Frontend -> Backend
  Backend:
    1. Загружает бота + последние N сообщений диалога (CHAT_CONTEXT_WINDOW)
    2. Определяет LLM endpoint:
       - bot.model_id NULL или Type='base'  → дефолтный llama-cpp
       - Type='finetuned' + Status='running' → model.endpoint_url
    3. AI Service: embed запроса -> Vector DB Service -> Qdrant (поиск)
    4. AI Service: advanced search (rerank, hybrid, self-correction)
    5. AI Service -> llama.cpp (генерация со streaming)
  Backend сохраняет user+assistant сообщения в PostgreSQL
       <- SSE стрим обратно пользователю
       (первое событие: {"type":"meta","conversation_id":...,"message_id":...})
```

### Порядок запуска и зависимости

`depends_on` с `condition: service_healthy` для критичных зависимостей:

```
postgres (healthy) ─────────────────────────────┐
qdrant ──> vector-db ───────────────────────────┤
llama-cpp ──> ai-service ───────────────────────┤
                          document-parser ──────┤
                                                 v
                                          backend ──> frontend
```

Backend дополнительно ждёт healthcheck PostgreSQL. AI Service ждёт llama-cpp. Backend также пробует подключиться к Docker daemon при старте — если сокет не смонтирован, эндпоинты деплоя моделей возвращают 503, но всё остальное продолжает работать.

---

## Микросервисы

### 1. Frontend (React + Nginx)

**Порт:** 3000 | **Контейнер:** `chatbot-frontend`

Маршруты приложения (определяются inline-парсером в [App.jsx](frontend/src/App.jsx)):
- `/` — дашборд со списком ботов.
- `/chat/:botId` — чат с ботом (требует логина).
- `/public/:botId` — публичный чат, авторизация не нужна.
- `/analytics/:botId` — страница аналитики бота.
- `/admin` — админ-панель (только для пользователей с `role='admin'`).
- `/models` — реестр моделей с кнопками Deploy / Stop.

Компоненты:
- `Auth.jsx` / `Login.jsx` — формы входа и регистрации.
- `Dashboard.jsx` — список ботов с поиском, пагинацией, переходом в чат / аналитику / админку / реестр моделей.
- `BotForm.jsx` — создание/редактирование бота, выбор модели и коллабораторов.
- `BotChat.jsx` — чат с боковой панелью диалогов, остановкой генерации, фидбеком.
- `PublicChat.jsx` — упрощённый чат для публичного URL.
- `ChatArea.jsx` — область сообщений, кнопки thumbs-up/down, кнопка стоп.
- `ModelSettings.jsx` — temperature, top_p, top_k, max tokens, system prompt.
- `FileUpload.jsx` / `DocumentSearch.jsx` — управление документами.
- `Analytics.jsx` — карточки метрик и графики (см. ниже).
- `AdminPanel.jsx` — таблицы пользователей и ботов, общая статистика.
- `ModelsPage.jsx` — карточки моделей с Deploy / Stop / Delete.
- `Pagination.jsx` — переиспользуемый компонент пагинации.
- `ThemeToggle.jsx` — переключатель темы.

### 2. Backend Gateway (Go + Fiber + GORM)

**Порт:** 8080 | **Контейнер:** `chatbot-backend`

- API Gateway для всех запросов.
- JWT-аутентификация и `auth.Middleware`, `auth.AdminMiddleware`.
- Оркестрация RAG pipeline (координирует Document Parser, Vector DB, AI Service).
- CRUD ботов, диалогов, документов, коллабораторов, фидбека.
- Rate limiting (100 req/min per IP).
- SSE проксирование (с корректным завершением апстрима при отключении клиента).
- Container Manager — управление dynamic llama.cpp контейнерами через Docker SDK.
- Бутстрап админа из `ADMIN_EMAIL` / `ADMIN_PASSWORD` при старте.
- Seed base моделей из `./models/*.gguf` в БД при старте.
- Конфигурация полностью из `.env` (см. [CONFIGURATION.md](CONFIGURATION.md)).

Пакеты:
- `auth/` — JWT, middleware (общий и admin), bcrypt.
- `clients/` — HTTP клиенты к микросервисам, единый retry/timeout.
- `config/` — загрузка и валидация конфигурации.
- `database/` — модели GORM, репозитории, seed моделей, AutoMigrate с CHECK-constraints.
- `handlers/` — HTTP handlers (auth, bot, conversation, analytics, admin, model, RAG).
- `services/container_manager.go` — Docker SDK, deploy/stop/reconcile.
- `pagination/` — общая утилита пагинации.

### 3. Document Parser (Go)

**Порт:** 8081 | **Контейнер:** `chatbot-document-parser`

Парсинг документов в текст: PDF, DOCX, TXT, JSON, CSV, XLSX, HTML, Markdown.

### 4. Vector DB Service (Go + Qdrant gRPC)

**Порт:** 8082 | **Контейнер:** `chatbot-vector-db`

- Управление коллекциями по `bot_id` (коллекция `bot_{uuid}`).
- Добавление, поиск, удаление документов.
- Cosine similarity через gRPC.

### 5. AI Service (Python + FastAPI)

**Порт:** 8000 | **Контейнер:** `chatbot-ai-service`

- Создание embeddings (multilingual-e5-base, 768D, с `passage:` / `query:` prefix).
- Semantic chunking (RecursiveCharacterTextSplitter).
- Advanced RAG: Agentic Router, hybrid search, cosine re-ranking, cross-encoder reranking, self-correction, window retrieval.
- Проксирование генерации к llama.cpp через httpx (OpenAI Chat API). Поддерживает override `llm_endpoint` в теле запроса для finetuned моделей.
- Embedding и reranker модели подгружаются из bind-mount директорий (`./models/embedding`, `./models/hf-cache`).

**Внутренние endpoints AI Service:**

| Endpoint | Назначение |
|----------|-----------|
| `GET /health` | Health check |
| `POST /ask` | Генерация ответа (non-streaming) |
| `POST /generate` | Генерация ответа (streaming SSE) |
| `POST /embeddings` | Создание embeddings для текстов |
| `POST /advanced-search` | Advanced RAG search с reranking |
| `POST /split-document` | Semantic chunking документа |

### 6. llama.cpp server (Docker)

**Порт:** 8090 (внешний) / 8080 (внутренний) | **Контейнер:** `chatbot-llama-cpp`

- LLM inference через OpenAI-совместимый API.
- Поддержка GGUF моделей (Qwen3, LLaMA, Mistral и др.).
- CPU режим: `ghcr.io/ggml-org/llama.cpp:server`.
- GPU режим: `ghcr.io/ggml-org/llama.cpp:server-cuda`.
- Параллельные слоты (`LLAMA_PARALLEL`).
- Управление thinking-режимом через `--reasoning-budget` (для Qwen3, DeepSeek-R1, GLM).

### 7. Qdrant

**Порты:** 6333 (REST), 6334 (gRPC) | **Контейнер:** `chatbot-qdrant`

Векторная БД для хранения и семантического поиска документов. Данные хранятся в Docker volume `qdrant_storage`.

### 8. PostgreSQL

**Порт:** 5432 | **Контейнер:** `chatbot-postgres`

Хранение пользователей, ботов, документов, диалогов, сообщений, фидбека, коллабораторов, реестра моделей. Данные хранятся в Docker volume `postgres_data`. Схема создаётся и поддерживается через GORM `AutoMigrate` (см. [database/db.go](services/backend/database/db.go)). CHECK-констрейнты на role/type/status добавляются вручную через DDL.

---

## RAG Pipeline

### Advanced Agentic RAG

Платформа использует продвинутый RAG pipeline:

1. **Agentic Router** — классифицирует запросы: simple, global, enumeration, multi_hop, relation.
2. **Tiered Retrieval** — vector_search → hybrid_search → full_document_read.
3. **Window Retrieval** — подтягивание соседних чанков для полноты контекста.
4. **Cosine Re-ranking** — переранжирование по embedding similarity.
5. **Cross-Encoder Reranking** — точное переранжирование через ms-marco-MiniLM-L-6-v2.
6. **BM25 Hybrid Search** — комбинация vector + keyword search (веса настраиваются).
7. **Self-Correction Loop** — повторные попытки с перефразировкой при низкой релевантности.
8. **Enumeration Prompts** — специальные инструкции для запросов-списков.
9. **Language Matching** — детекция языкового несоответствия query/documents.
10. **Pipeline Trace** — структурированное логирование для отладки.

### Этапы работы

#### Индексация

```
1. Загрузка файла -> Document Parser (извлечение текста)
2. Semantic chunking (RecursiveCharacterTextSplitter, CHUNK_SIZE/CHUNK_OVERLAP)
3. Embeddings (multilingual-e5-base, 768D, с passage: prefix)
4. Сохранение в Qdrant (коллекция bot_{uuid})
5. Метаданные документа (filename, file_size, chunks_count) -> PostgreSQL
```

#### Генерация ответа

```
1. Запрос пользователя -> Backend
2. Backend подгружает последние CHAT_CONTEXT_WINDOW сообщений из conversation
3. Запрос -> Embedding (с query: prefix)
4. Vector search в Qdrant (до RAG_MAX_RESULTS кандидатов)
5. Advanced Search в AI Service:
   a. Agentic Router определяет тип запроса
   b. Tiered retrieval (vector -> hybrid -> full_document_read)
   c. Window retrieval (соседние чанки)
   d. Cross-encoder reranking
   e. Self-correction при низкой релевантности (RELEVANCE_ESCALATION_THRESHOLD)
6. Формирование контекста (до RAG_MAX_CONTEXT_CHARS символов)
7. Streaming генерация через llama.cpp:
   - bot.model_id NULL/base → дефолтный llama-cpp
   - finetuned → model.endpoint_url (через AI Service параметр llm_endpoint)
8. Сохранение user + assistant сообщений в conversations/messages
9. SSE стрим: {"type":"meta",...} -> chunks -> {"type":"done"}
```

---

## Реестр моделей и динамические контейнеры

### Принцип

Дообученная модель — самостоятельная сущность, отвязанная от бота. Один контейнер дообученной модели может обслуживать несколько ботов одного владельца.

### Типы моделей

| Type | owner_id | Видимость | Endpoint URL |
|------|----------|-----------|--------------|
| `base` | NULL | Все авторизованные пользователи | `http://llama-cpp:8080` (дефолтный) |
| `finetuned` | user_id владельца | Только владелец | `http://chatbot-llama-ft-{short_id}:8080` (свой контейнер) |

### Жизненный цикл finetuned модели

1. **Регистрация в БД** — запись в таблице `models` со `status='ready'` (после fine-tune сервиса) или импорт существующего GGUF.
2. **Deploy** — `POST /api/v1/models/:id/deploy`:
   - Container Manager выбирает свободный порт из `LLAMA_PORT_MIN..LLAMA_PORT_MAX`.
   - Создаёт контейнер `chatbot-llama-ft-{short_id}` (image: `ghcr.io/ggml-org/llama.cpp:server` или `:server-cuda` если `LLAMA_USE_GPU=true`).
   - Bind-mount `${LLAMA_MODELS_HOST_DIR}:/models` (абсолютный путь хоста!).
   - Сохраняет в БД `container_name`, `container_port`, `endpoint_url`, `status='running'`.
3. **Использование** — при создании/редактировании бота владелец выбирает модель из dropdown (`GET /api/v1/models`). Backend проверяет `CheckModelAccess(modelID, userID)` — finetuned доступна только владельцу.
4. **Роутинг запросов** — при RAG чате backend читает `bot.model_id` → `models.endpoint_url` → передаёт в AI Service как `llm_endpoint`.
5. **Stop** — `POST /api/v1/models/:id/stop` останавливает контейнер, `status='stopped'`.
6. **Reconcile** — при старте backend синхронизирует записи БД с фактическим состоянием Docker (очищает stale `running` записи).

### Бутстрап base моделей

При старте backend сканирует `MODELS_DIR` (`/models` внутри контейнера) на `*.gguf` файлы и регистрирует их как base модели. Активный файл из `GGUF_MODEL_FILE` помечается как тот, что обслуживается дефолтным `llama-cpp`. Идемпотентно: уже зарегистрированные файлы не дублируются.

### Требования

- Docker сокет должен быть смонтирован в backend (`/var/run/docker.sock:/var/run/docker.sock`) — это уже сделано в `docker-compose.yml`.
- `LLAMA_MODELS_HOST_DIR` должен быть **абсолютным** путём к `./models` на хосте, потому что Docker daemon не принимает относительные пути для bind-mount.

> **Fine-tune сервис** (генерация новых finetuned моделей через QLoRA / LoRA / Prompt Tuning / Adapter Tuning) находится в разработке — реестр моделей и инфраструктура контейнеров уже готовы.

---

## Совместный доступ и роли

### Роли пользователя (`users.role`)

| Роль | Возможности |
|------|-------------|
| `user` | Регистрация, создание/редактирование собственных ботов, доступ к share-ботам, реестр моделей (свои + base). |
| `admin` | Всё, что доступно `user`, плюс админ-панель: статистика, управление любыми пользователями и ботами. |

Бутстрап админа: при первом старте backend читает `ADMIN_EMAIL` / `ADMIN_PASSWORD` из `.env`:
- Если пользователя с таким email нет и `ADMIN_PASSWORD` задан — создаёт нового с `role='admin'`.
- Если уже есть — повышает до `admin`.

### Роли коллабораторов на бота (`bot_collaborators.role`)

| Роль | Возможности |
|------|-------------|
| `viewer` | Чат с ботом (включая историю диалогов). |
| `editor` | Чат + загрузка/удаление документов + редактирование настроек бота. |

Владелец (`bots.owner_id`) — отдельная роль, не хранится в `bot_collaborators`. Он может управлять списком коллабораторов.

Эндпоинты управления коллабораторами: `GET/POST/PUT/DELETE /api/v1/bots/:id/collaborators[/:user_id]`.

---

## Автономная работа

Платформа спроектирована для полной автономной работы без доступа к интернету после сборки Docker-образов.

### Где хранятся модели

| Модель | Путь хоста | Размер |
|--------|-----------|--------|
| LLM (GGUF) | `./models/*.gguf` (bind-mount в llama-cpp) | зависит от модели |
| Embedding (multilingual-e5-base) | `./models/embedding/` (bind-mount, заполняется при первой сборке) | ~1.1 GB |
| Reranker (ms-marco-MiniLM) | `./models/hf-cache/` (bind-mount, заполняется при первой сборке) | ~90 MB |
| NLTK (punkt_tab) | Встроен в образ `ai-service` | ~5 MB |

При первой сборке `ai-service` скачивает embedding и reranker модели и сохраняет их в bind-mount директории. При последующих запусках модели подгружаются из локального кеша.

### HF_TOKEN

Если модель embedding/reranker/LLM является gated на Hugging Face — задайте `HF_TOKEN` в `.env`. Для дефолтного стека (multilingual-e5-base + ms-marco-MiniLM + Qwen3) токен не требуется.

---

## API Documentation

Base URL: `http://localhost:8080`

### Публичные эндпоинты

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/health` | Health check |
| POST | `/api/v1/auth/register` | Регистрация |
| POST | `/api/v1/auth/login` | Вход |
| GET | `/api/v1/config/defaults` | Дефолтные параметры генерации |
| GET | `/api/v1/bots/:id` | Информация о боте (PublicBot — id, name, description) |
| POST | `/api/v1/chat/public/:bot_id` | Публичный чат (streaming SSE) |
| POST | `/api/v1/public/messages/:message_id/feedback` | Фидбек на сообщение из публичного чата |

### Защищённые (Authorization: Bearer TOKEN)

#### Аутентификация

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/auth/me` | Текущий пользователь (id, email, name, role) |

#### Боты

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/api/v1/bots` | Создать бота |
| GET | `/api/v1/bots` | Список ботов (свои + shared); поддерживает `?page&limit&search` |
| PUT | `/api/v1/bots/:id` | Обновить бота (owner или editor) |
| DELETE | `/api/v1/bots/:id` | Удалить бота (owner) |
| GET | `/api/v1/bots/:id/documents` | Документы бота |

#### Коллабораторы

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/bots/:id/collaborators` | Список коллабораторов |
| POST | `/api/v1/bots/:id/collaborators` | Пригласить (email + role) |
| PUT | `/api/v1/bots/:id/collaborators/:user_id` | Изменить роль (viewer/editor) |
| DELETE | `/api/v1/bots/:id/collaborators/:user_id` | Удалить коллаборатора |

#### Документы

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/api/v1/bots/:id/documents/upload` | Загрузить документ (multipart/form-data) |
| DELETE | `/api/v1/bots/:id/documents/:doc_id` | Удалить документ |

#### Чат, диалоги, фидбек

| Метод | Путь | Описание |
|-------|------|----------|
| POST | `/api/v1/chat/rag` | RAG чат (streaming SSE); тело принимает `conversation_id` |
| POST | `/api/v1/conversations` | Создать новый диалог |
| GET | `/api/v1/bots/:id/conversations` | Список диалогов бота |
| GET | `/api/v1/conversations/:conv_id` | Сообщения диалога |
| DELETE | `/api/v1/conversations/:conv_id` | Удалить диалог |
| POST | `/api/v1/messages/:message_id/feedback` | Thumbs-up/down (rating: 1 или -1) |
| GET | `/api/v1/bots/:id/feedback/stats` | Агрегированная статистика фидбека |

#### Аналитика

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/bots/:id/analytics` | Кол-во диалогов/сообщений, средн. время ответа, активность по дням |

#### Реестр моделей

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/models` | Список доступных моделей (base + свои finetuned) |
| GET | `/api/v1/models/:id` | Детали модели |
| POST | `/api/v1/models/:id/deploy` | Поднять контейнер (только владелец finetuned) |
| POST | `/api/v1/models/:id/stop` | Остановить контейнер |
| DELETE | `/api/v1/models/:id` | Удалить finetuned модель + контейнер |

#### Админ (role='admin')

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/admin/stats` | Общая статистика платформы |
| GET | `/api/v1/admin/users` | Все пользователи |
| PUT | `/api/v1/admin/users/:id/role` | Изменить роль (`user` / `admin`) |
| DELETE | `/api/v1/admin/users/:id` | Удалить пользователя |
| GET | `/api/v1/admin/bots` | Все боты |
| DELETE | `/api/v1/admin/bots/:id` | Удалить бота |

### Примеры

#### Регистрация

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "password123", "name": "User"}'
```

#### Создание бота с привязкой к модели

```bash
curl -X POST http://localhost:8080/api/v1/bots \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Bot",
    "temperature": 0.7,
    "system_prompt": "You are helpful.",
    "model_id": "<uuid-from-models-list>"
  }'
```

#### Загрузка документа

```bash
curl -X POST http://localhost:8080/api/v1/bots/$BOT_ID/documents/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@document.pdf"
```

#### RAG чат с указанием диалога

```bash
curl -N -X POST http://localhost:8080/api/v1/chat/rag \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": "<bot_id>",
    "conversation_id": "<conv_uuid-or-null>",
    "query": "Что такое машинное обучение?"
  }'
# SSE: {"type":"meta","conversation_id":"...","message_id":"..."}
#      data: {"chunk":"..."} ... data: [DONE]
```

#### Деплой finetuned модели

```bash
curl -X POST http://localhost:8080/api/v1/models/$MODEL_ID/deploy \
  -H "Authorization: Bearer $TOKEN"
# {"status":"running","endpoint_url":"http://chatbot-llama-ft-a1b2c3:8080","container_port":8103}
```

#### Фидбек на сообщение

```bash
curl -X POST http://localhost:8080/api/v1/messages/$MESSAGE_ID/feedback \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"rating": 1}'   # 1 = thumbs-up, -1 = thumbs-down
```

#### Аналитика бота

```bash
curl http://localhost:8080/api/v1/bots/$BOT_ID/analytics \
  -H "Authorization: Bearer $TOKEN"
# {
#   "total_conversations": 14,
#   "total_messages": 132,
#   "avg_response_time_ms": 2480,
#   "feedback": {"up": 38, "down": 4},
#   "messages_per_day": [{"date":"2026-05-10","count":21}, ...]
# }
```

---

## База данных

Схема создаётся через GORM `AutoMigrate` в [services/backend/database/db.go](services/backend/database/db.go). Все таблицы используют UUID (тип `uuid`, не SERIAL).

### Таблица `users`

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | Первичный ключ |
| email | VARCHAR(255) UNIQUE | Email пользователя |
| password_hash | VARCHAR(255) | Хэш пароля (bcrypt) |
| name | VARCHAR(255) | Имя |
| role | VARCHAR(20) | `user` или `admin` (CHECK constraint) |
| created_at, updated_at | TIMESTAMPTZ | Автозаполнение |

### Таблица `bots`

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| owner_id | UUID | FK на users.id |
| name | VARCHAR(255) | Имя бота |
| description | TEXT | Описание |
| config | JSONB | Доп. конфигурация |
| temperature, top_p, top_k, max_new_tokens, do_sample | — | Параметры генерации |
| system_prompt | TEXT | Системный промпт |
| chunk_size, chunk_overlap | INT | Параметры чанкинга при загрузке |
| context_window | INT | Override `CHAT_CONTEXT_WINDOW` для бота (0 = использовать дефолт) |
| model_id | UUID NULL | FK на models.id (NULL = дефолтный llama-cpp) |
| is_active | BOOLEAN | Активен ли |
| created_at, updated_at | TIMESTAMPTZ | — |

### Таблица `bot_documents`

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| bot_id | UUID | FK на bots.id |
| filename, file_type, file_size, chunks_count | — | Метаданные |
| uploaded_at | TIMESTAMPTZ | — |

### Таблица `conversations`

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| bot_id | UUID | FK на bots.id |
| user_id | UUID NULL | FK на users.id (NULL для публичных чатов) |
| title | VARCHAR(255) | Авто-генерируется из первого сообщения |
| created_at, updated_at | TIMESTAMPTZ | — |

### Таблица `messages`

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| conversation_id | UUID NULL | FK на conversations.id |
| bot_id | UUID NULL | Дублируется для быстрых выборок по боту |
| role | VARCHAR(20) | `user` / `assistant` / `system` |
| content | TEXT | Текст сообщения |
| metadata | JSONB | `response_time_ms`, источники, и т.п. |
| created_at | TIMESTAMPTZ | — |

### Таблица `message_feedback`

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| message_id | UUID | FK на messages.id |
| user_id | UUID NULL | FK на users.id (NULL для публичных оценок) |
| rating | SMALLINT | 1 = thumbs-up, -1 = thumbs-down |
| created_at | TIMESTAMPTZ | — |

UNIQUE constraint `(message_id, user_id)` — один пользователь может изменить оценку, но не дублировать.

### Таблица `bot_collaborators`

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| bot_id | UUID | FK на bots.id |
| user_id | UUID | FK на users.id |
| role | VARCHAR(20) | `viewer` или `editor` (CHECK constraint) |
| created_at | TIMESTAMPTZ | — |

UNIQUE constraint `(bot_id, user_id)`.

### Таблица `models`

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| owner_id | UUID NULL | FK на users.id (NULL для base моделей) |
| name | VARCHAR(255) | Отображаемое имя |
| type | VARCHAR(20) | `base` или `finetuned` (CHECK) |
| file_path | VARCHAR(500) | Путь к исходному файлу/директории |
| base_model_id | UUID NULL | Для finetuned — на какой base основана |
| gguf_path | VARCHAR(500) | Путь к GGUF (внутри `./models/`) |
| container_name | VARCHAR(255) | Имя docker-контейнера (для finetuned) |
| container_port | INT | Внутренний порт |
| endpoint_url | VARCHAR(500) | URL для AI Service (например `http://chatbot-llama-ft-a1b2c3:8080`) |
| status | VARCHAR(20) | `ready` / `training` / `converting` / `deploying` / `running` / `stopped` / `error` (CHECK) |
| parameters | JSONB | Гиперпараметры (для finetuned: epochs, lr, и т.п.) |
| created_at, updated_at | TIMESTAMPTZ | — |

---

## Структура проекта

```
chat-bot-platfrom/
├── .env                          # Единая конфигурация (git-ignored)
├── .env.example                  # Шаблон с описанием каждой переменной
├── docker-compose.yml            # Основная оркестрация (CPU)
├── docker-compose.gpu.yml        # GPU override для llama-cpp + динамических контейнеров
├── models/                       # GGUF модели + кеш embedding/reranker (git-ignored)
│   └── .gitkeep
├── scripts/
│   └── bench_parallel.py         # Бенчмарк параллельных SSE-запросов
├── frontend/
│   ├── Dockerfile
│   ├── nginx.conf.template       # Шаблон nginx (envsubst при старте)
│   ├── vite.config.js
│   └── src/
│       ├── api/client.js
│       ├── App.jsx               # Inline router
│       ├── hooks/useTheme.js
│       └── components/
│           ├── Auth.jsx, Login.jsx
│           ├── Dashboard.jsx, BotForm.jsx
│           ├── BotChat.jsx, PublicChat.jsx, ChatArea.jsx
│           ├── ModelSettings.jsx, FileUpload.jsx, DocumentSearch.jsx
│           ├── Analytics.jsx, AdminPanel.jsx, ModelsPage.jsx
│           ├── Pagination.jsx, ThemeToggle.jsx
│           └── *.css
├── services/
│   ├── backend/
│   │   ├── Dockerfile
│   │   ├── main.go               # Точка входа, роутинг, middleware
│   │   ├── auth/                 # JWT, middleware (общий + admin)
│   │   ├── clients/              # HTTP клиенты к микросервисам
│   │   ├── config/config.go      # Конфигурация из env
│   │   ├── database/             # Модели + репозитории + AutoMigrate
│   │   │   ├── db.go
│   │   │   ├── models.go
│   │   │   ├── user_repository.go
│   │   │   ├── bot_repository.go
│   │   │   ├── conversation_repository.go
│   │   │   ├── collaborator_repository.go
│   │   │   ├── model_repository.go
│   │   │   └── model_seed.go     # Seed base моделей из ./models/*.gguf
│   │   ├── handlers/             # auth, bot, conversation, analytics, admin, model, RAG
│   │   ├── services/
│   │   │   └── container_manager.go  # Docker SDK
│   │   ├── pagination/
│   │   ├── utils/
│   │   └── models/types.go
│   ├── document-parser-service/  # Парсеры PDF, DOCX, XLSX, CSV, JSON, HTML, MD, TXT
│   ├── vector-db-service/        # Qdrant gRPC
│   └── python-ai/
│       ├── Dockerfile
│       ├── requirements.txt
│       └── app/
│           ├── main.py
│           ├── config/settings.py
│           ├── api/routes.py
│           ├── models/schemas.py
│           └── services/
│               ├── model_service_gguf.py  # HTTP клиент для llama.cpp
│               └── rag_service.py         # Advanced RAG
├── CONFIGURATION.md              # Все переменные окружения
├── DEPLOYMENT.md                 # Развёртывание и управление
└── PLATFORM_GUIDE.md             # Это руководство
```

---

## Troubleshooting

### Контейнеры не запускаются

```bash
docker compose logs -f
docker compose ps
```

### llama.cpp не загружает модель

```bash
ls -lh models/*.gguf
docker compose logs llama-cpp
# Убедитесь что GGUF_MODEL_FILE в .env совпадает с именем файла в models/
```

### AI Service долго стартует

При первой сборке `ai-service` скачивает embedding (~1.1 GB) и reranker (~90 MB) модели в bind-mount директории `./models/embedding` и `./models/hf-cache`. После сборки модели загружаются из локального кэша (~10-20 секунд).

```bash
docker compose logs ai-service
# "Embedding model loaded, service ready" = готов
```

### Backend не подключается к сервисам

Backend ожидает healthcheck PostgreSQL и старт остальных зависимостей. Если один из сервисов не healthy/started, backend не запустится.

```bash
docker compose ps
curl http://localhost:8080/health
```

### Эндпоинты `/api/v1/models/:id/deploy` возвращают 503

Backend не смог подключиться к Docker daemon. Проверьте, что Docker сокет смонтирован в `docker-compose.yml`:

```yaml
backend:
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock
```

В логах backend при старте должно быть `✓ Docker daemon reachable`.

### Деплой finetuned модели падает с "invalid bind mount"

Это значит, что `LLAMA_MODELS_HOST_DIR` не указывает на абсолютный путь хоста. Docker daemon не принимает относительные пути.

```bash
# .env
LLAMA_MODELS_HOST_DIR=C:/files/Developing/diplom/chat-bot-platfrom/models   # Windows
LLAMA_MODELS_HOST_DIR=/home/user/chat-bot-platfrom/models                    # Linux
```

В bash docker-compose подставит `${PWD}/models` автоматически. В PowerShell — `$PWD` не экспортируется, задайте явно.

### Не работает админ-панель

Проверьте, что у пользователя `role='admin'`:

```bash
docker compose exec postgres psql -U chatbot -d chatbot -c "SELECT email, role FROM users;"
```

Если нужно повысить — либо задайте `ADMIN_EMAIL` в `.env` и перезапустите backend, либо вручную:

```sql
UPDATE users SET role = 'admin' WHERE email = 'me@example.com';
```

### Медленная генерация

1. GPU: `docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build`.
2. Увеличить `N_THREADS` в `.env`.
3. Уменьшить `N_CTX`.
4. Использовать меньшую/более квантизированную модель.

### GPU не отображается в Task Manager (Windows)

При использовании WSL2 + Docker Desktop нагрузка на GPU отображается через процесс `Vmmem`, а не в разделе GPU диспетчера задач. Это нормальное поведение WSL2. Проверить реальное использование GPU:

```bash
docker compose logs llama-cpp | grep -i "gpu\|layer\|CUDA"
# "offloaded 33/33 layers to GPU"
```

### Qdrant недоступен

```bash
curl http://localhost:6333/healthz
docker compose logs qdrant
```

---

Версия: 2.2
