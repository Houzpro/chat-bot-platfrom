# Конфигурация RAG Chat Platform

Все настройки системы централизованы в файле `.env`. Это единственный источник правды для всех микросервисов. Ни один параметр не захардкожен в коде — все читается из переменных окружения.

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

### 2. llama.cpp server

```bash
LLAMA_CPP_PORT=8090
GGUF_MODEL_FILE=qwen3-4b-q4_k_m.gguf    # Имя файла модели в ./models/
GGUF_MODEL_PATH=./models/qwen3-4b-q4_k_m.gguf
N_CTX=32768          # Размер контекста (токены)
N_THREADS=6          # CPU потоки
N_GPU_LAYERS=0       # 0 = CPU only, -1 = все слои на GPU
LLAMA_PARALLEL=2     # Количество параллельных слотов
```

Модель хостится в отдельном Docker контейнере `ghcr.io/ggml-org/llama.cpp:server` и предоставляет OpenAI-совместимый API. Для GPU используйте `docker-compose.gpu.yml`.

---

### 3. База данных

```bash
POSTGRES_DB=chatbot
POSTGRES_USER=chatbot
POSTGRES_PASSWORD=chatbot_password
DATABASE_URL=postgresql://chatbot:chatbot_password@postgres:5432/chatbot?sslmode=disable
JWT_SECRET=your-secret-key-change-in-production-must-be-at-least-32-chars
```

**Важно:** `JWT_SECRET` обязателен. При отсутствии сервис не запустится.

---

### 4. Qdrant

```bash
QDRANT_HOST=qdrant
QDRANT_PORT_REST=6333
QDRANT_PORT_GRPC=6334
QDRANT_COLLECTION_SIZE=768   # Размерность векторов (768 для multilingual-e5-base)
```

---

### 5. Параметры генерации

```bash
GEN_MAX_NEW_TOKENS=8192      # Максимальная длина ответа
GEN_TEMPERATURE=0.75         # Креативность (0.0-2.0)
GEN_TOP_P=0.92               # Nucleus sampling
GEN_TOP_K=40                 # Top-K sampling
GEN_DO_SAMPLE=true           # Sampling вкл/выкл
GEN_STOP_SEQUENCES=<|im_end|>,<|endoftext|>
```

Эти значения — дефолты. Пользователи могут переопределять их через UI при создании бота.

---

### 6. System prompts

```bash
GEN_SYSTEM_BASE_PROMPT=DO NOT use markdown formatting. Use plain text only. /no_think
GEN_USER_PROMPT=You are a highly knowledgeable and precise assistant...
```

- `GEN_SYSTEM_BASE_PROMPT` — базовая инструкция (добавляется к каждому запросу)
- `GEN_USER_PROMPT` — пользовательская инструкция по умолчанию

---

### 7. Embeddings и Reranking

```bash
EMBEDDING_MODEL_NAME=intfloat/multilingual-e5-base
EMBEDDING_CACHE_FOLDER=./models/embedding
USE_RERANKER=true
RERANKER_MODEL_NAME=cross-encoder/ms-marco-MiniLM-L-6-v2
USE_QUERY_EXPANSION=true
QUERY_EXPANSION_COUNT=2
USE_CONTEXTUAL_COMPRESSION=false
```

---

### 8. RAG конфигурация

```bash
RAG_MAX_DOC_CHARS=50000       # Макс символов из документов
RAG_MAX_CONTEXT_CHARS=30000   # Макс размер контекста для LLM
RAG_SCORE_THRESHOLD=0.0       # Порог релевантности
RAG_MAX_RESULTS=60            # Максимум результатов поиска

# Hybrid Search (Vector + BM25)
USE_HYBRID_SEARCH=true
BM25_WEIGHT=0.35
VECTOR_WEIGHT=0.65

# Chunking
CHUNK_SIZE=1200               # Размер чанка (символы)
CHUNK_OVERLAP=200             # Перекрытие

# Relevance thresholds
RELEVANCE_ESCALATION_THRESHOLD=2.0
EMBEDDING_SIMILARITY_AUTOPASS=0.65
```

---

### 9. Обработка документов

```bash
MAX_FILE_SIZE=1048576000      # Макс размер файла (байты, ~1GB)
BODY_LIMIT=52428800000        # Лимит HTTP body
```

---

### 10. HTTP и CORS

```bash
HTTP_TIMEOUT_SEC=300
CORS_ALLOW_ORIGINS=*
CORS_ALLOW_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOW_HEADERS=Origin,Content-Type,Accept,Authorization
```

