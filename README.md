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

**7 сервисов:**

| Сервис | Технология | Назначение |
|--------|------------|------------|
| Frontend | React 18 + Vite + Nginx | UI: чат, управление ботами |
| Backend Gateway | Go + Fiber | API Gateway, оркестрация |
| Document Parser | Go | Парсинг PDF, DOCX, TXT, CSV, JSON, HTML, MD |
| Vector DB Service | Go + Qdrant gRPC | Управление векторными коллекциями |
| AI Service | Python + FastAPI | Embeddings, RAG pipeline, reranking |
| llama.cpp server | C++ (Docker) | LLM inference (OpenAI-совместимый API) |
| Qdrant | Vector Database | Хранение и поиск векторов |

---

## Возможности

- **Multi-user** - регистрация, JWT-аутентификация, управление ботами
- **Загрузка документов** - PDF, DOCX, TXT, CSV, JSON, HTML, MD, XLSX
- **Advanced RAG** - Agentic Router, hybrid search, cosine re-ranking, self-correction
- **Streaming** - потоковая генерация ответов через SSE
- **Настраиваемые боты** - temperature, top_p, top_k, system prompt через UI
- **GPU ускорение** - llama.cpp server с CUDA через Docker
- **Публичный чат** - доступ к боту по URL без авторизации

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

# Публичный чат с ботом (streaming SSE)
POST /api/v1/chat/public/:bot_id
{"query": "Что такое JSON?"}
```

### Защищённые эндпоинты (Authorization: Bearer TOKEN)

```bash
# CRUD боты
POST   /api/v1/bots
GET    /api/v1/bots
PUT    /api/v1/bots/:id
DELETE /api/v1/bots/:id

# Загрузка документов
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
├── docker-compose.gpu.yml        # GPU override для llama.cpp
├── models/                       # GGUF модели (git-ignored)
├── frontend/                     # React UI
├── services/
│   ├── backend/                  # Go API Gateway + Auth
│   ├── document-parser-service/  # Go парсер документов
│   ├── vector-db-service/        # Go Qdrant клиент
│   └── python-ai/                # Python RAG + embeddings
├── CONFIGURATION.md
├── DEPLOYMENT.md
└── PLATFORM_GUIDE.md
```

---

## Документация

- [PLATFORM_GUIDE.md](PLATFORM_GUIDE.md) - полное руководство по платформе
- [CONFIGURATION.md](CONFIGURATION.md) - все переменные окружения
- [DEPLOYMENT.md](DEPLOYMENT.md) - развёртывание и управление

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
| AI Service | Python 3.10, FastAPI |
| LLM Server | llama.cpp (Docker) |
| LLM | Qwen3-4B (GGUF) |
| Vector DB | Qdrant |
| Embeddings | multilingual-e5-base |
| Reranker | cross-encoder/ms-marco-MiniLM-L-6-v2 |
| Database | PostgreSQL 15 |
| Auth | JWT |

---

Версия: 2.0
