# Запуск RAG Chat Platform

## Требования

- Docker и Docker Compose v2.
- 8 GB RAM минимум (16 GB рекомендуется).
- 15 GB свободного места (включая Docker-образы с встроенными моделями + bind-mount кеши).
- GGUF модель в директории `./models/`.
- Опционально: NVIDIA GPU + nvidia-container-toolkit для GPU режима.

## Быстрый старт

### 1. Заполните `.env`

```bash
cp .env.example .env
```

Обязательно поменяйте:

- `JWT_SECRET` — минимум 32 символа.
- `POSTGRES_PASSWORD` — пароль БД.
- `DATABASE_URL` — должен использовать тот же пароль.
- `ADMIN_EMAIL` и `ADMIN_PASSWORD` — учётка администратора (создаётся автоматически при первом старте).
- `LLAMA_MODELS_HOST_DIR` — **абсолютный** путь к директории `./models` на хосте (для динамических контейнеров моделей).

### 2. Скачайте LLM модель

```bash
mkdir -p models
# Скачайте GGUF модель, например Qwen3-4B:
# https://huggingface.co/Qwen/Qwen3-4B-GGUF
# Имя файла должно совпадать с GGUF_MODEL_FILE в .env
```

### 3. Запустите

```bash
# CPU режим
docker compose up -d --build

# GPU режим (NVIDIA)
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build
```

> Первая сборка занимает дольше: `ai-service` скачивает embedding (~1.1 GB) и reranker (~90 MB) модели в bind-mount директории `./models/embedding` и `./models/hf-cache`. Последующие запуски используют локальный кэш.

### 4. Дождитесь готовности

```bash
docker compose ps
# Все сервисы должны быть healthy/running
```

Порядок запуска контролируется автоматически через `depends_on` + healthchecks:
1. PostgreSQL, Qdrant.
2. llama-cpp, document-parser, vector-db.
3. ai-service (после llama-cpp).
4. backend (после Postgres healthy + остальных started).
5. frontend (после backend).

Backend дополнительно пробует подключиться к Docker daemon — если сокет смонтирован, в логах будет `✓ Docker daemon reachable` (нужно для деплоя finetuned моделей). При первом старте backend выполняет:

- AutoMigrate схемы БД (включая CHECK-constraints на роли).
- Seed base моделей из `./models/*.gguf` в таблицу `models`.
- Бутстрап админа из `ADMIN_EMAIL` / `ADMIN_PASSWORD`.
- Reconcile dynamic-контейнеров (синхронизация БД с фактическим состоянием Docker).

### 5. Войдите

Откройте http://localhost:3000. Логин — `ADMIN_EMAIL` / `ADMIN_PASSWORD` из `.env`. Регистрация обычных пользователей доступна на той же странице.

---

## Автономная работа (Offline)

После сборки Docker-образов платформа работает полностью автономно без доступа к интернету:

| Компонент | Где хранится |
|-----------|-------------|
| LLM модель (GGUF) | `./models/` (bind-mount в llama-cpp и динамические контейнеры) |
| Embedding (multilingual-e5-base) | `./models/embedding/` (bind-mount в `ai-service`) |
| Reranker (ms-marco-MiniLM) | `./models/hf-cache/` (bind-mount в `ai-service`) |
| NLTK данные | Встроены в Docker-образ `ai-service` |

Embedding и reranker скачиваются один раз при первой сборке. Чтобы пересобрать без удаления кеша, не трогайте `./models/embedding` и `./models/hf-cache`.

> Если используется gated модель — задайте `HF_TOKEN` в `.env`. Для дефолтного стека (multilingual-e5-base + ms-marco-MiniLM + Qwen3) токен не нужен.

---

## Доступ к сервисам

| Сервис | URL | Описание |
|--------|-----|----------|
| Frontend | http://localhost:3000 | React UI |
| Backend API | http://localhost:8080 | API Gateway |
| AI Service | http://localhost:8000 | RAG pipeline |
| llama.cpp API | http://localhost:8090 | LLM inference (default) |
| Qdrant Dashboard | http://localhost:6333/dashboard | Vector DB UI |

Динамические контейнеры моделей (finetuned) поднимаются на портах из пула `LLAMA_PORT_MIN..LLAMA_PORT_MAX` (по умолчанию 8100–8199) и доступны внутри Docker network под именем `chatbot-llama-ft-{short_id}`.

---

## Использование

### Регистрация и вход

1. Откройте http://localhost:3000.
2. Войдите как админ (`ADMIN_EMAIL` / `ADMIN_PASSWORD`) или зарегистрируйте обычного пользователя.
3. Дашборд показывает свои боты + те, к которым вас пригласили коллабораторами.

### Создание бота

1. Нажмите "Создать бота".
2. Заполните имя, описание, system prompt.
3. Выберите модель в dropdown (по умолчанию — дефолтный llama-cpp; ниже — свои finetuned, если развёрнуты).
4. Настройте параметры генерации (temperature, top_p, top_k, max tokens) — дефолты загружаются с сервера.
5. Загрузите документы (drag & drop, до `MAX_FILE_SIZE`).
6. Сохраните.

