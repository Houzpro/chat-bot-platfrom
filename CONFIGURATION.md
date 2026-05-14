# Конфигурация RAG Chat Platform

Все настройки централизованы в `.env`. Это единственный источник правды для всех микросервисов — ни один параметр не захардкожен в коде. Используйте `.env.example` как шаблон (`cp .env.example .env`); `.env` git-ignored.

---

## Структура конфигурации

### 1. Порты и URL микросервисов

```bash
BACKEND_PORT=8080
DOCUMENT_PARSER_PORT=8081
VECTOR_DB_PORT=8082
AI_SERVICE_PORT=8000
FRONTEND_PORT=3000

DOC_PARSER_URL=http://document-parser:8081
VECTOR_URL=http://vector-db:8082
AI_URL=http://ai-service:8000
```

В Docker Compose используются имена контейнеров. Для локальной разработки замените на `http://localhost:XXXX`.

---

### 2. llama.cpp server (default)

```bash
LLAMA_CPP_PORT=8090
GGUF_MODEL_FILE=Qwen3.5-4B-Q4_K_M.gguf    # Имя файла в ./models/
GGUF_MODEL_PATH=./models/Qwen3.5-4B-Q4_K_M.gguf
N_CTX=32768          # Размер контекста (токены)
N_THREADS=6          # CPU потоки
N_GPU_LAYERS=0       # 0 = CPU only, -1 = все слои на GPU
LLAMA_PARALLEL=2     # Количество параллельных слотов

# Управление режимом "thinking" (для Qwen3, DeepSeek-R1, GLM):
#   -1 = по умолчанию (модель решает сама)
#    0 = выключить thinking-блок
#   >0 = максимум токенов на reasoning
LLAMA_REASONING_BUDGET=0
LLAMA_CHAT_TEMPLATE_KWARGS={"enable_thinking": false}
```

Модель хостится в Docker контейнере `ghcr.io/ggml-org/llama.cpp:server` и предоставляет OpenAI-совместимый API (`/v1/chat/completions`). Для GPU используйте `docker-compose.gpu.yml` (override автоматически устанавливает `--n-gpu-layers -1`).

> `N_GPU_LAYERS` в `.env` работает только в CPU-режиме. В GPU-режиме (`docker-compose.gpu.yml`) всегда используется `-1` (все слои на GPU).

---

### 3. Динамические контейнеры моделей

Backend через Docker API поднимает отдельные llama.cpp контейнеры под finetuned модели.

```bash
# Абсолютный путь на ХОСТЕ к директории ./models. Docker daemon не принимает
# относительные пути для bind-mount, поэтому переменная обязательна.
# Linux/macOS: /home/user/chat-bot-platfrom/models
# Windows:     C:/files/Developing/diplom/chat-bot-platfrom/models
LLAMA_MODELS_HOST_DIR=

# Пул портов для динамических контейнеров
LLAMA_PORT_MIN=8100
LLAMA_PORT_MAX=8199

# Docker network — динамические контейнеры подключаются в ту же сеть,
# что и остальные сервисы (по умолчанию compose проект "chat-bot-platfrom").
LLAMA_DOCKER_NETWORK=chat-bot-platfrom_default

# Образ для динамических контейнеров. Пустая строка = выбрать автоматически:
#   LLAMA_USE_GPU=true  → ghcr.io/ggml-org/llama.cpp:server-cuda
#   иначе               → ghcr.io/ggml-org/llama.cpp:server
LLAMA_IMAGE=
LLAMA_USE_GPU=false

# KV-кеш квантизация для динамических контейнеров (пустая = f16, как у CPU)
LLAMA_CACHE_TYPE_K=
LLAMA_CACHE_TYPE_V=
```

`LLAMA_N_CTX`, `LLAMA_N_THREADS`, `LLAMA_N_GPU_LAYERS`, `LLAMA_PARALLEL` для динамических контейнеров берутся из соответствующих переменных `N_CTX`, `N_THREADS`, `N_GPU_LAYERS`, `LLAMA_PARALLEL` (см. раздел 2).

---

### 4. База данных и админ

```bash
POSTGRES_DB=chatbot
POSTGRES_USER=chatbot
POSTGRES_PASSWORD=change-me        # SECRET
DATABASE_URL=postgresql://chatbot:change-me@postgres:5432/chatbot?sslmode=disable

JWT_SECRET=change-me-must-be-at-least-32-characters-long   # SECRET, минимум 32 символа

# Бутстрап админа. При первом старте backend:
#  - если пользователя с ADMIN_EMAIL нет и ADMIN_PASSWORD задан → создаёт нового с role='admin'
#  - если есть → повышает до admin
#  - если ADMIN_EMAIL пустой → бутстрап пропускается
ADMIN_EMAIL=admin@local
ADMIN_PASSWORD=change-me-admin
```

