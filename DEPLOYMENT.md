# 🚀 Запуск RAG Chat Platform

## Быстрый старт

```bash
# Клонируйте репозиторий
git clone <repo-url>
cd chat-bot-platfrom

# Запустите все сервисы
docker-compose up -d --build

# Дождитесь запуска (30-60 секунд)
docker-compose ps
```

## Доступ к сервисам

| Сервис | URL | Описание |
|--------|-----|----------|
| **Frontend** | http://localhost:3000 | React UI с настройками модели |
| **Backend API** | http://localhost:8080 | API Gateway |
| **AI Service** | http://localhost:8000 | LLM генерация |
| **Qdrant UI** | http://localhost:6333/dashboard | Векторная БД |
| **Document Parser** | http://localhost:8081 | Парсинг документов |
| **Vector DB Service** | http://localhost:8082 | Векторный поиск |

## Использование Frontend

1. Откройте http://localhost:3000
2. Сгенерируется автоматический Client ID (или введите свой)
3. Загрузите документ через drag & drop
4. Задавайте вопросы в чате

### Настройка параметров модели

Нажмите ⚙️ в правом верхнем углу для настройки:

- **Temperature** (0-2): Случайность ответов
- **Top P** (0-1): Nucleus sampling
- **Top K** (1-100): Ограничение выбора токенов
- **Max New Tokens** (32-2048): Длина ответа
- **Do Sample**: Включить/выключить sampling
- **System Prompt**: Роль и поведение AI

## Архитектура

```
┌──────────────┐
│   Frontend   │ :3000 (nginx + React)
│   (Docker)   │
└──────┬───────┘
       │
       ▼
┌──────────────────────────────┐
│  Backend Gateway (Go)        │ :8080
└───┬─────────┬──────────┬─────┘
    │         │          │
    ▼         ▼          ▼
┌─────────┐ ┌────────┐ ┌──────────┐
│Document │ │Vector  │ │AI Service│
│Parser   │ │DB Svc  │ │(Python)  │
│:8081    │ │:8082   │ │:8000     │
└─────────┘ └───┬────┘ └──────────┘
                │
                ▼
           ┌─────────┐
           │ Qdrant  │ :6333/:6334
           └─────────┘
```

## Управление

### Просмотр логов

```bash
# Все сервисы
docker-compose logs -f

# Конкретный сервис
docker-compose logs -f frontend
docker-compose logs -f backend
docker-compose logs -f ai-service
```

### Перезапуск сервисов

```bash
# Все сервисы
docker-compose restart

# Только frontend после изменений
docker-compose up -d --build frontend

# Только backend после изменений Go кода
docker-compose up -d --build backend
```

### Остановка

```bash
# Остановить все
docker-compose down

# Остановить и удалить volumes (БД очистится)
docker-compose down -v
```

## Интеграционное тестирование

```bash
# Запустить тесты
./test-integration.sh

# С кастомным URL
BASE_URL=http://localhost:8080 ./test-integration.sh
```

Тесты проверяют:
- ✅ Health check всех сервисов
- ✅ Загрузку документов
- ✅ Векторный поиск
- ✅ RAG генерацию с streaming

## Разработка

### Frontend (React)

```bash
cd frontend

# Dev режим (hot reload)
npm run dev

# Сборка
npm run build

# Preview production build
npm run preview
```

### Backend (Go)

```bash
cd services/backend

# Локальный запуск
go run main.go

# Сборка
go build -o backend main.go
```

### AI Service (Python)

```bash
cd services/python-ai

# Установка зависимостей
pip install -r requirements.txt

# Локальный запуск
./start.sh
```

## Конфигурация

Переменные окружения в `.env`:

```bash
# Модель
GGUF_MODEL_PATH=./models/qwen2.5-3b-instruct-q4_k_m.gguf
N_THREADS=6
N_CTX=4096

# Генерация по умолчанию
GEN_MAX_NEW_TOKENS=256
GEN_TEMPERATURE=0.7
GEN_TOP_P=0.9
GEN_TOP_K=50

# RAG
RAG_TOP_K_DOCS=3
RAG_MAX_DOC_CHARS=400

# Document parsing
CHUNK_SIZE=400
CHUNK_OVERLAP=80
```

## Troubleshooting

### Frontend не открывается

```bash
# Проверить статус
docker-compose ps frontend

# Посмотреть логи
docker-compose logs frontend

# Перезапустить
docker-compose restart frontend
```

### Backend ошибки

```bash
# Проверить что все микросервисы запущены
docker-compose ps

# Проверить логи
docker-compose logs backend

# Проверить health check
curl http://localhost:8080/health
```

### Модель не загружается

```bash
# Проверить наличие модели
ls -lh services/python-ai/models/*.gguf

# Проверить логи AI сервиса
docker-compose logs ai-service

# Дождаться загрузки (может занять 1-2 минуты)
docker-compose logs -f ai-service
```

### Порты заняты

Измените порты в `docker-compose.yml`:

```yaml
ports:
  - "3001:80"  # Frontend на :3001
  - "8090:8080"  # Backend на :8090
```

## Производительность

- **Модель**: Qwen2.5-3B Q4_K_M (2.0 GB)
- **Скорость**: 70-90 токенов/сек на CPU
- **Контекст**: 4096 токенов
- **Потоки**: 6 CPU threads

### Ускорение

Для увеличения производительности:

1. Увеличить `N_THREADS` в `.env`
2. Использовать GPU (требует CUDA)
3. Использовать меньшую модель

## Технологии

- **Frontend**: React 18, Vite, Lucide Icons
- **Backend**: Go, Fiber v2
- **AI**: Python, FastAPI, llama-cpp-python
- **Vector DB**: Qdrant
- **Embeddings**: sentence-transformers
- **Streaming**: Server-Sent Events (SSE)
- **Containerization**: Docker, Docker Compose

## Документация

- [Frontend README](frontend/README.md) - Детали React приложения
- [Backend README](services/backend/README.md) - API документация
- [AI Service README](services/python-ai/README.md) - LLM сервис
- [RAG Documentation](docs/RAG_DOCUMENTATION.md) - Полная RAG документация