### Чат

- **Через дашборд** — нажмите "Открыть чат" на карточке бота. Слева — список диалогов, можно создать новый.
- **Публичный URL** — `/public/:bot_id`, работает без авторизации, поддерживает фидбек.
- **Остановка генерации** — кнопка "Стоп" во время стрима прерывает SSE и помечает сообщение как `cancelled`.

### Контекст диалога

Платформа передаёт модели последние `CHAT_CONTEXT_WINDOW` сообщений из текущего диалога (по умолчанию 5). Это значение настраивается глобально в `.env` или индивидуально на боте через поле `context_window`.

### Аналитика

На карточке бота — кнопка "Аналитика" → `/analytics/:bot_id`. Показывает:

- Количество диалогов и сообщений.
- Среднее время ответа.
- Соотношение thumbs-up / thumbs-down.
- График сообщений по дням (последние 30 дней).

### Реестр моделей (`/models`)

- Base модели сканируются из `./models/*.gguf` при старте.
- Дообученные модели появляются после fine-tune задачи (отдельный сервис в разработке).
- Кнопки `Deploy` / `Stop` поднимают/останавливают отдельный llama.cpp контейнер.
- `Delete` доступен только для finetuned моделей и только владельцу.

### Админ-панель (`/admin`)

Доступна только пользователям с `role='admin'`. В UI:

- Общая статистика платформы.
- Таблица пользователей (изменение роли, удаление).
- Таблица всех ботов (с возможностью удаления).

---

## Управление

### Логи

```bash
docker compose logs -f                  # Все сервисы
docker compose logs -f backend          # Backend
docker compose logs -f ai-service       # AI Service
docker compose logs -f llama-cpp        # Дефолтный LLM Server
docker logs -f chatbot-llama-ft-a1b2c3  # Динамический контейнер finetuned модели
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

Динамические контейнеры моделей не управляются `docker compose down` — для остановки используйте UI (Stop в `/models`) или вручную:

```bash
docker ps --filter "name=chatbot-llama-ft-" -q | xargs -r docker stop
docker ps -a --filter "name=chatbot-llama-ft-" -q | xargs -r docker rm
```

### Статус

```bash
docker compose ps                       # Статус контейнеров
docker stats                            # Использование ресурсов
curl http://localhost:8080/health        # Health backend
curl http://localhost:8000/health        # Health AI Service
curl http://localhost:8090/health        # Health llama.cpp
```

---

## GPU ускорение

### Требования

1. NVIDIA GPU с поддержкой CUDA.
2. Установленный [nvidia-container-toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html).
3. Docker Desktop с поддержкой WSL2 (для Windows).

### Запуск

```bash
docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build
```

GPU override (`docker-compose.gpu.yml`):

- Заменяет образ `llama-cpp` на `ghcr.io/ggml-org/llama.cpp:server-cuda`.
- Устанавливает `--n-gpu-layers -1` (все слои LLM на GPU).
- Включает `--cache-type-k q8_0 --cache-type-v q8_0` (квантизация KV-кеша).
- Резервирует NVIDIA GPU устройство.
- Включает `LLAMA_USE_GPU=true` для backend — динамические контейнеры моделей тоже создаются в GPU-режиме.

### Проверка

```bash
docker compose logs llama-cpp | grep -i "gpu\|layer\|CUDA\|offload"
# Ожидаемый вывод: "offloaded 33/33 layers to GPU"
```

### GPU не видно в Task Manager (Windows)

При использовании WSL2 + Docker Desktop нагрузка на GPU не отображается в Task Manager Windows. Вместо этого процесс `Vmmem` показывает повышенное CPU usage. Это нормальное поведение WSL2. Реальное использование GPU видно в логах llama-cpp (см. выше).

---

## Смена модели

1. Скачайте новую GGUF модель в `./models/`.
2. Обновите `.env`:

   ```bash
   GGUF_MODEL_FILE=new-model-name.gguf
   GGUF_MODEL_PATH=./models/new-model-name.gguf
   ```

3. Перезапустите backend + llama-cpp:

   ```bash
   docker compose restart llama-cpp backend
   ```

Backend при старте просканирует `./models/` и обновит запись base модели в таблице `models`.

> Убедитесь, что stop sequences (`GEN_STOP_SEQUENCES`) и chat-template-kwargs соответствуют формату новой модели.

---

## Бутстрап админа

При старте backend проверяет `ADMIN_EMAIL`:

- Если пользователя с таким email **нет** и задан `ADMIN_PASSWORD` — создаётся новый с `role='admin'`.
- Если пользователь **есть** — повышается до `admin` (если ещё не админ).
- Если `ADMIN_EMAIL` пустой — бутстрап пропускается.

Чтобы вручную сделать пользователя админом:

```bash
docker compose exec postgres psql -U chatbot -d chatbot \
  -c "UPDATE users SET role='admin' WHERE email='me@example.com';"
```

---

## Конфигурация

Все параметры в файле `.env`. Подробности: [CONFIGURATION.md](CONFIGURATION.md).

### Ключевые параметры

```bash
# LLM
GGUF_MODEL_FILE=Qwen3.5-4B-Q4_K_M.gguf
N_THREADS=6
N_CTX=32768
LLAMA_REASONING_BUDGET=0    # 0 = выключить thinking для Qwen3 / DeepSeek-R1