> `JWT_SECRET` обязателен. При отсутствии backend завершится с ошибкой.
> Поменяйте `POSTGRES_PASSWORD`, `JWT_SECRET`, `ADMIN_PASSWORD` перед любым серьёзным деплоем — дефолты помечены как `change-me`.

---

### 5. Qdrant

```bash
QDRANT_HOST=qdrant
QDRANT_PORT_REST=6333
QDRANT_PORT_GRPC=6334
QDRANT_COLLECTION_SIZE=768         # Размерность векторов (768 для multilingual-e5-base)
```

---

### 6. Параметры генерации

```bash
GEN_MAX_NEW_TOKENS=8192            # Максимальная длина ответа
GEN_TEMPERATURE=0.7                # Креативность (0.0-2.0)
GEN_TOP_P=0.8                      # Nucleus sampling
GEN_TOP_K=20                       # Top-K sampling
GEN_DO_SAMPLE=true                 # Sampling вкл/выкл
GEN_PRESENCE_PENALTY=1.5
GEN_FREQUENCY_PENALTY=0
GEN_MIN_P=0
GEN_STOP_SEQUENCES=<|im_end|>,<|endoftext|>
```

Серверные дефолты. Пользователи переопределяют через UI при создании/настройке бота. Frontend загружает дефолты через `GET /api/v1/config/defaults`.

---

### 7. System prompts

```bash
GEN_SYSTEM_BASE_PROMPT=DO NOT use markdown formatting, asterisks, or special symbols. Use plain text only with clear paragraph breaks and natural structure.
GEN_USER_PROMPT=You are a highly knowledgeable and precise assistant. Provide comprehensive, detailed, and well-structured answers based on the given context. Include all relevant information and explain concepts thoroughly.
```

- `GEN_SYSTEM_BASE_PROMPT` — базовая инструкция, добавляется к каждому запросу (не переопределяется пользователем).
- `GEN_USER_PROMPT` — пользовательская инструкция по умолчанию (переопределяется полем `system_prompt` бота).

---

### 8. Embeddings и Reranking

```bash
EMBEDDING_MODEL_NAME=intfloat/multilingual-e5-base
EMBEDDING_CACHE_FOLDER=./models/embedding
USE_RERANKER=true
RERANKER_MODEL_NAME=cross-encoder/ms-marco-MiniLM-L-6-v2

# Hugging Face токен. Используется при сборке (download) и runtime.
# Получить: https://huggingface.co/settings/tokens (scope: Read)
# SECRET — нужен только для gated/private моделей.
HF_TOKEN=
```

Embedding и reranker модели подгружаются из bind-mount директорий:
- Embedding: `./models/embedding/` (через `EMBEDDING_CACHE_FOLDER`).
- Reranker: `./models/hf-cache/` (HF дефолтный кеш).

При первой сборке `ai-service` скачивает модели в эти директории. Последующие запуски используют локальный кеш.

---

### 9. RAG конфигурация

```bash
RAG_MAX_DOC_CHARS=50000            # Макс символов из документов
RAG_MAX_CONTEXT_CHARS=30000        # Макс размер контекста для LLM
RAG_SCORE_THRESHOLD=0.0            # Порог релевантности
RAG_MAX_RESULTS=20                 # Максимум результатов поиска
RAG_CONTEXT_TIMEOUT_SEC=45         # Таймаут RAG операции (секунды)

# Hybrid Search (Vector + BM25)
USE_HYBRID_SEARCH=true
BM25_WEIGHT=0.40
VECTOR_WEIGHT=0.60

# Chunking
CHUNK_SIZE=1200                    # Размер чанка (символы)
CHUNK_OVERLAP=200                  # Перекрытие

# Relevance thresholds
RELEVANCE_ESCALATION_THRESHOLD=3.0    # Порог эскалации retrieval tier
EMBEDDING_SIMILARITY_AUTOPASS=0.75    # Порог автопрохождения по embedding similarity

# Контекстное окно диалога — сколько последних сообщений передавать модели
CHAT_CONTEXT_WINDOW=5
```

`CHAT_CONTEXT_WINDOW` может быть переопределён индивидуально на боте (поле `bots.context_window`, 0 = использовать глобальный дефолт).

---

### 10. Обработка документов

