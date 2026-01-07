# 🤖 RAG Chat Platform - Полное руководство

## 📋 Содержание

- [Обзор платформы](#обзор-платформы)
- [Архитектура](#архитектура)
- [Микросервисы](#микросервисы)
- [Быстрый старт](#быстрый-старт)
- [Конфигурация](#конфигурация)
- [RAG Pipeline](#rag-pipeline)
- [API Documentation](#api-documentation)
- [Разработка](#разработка)
- [Deployment](#deployment)
- [Troubleshooting](#troubleshooting)

---

## Обзор платформы

**RAG Chat Platform** — это микросервисная платформа для интеллектуального чата с документами на базе RAG (Retrieval-Augmented Generation).

### Ключевые возможности

✅ **Загрузка документов** - PDF, DOCX, TXT, CSV, JSON, HTML, Markdown  
✅ **Векторный поиск** - Семантический поиск по содержимому через Qdrant  
✅ **RAG генерация** - Ответы на основе контекста из документов  
✅ **Streaming** - Потоковая генерация ответов в реальном времени  
✅ **Настройки модели** - Полный контроль параметров генерации через UI  
✅ **CPU-оптимизированная** - Работает на CPU с GGUF моделями  

### Технологический стек

| Компонент | Технология |
|-----------|------------|
| Frontend | React 18 + Vite |
| Backend Gateway | Go 1.23 + Fiber |
| Document Parser | Go 1.24 |
| Vector DB Service | Go 1.23 + Qdrant gRPC |
| AI Service | Python 3.10 + FastAPI + llama-cpp-python |
| Vector DB | Qdrant |
| Embeddings | sentence-transformers |
| LLM | Qwen3-4B (GGUF) |

---

## Архитектура

### Высокоуровневая диаграмма

```
┌─────────────────────────────────────────────────────────────┐
│                        Frontend (React)                      │
│                     http://localhost:3000                    │
│  - Drag & Drop загрузка файлов                              │
│  - Чат интерфейс с streaming                                │
│  - Настройки параметров модели                              │
└───────────────────────────┬─────────────────────────────────┘
                            │ HTTP/SSE
                            ▼
┌─────────────────────────────────────────────────────────────┐
│              Backend Gateway (Go) :8080                      │
│  - Точка входа для всех клиентских запросов                │
│  - Оркестрация между микросервисами                         │
│  - CORS, валидация, обработка ошибок                        │
└──┬──────────────────┬───────────────────┬──────────────────┘
   │                  │                   │
   │ HTTP             │ HTTP              │ HTTP
   ▼                  ▼                   ▼
┌─────────────┐  ┌──────────────┐  ┌────────────────┐
│ Document    │  │  Vector DB   │  │  AI Service    │
│  Parser     │  │   Service    │  │   (Python)     │
│  (Go)       │  │    (Go)      │  │    :8000       │
│  :8081      │  │    :8082     │  │                │
└─────────────┘  └──────┬───────┘  └────────────────┘
                        │ gRPC
                        ▼
                 ┌─────────────┐
                 │   Qdrant    │
                 │ (Vector DB) │
                 │ :6333/:6334 │
                 └─────────────┘
```

### Потоки данных

#### 1. Загрузка документа

```
Client → Backend → Document Parser
          ↓ (text)
       Backend → AI Service (create embeddings)
          ↓ (vectors)
       Backend → Vector DB Service → Qdrant
          ↓ (store vectors)
       Backend → Client (success)
```

#### 2. RAG запрос

```
Client → Backend (query)
          ↓
       Backend → AI Service (embed query)
          ↓ (query_vector)
       Backend → Vector DB Service → Qdrant (search)
          ↓ (top-k documents)
       Backend → AI Service (generate with context)
          ↓ (streaming tokens via SSE)
       Backend → Client (stream response)
```

---

## Микросервисы

### 1. Frontend (React + Nginx)

**Порт:** 3000  
**Технологии:** React 18, Vite, CSS Modules  

**Ответственность:**
- Пользовательский интерфейс
- Drag & Drop загрузка файлов
- Чат с streaming ответами
- Настройки параметров модели (temperature, top_p, top_k, max_tokens)
- Управление client_id

**Основные компоненты:**
- `App.jsx` - главный компонент, state management
- `ChatArea.jsx` - область чата с сообщениями
- `FileUpload.jsx` - загрузка файлов
- `ModelSettings.jsx` - настройки генерации
- `DocumentSearch.jsx` - поиск по документам

**Build:**
```bash
cd frontend
npm install
npm run build  # → dist/
```

---

### 2. Backend Gateway (Go)

**Порт:** 8080  
**Файлы:** `services/backend/`  
**Framework:** Fiber v2  

**Ответственность:**
- API Gateway для всех клиентских запросов
- Оркестрация между микросервисами
- RAG pipeline: parse → embed → search → generate
- Streaming SSE для генерации
- Валидация и обработка ошибок

**Основные эндпоинты:**

| Метод | Путь | Описание |
|-------|------|----------|
| GET | `/health` | Health check |
| POST | `/api/v1/documents/upload` | Загрузка документа |
| POST | `/api/v1/search` | Поиск по векторной БД |
| POST | `/api/v1/chat/rag` | RAG чат (streaming) |

**Конфигурация:**
```go
type Config struct {
    Server     ServerConfig
    Services   ServicesConfig  // URLs микросервисов
    RAG        RAGConfig       // Параметры RAG
    Generation GenerationDefaults
}
```

**Зависимости:**
- Document Parser Service
- Vector DB Service  
- AI Service

---

### 3. Document Parser Service (Go)

**Порт:** 8081  
**Файлы:** `services/document-parser-service/`  

**Ответственность:**
- Парсинг документов различных форматов
- Извлечение текста из файлов
- Поддержка форматов: PDF, DOCX, TXT, JSON, CSV, XLSX, HTML, Markdown

**API:**
```bash
POST /parse
Content-Type: multipart/form-data
- file: <binary>

Response:
{
  "content": "Extracted text...",
  "metadata": {
    "filename": "doc.pdf",
    "size": 12345,
    "format": "pdf"
  }
}
```

**Библиотеки:**
- PDF: `pdfcpu`
- DOCX: `docx` parser
- Excel: `xlsx` reader
- HTML: `goquery`

---

### 4. Vector DB Service (Go)

**Порт:** 8082  
**Файлы:** `services/vector-db-service/`  

**Ответственность:**
- Работа с Qdrant через gRPC
- Управление коллекциями (по client_id)
- Векторный поиск (cosine similarity)
- Хранение и удаление документов

**API:**

```bash
# Создать коллекцию
POST /collections/ensure
{
  "client_id": "user123",
  "vector_size": 384
}

# Добавить документы
POST /documents/add
{
  "client_id": "user123",
  "documents": [
    {
      "id": "doc1",
      "vector": [0.1, 0.2, ...],
      "text": "content",
      "metadata": {...}
    }
  ]
}

# Поиск
POST /documents/search
{
  "client_id": "user123",
  "query_vector": [0.1, 0.2, ...],
  "limit": 3
}

# Удалить все документы клиента
DELETE /documents/delete/{client_id}
```

**Qdrant схема:**
- **Collection name:** `rag_collection_{client_id}`
- **Vector size:** 384 (для paraphrase-multilingual-MiniLM-L12-v2)
- **Distance:** Cosine
- **Payload:** `{text: string, filename: string, chunk_id: int, ...}`

---

### 5. AI Service (Python + FastAPI)

**Порт:** 8000  
**Файлы:** `services/python-ai/`  
**Framework:** FastAPI + Uvicorn  

**Ответственность:**
- LLM генерация (Qwen3-4B GGUF через llama-cpp-python)
- Создание embeddings (sentence-transformers)
- Streaming генерация через SSE
- Обработка параметров генерации

**API:**

```bash
# Health check
GET /health

# Создать embeddings
POST /embeddings
{
  "texts": ["text1", "text2", ...]
}
Response: {
  "embeddings": [[0.1, 0.2, ...], ...]
}

# Streaming генерация
POST /ask
{
  "messages": [
    {"role": "user", "content": "question"}
  ],
  "max_new_tokens": 512,
  "temperature": 0.75,
  "top_p": 0.92,
  "top_k": 40,
  "do_sample": true,
  "system_prompt": "..."
}
Response: SSE stream
data: {"type": "token", "token": "Hello"}
data: {"type": "token", "token": " world"}
data: {"type": "done"}
```

**Модели:**

| Файл | Размер | Описание |
|------|--------|----------|
| `qwen2.5-3b-instruct-q4_k_m.gguf` | 2.0 GB | Быстрая |
| `qwen3-4b-q4_k_m.gguf` | 2.4 GB | Качественнее (по умолчанию) |

**Embedding модель:**
- `sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2`
- Размерность: 384
- Мультиязычная (русский, английский, и др.)

**Оптимизации:**
- CPU inference через llama-cpp
- Multi-threading (6 потоков по умолчанию)
- Контекст 8192 токена
- Streaming для быстрого отображения

---

### 6. Qdrant (Vector Database)

**Порты:** 6333 (REST), 6334 (gRPC)  
**Image:** `qdrant/qdrant:latest`  

**Использование:**
- Хранение векторных представлений документов
- Быстрый семантический поиск (cosine similarity)
- Коллекции по client_id для изоляции данных
- Web UI: http://localhost:6333/dashboard

---

## Быстрый старт

### Предварительные требования

- Docker & Docker Compose
- 8 GB RAM минимум
- 10 GB свободного места (для моделей)

### Запуск

```bash
# 1. Клонировать репозиторий
git clone <repo-url>
cd chat-bot-platfrom

# 2. Запустить все сервисы
docker-compose up -d --build

# 3. Дождаться запуска (30-60 секунд)
docker-compose ps

# 4. Проверить health
curl http://localhost:8080/health
curl http://localhost:8000/health

# 5. Открыть в браузере
open http://localhost:3000
```

### Первое использование

1. **Откройте** http://localhost:3000
2. **Client ID** сгенерируется автоматически (или введите свой)
3. **Загрузите документ** через drag & drop
4. **Задайте вопрос** в чате
5. **Настройте параметры** через ⚙️ (опционально)

---

## Конфигурация

Вся конфигурация в файле **`.env`** в корне проекта.

### Основные параметры

```bash
# Модель
GGUF_MODEL_PATH=./models/qwen3-4b-q4_k_m.gguf
N_THREADS=6                    # CPU потоки
N_CTX=8192                     # Размер контекста

# Генерация
GEN_MAX_NEW_TOKENS=512         # Длина ответа
GEN_TEMPERATURE=0.75           # 0.0-2.0, креативность
GEN_TOP_P=0.92                 # Nucleus sampling
GEN_TOP_K=40                   # Top-K sampling
GEN_DO_SAMPLE=true             # Sampling вкл/выкл

# System prompts
GEN_SYSTEM_BASE_PROMPT="DO NOT use markdown..."
GEN_USER_PROMPT="You are a helpful assistant..."

# RAG
RAG_TOP_K=3                    # Сколько документов искать
RAG_MAX_DOC_CHARS=3000         # Макс символов из документа
CHUNK_SIZE=2500                # Размер чанка
CHUNK_OVERLAP=500              # Перекрытие чанков

# Embeddings
EMBEDDING_MODEL_NAME=sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2
```

**Подробнее:** см. [CONFIGURATION.md](CONFIGURATION.md)

---

## RAG Pipeline

### Полный цикл работы RAG

#### Этап 1: Индексация документа

```
1. Пользователь загружает файл (PDF/DOCX/TXT/...)
   ↓
2. Backend → Document Parser Service
   - Извлекает текст из файла
   - Возвращает plain text
   ↓
3. Backend разбивает текст на чанки
   - chunk_size=2500 символов
   - overlap=500 символов
   - Пример: doc длиной 10000 символов → 5 чанков
   ↓
4. Backend → AI Service (embeddings)
   - Запрос: POST /embeddings с массивом чанков
   - Модель: paraphrase-multilingual-MiniLM-L12-v2
   - Ответ: векторы размерности 384 для каждого чанка
   ↓
5. Backend → Vector DB Service
   - Создает коллекцию если нужно
   - Сохраняет векторы + текст + метаданные в Qdrant
   - Collection: rag_collection_{client_id}
   ↓
6. Backend → Client (success response)
```

#### Этап 2: Поиск и генерация (RAG)

```
1. Пользователь задает вопрос: "Что такое JSON?"
   ↓
2. Backend → AI Service (embed query)
   - Вопрос превращается в вектор [0.1, 0.2, ..., 0.384]
   ↓
3. Backend → Vector DB Service (search)
   - Ищет топ-K (default: 3) наиболее похожих чанков
   - Метрика: cosine similarity
   - Возвращает тексты найденных чанков
   ↓
4. Backend формирует контекст
   - Собирает тексты найденных документов
   - Обрезает до RAG_MAX_DOC_CHARS символов
   ↓
5. Backend → AI Service (generate)
   - Prompt:
     <system>You are a helpful assistant</system>
     <context>
     [Найденный текст из документов]
     </context>
     <user>Что такое JSON?</user>
   - Параметры: temperature, top_p, top_k, max_tokens
   ↓
6. AI Service → Backend (streaming SSE)
   - Генерирует токен за токеном
   - data: {"type": "token", "token": "JSON"}
   - data: {"type": "token", "token": " is"}
   - ...
   ↓
7. Backend → Client (streaming SSE)
   - Пересылает токены клиенту
   - Frontend обновляет UI в реальном времени
   ↓
8. Завершение
   - data: {"type": "done"}
   - Соединение закрывается
```

### Оптимизации RAG

**Chunking стратегия:**
- Большие чанки (2500 символов) → лучше контекст
- Overlap (500 символов) → нет потери информации на границах
- Метаданные: filename, chunk_id, timestamp

**Поиск:**
- Top-K=3 → баланс между качеством и скоростью
- Cosine similarity → учитывает семантику
- Фильтрация по client_id → изоляция данных

**Генерация:**
- Streaming SSE → быстрый первый токен
- System prompt с `/no_think` → пропуск внутренних размышлений
- Температура 0.75 → баланс детерминизма и креативности

---

## API Documentation

### Backend Gateway API

Base URL: `http://localhost:8080`

#### Health Check

```bash
GET /health

Response 200:
{
  "status": "healthy",
  "services": {
    "document_parser": "ok",
    "vector_db": "ok",
    "ai_service": "ok"
  }
}
```

#### Upload Document

```bash
POST /api/v1/documents/upload
Content-Type: multipart/form-data

Form data:
- file: <binary>
- client_id: "user123"

Response 200:
{
  "success": true,
  "message": "Document uploaded and indexed",
  "chunks_created": 5,
  "document_id": "doc_abc123"
}
```

#### Search Documents

```bash
POST /api/v1/search
Content-Type: application/json

Body:
{
  "client_id": "user123",
  "query": "что такое JSON",
  "limit": 3
}

Response 200:
{
  "documents": [
    {
      "text": "JSON is a data format...",
      "score": 0.95,
      "metadata": {
        "filename": "doc.pdf",
        "chunk_id": 2
      }
    },
    ...
  ]
}
```

#### RAG Chat (Streaming)

```bash
POST /api/v1/chat/rag
Content-Type: application/json

Body:
{
  "client_id": "user123",
  "query": "что такое JSON",
  "limit": 3,
  "temperature": 0.75,      // опционально
  "top_p": 0.92,            // опционально
  "top_k": 40,              // опционально
  "max_new_tokens": 512,    // опционально
  "system_prompt": "..."    // опционально
}

Response 200: (SSE stream)
data: {"documents": [...]}

data: {"type": "token", "token": "JSON"}

data: {"type": "token", "token": " is"}

data: {"type": "token", "token": " a"}

...

data: {"type": "done"}
```

---

## Разработка

### Локальная разработка без Docker

#### 1. Запустить Qdrant

```bash
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant:latest
```

#### 2. Document Parser Service

```bash
cd services/document-parser-service
go mod download
go run main.go
# Будет доступен на :8081
```

#### 3. Vector DB Service

```bash
cd services/vector-db-service
export QDRANT_HOST=localhost
export QDRANT_PORT=6334
go run main.go
# Будет доступен на :8082
```

#### 4. AI Service

```bash
cd services/python-ai
pip install -r requirements.txt
export GGUF_MODEL_PATH=./models/qwen3-4b-q4_k_m.gguf
export N_THREADS=6
export N_CTX=8192
# ... другие переменные из .env
./start.sh
# Будет доступен на :8000
```

#### 5. Backend Gateway

```bash
cd services/backend
export DOC_PARSER_URL=http://localhost:8081
export VECTOR_URL=http://localhost:8082
export AI_URL=http://localhost:8000
# ... другие переменные из .env
go run main.go
# Будет доступен на :8080
```

#### 6. Frontend

```bash
cd frontend
npm install
npm run dev
# Будет доступен на :5173 (Vite dev server)
```

### Hot Reload

- **Frontend:** Vite автоматически перезагружает при изменениях
- **Go сервисы:** Используйте `air` или `reflex` для hot reload
- **Python:** FastAPI с `--reload` флагом

### Тестирование

```bash
# Интеграционные тесты
./test-integration.sh

# Тест параметров модели
./test-model-params.sh

# Unit тесты Go сервисов
cd services/backend
go test ./...

# Python тесты
cd services/python-ai
pytest
```

---

## Deployment

### Production с Docker Compose

1. **Настроить `.env` для production:**

```bash
# Безопасность
CORS_ALLOW_ORIGINS=https://your-domain.com

# Производительность
N_THREADS=16              # Больше для мощного CPU
GEN_MAX_NEW_TOKENS=1024   # Длинные ответы
CHUNK_SIZE=3000           # Большие чанки

# URLs для production
DOC_PARSER_URL=http://document-parser:8081
VECTOR_URL=http://vector-db:8082
AI_URL=http://ai-service:8000
```

2. **Запустить:**

```bash
docker-compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

3. **Nginx reverse proxy (опционально):**

```nginx
server {
    listen 80;
    server_name your-domain.com;

    location / {
        proxy_pass http://localhost:3000;
    }

    location /api/ {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### Kubernetes Deployment

Для Kubernetes деплоя нужно создать:
- `Deployment` для каждого микросервиса
- `Service` для внутренней коммуникации
- `Ingress` для внешнего доступа
- `PersistentVolumeClaim` для Qdrant
- `ConfigMap` для конфигурации
- `Secret` для чувствительных данных

### Масштабирование

**Горизонтальное:**
- Frontend: stateless, можно масштабировать
- Backend Gateway: stateless, можно масштабировать
- Document Parser: stateless, можно масштабировать
- Vector DB Service: stateless, можно масштабировать
- AI Service: stateful (модель в памяти), сложнее масштабировать
- Qdrant: нужен cluster mode для масштабирования

**Вертикальное:**
- AI Service: увеличить CPU/RAM для быстрой генерации
- Qdrant: увеличить RAM для больших коллекций

---

## Troubleshooting

### Проблема: Контейнеры не запускаются

```bash
# Проверить логи
docker-compose logs -f

# Проверить статус
docker-compose ps

# Перезапустить
docker-compose down
docker-compose up -d --build
```

### Проблема: Frontend не отображается

```bash
# Проверить nginx логи
docker logs chatbot-frontend

# Проверить сборку
docker exec chatbot-frontend ls -la /usr/share/nginx/html

# Пересобрать
docker-compose up -d --build frontend
```

### Проблема: Backend не может подключиться к микросервисам

```bash
# Проверить, что все сервисы запущены
docker-compose ps

# Проверить сеть
docker network inspect chat-bot-platfrom_default

# Проверить .env
cat .env | grep URL

# Должно быть:
# DOC_PARSER_URL=http://document-parser:8081
# VECTOR_URL=http://vector-db:8082
# AI_URL=http://ai-service:8000
```

### Проблема: Модель не загружается

```bash
# Проверить наличие модели
docker exec chatbot-ai-service ls -lh /app/models/*.gguf

# Проверить путь в .env
docker exec chatbot-ai-service printenv GGUF_MODEL_PATH

# Проверить логи загрузки
docker logs chatbot-ai-service --tail 50
```

### Проблема: Qdrant недоступен

```bash
# Проверить статус
docker logs chatbot-qdrant

# Проверить доступность
curl http://localhost:6333/health

# Web UI
open http://localhost:6333/dashboard
```

### Проблема: Текст без пробелов при генерации

Это исправлено в коде. Проверьте версию:
- Frontend: убран `.trim()` из `cleanMarkdown()`
- AI Service: фильтрация пустых токенов в начале

```bash
# Пересобрать
docker-compose up -d --build frontend ai-service
```

### Проблема: Ошибки валидации конфигурации

Все переменные в `.env` обязательны. Проверьте:

```bash
# Backend
docker logs chatbot-backend | grep -i error

# AI Service
docker logs chatbot-ai-service | grep -i error

# Должны быть установлены все переменные из .env
```

### Проблема: Медленная генерация

1. Увеличьте `N_THREADS` в `.env`
2. Используйте меньшую модель (qwen2.5-3b вместо qwen3-4b)
3. Уменьшите `GEN_MAX_NEW_TOKENS`
4. Используйте greedy decoding: `GEN_TEMPERATURE=0.0`, `GEN_DO_SAMPLE=false`

### Получение помощи

1. Проверьте логи: `docker-compose logs -f`
2. Проверьте health checks: `curl localhost:8080/health`
3. Запустите интеграционный тест: `./test-integration.sh`
4. Создайте issue с логами

---

## Полезные команды

```bash
# Просмотр логов
docker-compose logs -f                    # Все сервисы
docker-compose logs -f ai-service         # Конкретный сервис

# Рестарт
docker-compose restart                    # Все сервисы
docker-compose restart ai-service         # Конкретный сервис

# Пересборка
docker-compose up -d --build             # Все сервисы
docker-compose up -d --build frontend    # Конкретный сервис

# Остановка
docker-compose down                       # Остановить
docker-compose down -v                    # Остановить + удалить volumes

# Статус
docker-compose ps                         # Статус контейнеров
docker stats                              # Использование ресурсов

# Exec в контейнер
docker exec -it chatbot-ai-service bash
docker exec -it chatbot-backend sh

# Очистка
docker system prune -a                    # Удалить неиспользуемые образы
docker volume prune                       # Удалить неиспользуемые volumes
```

---

## Лицензия и благодарности

**Лицензия:** MIT

**Используемые технологии:**
- [Qwen](https://huggingface.co/Qwen) - LLM модель
- [Qdrant](https://qdrant.tech/) - векторная БД
- [llama-cpp-python](https://github.com/abetlen/llama-cpp-python) - CPU inference
- [sentence-transformers](https://www.sbert.net/) - embeddings
- [FastAPI](https://fastapi.tiangolo.com/) - Python API framework
- [Fiber](https://gofiber.io/) - Go web framework
- [React](https://react.dev/) - UI framework

---

**Версия документации:** 1.0  
**Дата обновления:** 3 января 2026  
**Статус платформы:** Production Ready ✅
