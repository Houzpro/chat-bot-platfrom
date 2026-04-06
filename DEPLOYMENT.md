# Запуск RAG Chat Platform

## Требования

- Docker и Docker Compose v2
- 8 GB RAM минимум (16 GB рекомендуется)
- 10 GB свободного места
- GGUF модель в директории `models/`
- Опционально: NVIDIA GPU + nvidia-container-toolkit

## Быстрый старт

### 1. Скачайте модель

```bash
mkdir -p models
# Скачайте GGUF модель, например:
# https://huggingface.co/Qwen/Qwen3-4B-GGUF
# Поместите файл в models/qwen3-4b-q4_k_m.gguf
```

### 2. Запустите

```bash
# CPU режим
docker compose up -d --build

# GPU режим (NVIDIA)
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build
```

### 3. Дождитесь готовности (1-2 минуты)

```bash
docker compose ps
# Все сервисы должны быть healthy/running
```

### 4. Откройте в браузере

http://localhost:3000

---

## Доступ к сервисам

| Сервис | URL | Описание |
|--------|-----|----------|
| Frontend | http://localhost:3000 | React UI |
| Backend API | http://localhost:8080 | API Gateway |
| AI Service | http://localhost:8000 | RAG pipeline |
| llama.cpp API | http://localhost:8090 | LLM inference |
| Qdrant Dashboard | http://localhost:6333/dashboard | Vector DB UI |

---

## Использование

### Регистрация и вход

1. Откройте http://localhost:3000
2. Зарегистрируйте аккаунт (email + пароль)
3. Войдите в систему

### Создание бота

1. Нажмите "Create Bot"
2. Укажите имя, описание, system prompt
3. Настройте параметры генерации (temperature, top_p, etc.)
4. Загрузите документы (drag & drop)
5. Сохраните

### Чат

- **Через Dashboard** — нажмите "Open Chat" на карточке бота
- **Публичный URL** — скопируйте ссылку и поделитесь (работает без авторизации)

### Настройка параметров модели

В чате нажмите иконку настроек для изменения:
- Temperature (0-2)
- Top P (0-1)
- Top K (1-100)
- Max New Tokens
- System Prompt

---

## Управление

### Логи

```bash
docker compose logs -f                  # Все сервисы
docker compose logs -f backend          # Backend
docker compose logs -f ai-service       # AI Service
docker compose logs -f llama-cpp        # LLM Server
```

### Перезапуск

```bash
docker compose restart                  # Все
docker compose restart ai-service       # Один сервис
docker compose up -d --build frontend   # Пересборка
```

### Остановка

```bash
docker compose down                     # Остановить
docker compose down -v                  # Остановить + удалить данные
```

### Статус

```bash
docker compose ps                       # Статус контейнеров
docker stats                            # Использование ресурсов
curl http://localhost:8080/health        # Health check
```

---

## GPU ускорение

### Установка

1. Установите [nvidia-container-toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html)
2. Запустите с GPU override:

```bash
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build
```

GPU override автоматически:
- Использует образ `ghcr.io/ggml-org/llama.cpp:server-cuda`
- Устанавливает `N_GPU_LAYERS=-1` (все слои на GPU)
- Резервирует NVIDIA GPU устройство

### Проверка

```bash
docker compose logs llama-cpp | grep -i gpu
# Должно показать загрузку слоев на GPU
```

---

## Конфигурация

Все параметры в файле `.env`. Подробности: [CONFIGURATION.md](CONFIGURATION.md)

### Ключевые параметры

```bash
# Модель
GGUF_MODEL_FILE=qwen3-4b-q4_k_m.gguf
N_THREADS=6
N_CTX=32768

# Генерация
GEN_MAX_NEW_TOKENS=8192
GEN_TEMPERATURE=0.75

# RAG
CHUNK_SIZE=1200
RAG_MAX_RESULTS=60
```

---

## Troubleshooting

### llama.cpp не запускается

```bash
docker compose logs llama-cpp
# Проверьте наличие модели:
ls -lh models/*.gguf
# Проверьте имя файла в .env:
grep GGUF_MODEL_FILE .env
```

### AI Service не подключается к llama.cpp

```bash
docker compose logs ai-service
# llama.cpp должен быть healthy:
docker compose ps llama-cpp
# Проверьте health:
curl http://localhost:8090/health
```

### Backend ошибки

```bash
docker compose logs backend
curl http://localhost:8080/health
# Проверьте что все зависимости запущены:
docker compose ps
```

### Медленная генерация

1. Используйте GPU: `docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d`
2. Увеличьте `N_THREADS` в `.env`
3. Используйте меньшую модель
4. Уменьшите `N_CTX`

### Порты заняты

Измените порты в `.env`:

```bash
BACKEND_PORT=8081
FRONTEND_PORT=3001
LLAMA_CPP_PORT=8091
```

---

## Технологии

- **Frontend**: React 18, Vite, Nginx, Lucide Icons
- **Backend**: Go 1.24, Fiber v2
- **AI Service**: Python 3.10, FastAPI, sentence-transformers
- **LLM Server**: llama.cpp (OpenAI-compatible API)
- **Vector DB**: Qdrant (gRPC)
- **Database**: PostgreSQL 15
- **Auth**: JWT (bcrypt)
- **Streaming**: Server-Sent Events (SSE)
- **Containerization**: Docker Compose