```bash
MAX_FILE_SIZE=20971520             # 20 MB
BODY_LIMIT=26214400                # ~25 MB (должен быть >= MAX_FILE_SIZE)
ALLOWED_EXTENSIONS=.pdf,.txt,.docx,.doc,.csv,.xlsx,.json,.md,.html
```

Лимиты валидируются на backend через конфигурацию. `BODY_LIMIT` должен быть чуть больше `MAX_FILE_SIZE` из-за multipart overhead.

---

### 11. HTTP и CORS

```bash
HTTP_TIMEOUT_SEC=300
CORS_ALLOW_ORIGINS=*
CORS_ALLOW_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOW_HEADERS=Origin,Content-Type,Accept,Authorization
```

---

### 12. Логирование

```bash
LOG_LEVEL=info
APP_NAME=RAG Chat Platform
APP_VERSION=2.0.0
ENVIRONMENT=production
```

---

## Примеры конфигураций

### Быстрый режим (CPU, короткие ответы)

```bash
N_CTX=4096
GEN_MAX_NEW_TOKENS=512
GEN_TEMPERATURE=0.3
RAG_MAX_RESULTS=10
CHUNK_SIZE=800
CHAT_CONTEXT_WINDOW=3
```

### Качественный режим (GPU, длинные ответы)

```bash
# Запуск: docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build
N_CTX=32768
GEN_MAX_NEW_TOKENS=8192
GEN_TEMPERATURE=0.7
RAG_MAX_RESULTS=20
USE_RERANKER=true
USE_HYBRID_SEARCH=true
LLAMA_USE_GPU=true
LLAMA_CACHE_TYPE_K=q8_0
LLAMA_CACHE_TYPE_V=q8_0
```

### Детерминированный режим (для тестов)

```bash
GEN_TEMPERATURE=0.0
GEN_DO_SAMPLE=false
GEN_TOP_P=1.0
GEN_TOP_K=1
CHAT_CONTEXT_WINDOW=0    # отключить контекст истории
```

---

## Валидация

Каждый микросервис валидирует конфигурацию при старте. Если обязательная переменная не задана — сервис не запустится.

**Backend (Go):**
```bash
docker compose logs backend
# "Failed to load configuration: config validation failed: PORT cannot be empty"
# "DATABASE_URL environment variable is required"
# "JWT_SECRET environment variable is required"
```

**AI Service (Python):**
```bash
docker compose logs ai-service
# "Configuration validation failed:
#   - EMBEDDING_MODEL_NAME is required
#   - EMBEDDING_CACHE_FOLDER is required"
```

При старте backend в логах также видны:
- `✓ Database connected` / `✓ Database migrations completed`
- `✓ Docker daemon reachable` (если сокет смонтирован)
- `✓ Created admin account ...` или `✓ Promoted ... to admin` (если бутстрап сработал)

---

## Список всех переменных