---

### 11. Логирование

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
```

### Качественный режим (GPU, длинные ответы)

```bash
N_GPU_LAYERS=-1
N_CTX=32768
GEN_MAX_NEW_TOKENS=8192
GEN_TEMPERATURE=0.75
RAG_MAX_RESULTS=60
USE_RERANKER=true
```

### Детерминированный режим (для тестов)

```bash
GEN_TEMPERATURE=0.0
GEN_DO_SAMPLE=false
GEN_TOP_P=1.0
GEN_TOP_K=1
```

---

## Валидация

Каждый микросервис валидирует конфигурацию при старте. Если обязательная переменная не задана — сервис не запустится с ошибкой.

```bash
# Пример ошибки
docker compose logs backend
# "Failed to load configuration: config validation failed: PORT cannot be empty"
```

---

## Список всех переменных

| Переменная | Тип | Значение по умолчанию |
|------------|-----|----------------------|
| `BACKEND_PORT` | int | 8080 |
| `DOCUMENT_PARSER_PORT` | int | 8081 |
| `VECTOR_DB_PORT` | int | 8082 |
| `AI_SERVICE_PORT` | int | 8000 |
| `FRONTEND_PORT` | int | 3000 |
| `LLAMA_CPP_PORT` | int | 8090 |
| `DOC_PARSER_URL` | string | http://document-parser:8081 |
| `VECTOR_URL` | string | http://vector-db:8082 |
| `AI_URL` | string | http://ai-service:8000 |
| `GGUF_MODEL_FILE` | string | qwen3-4b-q4_k_m.gguf |
| `GGUF_MODEL_PATH` | string | ./models/... |
| `N_CTX` | int | 32768 |
| `N_THREADS` | int | 6 |
| `N_GPU_LAYERS` | int | 0 |
| `LLAMA_PARALLEL` | int | 2 |
| `POSTGRES_DB` | string | chatbot |
| `POSTGRES_USER` | string | chatbot |
| `POSTGRES_PASSWORD` | string | chatbot_password |
| `DATABASE_URL` | string | postgresql://... |
| `JWT_SECRET` | string | (обязательно задать) |
| `QDRANT_HOST` | string | qdrant |
| `QDRANT_PORT_REST` | int | 6333 |
| `QDRANT_PORT_GRPC` | int | 6334 |
| `QDRANT_COLLECTION_SIZE` | int | 768 |
| `GEN_MAX_NEW_TOKENS` | int | 8192 |
| `GEN_TEMPERATURE` | float | 0.75 |
| `GEN_TOP_P` | float | 0.92 |
| `GEN_TOP_K` | int | 40 |
| `GEN_DO_SAMPLE` | bool | true |
| `GEN_STOP_SEQUENCES` | string | <\|im_end\|>,<\|endoftext\|> |
| `GEN_SYSTEM_BASE_PROMPT` | string | (см. .env) |
| `GEN_USER_PROMPT` | string | (см. .env) |
| `EMBEDDING_MODEL_NAME` | string | intfloat/multilingual-e5-base |
| `EMBEDDING_CACHE_FOLDER` | string | ./models/embedding |
| `USE_RERANKER` | bool | true |
| `RERANKER_MODEL_NAME` | string | cross-encoder/ms-marco-MiniLM-L-6-v2 |
| `USE_HYBRID_SEARCH` | bool | true |
| `BM25_WEIGHT` | float | 0.35 |
| `VECTOR_WEIGHT` | float | 0.65 |
| `RAG_MAX_DOC_CHARS` | int | 50000 |
| `RAG_MAX_CONTEXT_CHARS` | int | 30000 |
| `RAG_SCORE_THRESHOLD` | float | 0.0 |
| `RAG_MAX_RESULTS` | int | 60 |
| `RELEVANCE_ESCALATION_THRESHOLD` | float | 2.0 |
| `EMBEDDING_SIMILARITY_AUTOPASS` | float | 0.65 |
| `CHUNK_SIZE` | int | 1200 |
| `CHUNK_OVERLAP` | int | 200 |
| `MAX_FILE_SIZE` | int | 1048576000 |
| `BODY_LIMIT` | int | 52428800000 |
| `HTTP_TIMEOUT_SEC` | int | 300 |
| `CORS_ALLOW_ORIGINS` | string | * |
| `LOG_LEVEL` | string | info |