# Динамические контейнеры моделей
LLAMA_MODELS_HOST_DIR=C:/files/Developing/diplom/chat-bot-platfrom/models
LLAMA_PORT_MIN=8100
LLAMA_PORT_MAX=8199

# Админ
ADMIN_EMAIL=admin@local
ADMIN_PASSWORD=change-me-admin

# Чат
CHAT_CONTEXT_WINDOW=5

# Генерация (дефолты, переопределяются через UI)
GEN_MAX_NEW_TOKENS=8192
GEN_TEMPERATURE=0.7

# RAG
CHUNK_SIZE=1200
RAG_MAX_RESULTS=20
USE_RERANKER=true
USE_HYBRID_SEARCH=true
```

---

## Бенчмарк параллельных запросов

Скрипт `scripts/bench_parallel.py` запускает N параллельных SSE-запросов к публичному эндпоинту бота и замеряет время каждого + общее wall-clock. Зависит только от стандартной библиотеки Python.

```bash
python scripts/bench_parallel.py <bot_id> 4
# HOST=http://localhost:8080 python scripts/bench_parallel.py <bot_id> 8
```

Бот должен быть публично доступен (`/api/v1/chat/public/:bot_id` не требует токена).

---

## Troubleshooting

### llama.cpp не запускается

```bash
docker compose logs llama-cpp
ls -lh models/*.gguf
grep GGUF_MODEL_FILE .env
```

### AI Service долго стартует

При первом запуске после `docker compose build` embedding и reranker модели скачиваются в bind-mount директории `./models/embedding` и `./models/hf-cache` (~1.2 GB всего). Healthcheck ожидает до ~5 минут (retries=30 × 10s).

```bash
docker compose logs ai-service
# "Embedding model loaded, service ready" = готов
```

### Backend не подключается к сервисам

Backend ожидает healthcheck PostgreSQL + старт остальных зависимостей. Если один из сервисов не healthy/started, backend не запустится.

```bash
docker compose ps
```

### Эндпоинты `/api/v1/models/:id/deploy` возвращают 503

Backend не смог подключиться к Docker daemon. Убедитесь, что сокет смонтирован (в `docker-compose.yml` это уже сделано). При запуске backend в логах должно быть:

```
✓ Docker daemon reachable
```

Если видите `⚠️  Docker daemon unreachable` — проверьте, что Docker запущен и сокет доступен из контейнера.

### Деплой finetuned модели падает с "invalid bind mount" или "no such file"

Docker daemon не принимает относительные пути для bind-mount. `LLAMA_MODELS_HOST_DIR` должен быть **абсолютным** путём:

```bash
LLAMA_MODELS_HOST_DIR=C:/files/Developing/diplom/chat-bot-platfrom/models   # Windows
LLAMA_MODELS_HOST_DIR=/home/user/chat-bot-platfrom/models                    # Linux
```

В bash docker-compose подставит `${PWD}/models` автоматически. В PowerShell `$PWD` не экспортируется как env-переменная, поэтому задайте `LLAMA_MODELS_HOST_DIR` явно.

### Ошибка "connection refused" при загрузке документов

AI Service имеет healthcheck `interval=10s retries=30` (≈5 минут). Backend начнёт принимать загрузки только когда ai-service станет healthy.

### Не работает админ-панель

Проверьте роль пользователя:

```bash
docker compose exec postgres psql -U chatbot -d chatbot \
  -c "SELECT email, role FROM users;"
```

Если нужно — либо задайте `ADMIN_EMAIL` в `.env` и перезапустите backend, либо обновите вручную (см. раздел "Бутстрап админа").

### Медленная генерация

1. Используйте GPU: `docker compose -f docker-compose.yml -f docker-compose.gpu.yml up -d --build`.
2. Увеличьте `N_THREADS` в `.env`.
3. Используйте меньшую или более квантизированную модель.
4. Уменьшите `N_CTX`.
5. Уменьшите `CHAT_CONTEXT_WINDOW`, если не нужен длинный контекст диалога.

### Порты заняты

Измените порты в `.env`:

```bash
BACKEND_PORT=8081
FRONTEND_PORT=3001
LLAMA_CPP_PORT=8091
# Если конфликтует пул динамических моделей:
LLAMA_PORT_MIN=8200
LLAMA_PORT_MAX=8299
```

---

## Технологии

| Компонент | Технология |
|-----------|------------|
| Frontend | React 18, Vite, Nginx, Lucide Icons |
| Backend | Go 1.24, Fiber v2, GORM, Docker SDK |
| AI Service | Python 3.10, FastAPI, sentence-transformers, httpx |
| LLM Server | llama.cpp (OpenAI-compatible API) |
| Vector DB | Qdrant (gRPC) |
| Database | PostgreSQL 15 |
| Auth | JWT + bcrypt + RBAC |
| Streaming | Server-Sent Events (SSE) |
| Containerization | Docker Compose + dynamic per-model containers |
