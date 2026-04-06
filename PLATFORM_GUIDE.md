# RAG Chat Platform - Полное руководство

## Содержание

- [Обзор платформы](#обзор-платформы)
- [Архитектура](#архитектура)
- [Микросервисы](#микросервисы)
- [RAG Pipeline](#rag-pipeline)
- [Автономная работа](#автономная-работа)
- [API Documentation](#api-documentation)
- [База данных](#база-данных)
- [Разработка](#разработка)
- [Troubleshooting](#troubleshooting)

---

## Обзор платформы

RAG Chat Platform — микросервисная платформа для интеллектуального чата с документами. Пользователи регистрируются, создают ботов с индивидуальными настройками, загружают документы и получают ответы на вопросы по их содержимому.

### Ключевые возможности

- **Multi-user** - регистрация, JWT-аутентификация, управление ботами
- **Загрузка документов** - PDF, DOCX, TXT, CSV, JSON, HTML, Markdown, XLSX
- **Advanced RAG** - Agentic Router, tiered retrieval, hybrid search, cosine re-ranking, self-correction
- **Streaming** - потоковая генерация ответов через SSE
- **GPU ускорение** - llama.cpp server с CUDA через Docker
- **Публичный чат** - доступ к боту по URL без авторизации
- **Настраиваемые боты** - temperature, top_p, top_k, max tokens, system prompt через UI
- **Полная автономность** - все модели (LLM, embedding, reranker) встроены в Docker-образы, интернет не требуется после сборки

### Технологический стек

| Компонент | Технология |
|-----------|------------|
| Frontend | React 18 + Vite + Nginx |
| Backend Gateway | Go 1.24 + Fiber v2 |
| Document Parser | Go 1.24 |
| Vector DB Service | Go 1.24 + Qdrant gRPC |
| AI Service | Python 3.10 + FastAPI + httpx |
| LLM Server | llama.cpp (Docker, OpenAI API) |
| Vector DB | Qdrant |
| Embeddings | intfloat/multilingual-e5-base (768D) |
| Reranker | cross-encoder/ms-marco-MiniLM-L-6-v2 |
| Database | PostgreSQL 15 |
| LLM | Qwen3-4B (GGUF, формат настраивается) |

---

## Архитектура

### Диаграмма

```
+-----------------------------------------------------------+
|                   Frontend (React + Nginx)                  |
|                     http://localhost:3000                    |
+-----------------------------+-------------------------------+
                              | HTTP/SSE
                              v
+-----------------------------------------------------------+
|              Backend Gateway (Go + Fiber) :8080             |
|   Auth, CORS, Rate Limiting, RAG Orchestration              |
+------+------------------+------------------+---------------+
       |                  |                  |
       v                  v                  v
+-------------+   +-------------+   +-----------------+
| Document    |   | Vector DB   |   |   AI Service    |
| Parser (Go) |   | Service(Go) |   |   (Python)      |
| :8081       |   | :8082       |   |   :8000         |
+-------------+   +------+------+   +--------+--------+
                         |                   |
                         v                   v
                  +------------+    +-----------------+
                  |   Qdrant   |    | llama.cpp server|
                  | :6333/6334 |    | :8090           |
                  +------------+    +-----------------+
```

### Разделение ответственности LLM и AI Service

Платформа использует два отдельных контейнера для работы с нейросетями:

| Контейнер | Роль | Модели |
|-----------|------|--------|
| **llama-cpp** | LLM inference (генерация текста) | Qwen3-4B GGUF (или другая модель) |
| **ai-service** | Embeddings, RAG pipeline, reranking | multilingual-e5-base, ms-marco-MiniLM |

**llama-cpp** — это чистый LLM сервер на C++ с OpenAI-совместимым API (`/v1/chat/completions`). Он загружает GGUF модель и выполняет inference. Может работать на CPU или GPU.

**ai-service** — Python сервис, который:
- Создает embeddings для документов и запросов (sentence-transformers)
- Выполняет semantic chunking документов
- Реализует Advanced RAG pipeline (agentic router, hybrid search, reranking)
- Проксирует запросы генерации к llama-cpp через HTTP (httpx)

AI Service не загружает LLM модель — он обращается к llama-cpp server по HTTP.

### Потоки данных

#### Загрузка документа

```
Пользователь -> Frontend -> Backend -> Document Parser (парсинг файла)
                                    -> AI Service (semantic chunking + создание embeddings)
                                    -> Vector DB Service -> Qdrant (сохранение векторов)
```

#### RAG запрос (Advanced)

```
Пользователь -> Frontend -> Backend -> AI Service (embed запроса)
                                    -> Vector DB Service (векторный поиск)
                                    -> AI Service (advanced search: reranking, hybrid, self-correction)
                                    -> AI Service -> llama.cpp (генерация с контекстом, streaming)
                                    <- SSE streaming обратно пользователю
```

### Порядок запуска и зависимости

Все зависимости контролируются через `depends_on` с `condition: service_healthy`:

```
postgres (healthcheck) ─────────────────┐
qdrant ──> vector-db (healthcheck) ─────┤
llama-cpp (healthcheck) ──> ai-service (healthcheck) ──┤
                    document-parser (healthcheck) ──┤
                                                    v
                                              backend ──> frontend
```

Backend не запустится, пока все зависимые сервисы не пройдут healthcheck. AI Service ожидает готовности llama-cpp server перед стартом.

---

## Микросервисы

### 1. Frontend (React + Nginx)

**Порт:** 3000 | **Контейнер:** `chatbot-frontend`

Компоненты:
- `Auth.jsx` — обёртка аутентификации
- `Login.jsx` — форма входа/регистрации
- `Dashboard.jsx` — список ботов пользователя
- `BotForm.jsx` — создание/редактирование бота (загружает дефолты с сервера)
- `BotChat.jsx` — чат с ботом (streaming SSE)
- `ChatArea.jsx` — область сообщений чата
- `ModelSettings.jsx` — настройки параметров генерации (temperature, top_p, top_k, max tokens)
- `FileUpload.jsx` — загрузка документов (drag & drop)
- `DocumentSearch.jsx` — поиск по документам бота

### 2. Backend Gateway (Go + Fiber)

**Порт:** 8080 | **Контейнер:** `chatbot-backend`

- API Gateway для всех запросов
- JWT-аутентификация и middleware
- Оркестрация RAG pipeline (координирует Document Parser, Vector DB, AI Service)
- CRUD операции для ботов
- Rate limiting (100 req/min per IP)
- SSE streaming проксирование
- Конфигурация полностью из `.env` (CORS, upload limits, timeouts)

### 3. Document Parser (Go)

**Порт:** 8081 | **Контейнер:** `chatbot-document-parser`

Парсинг документов в текст: PDF, DOCX, TXT, JSON, CSV, XLSX, HTML, Markdown.

### 4. Vector DB Service (Go + Qdrant gRPC)

**Порт:** 8082 | **Контейнер:** `chatbot-vector-db`

- Управление коллекциями по bot_id (коллекция `bot_{uuid}`)
- Добавление, поиск, удаление документов
- Cosine similarity через gRPC

### 5. AI Service (Python + FastAPI)

**Порт:** 8000 | **Контейнер:** `chatbot-ai-service`

- Создание embeddings (multilingual-e5-base, 768D, с `passage:` / `query:` prefix)
- Semantic chunking (RecursiveCharacterTextSplitter)
- Advanced RAG: Agentic Router, hybrid search, cosine re-ranking, self-correction, window retrieval
- Проксирование генерации к llama.cpp server через httpx (OpenAI Chat API)
- Embedding и reranker модели встроены в Docker-образ при сборке

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

- LLM inference через OpenAI-совместимый API
- Поддержка GGUF моделей (Qwen3, LLaMA, Mistral и др.)
- CPU режим: `ghcr.io/ggml-org/llama.cpp:server`
- GPU режим: `ghcr.io/ggml-org/llama.cpp:server-cuda`
- Параллельные слоты для обработки нескольких запросов (`LLAMA_PARALLEL`)

### 7. Qdrant

**Порты:** 6333 (REST), 6334 (gRPC) | **Контейнер:** `chatbot-qdrant`

Векторная БД для хранения и семантического поиска документов. Данные хранятся в Docker volume `qdrant_storage`.

### 8. PostgreSQL

**Порт:** 5432 | **Контейнер:** `chatbot-postgres`

Хранение пользователей, ботов и метаданных документов. Данные хранятся в Docker volume `postgres_data`. Схема автоматически инициализируется из `schema.sql`.

---

## RAG Pipeline

### Advanced Agentic RAG

Платформа использует продвинутый RAG pipeline:

1. **Agentic Router** — классифицирует запросы: simple, global, enumeration, multi_hop, relation
2. **Tiered Retrieval** — vector_search -> hybrid_search -> full_document_read
3. **Window Retrieval** — подтягивание соседних чанков для полноты контекста
4. **Cosine Re-ranking** — переранжирование по embedding similarity
5. **Cross-Encoder Reranking** — точное переранжирование через ms-marco-MiniLM-L-6-v2
6. **BM25 Hybrid Search** — комбинация vector + keyword search (веса настраиваются)
7. **Self-Correction Loop** — повторные попытки с перефразировкой при низкой релевантности
8. **Enumeration Prompts** — специальные инструкции для запросов-списков
9. **Language Matching** — детекция языкового несоответствия query/documents
10. **Pipeline Trace** — структурированное логирование для отладки

### Этапы работы

#### Индексация

```
1. Загрузка файла -> Document Parser (извлечение текста)
2. Semantic chunking (RecursiveCharacterTextSplitter, настраивается через CHUNK_SIZE/CHUNK_OVERLAP)
3. Embeddings (multilingual-e5-base, 768D, с passage: prefix)
4. Сохранение в Qdrant (коллекция bot_{uuid})
```

#### Генерация ответа

```
1. Запрос -> Embedding (с query: prefix)
2. Vector search в Qdrant (до RAG_MAX_RESULTS кандидатов)
3. Advanced Search:
   a. Agentic Router определяет тип запроса
   b. Tiered retrieval (vector -> hybrid -> full_document_read)
   c. Window retrieval (соседние чанки)
   d. Cross-encoder reranking
   e. Self-correction при низкой релевантности (порог RELEVANCE_ESCALATION_THRESHOLD)
4. Формирование контекста (до RAG_MAX_CONTEXT_CHARS символов)
5. Streaming генерация через llama.cpp server (/v1/chat/completions)
```

---

## Автономная работа

Платформа спроектирована для полной автономной работы без доступа к интернету после сборки Docker-образов.

### Встроенные модели

При сборке Docker-образа `ai-service` автоматически скачиваются и встраиваются:

| Модель | Назначение | Размер |
|--------|-----------|--------|
| intfloat/multilingual-e5-base | Embedding (768D) | ~1.1 GB |
| cross-encoder/ms-marco-MiniLM-L-6-v2 | Reranker | ~90 MB |
| NLTK punkt_tab | Токенизация | ~5 MB |

Модели кэшируются в образе:
- Embedding: `/app/models/embedding`
- Reranker и другие: `/app/models/transformers` (через `SENTENCE_TRANSFORMERS_HOME`)

**LLM модель** (GGUF) монтируется из локальной директории `./models/` в контейнер llama-cpp через Docker volume.

### Почему нет Docker volume для AI моделей

Embedding и reranker модели встроены в Docker-образ при сборке. Docker volume `ai_models_cache` не используется, чтобы не создавать пустой overlay поверх встроенных моделей.

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
| GET | `/api/v1/bots/:id` | Информация о боте (публичная) |
| POST | `/api/v1/chat/public/:bot_id` | Публичный чат (streaming SSE) |

### Защищённые (Authorization: Bearer TOKEN)

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/api/v1/auth/me` | Текущий пользователь |
| POST | `/api/v1/bots` | Создать бота |
| GET | `/api/v1/bots` | Список ботов |
| PUT | `/api/v1/bots/:id` | Обновить бота |
| DELETE | `/api/v1/bots/:id` | Удалить бота |
| GET | `/api/v1/bots/:id/documents` | Документы бота |
| POST | `/api/v1/bots/:id/documents/upload` | Загрузить документ (multipart/form-data) |
| POST | `/api/v1/chat/rag` | RAG чат (streaming SSE) |

### Примеры

#### Регистрация

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "password": "password123", "name": "User"}'
```

#### Создание бота

```bash
curl -X POST http://localhost:8080/api/v1/bots \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "My Bot", "temperature": 0.7, "system_prompt": "You are helpful."}'
```

#### Загрузка документа

```bash
curl -X POST http://localhost:8080/api/v1/bots/$BOT_ID/documents/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@document.pdf"
```

#### Публичный чат (streaming)

```bash
curl -N -X POST http://localhost:8080/api/v1/chat/public/$BOT_ID \
  -H "Content-Type: application/json" \
  -d '{"query": "Что такое машинное обучение?"}'
```

#### Получение дефолтных параметров

```bash
curl http://localhost:8080/api/v1/config/defaults
# {"temperature": 0.75, "top_p": 0.92, "top_k": 40, ...}
```

---

## База данных

### Схема PostgreSQL

#### Таблица `users`

| Поле | Тип | Описание |
|------|-----|----------|
| id | SERIAL | Первичный ключ |
| email | VARCHAR(255) | Уникальный email |
| password_hash | VARCHAR(255) | Хэш пароля (bcrypt) |
| name | VARCHAR(255) | Имя пользователя |
| created_at | TIMESTAMPTZ | Дата создания |
| updated_at | TIMESTAMPTZ | Дата обновления (автотриггер) |

#### Таблица `bots`

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | Первичный ключ |
| owner_id | INTEGER | FK на users.id (CASCADE) |
| name | VARCHAR(255) | Имя бота |
| description | TEXT | Описание |
| config | JSONB | Конфигурация (JSON) |
| temperature | DECIMAL(3,2) | Температура генерации |
| top_p | DECIMAL(3,2) | Nucleus sampling |
| top_k | INTEGER | Top-K sampling |
| max_new_tokens | INTEGER | Макс длина ответа |
| do_sample | BOOLEAN | Sampling вкл/выкл |
| system_prompt | TEXT | Системный промпт |
| chunk_size | INTEGER | Размер чанка |
| chunk_overlap | INTEGER | Перекрытие чанков |
| is_active | BOOLEAN | Активен ли бот |
| created_at | TIMESTAMPTZ | Дата создания |
| updated_at | TIMESTAMPTZ | Дата обновления (автотриггер) |

#### Таблица `bot_documents`

| Поле | Тип | Описание |
|------|-----|----------|
| id | SERIAL | Первичный ключ |
| bot_id | UUID | FK на bots.id (CASCADE) |
| filename | VARCHAR(255) | Имя файла |
| file_type | VARCHAR(50) | Тип файла |
| file_size | BIGINT | Размер файла (байты) |
| chunks_count | INTEGER | Количество чанков |
| uploaded_at | TIMESTAMPTZ | Дата загрузки |

Метаданные документов хранятся в PostgreSQL, а векторные представления (embeddings) — в Qdrant.

---

## Разработка

### Структура проекта

```
chat-bot-platfrom/
├── .env                          # Единая конфигурация (все параметры)
├── docker-compose.yml            # Docker оркестрация (CPU)
├── docker-compose.gpu.yml        # GPU override для llama-cpp
├── models/                       # GGUF модели (git-ignored)
│   └── .gitkeep
├── frontend/
│   ├── Dockerfile
│   ├── nginx.conf                # Nginx конфигурация (reverse proxy)
│   ├── vite.config.js
│   └── src/
│       ├── api/client.js         # API клиент (fetch + auth)
│       ├── App.jsx               # Роутинг
│       └── components/
│           ├── Auth.jsx          # Обёртка аутентификации
│           ├── Login.jsx         # Форма входа/регистрации
│           ├── Dashboard.jsx     # Список ботов
│           ├── BotForm.jsx       # Создание/редактирование бота
│           ├── BotChat.jsx       # Чат с ботом
│           ├── ChatArea.jsx      # Область сообщений
│           ├── ModelSettings.jsx # Настройки генерации
│           ├── FileUpload.jsx    # Загрузка документов
│           └── DocumentSearch.jsx # Поиск по документам
├── services/
│   ├── backend/
│   │   ├── Dockerfile
│   │   ├── main.go               # Точка входа, роутинг, middleware
│   │   ├── config/config.go      # Конфигурация из env
│   │   ├── handlers/handlers.go  # HTTP handlers (RAG, upload, chat)
│   │   ├── clients/              # HTTP клиенты к микросервисам
│   │   ├── auth/                 # JWT, middleware, bcrypt
│   │   ├── database/             # PostgreSQL подключение
│   │   │   └── schema.sql        # DDL схема (auto-init)
│   │   └── models/types.go       # Структуры данных
│   ├── document-parser-service/
│   │   ├── Dockerfile
│   │   ├── main.go
│   │   └── parsers/              # PDF, DOCX, XLSX, CSV, JSON, HTML, MD, TXT
│   ├── vector-db-service/
│   │   ├── Dockerfile
│   │   ├── main.go
│   │   └── handlers/             # Qdrant gRPC операции
│   └── python-ai/
│       ├── Dockerfile            # Встраивает embedding + reranker модели
│       ├── requirements.txt
│       └── app/
│           ├── main.py           # FastAPI + lifespan
│           ├── config/settings.py # Конфигурация из env
│           ├── api/routes.py     # Endpoints
│           ├── models/schemas.py # Pydantic модели
│           └── services/
│               ├── model_service_gguf.py  # HTTP клиент для llama.cpp server
│               └── rag_service.py         # Advanced RAG pipeline
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

AI Service скачивает embedding и reranker модели при первой сборке Docker-образа (~1.2 GB). После сборки модели встроены в образ и загружаются из локального кэша.

```bash
docker compose logs ai-service
# "Embedding model loaded" = сервис готов
```

### Backend не подключается к сервисам

Backend ожидает healthcheck от всех зависимостей (ai-service, document-parser, vector-db, postgres). Если один из сервисов не healthy, backend не запустится.

```bash
docker compose ps
# Все зависимости должны быть healthy
curl http://localhost:8080/health
```

### Ошибка "connection refused" при загрузке документов

AI Service имеет `start_period: 120s` в healthcheck, так как загрузка embedding модели занимает время. Backend начнёт работу только когда ai-service станет healthy. Если ошибка всё равно возникает — увеличьте `start_period` и `retries` в docker-compose.yml для ai-service.

### Медленная генерация

1. GPU: `docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build`
2. Увеличить `N_THREADS` в `.env`
3. Уменьшить `N_CTX`
4. Использовать меньшую модель

### GPU не отображается в Task Manager (Windows)

При использовании WSL2 + Docker Desktop нагрузка на GPU отображается через процесс `Vmmem`, а не в разделе GPU диспетчера задач. Это нормальное поведение. Проверить реальное использование GPU:

```bash
docker compose logs llama-cpp | grep -i "gpu\|layer\|CUDA"
# Должно показать загрузку слоев на GPU, например:
# "offloaded 33/33 layers to GPU"
```

### Qdrant недоступен

```bash
curl http://localhost:6333/health
docker compose logs qdrant
```

---

Версия: 2.1