| Переменная | Тип | Значение по умолчанию | Сервис |
|------------|-----|----------------------|--------|
| `BACKEND_PORT` | int | 8080 | backend |
| `DOCUMENT_PARSER_PORT` | int | 8081 | document-parser |
| `VECTOR_DB_PORT` | int | 8082 | vector-db |
| `AI_SERVICE_PORT` | int | 8000 | ai-service |
| `FRONTEND_PORT` | int | 3000 | frontend |
| `LLAMA_CPP_PORT` | int | 8090 | llama-cpp |
| `DOC_PARSER_URL` | string | http://document-parser:8081 | backend |
| `VECTOR_URL` | string | http://vector-db:8082 | backend |
| `AI_URL` | string | http://ai-service:8000 | backend |
| `GGUF_MODEL_FILE` | string | Qwen3.5-4B-Q4_K_M.gguf | llama-cpp, backend |
| `GGUF_MODEL_PATH` | string | ./models/... | ai-service |
| `N_CTX` | int | 32768 | llama-cpp, backend (dynamic) |
| `N_THREADS` | int | 6 | llama-cpp, backend (dynamic) |
| `N_GPU_LAYERS` | int | 0 | llama-cpp, backend (dynamic) |
| `LLAMA_PARALLEL` | int | 2 | llama-cpp, backend (dynamic) |
| `LLAMA_REASONING_BUDGET` | int | 0 | llama-cpp |
| `LLAMA_CHAT_TEMPLATE_KWARGS` | string (JSON) | `{}` | llama-cpp |
| `LLAMA_MODELS_HOST_DIR` | string | (требуется) | backend |
| `LLAMA_PORT_MIN` | int | 8100 | backend |
| `LLAMA_PORT_MAX` | int | 8199 | backend |
| `LLAMA_DOCKER_NETWORK` | string | chat-bot-platfrom_default | backend |
| `LLAMA_IMAGE` | string | (auto) | backend |
| `LLAMA_USE_GPU` | bool | false | backend |
| `LLAMA_CACHE_TYPE_K` | string | (empty) | backend |
| `LLAMA_CACHE_TYPE_V` | string | (empty) | backend |
| `POSTGRES_DB` | string | chatbot | postgres |
| `POSTGRES_USER` | string | chatbot | postgres |
| `POSTGRES_PASSWORD` | string | change-me (SECRET) | postgres |
| `DATABASE_URL` | string | postgresql://... | backend |
| `JWT_SECRET` | string | (требуется, SECRET) | backend |
| `ADMIN_EMAIL` | string | admin@local | backend |
| `ADMIN_PASSWORD` | string | change-me-admin (SECRET) | backend |
| `QDRANT_HOST` | string | qdrant | vector-db |
| `QDRANT_PORT_REST` | int | 6333 | qdrant |
| `QDRANT_PORT_GRPC` | int | 6334 | vector-db |
| `QDRANT_COLLECTION_SIZE` | int | 768 | vector-db |
| `GEN_MAX_NEW_TOKENS` | int | 8192 | ai-service, backend |
| `GEN_TEMPERATURE` | float | 0.7 | ai-service, backend |
| `GEN_TOP_P` | float | 0.8 | ai-service, backend |
| `GEN_TOP_K` | int | 20 | ai-service, backend |
| `GEN_DO_SAMPLE` | bool | true | ai-service, backend |
| `GEN_PRESENCE_PENALTY` | float | 1.5 | ai-service, backend |
| `GEN_FREQUENCY_PENALTY` | float | 0 | ai-service, backend |
| `GEN_MIN_P` | float | 0 | ai-service, backend |
| `GEN_STOP_SEQUENCES` | string | `<\|im_end\|>,<\|endoftext\|>` | ai-service |
| `GEN_SYSTEM_BASE_PROMPT` | string | (см. .env.example) | ai-service, backend |
| `GEN_USER_PROMPT` | string | (см. .env.example) | ai-service, backend |
| `EMBEDDING_MODEL_NAME` | string | intfloat/multilingual-e5-base | ai-service |
| `EMBEDDING_CACHE_FOLDER` | string | ./models/embedding | ai-service |
| `USE_RERANKER` | bool | true | ai-service |
| `RERANKER_MODEL_NAME` | string | cross-encoder/ms-marco-MiniLM-L-6-v2 | ai-service |
| `HF_TOKEN` | string | (empty, SECRET) | ai-service |
| `USE_HYBRID_SEARCH` | bool | true | ai-service |
| `BM25_WEIGHT` | float | 0.40 | ai-service |
| `VECTOR_WEIGHT` | float | 0.60 | ai-service |
| `RAG_MAX_DOC_CHARS` | int | 50000 | backend |
| `RAG_MAX_CONTEXT_CHARS` | int | 30000 | backend |
| `RAG_SCORE_THRESHOLD` | float | 0.0 | backend, vector-db |
| `RAG_MAX_RESULTS` | int | 20 | backend |
| `RAG_CONTEXT_TIMEOUT_SEC` | int | 45 | backend |
| `RELEVANCE_ESCALATION_THRESHOLD` | float | 3.0 | ai-service |
| `EMBEDDING_SIMILARITY_AUTOPASS` | float | 0.75 | ai-service |
| `CHAT_CONTEXT_WINDOW` | int | 5 | backend |
| `CHUNK_SIZE` | int | 1200 | backend |
| `CHUNK_OVERLAP` | int | 200 | backend |
| `MAX_FILE_SIZE` | int | 20971520 | backend, document-parser |
| `BODY_LIMIT` | int | 26214400 | backend, document-parser, frontend |
| `ALLOWED_EXTENSIONS` | string | .pdf,.txt,.docx,.doc,.csv,.xlsx,.json,.md,.html | backend |
| `HTTP_TIMEOUT_SEC` | int | 300 | backend, frontend |
| `CORS_ALLOW_ORIGINS` | string | * | backend, document-parser, vector-db |
| `CORS_ALLOW_METHODS` | string | GET,POST,PUT,DELETE,OPTIONS | backend, document-parser, vector-db |
| `CORS_ALLOW_HEADERS` | string | Origin,Content-Type,Accept,Authorization | backend, document-parser, vector-db |
| `LOG_LEVEL` | string | info | все сервисы |
| `APP_NAME` | string | RAG Chat Platform | metadata |
| `APP_VERSION` | string | 2.0.0 | metadata |
| `ENVIRONMENT` | string | production | metadata |
