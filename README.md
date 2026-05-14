# RAG Chat Platform

Микросервисная платформа для интеллектуального чата с документами на базе RAG (Retrieval-Augmented Generation). Пользователи регистрируются, создают ботов, загружают документы, ведут диалоги с историей и могут делиться ботами с коллегами. Администратор управляет всеми пользователями и ботами, а владелец может развернуть собственный llama.cpp контейнер под выбранную GGUF-модель.

---

## Quick Start

```bash
# 1. Скопируйте шаблон окружения и заполните секреты (JWT_SECRET, POSTGRES_PASSWORD, ADMIN_*)
cp .env.example .env

# 2. Положите GGUF модель в директорию models/
mkdir -p models
# Например: https://huggingface.co/Qwen/Qwen3-4B-GGUF
# Имя файла должно совпадать с GGUF_MODEL_FILE в .env

# 3. Запустите все сервисы
docker compose up -d --build

# 4. Откройте в браузере
# http://localhost:3000
# Админ-аккаунт создаётся автоматически (ADMIN_EMAIL / ADMIN_PASSWORD из .env)
```

### С GPU (NVIDIA)

```bash
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build
```

Требуется [nvidia-container-toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html). GPU override включает `LLAMA_USE_GPU=true` для динамических контейнеров (см. ниже про реестр моделей).

> Первая сборка занимает дольше — Docker-образ `ai-service` скачивает и встраивает embedding (~1.1 GB) и reranker (~90 MB) модели. После сборки платформа работает полностью автономно без интернета.

---

## Архитектура

```
Frontend (React) :3000
       |
Backend Gateway (Go) :8080
       |
  +----+--------+-----------+----------+
  |             |            |          |
Document    Vector DB    AI Service   Docker daemon
Parser      Service      (Python)     (динамические
:8081       :8082          :8000       llama.cpp
            |              |           контейнеры
         Qdrant      llama.cpp server  для finetuned
      :6333/:6334        :8090         моделей)
```

**8 сервисов + динамические контейнеры:**

| Сервис | Технология | Назначение |
|--------|------------|------------|
| Frontend | React 18 + Vite + Nginx | UI: чат, дашборд, аналитика, админка, реестр моделей |
| Backend Gateway | Go 1.24 + Fiber v2 + GORM | API Gateway, JWT auth, RBAC, оркестрация, container manager |
| Document Parser | Go | Парсинг PDF, DOCX, TXT, CSV, JSON, HTML, MD, XLSX |
| Vector DB Service | Go + Qdrant gRPC | Управление векторными коллекциями |
| AI Service | Python + FastAPI + httpx | Embeddings, RAG pipeline, reranking |
| llama.cpp server | C++ (Docker) | LLM inference (OpenAI-совместимый API) — дефолтный контейнер |
| Qdrant | Vector Database | Хранение и поиск векторов |
| PostgreSQL 15 | Реляционная БД | Пользователи, боты, диалоги, сообщения, фидбек, коллабораторы, реестр моделей |

Дополнительно: при деплое finetuned модели backend через Docker API поднимает отдельный llama.cpp контейнер на порту из пула `LLAMA_PORT_MIN..LLAMA_PORT_MAX` (по умолчанию 8100–8199).

### Разделение LLM и AI Service

- **llama-cpp** — LLM inference (генерация текста). Загружает GGUF модель, предоставляет OpenAI-совместимый API. Может работать на CPU или GPU.
- **ai-service** — Embeddings, RAG pipeline, reranking. Не загружает LLM, а обращается к llama-cpp по HTTP (httpx). Embedding и reranker модели встроены в Docker-образ.

---

## Возможности

