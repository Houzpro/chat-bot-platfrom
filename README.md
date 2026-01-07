# 🤖 RAG Chat Platform

**Микросервисная платформа для интеллектуального чата с документами на базе RAG (Retrieval-Augmented Generation)**

[![Production Ready](https://img.shields.io/badge/status-production%20ready-brightgreen)]()
[![Docker](https://img.shields.io/badge/docker-ready-blue)]()
[![License](https://img.shields.io/badge/license-MIT-green)]()

---

## ⚡ Quick Start

```bash
# Клонировать и запустить
git clone <repo-url>
cd chat-bot-platfrom
docker-compose up -d --build

# Открыть в браузере
open http://localhost:3000
```

Через 30-60 секунд все сервисы будут готовы!

---

## 🎯 Возможности

- ✅ **Загрузка документов** - PDF, DOCX, TXT, CSV, JSON, HTML, MD
- ✅ **Векторный поиск** - Семантический поиск через Qdrant
- ✅ **RAG генерация** - Ответы на основе контекста документов
- ✅ **Streaming** - Потоковая генерация в реальном времени
- ✅ **Настраиваемая модель** - Полный контроль параметров через UI
- ✅ **CPU-оптимизация** - Работает на CPU с GGUF моделями

---

## 🏗️ Архитектура

```
Frontend (React) :3000
       ↓
Backend Gateway (Go) :8080
       ↓
  ┌────┴────┬──────────┐
  ↓         ↓          ↓
Document  Vector   AI Service
Parser    DB Svc   (Python)
:8081     :8082      :8000
          ↓
       Qdrant
    :6333/:6334
```

**Микросервисы:**
- **Frontend** - React 18 + Vite
- **Backend Gateway** - Go + Fiber (оркестрация)
- **Document Parser** - Go (парсинг файлов)
- **Vector DB Service** - Go + Qdrant gRPC
- **AI Service** - Python + FastAPI + llama-cpp
- **Qdrant** - Vector Database

---

## 📚 Документация

- **[PLATFORM_GUIDE.md](PLATFORM_GUIDE.md)** - 📖 Полное руководство по платформе
- **[CONFIGURATION.md](CONFIGURATION.md)** - ⚙️ Конфигурация и переменные окружения
- **[DEPLOYMENT.md](DEPLOYMENT.md)** - 🚀 Развертывание и управление

---

## 🚀 Использование

### 1. Загрузка документа

Откройте http://localhost:3000 и загрузите файл через drag & drop.

### 2. Задайте вопрос

Напишите вопрос в чате - система найдет релевантные части документа и сгенерирует ответ.

### 3. Настройте параметры (опционально)

Нажмите ⚙️ для настройки:
- Temperature (0-2)
- Top P (0-1)
- Top K (1-100)
- Max Tokens (32-2048)
- System Prompt

---

## 🛠️ Технологический стек

| Компонент | Технология |
|-----------|------------|
| Frontend | React 18, Vite |
| Backend | Go 1.23, Fiber |
| AI Service | Python 3.10, FastAPI |
| LLM | Qwen3-4B (GGUF) |
| Vector DB | Qdrant |
| Embeddings | sentence-transformers |

---

## 📊 Основные параметры

```bash
# Модель
GGUF_MODEL_PATH=./models/qwen3-4b-q4_k_m.gguf
N_THREADS=6
N_CTX=8192

# Генерация (можно менять через UI)
GEN_MAX_NEW_TOKENS=512
GEN_TEMPERATURE=0.75
GEN_TOP_P=0.92
GEN_TOP_K=40

# RAG
RAG_TOP_K=3
CHUNK_SIZE=2500
```

Все параметры настраиваются в файле `.env`.

---

## 🧪 Тестирование

```bash
# Интеграционные тесты
./test-integration.sh

# Тест параметров модели
./test-model-params.sh
```

---

## 📦 API Examples

### Загрузка документа

```bash
curl -X POST http://localhost:8080/api/v1/documents/upload \
  -F "file=@document.pdf" \
  -F "client_id=user123"
```

### RAG чат

```bash
curl -X POST http://localhost:8080/api/v1/chat/rag \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": "user123",
    "query": "что такое JSON",
    "limit": 3
  }'
```

Подробнее в [PLATFORM_GUIDE.md](PLATFORM_GUIDE.md#api-documentation)

---

## 🔧 Разработка

### Локальный запуск

```bash
# 1. Запустить Qdrant
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant

# 2. Запустить микросервисы
cd services/document-parser-service && go run main.go &
cd services/vector-db-service && go run main.go &
cd services/python-ai && ./start.sh &
cd services/backend && go run main.go &

# 3. Запустить frontend
cd frontend && npm run dev
```

### Структура проекта

```
chat-bot-platfrom/
├── .env                      # Единая конфигурация
├── docker-compose.yml        # Docker оркестрация
├── PLATFORM_GUIDE.md         # Полное руководство
├── frontend/                 # React UI
├── services/
│   ├── backend/             # Go API Gateway
│   ├── document-parser-service/  # Go парсер
│   ├── vector-db-service/   # Go Qdrant клиент
│   └── python-ai/           # Python LLM сервис
└── test-*.sh                # Интеграционные тесты
```

---

## 🐛 Troubleshooting

### Контейнеры не запускаются

```bash
docker-compose logs -f
docker-compose ps
```

### Модель не загружается

```bash
docker logs chatbot-ai-service
ls -lh services/python-ai/models/*.gguf
```

### Backend ошибки

```bash
curl http://localhost:8080/health
docker-compose ps
```

Подробнее в [PLATFORM_GUIDE.md](PLATFORM_GUIDE.md#troubleshooting)

---

## 📋 Управление

```bash
# Просмотр логов
docker-compose logs -f [service]

# Перезапуск
docker-compose restart [service]

# Пересборка
docker-compose up -d --build [service]

# Остановка
docker-compose down
```

---

## 🎓 Как это работает

### RAG Pipeline

1. **Индексация документа:**
   - Парсинг файла → Chunking → Embeddings → Сохранение в Qdrant

2. **Генерация ответа:**
   - Вопрос → Embedding → Поиск в Qdrant → Контекст + Вопрос → LLM → Streaming ответ

Подробное объяснение в [PLATFORM_GUIDE.md](PLATFORM_GUIDE.md#rag-pipeline)

---

## 🌟 Особенности

- **Без GPU** - Работает на CPU через llama-cpp
- **Мультиязычность** - Русский, английский и другие языки
- **Streaming** - Быстрая генерация токенов
- **Изоляция данных** - Отдельные коллекции для каждого client_id
- **Настраиваемость** - Все параметры в .env и через UI
- **Production Ready** - Docker Compose, health checks, logging

---

## 📄 Лицензия

MIT License

---

## �� Вклад

1. Fork проекта
2. Создайте feature branch
3. Commit изменения
4. Push в branch
5. Создайте Pull Request

---

## 📞 Поддержка

- 📖 [Полное руководство](PLATFORM_GUIDE.md)
- ⚙️ [Конфигурация](CONFIGURATION.md)
- 🚀 [Развертывание](DEPLOYMENT.md)
- 🐛 Создайте issue для багов

---

**Версия:** 1.0  
**Статус:** Production Ready ✅  
**Дата:** 3 января 2026
