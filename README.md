# RAG Chat Platform

Микросервисная платформа для интеллектуального чата с документами на базе RAG (Retrieval-Augmented Generation). Пользователи создают ботов, загружают документы и получают ответы на вопросы по содержимому документов.

---

## Quick Start

```bash
# 1. Скачайте GGUF модель в директорию models/
mkdir -p models
# Например: https://huggingface.co/Qwen/Qwen3-4B-GGUF
# Поместите файл qwen3-4b-q4_k_m.gguf в models/

# 2. Запустите все сервисы
docker compose up -d --build

# 3. Откройте в браузере
# http://localhost:3000
```

### С GPU (NVIDIA)

```bash
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build
```

Требуется [nvidia-container-toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html).

> Первая сборка занимает дольше — Docker-образ `ai-service` скачивает и встраивает embedding (~1.1 GB) и reranker (~90 MB) модели. После сборки платформа работает полностью автономно без интернета.

---

## Архитектура

```
Frontend (React) :3000
       |
Backend Gateway (Go) :8080
       |
  +----+--------+-----------+
  |             |            |
Document    Vector DB    AI Service
Parser      Service      (Python)
:8081       :8082          :8000
            |              |
         Qdrant      llama.cpp server
      :6333/:6334        :8090
```

**8 сервисов:**

| Сервис | Технология | Назначение |
|--------|------------|------------|
| Frontend | React 18 + Vite + Nginx | UI: чат, управление ботами |
| Backend Gateway | Go + Fiber | API Gateway, JWT auth, оркестрация |
| Document Parser | Go | Парсинг PDF, DOCX, TXT, CSV, JSON, HTML, MD, XLSX |
| Vector DB Service | Go + Qdrant gRPC | Управление векторными коллекциями |
| AI Service | Python + FastAPI + httpx | Embeddings, RAG pipeline, reranking |
| llama.cpp server | C++ (Docker) | LLM inference (OpenAI-совместимый API) |
| Qdrant | Vector Database | Хранение и поиск векторов |
| PostgreSQL | Реляционная БД | Пользователи, боты, метаданные документов |

### Разделение LLM и AI Service

- **llama-cpp** — LLM inference (генерация текста). Загружает GGUF модель, предоставляет OpenAI API. Может работать на CPU или GPU.
- **ai-service** — Embeddings, RAG pipeline, reranking. Не загружает LLM, а обращается к llama-cpp по HTTP. Embedding и reranker модели встроены в Docker-образ.

---

## Возможности

- **Multi-user** - регистрация, JWT-аутентификация, управление ботами
- **Загрузка документов** - PDF, DOCX, TXT, CSV, JSON, HTML, MD, XLSX
- **Advanced RAG** - Agentic Router, hybrid search (BM25 + vector), cosine re-ranking, cross-encoder reranking, self-correction loop
- **Streaming** - потоковая генерация ответов через SSE
- **Настраиваемые боты** - temperature, top_p, top_k, max tokens, system prompt через UI
- **GPU ускорение** - llama.cpp server с CUDA через Docker
- **Публичный чат** - доступ к боту по URL без авторизации
- **Полная автономность** - все модели встроены в Docker-образы, интернет не нужен после сборки

---

## Автономная работа

После `docker compose build` платформа не обращается к внешним сервисам:

| Модель | Хранение | Размер |
|--------|----------|--------|
| LLM (GGUF) | `./models/` (volume mount) | зависит от модели |
| Embedding (multilingual-e5-base) | Встроена в образ ai-service | ~1.1 GB |
| Reranker (ms-marco-MiniLM) | Встроен в образ ai-service | ~90 MB |
| NLTK (punkt_tab) | Встроен в образ ai-service | ~5 MB |

---

## Конфигурация

Все параметры задаются в `.env` файле. Ни один параметр не захардкожен в коде.

```bash
# LLM модель
GGUF_MODEL_FILE=qwen3-4b-q4_k_m.gguf
N_CTX=32768
N_THREADS=6
N_GPU_LAYERS=0    # -1 для GPU (все слои)

# Генерация (настраивается также через UI для каждого бота)
GEN_TEMPERATURE=0.75
GEN_TOP_P=0.92
GEN_MAX_NEW_TOKENS=8192

# RAG
CHUNK_SIZE=1200
RAG_MAX_RESULTS=60
USE_HYBRID_SEARCH=true
USE_RERANKER=true
```

Полный список: [CONFIGURATION.md](CONFIGURATION.md)

---

## API

### Публичные эндпоинты

```bash
# Регистрация
POST /api/v1/auth/register
{"email": "user@example.com", "password": "pass", "name": "User"}

# Логин
POST /api/v1/auth/login
{"email": "user@example.com", "password": "pass"}

# Дефолтные параметры генерации
GET /api/v1/config/defaults

# Информация о боте
GET /api/v1/bots/:id

# Публичный чат с ботом (streaming SSE)
POST /api/v1/chat/public/:bot_id
{"query": "Что такое JSON?"}
```

### Защищённые эндпоинты (Authorization: Bearer TOKEN)

```bash
# Текущий пользователь
GET /api/v1/auth/me

# CRUD боты
POST   /api/v1/bots
GET    /api/v1/bots
PUT    /api/v1/bots/:id
DELETE /api/v1/bots/:id

# Документы
GET  /api/v1/bots/:id/documents
POST /api/v1/bots/:id/documents/upload  (multipart/form-data)

# RAG чат
POST /api/v1/chat/rag
{"client_id": "bot-uuid", "query": "вопрос"}
```

---

## Структура проекта

```
chat-bot-platfrom/
├── .env                          # Единая конфигурация
├── docker-compose.yml            # Основная оркестрация
├── docker-compose.gpu.yml        # GPU override для llama-cpp
├── models/                       # GGUF модели (git-ignored)
├── frontend/                     # React UI
├── services/
│   ├── backend/                  # Go API Gateway + Auth
│   ├── document-parser-service/  # Go парсер документов
│   ├── vector-db-service/        # Go Qdrant клиент
│   └── python-ai/                # Python RAG + embeddings (модели встроены в образ)
├── CONFIGURATION.md              # Все переменные окружения
├── DEPLOYMENT.md                 # Развёртывание и управление
└── PLATFORM_GUIDE.md             # Полное руководство по платформе
```

---

## Документация

- [PLATFORM_GUIDE.md](PLATFORM_GUIDE.md) - полное руководство по платформе и архитектуре
- [CONFIGURATION.md](CONFIGURATION.md) - все переменные окружения
- [DEPLOYMENT.md](DEPLOYMENT.md) - развёртывание, GPU, troubleshooting

---

## Управление

```bash
docker compose logs -f [service]     # Логи
docker compose restart [service]     # Перезапуск
docker compose up -d --build         # Пересборка
docker compose down                  # Остановка
docker compose down -v               # Остановка + удаление данных
```

---

## Технологический стек

| Компонент | Технология |
|-----------|------------|
| Frontend | React 18, Vite, Nginx |
| Backend | Go 1.24, Fiber v2 |
| AI Service | Python 3.10, FastAPI, httpx |
| LLM Server | llama.cpp (Docker) |
| LLM | Qwen3-4B (GGUF, настраивается) |
| Vector DB | Qdrant |
| Embeddings | multilingual-e5-base (768D, встроена в образ) |
| Reranker | cross-encoder/ms-marco-MiniLM-L-6-v2 (встроен в образ) |
| Database | PostgreSQL 15 |
| Auth | JWT + bcrypt |

---

Версия: 2.1