- **Multi-user** — регистрация, JWT-аутентификация, ролевая модель (`user` / `admin`).
- **Управление ботами** — CRUD, поиск, пагинация, выбор LLM модели для каждого бота.
- **Совместный доступ** — приглашение коллабораторов на бота с ролями `viewer` (только чат) и `editor` (чат + документы).
- **Загрузка документов** — PDF, DOCX, TXT, CSV, JSON, HTML, MD, XLSX.
- **Advanced RAG** — Agentic Router, tiered retrieval, hybrid search (BM25 + vector), cosine re-ranking, cross-encoder reranking, self-correction loop.
- **Streaming чат** — SSE стрим с возможностью остановки генерации (AbortController).
- **История диалогов** — persistent conversations + messages, контекстное окно последних N сообщений передаётся модели.
- **Обратная связь** — thumbs-up / thumbs-down на каждое сообщение ассистента (в т.ч. в публичном чате).
- **Аналитика** — на бота: количество диалогов, сообщений, среднее время ответа, статистика фидбека, активность по дням.
- **Админ-панель** — управление пользователями (изменение роли, удаление), удаление ботов, общая статистика платформы.
- **Реестр моделей** — base модели (общие, из `./models/*.gguf`) и finetuned модели (только владельца). Деплой/стоп отдельного llama.cpp контейнера через Docker API.
- **Публичный чат** — доступ к боту по URL `/public/:bot_id` без авторизации, с возможностью оставлять фидбек.
- **Темы оформления** — переключатель тёмной/светлой темы.
- **Полная автономность** — все модели встроены в Docker-образы, интернет не нужен после сборки.

---

## Автономная работа

После `docker compose build` платформа не обращается к внешним сервисам:

| Модель | Хранение | Размер |
|--------|----------|--------|
| LLM (GGUF) | `./models/` (bind-mount в llama-cpp) | зависит от модели |
| Embedding (multilingual-e5-base) | Bind-mount `./models/embedding` | ~1.1 GB |
| Reranker (ms-marco-MiniLM) | Bind-mount `./models/hf-cache` | ~90 MB |
| NLTK (punkt_tab) | Встроен в образ ai-service | ~5 MB |

При первой сборке `ai-service` скачивает embedding и reranker и сохраняет их в bind-mount директории, чтобы переиспользовать между пересборками.

---

## Конфигурация

Все параметры задаются в `.env`. Используйте `.env.example` как стартовый шаблон — `.env` не коммитится в git. Ни один параметр не захардкожен в коде.

```bash
# LLM модель
GGUF_MODEL_FILE=Qwen3.5-4B-Q4_K_M.gguf
N_CTX=32768
N_THREADS=6
N_GPU_LAYERS=0    # -1 для GPU (все слои)
LLAMA_REASONING_BUDGET=0    # 0 = выключить thinking-блок (Qwen3, DeepSeek-R1)

# Динамические контейнеры моделей
LLAMA_PORT_MIN=8100
LLAMA_PORT_MAX=8199
LLAMA_MODELS_HOST_DIR=  # абсолютный путь к ./models на хосте (см. .env.example)

# Бутстрап админа (создаётся при первом старте, если ADMIN_PASSWORD задан)
ADMIN_EMAIL=admin@local
ADMIN_PASSWORD=change-me-admin

# Генерация (дефолты, настраиваются также через UI для каждого бота)
GEN_TEMPERATURE=0.7
GEN_TOP_P=0.8
GEN_MAX_NEW_TOKENS=8192

# Контекст чата
CHAT_CONTEXT_WINDOW=5    # сколько последних сообщений отправлять модели

# RAG
CHUNK_SIZE=1200
RAG_MAX_RESULTS=20
USE_HYBRID_SEARCH=true
USE_RERANKER=true
```

Полный список: [CONFIGURATION.md](CONFIGURATION.md)

---

## API

Полный справочник эндпоинтов и примеры запросов — в [PLATFORM_GUIDE.md](PLATFORM_GUIDE.md).

### Публичные эндпоинты

```bash
POST /api/v1/auth/register
POST /api/v1/auth/login
GET  /api/v1/config/defaults
GET  /api/v1/bots/:id                       # информация о боте
POST /api/v1/chat/public/:bot_id            # публичный SSE стрим
POST /api/v1/public/messages/:id/feedback   # фидбек из публичного чата
```

### Защищённые (Authorization: Bearer TOKEN)

