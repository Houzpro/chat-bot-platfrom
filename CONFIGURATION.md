# 🔧 Конфигурация RAG Chat Platform

## Единый конфиг (.env)

Все настройки системы централизованы в файле `.env` - это единственный источник правды для всех микросервисов.

### ✅ Преимущества единого конфига

- **Нет дублирования** - все значения в одном месте
- **Нет хардкода** - все микросервисы обязаны читать из переменных окружения
- **Легкая настройка** - меняете `.env` и перезапускаете контейнеры
- **Валидация при старте** - если какая-то переменная не задана, сервис не запустится

---

## 📋 Структура конфигурации

### 1. Порты и URL микросервисов

```bash
# Backend Gateway
BACKEND_PORT=8080

# Document Parser Service
DOCUMENT_PARSER_PORT=8081
DOC_PARSER_URL=http://document-parser:8081

# Vector DB Service
VECTOR_DB_PORT=8082
VECTOR_URL=http://vector-db:8082

# AI Service
AI_SERVICE_PORT=8000
AI_URL=http://ai-service:8000

# Frontend
FRONTEND_PORT=3000
```

**Важно:**
- В Docker Compose используются имена контейнеров вместо `localhost`
- Для локальной разработки замените на `http://localhost:XXXX`

---

### 2. Qdrant (Векторная БД)

```bash
QDRANT_HOST=qdrant
QDRANT_PORT_REST=6333
QDRANT_PORT_GRPC=6334
QDRANT_COLLECTION_SIZE=384
```

**Описание:**
- `QDRANT_HOST` - hostname Qdrant (в Docker: `qdrant`, локально: `localhost`)
- `QDRANT_PORT_REST` - REST API порт
- `QDRANT_PORT_GRPC` - gRPC порт (используется микросервисами)
- `QDRANT_COLLECTION_SIZE` - размерность векторов (384 для paraphrase-multilingual-MiniLM-L12-v2)

---

### 3. AI модель (GGUF)

```bash
GGUF_MODEL_PATH=./models/qwen3-4b-q4_k_m.gguf
N_THREADS=6
N_CTX=8192
```

**Описание:**
- `GGUF_MODEL_PATH` - путь к GGUF модели (внутри контейнера)
- `N_THREADS` - количество CPU потоков для инференса
- `N_CTX` - размер контекста модели (токены)

**Доступные модели:**
- `qwen2.5-3b-instruct-q4_k_m.gguf` - 2.0 GB, быстрая
- `qwen3-4b-q4_k_m.gguf` - 2.4 GB, более качественная (по умолчанию)

---

### 4. Параметры генерации

```bash
GEN_MAX_NEW_TOKENS=512
GEN_TEMPERATURE=0.75
GEN_TOP_P=0.92
GEN_TOP_K=40
GEN_DO_SAMPLE=true
```

**Описание:**
- `GEN_MAX_NEW_TOKENS` - максимальная длина ответа
- `GEN_TEMPERATURE` - креативность (0.0 = детерминированно, 2.0 = очень креативно)
- `GEN_TOP_P` - nucleus sampling (0.9-0.95 оптимально)
- `GEN_TOP_K` - ограничение топ-K токенов
- `GEN_DO_SAMPLE` - включить sampling (true) или greedy (false)

---

### 5. System prompts

```bash
GEN_SYSTEM_BASE_PROMPT="DO NOT use markdown formatting. Use plain text only. /no_think"
GEN_USER_PROMPT="You are a helpful assistant."
```

**Описание:**
- `GEN_SYSTEM_BASE_PROMPT` - базовый system prompt (скрыт от пользователя)
- `GEN_USER_PROMPT` - пользовательский system prompt

**Важно:** Если в промпте есть специальные символы, используйте кавычки.

---

### 6. Embeddings модель

```bash
EMBEDDING_MODEL_NAME=sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2
EMBEDDING_CACHE_FOLDER=./models/embedding
```

**Описание:**
- `EMBEDDING_MODEL_NAME` - HuggingFace модель для эмбеддингов
- `EMBEDDING_CACHE_FOLDER` - папка для кэша модели

