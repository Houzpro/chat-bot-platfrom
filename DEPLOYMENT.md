# Запуск RAG Chat Platform

## Требования

- Docker и Docker Compose v2
- 8 GB RAM минимум (16 GB рекомендуется)
- 15 GB свободного места (включая Docker-образы с встроенными моделями)
- GGUF модель в директории `models/`
- Опционально: NVIDIA GPU + nvidia-container-toolkit

## Быстрый старт

### 1. Скачайте LLM модель

```bash
mkdir -p models
# Скачайте GGUF модель, например Qwen3-4B:
# https://huggingface.co/Qwen/Qwen3-4B-GGUF
# Поместите файл qwen3-4b-q4_k_m.gguf в models/
```

### 2. Запустите

```bash
# CPU режим
docker compose up -d --build

# GPU режим (NVIDIA)
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build
```

> Первая сборка занимает дольше: Docker-образ `ai-service` скачивает embedding (~1.1 GB) и reranker (~90 MB) модели и встраивает их в образ. Последующие запуски используют кэшированный образ.

### 3. Дождитесь готовности

```bash
docker compose ps
# Все сервисы должны быть healthy/running
```

Порядок запуска контролируется автоматически:
1. PostgreSQL, Qdrant стартуют первыми
2. llama-cpp, document-parser, vector-db — после баз данных
3. ai-service — после llama-cpp (healthcheck)
4. backend — после всех сервисов (healthcheck)
5. frontend — после backend

AI Service имеет `start_period: 120s` для загрузки embedding модели из встроенного кэша.

### 4. Откройте в браузере

http://localhost:3000

---

## Автономная работа (Offline)

После сборки Docker-образов платформа работает полностью автономно без доступа к интернету:

| Компонент | Где хранится |
|-----------|-------------|
| LLM модель (GGUF) | `./models/` (монтируется в llama-cpp) |
| Embedding модель (multilingual-e5-base) | Встроена в Docker-образ ai-service (`/app/models/embedding`) |
| Reranker модель (ms-marco-MiniLM) | Встроена в Docker-образ ai-service (`/app/models/transformers`) |
| NLTK данные (punkt_tab) | Встроены в Docker-образ ai-service |

Модели embedding и reranker скачиваются один раз при `docker compose build` и сохраняются в Docker-образе. Docker volume для моделей ai-service не используется, чтобы не перекрывать встроенные файлы.

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
3. Настройте параметры генерации (temperature, top_p, etc.) — дефолты загружаются с сервера
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

Параметры сохраняются для каждого бота отдельно.

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
docker compose up -d --build ai-service # Пересборка одного сервиса
```

### Остановка

```bash
docker compose down                     # Остановить
docker compose down -v                  # Остановить + удалить данные (PostgreSQL, Qdrant)
```

### Статус

```bash
docker compose ps                       # Статус контейнеров
docker stats                            # Использование ресурсов
curl http://localhost:8080/health        # Health check backend
curl http://localhost:8000/health        # Health check AI Service
curl http://localhost:8090/health        # Health check llama.cpp
```

---

## GPU ускорение

### Требования

1. NVIDIA GPU с поддержкой CUDA
2. Установленный [nvidia-container-toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html)
3. Docker Desktop с включённой поддержкой WSL2 (для Windows)

### Запуск

```bash
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build
```

GPU override (`docker-compose.gpu.yml`) автоматически:
- Заменяет образ на `ghcr.io/ggml-org/llama.cpp:server-cuda`
- Устанавливает `--n-gpu-layers -1` (все слои LLM на GPU)
- Резервирует NVIDIA GPU устройство

### Проверка

```bash
docker compose logs llama-cpp | grep -i "gpu\|layer\|CUDA\|offload"
# Ожидаемый вывод: "offloaded 33/33 layers to GPU"
```

### GPU не видно в Task Manager (Windows)

При использовании WSL2 + Docker Desktop нагрузка на GPU не отображается в Task Manager Windows. Вместо этого процесс `Vmmem` показывает повышенное CPU usage. Это нормальное поведение WSL2. Реальное использование GPU видно в логах llama-cpp (см. выше).

---

## Смена модели

1. Скачайте новую GGUF модель в `models/`
2. Обновите `.env`:

```bash
GGUF_MODEL_FILE=new-model-name.gguf
GGUF_MODEL_PATH=./models/new-model-name.gguf
```

3. Перезапустите llama-cpp:

```bash
docker compose restart llama-cpp
```

> Убедитесь, что stop sequences (`GEN_STOP_SEQUENCES`) соответствуют формату новой модели.

---

## Конфигурация

Все параметры в файле `.env`. Подробности: [CONFIGURATION.md](CONFIGURATION.md)

### Ключевые параметры

```bash
# LLM модель
GGUF_MODEL_FILE=qwen3-4b-q4_k_m.gguf
N_THREADS=6
N_CTX=32768

# Генерация (дефолты, переопределяются через UI)
GEN_MAX_NEW_TOKENS=8192
GEN_TEMPERATURE=0.75

# RAG
CHUNK_SIZE=1200
RAG_MAX_RESULTS=60
USE_RERANKER=true
USE_HYBRID_SEARCH=true
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

### AI Service долго стартует

При первом запуске после `docker compose build` embedding модель загружается из встроенного кэша (~10-20 секунд). Healthcheck ожидает до 120 секунд.

```bash
docker compose logs ai-service
# "Embedding model loaded, service ready" = готов
```

### Backend не подключается к сервисам

Backend ожидает healthcheck от всех зависимостей. Если один из сервисов не healthy, backend не запустится.

```bash
docker compose ps
# Все зависимости должны быть healthy
```

### Ошибка "connection refused" при загрузке документов

AI Service имеет `start_period: 120s` и `retries: 30`. Backend начнёт работу только когда ai-service станет healthy. Если ошибка возникает — проверьте логи ai-service и убедитесь, что он прошёл healthcheck.

### Медленная генерация

1. Используйте GPU: `docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build`
2. Увеличьте `N_THREADS` в `.env`
3. Используйте меньшую или более квантизированную модель
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

| Компонент | Технология |
|-----------|------------|
| Frontend | React 18, Vite, Nginx, Lucide Icons |
| Backend | Go 1.24, Fiber v2 |
| AI Service | Python 3.10, FastAPI, sentence-transformers, httpx |
| LLM Server | llama.cpp (OpenAI-compatible API) |
| Vector DB | Qdrant (gRPC) |
| Database | PostgreSQL 15 |
| Auth | JWT (bcrypt) |
| Streaming | Server-Sent Events (SSE) |
| Containerization | Docker Compose |