```bash
# Аутентификация
GET    /api/v1/auth/me

# Боты
POST   /api/v1/bots
GET    /api/v1/bots
PUT    /api/v1/bots/:id
DELETE /api/v1/bots/:id

# Коллабораторы
GET    /api/v1/bots/:id/collaborators
POST   /api/v1/bots/:id/collaborators
PUT    /api/v1/bots/:id/collaborators/:user_id
DELETE /api/v1/bots/:id/collaborators/:user_id

# Документы
GET    /api/v1/bots/:id/documents
POST   /api/v1/bots/:id/documents/upload    # multipart/form-data
DELETE /api/v1/bots/:id/documents/:doc_id

# RAG чат
POST   /api/v1/chat/rag

# Диалоги
POST   /api/v1/conversations
GET    /api/v1/bots/:id/conversations
GET    /api/v1/conversations/:conv_id
DELETE /api/v1/conversations/:conv_id

# Фидбек и аналитика
POST   /api/v1/messages/:message_id/feedback
GET    /api/v1/bots/:id/feedback/stats
GET    /api/v1/bots/:id/analytics

# Реестр моделей
GET    /api/v1/models
GET    /api/v1/models/:id
POST   /api/v1/models/:id/deploy
POST   /api/v1/models/:id/stop
DELETE /api/v1/models/:id

# Админ (role='admin')
GET    /api/v1/admin/stats
GET    /api/v1/admin/users
PUT    /api/v1/admin/users/:id/role
DELETE /api/v1/admin/users/:id
GET    /api/v1/admin/bots
DELETE /api/v1/admin/bots/:id
```

---

## Структура проекта

```
chat-bot-platfrom/
├── .env                          # Единая конфигурация (git-ignored)
├── .env.example                  # Шаблон конфигурации
├── docker-compose.yml            # Основная оркестрация
├── docker-compose.gpu.yml        # GPU override для llama-cpp + динамических контейнеров
├── models/                       # GGUF модели + кеш embedding/reranker (git-ignored)
├── frontend/                     # React UI (Vite + Nginx)
├── scripts/
│   └── bench_parallel.py         # Бенчмарк параллельных SSE-запросов
├── services/
│   ├── backend/                  # Go API Gateway + Auth + Container Manager
│   ├── document-parser-service/  # Go парсер документов
│   ├── vector-db-service/        # Go Qdrant клиент
│   └── python-ai/                # Python RAG + embeddings
├── CONFIGURATION.md              # Все переменные окружения
├── DEPLOYMENT.md                 # Развёртывание, GPU, troubleshooting
└── PLATFORM_GUIDE.md             # Полное руководство по платформе
```

---

## Документация

- [PLATFORM_GUIDE.md](PLATFORM_GUIDE.md) — архитектура, микросервисы, RAG pipeline, API, схема БД.
- [CONFIGURATION.md](CONFIGURATION.md) — все переменные окружения.
- [DEPLOYMENT.md](DEPLOYMENT.md) — развёртывание, GPU, бутстрап админа, troubleshooting.

---

## Управление

```bash
docker compose logs -f [service]     # Логи
docker compose restart [service]     # Перезапуск
docker compose up -d --build         # Пересборка
docker compose down                  # Остановка
docker compose down -v               # Остановка + удаление данных (Postgres, Qdrant)
```

Бенчмарк параллельных запросов (нужен запущенный публичный бот):

```bash
python scripts/bench_parallel.py <bot_id> 4
```

---

## Технологический стек

| Компонент | Технология |
|-----------|------------|
| Frontend | React 18, Vite, Nginx, Lucide Icons |
| Backend | Go 1.24, Fiber v2, GORM, Docker SDK |
| AI Service | Python 3.10, FastAPI, httpx, sentence-transformers |
| LLM Server | llama.cpp (Docker, OpenAI API) |
| LLM | Qwen3.5-4B (GGUF, настраивается) |
| Vector DB | Qdrant |
| Embeddings | multilingual-e5-base (768D) |
| Reranker | cross-encoder/ms-marco-MiniLM-L-6-v2 |
| Database | PostgreSQL 15 |
| Auth | JWT + bcrypt + RBAC |

---

Версия: 2.2