**Рекомендуемые модели:**
- `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2` - мультиязычная, 384D
- `sentence-transformers/all-MiniLM-L6-v2` - английская, быстрая

---

### 7. RAG конфигурация

```bash
RAG_TOP_K=3
RAG_MAX_DOC_CHARS=3000
CHUNK_SIZE=2500
CHUNK_OVERLAP=500
```

**Описание:**
- `RAG_TOP_K` - сколько документов искать
- `RAG_MAX_DOC_CHARS` - максимум символов из каждого документа
- `CHUNK_SIZE` - размер чанка при разбиении документа
- `CHUNK_OVERLAP` - перекрытие между чанками

**Оптимальные значения:**
- `RAG_TOP_K`: 3-5 документов
- `CHUNK_SIZE`: 1500-2500 символов
- `CHUNK_OVERLAP`: 20-25% от CHUNK_SIZE

---

### 8. HTTP настройки

```bash
HTTP_TIMEOUT_SEC=300
CORS_ALLOW_ORIGINS=*
CORS_ALLOW_METHODS=GET,POST,PUT,DELETE,OPTIONS
CORS_ALLOW_HEADERS=Origin,Content-Type,Accept
```

**Описание:**
- `HTTP_TIMEOUT_SEC` - таймаут для HTTP запросов
- `CORS_*` - настройки CORS

---

### 9. Обработка документов

```bash
MAX_FILE_SIZE=10485760
BODY_LIMIT=52428800
```

**Описание:**
- `MAX_FILE_SIZE` - максимальный размер файла (байты)
- `BODY_LIMIT` - лимит на размер HTTP body

---

## 🚀 Как использовать

### Для Docker Compose

1. Отредактируйте `.env`
2. Перезапустите контейнеры:

```bash
docker-compose down
docker-compose up -d --build
```

### Для локальной разработки

1. Скопируйте `.env` в `.env.local`
2. Измените URL на `localhost`:

```bash
DOC_PARSER_URL=http://localhost:8081
VECTOR_URL=http://localhost:8082
AI_URL=http://localhost:8000
QDRANT_HOST=localhost
```

3. Запускайте сервисы с `.env.local`:

```bash
# Backend
cd services/backend
export $(cat ../../.env.local | xargs)
go run main.go

# AI Service
cd services/python-ai
export $(cat ../../.env.local | xargs)
uvicorn app.main:app --reload
```

---

## ⚠️ Важные правила

### 1. Все переменные обязательны

Если переменная не задана, сервис **не запустится** с ошибкой валидации.

### 2. Нет fallback значений

В коде микросервисов **нет дефолтных значений**. Все читается из `.env`.

### 3. Валидация при старте

Каждый микросервис валидирует конфигурацию при запуске:

**Backend (Go):**
```go
if c.Server.Port == "" {
    return fmt.Errorf("PORT cannot be empty")
}
```

**AI Service (Python):**
```python
if not self.gguf_model_path:
    errors.append("GGUF_MODEL_PATH is required")
```

### 4. Логирование конфигурации

При старте каждый сервис пишет в лог, какие значения использует:

```
🚀 Vector DB Service starting on port 8082
📊 Connected to Qdrant at qdrant:6334
   CORS origins: *
```

---

## 🔍 Troubleshooting

### Ошибка: "PORT environment variable is required"

**Причина:** Переменная не задана в `.env`

**Решение:** Добавьте в `.env`:
```bash
DOCUMENT_PARSER_PORT=8081
```

### Ошибка: "Failed to connect to Qdrant"

**Причина:** Неверный `QDRANT_HOST` или `QDRANT_PORT`

**Решение:** Проверьте в `.env`:
```bash
QDRANT_HOST=qdrant  # В Docker Compose
QDRANT_PORT_GRPC=6334
```

### Ошибка: "GGUF_MODEL_PATH is required"

**Причина:** Не задан путь к модели

**Решение:** Добавьте в `.env`:
```bash
GGUF_MODEL_PATH=./models/qwen3-4b-q4_k_m.gguf
```

---

## 📚 Примеры конфигураций

### Быстрый режим (низкое качество)

```bash
GGUF_MODEL_PATH=./models/qwen2.5-3b-instruct-q4_k_m.gguf
N_CTX=4096
GEN_MAX_NEW_TOKENS=256
GEN_TEMPERATURE=0.3
RAG_TOP_K=1
CHUNK_SIZE=1000
```

### Качественный режим (медленнее)

```bash
GGUF_MODEL_PATH=./models/qwen3-4b-q4_k_m.gguf
N_CTX=8192
GEN_MAX_NEW_TOKENS=1024
GEN_TEMPERATURE=0.75
RAG_TOP_K=5
CHUNK_SIZE=2500
```

### Детерминированный режим (для тестов)

```bash
GEN_TEMPERATURE=0.0
GEN_DO_SAMPLE=false
GEN_TOP_P=1.0
GEN_TOP_K=1
```

---

## 📄 Список всех переменных

| Переменная | Тип | Обязательная | Значение по умолчанию в .env |
|------------|-----|--------------|------------------------------|
| `BACKEND_PORT` | int | ✅ | 8080 |
| `DOCUMENT_PARSER_PORT` | int | ✅ | 8081 |
| `VECTOR_DB_PORT` | int | ✅ | 8082 |
| `AI_SERVICE_PORT` | int | ✅ | 8000 |
| `FRONTEND_PORT` | int | ✅ | 3000 |
| `DOC_PARSER_URL` | string | ✅ | http://document-parser:8081 |
| `VECTOR_URL` | string | ✅ | http://vector-db:8082 |
| `AI_URL` | string | ✅ | http://ai-service:8000 |
| `QDRANT_HOST` | string | ✅ | qdrant |
| `QDRANT_PORT_REST` | int | ✅ | 6333 |
| `QDRANT_PORT_GRPC` | int | ✅ | 6334 |
| `QDRANT_COLLECTION_SIZE` | int | ❌ | 384 |
| `GGUF_MODEL_PATH` | string | ✅ | ./models/qwen3-4b-q4_k_m.gguf |
| `N_THREADS` | int | ✅ | 6 |
| `N_CTX` | int | ✅ | 8192 |
| `GEN_MAX_NEW_TOKENS` | int | ✅ | 512 |
| `GEN_TEMPERATURE` | float | ✅ | 0.75 |
| `GEN_TOP_P` | float | ✅ | 0.92 |
| `GEN_TOP_K` | int | ✅ | 40 |
| `GEN_DO_SAMPLE` | bool | ✅ | true |
| `GEN_SYSTEM_BASE_PROMPT` | string | ✅ | (см. .env) |
| `GEN_USER_PROMPT` | string | ✅ | (см. .env) |
| `EMBEDDING_MODEL_NAME` | string | ✅ | sentence-transformers/... |
| `EMBEDDING_CACHE_FOLDER` | string | ✅ | ./models/embedding |
| `RAG_TOP_K` | int | ✅ | 3 |
| `RAG_MAX_DOC_CHARS` | int | ✅ | 3000 |
| `CHUNK_SIZE` | int | ✅ | 2500 |
| `CHUNK_OVERLAP` | int | ✅ | 500 |
| `MAX_FILE_SIZE` | int | ✅ | 10485760 |
| `BODY_LIMIT` | int | ✅ | 52428800 |
| `HTTP_TIMEOUT_SEC` | int | ✅ | 300 |
| `CORS_ALLOW_ORIGINS` | string | ❌ | * |
| `CORS_ALLOW_METHODS` | string | ❌ | GET,POST,... |
| `CORS_ALLOW_HEADERS` | string | ❌ | Origin,Content-Type,... |
| `LOG_LEVEL` | string | ❌ | info |
| `APP_NAME` | string | ❌ | RAG Chat Platform |
| `APP_VERSION` | string | ❌ | 1.0.0 |
| `ENVIRONMENT` | string | ❌ | production |

---

## 🎯 Заключение

Теперь вся конфигурация системы находится в одном файле `.env`. Никакие значения не захардкожены в коде микросервисов. Для изменения любых параметров достаточно отредактировать `.env` и перезапустить контейнеры.
