# Plan: Комплексная доработка RAG Chat-Bot Platform

## Context

Платформа — микросервисная система для создания RAG-чатботов с документами. Стек: Go/Fiber (backend), Python/FastAPI (AI service), llama.cpp (LLM), React (frontend), PostgreSQL, Qdrant. 8 сервисов в Docker Compose.

Цель — реализовать набор фич для дипломной работы: улучшение чата (история, контекст, стоп), аналитика, файнтюнинг моделей, админка, тесты и др.

---

## Фаза 1: Ядро чата (первоочередное)

### 1A. Остановка генерации

**Проблема**: `ChatArea.jsx` уже имеет кнопку стоп и проп `onStopGeneration`, но `BotChat.jsx` и `PublicChat.jsx` его НЕ используют — стриминг нельзя остановить.

**Frontend**:
- `frontend/src/components/BotChat.jsx`: Добавить `AbortController` ref. При отправке создавать новый controller, передавать `signal` в `fetch()`. Подключить `onStopGeneration` → `controller.abort()`. При abort пометить сообщение как `cancelled: true, streaming: false`.
- `frontend/src/components/PublicChat.jsx`: Аналогично.
- Рефакторинг: оба компонента перевести на использование `<ChatArea>` вместо дублирования UI сообщений.

**Backend**:
- `services/backend/handlers/handlers.go` в `streamRAGResponse`: При ошибке записи в SSE writer — закрывать `resp.Body` досрочно, чтобы AI service тоже остановился.
- Python AI service: FastAPI `StreamingResponse` уже корректно обрабатывает disconnect клиента.

**Файлы**: `BotChat.jsx`, `PublicChat.jsx`, `ChatArea.jsx`, `handlers.go`

---

### 1B. История чатов (персистентность)

**Новые таблицы** в `services/backend/database/schema.sql`:

```sql
conversations (id UUID PK, bot_id UUID FK, user_id INT FK nullable, title VARCHAR, created_at, updated_at)
messages (id SERIAL PK, conversation_id UUID FK, role VARCHAR, content TEXT, metadata JSONB, created_at)
```

**Backend**:
- `services/backend/database/models.go`: Структуры `Conversation`, `Message`
- Новый `services/backend/database/conversation_repository.go`: CRUD для conversations + messages
- Новый `services/backend/handlers/conversation_handler.go`:
  - `POST /api/v1/conversations` — создать диалог
  - `GET /api/v1/bots/:id/conversations` — список диалогов бота
  - `GET /api/v1/conversations/:conv_id` — диалог с сообщениями
  - `DELETE /api/v1/conversations/:conv_id` — удалить диалог
- `services/backend/handlers/handlers.go`: В `RAGChat`/`PublicRAGChat` принять `conversation_id`, сохранять сообщения в БД. SSE протокол расширить: первое событие `{"type":"meta","conversation_id":"...","message_id":N}`.
- `services/backend/models/types.go`: Добавить `ConversationID` в `RAGChatRequest`
- `services/backend/main.go`: Регистрация новых роутов

**Frontend**:
- `frontend/src/components/BotChat.jsx`: Боковая панель со списком диалогов. При клике — загрузка сообщений. Кнопка "Новый диалог". Хранение `conversationId` в state. Отправка `conversation_id` в теле запроса. Парсинг `meta` SSE события.
- `frontend/src/api/client.js`: Новый `conversationsAPI` (list, get, delete, create)

**Файлы**: `schema.sql`, `models.go`, новый `conversation_repository.go`, новый `conversation_handler.go`, `handlers.go`, `main.go`, `BotChat.jsx`, `client.js`

---

### 1C. Контекст чата (последние N сообщений)

**Зависит от**: 1B

**Backend**:
- `services/backend/handlers/handlers.go`: Если `conversation_id` указан — загрузить последние N сообщений из БД через `conversationRepo.GetRecentMessages(conversationID, N)`. Формировать `Messages` как `[...history, {role:"user", content: query}]` вместо текущего одного сообщения.
- `services/backend/config/config.go`: Новый параметр `ContextWindowSize int`, env `CHAT_CONTEXT_WINDOW=10`
- `.env`: Добавить `CHAT_CONTEXT_WINDOW=10`

**AI Service**: Изменений не нужно — `model_service_gguf.py._build_messages()` уже принимает массив messages и передает в llama.cpp.

**Файлы**: `handlers.go`, `config/config.go`, `.env`

---

## Фаза 2: Вовлечение и качество

### 2A. Обратная связь (thumbs up/down)

**Зависит от**: 1B (нужен `message_id`)

**Новая таблица**:
```sql
message_feedback (id SERIAL PK, message_id INT FK, user_id INT FK nullable, rating SMALLINT CHECK(-1,1), created_at, UNIQUE(message_id, user_id))
```

**Backend**:
- `services/backend/database/models.go`: Структура `MessageFeedback`
- Расширить `conversation_repository.go`: `AddFeedback`, `GetFeedbackStats(botID)`
- `services/backend/handlers/conversation_handler.go`:
  - `POST /api/v1/messages/:message_id/feedback` — отправить оценку
  - `GET /api/v1/bots/:id/feedback/stats` — агрегированная статистика

**Frontend**:
- `frontend/src/components/ChatArea.jsx`: Кнопки ThumbsUp/ThumbsDown на каждом сообщении ассистента. Подсветка выбранной оценки.
- `frontend/src/api/client.js`: `feedbackAPI.submit(messageId, rating)`

**Файлы**: `schema.sql`, `models.go`, `conversation_repository.go`, `conversation_handler.go`, `ChatArea.jsx`, `client.js`

---

### 2B. Аналитика

**Зависит от**: 1B + 2A

**Backend**:
- Новый `services/backend/handlers/analytics_handler.go`:
  - `GET /api/v1/bots/:id/analytics` — total_conversations, total_messages, avg_response_time, feedback_summary, messages_per_day (30 дней)
- Опционально: добавить `response_time_ms` в metadata сообщений ассистента

**Frontend**:
- Новый `frontend/src/components/Analytics.jsx` + `Analytics.css`: Карточки метрик, график сообщений по дням (CSS bar chart или `recharts`), соотношение feedback
- `frontend/src/App.jsx`: Роут `/analytics/:botId`
- Кнопка "Аналитика" на карточке бота в Dashboard

**Файлы**: Новый `analytics_handler.go`, новый `Analytics.jsx`, `App.jsx`, `Dashboard.jsx`

---

### 2C. Пагинация

**Backend**:
- `services/backend/database/bot_repository.go`: `GetByOwnerID` принимает `page, limit`. Возвращает `total`.
- `services/backend/handlers/bot_handler.go`: Парсинг `?page=1&limit=20` в `GetMyBots`. Формат ответа: `{bots, total, page, limit}`
- Аналогично для документов и диалогов

**Frontend**:
- Новый `frontend/src/components/Pagination.jsx` — переиспользуемый компонент
- `Dashboard.jsx`: Пагинация под сеткой ботов
- `BotChat.jsx`: Пагинация списка диалогов

**Файлы**: `bot_repository.go`, `bot_handler.go`, новый `Pagination.jsx`, `Dashboard.jsx`

---

### 2D. Поиск ботов

**Backend**:
- `services/backend/database/bot_repository.go`: `SearchByOwnerID(ownerID, query, page, limit)` с `LOWER(name) LIKE %query%`
- `bot_handler.go`: Парсинг `?search=...`

**Frontend**:
- `Dashboard.jsx`: Поле поиска над сеткой ботов с debounce (300ms)

**Файлы**: `bot_repository.go`, `bot_handler.go`, `Dashboard.jsx`

---

## Фаза 3: Платформенные фичи

### ~~3A. Сброс пароля / верификация email~~ — ОТЛОЖЕНО

> **Статус**: Убрано из текущего плана. Нет доступа к SMTP-серверу.
> Можно вернуть позже — вся логика ниже готова к реализации, достаточно добавить Mailhog в docker-compose для локальной разработки.

---

### 3B. Совместный доступ к ботам

**Новая таблица**:
```sql
bot_collaborators (id SERIAL PK, bot_id UUID FK, user_id INT FK, role VARCHAR CHECK('editor','viewer'), created_at, UNIQUE(bot_id,user_id))
```

**Backend**:
- Новый `services/backend/database/collaborator_repository.go`
- `bot_handler.go`:
  - `POST /api/v1/bots/:id/collaborators` — пригласить по email
  - `DELETE /api/v1/bots/:id/collaborators/:user_id` — удалить
  - `GET /api/v1/bots/:id/collaborators` — список
  - `GetMyBots`: включить shared ботов
- Обновить авторизацию: viewer может чатить, editor — загружать документы и редактировать

**Frontend**: UI управления коллабораторами в `BotForm.jsx`

**Файлы**: Новый `collaborator_repository.go`, `bot_handler.go`, `models.go`, `BotForm.jsx`

---

### 3C. Админ-панель

**БД**: Alter users: `role VARCHAR DEFAULT 'user' CHECK('user','admin')`

**Backend**:
- Новый `services/backend/auth/admin_middleware.go`
- Новый `services/backend/handlers/admin_handler.go`:
  - `GET /api/v1/admin/users` — все пользователи
  - `GET /api/v1/admin/bots` — все боты
  - `GET /api/v1/admin/stats` — общая статистика
  - `DELETE /api/v1/admin/users/:id`, `PUT /api/v1/admin/users/:id/role`
  - `DELETE /api/v1/admin/bots/:id`

**Frontend**:
- Новый `AdminPanel.jsx` + CSS: Таблицы пользователей/ботов, карточки статистики
- `App.jsx`: Роут `/admin`, показывать только если `user.role === 'admin'`

**Файлы**: Новый `admin_middleware.go`, новый `admin_handler.go`, новый `AdminPanel.jsx`, `App.jsx`

---

## Фаза 4: Технический долг

### 4A. Structured Logging + Request Tracing

**Go**: Заменить все `log.Printf` на `log/slog` (JSON формат). Middleware в `main.go` генерирует `X-Request-ID` (UUID), сохраняет в Fiber locals, передает во все исходящие запросы через `service_client.go`.

**Python**: Заменить `print()` на `logging` модуль. Извлекать `X-Request-ID` из header, добавлять в контекст логов.

**Файлы**: Все Go файлы с `log.Printf`, `main.go`, `service_client.go`, `main.py`, `model_service_gguf.py`, `rag_service.py`

---

### 4B. Swagger / OpenAPI

**Go**: Добавить `swaggo/swag` + `swaggo/fiber-swagger`. Аннотации на все хендлеры. `swag init` → `docs/`. Роут `/swagger/*`.

**Python**: FastAPI уже генерирует — дополнить Pydantic модели описаниями полей, добавить `response_model` и `tags`.

**Файлы**: Все handler файлы в `services/backend/handlers/`, `routes.py`, `schemas.py`, `go.mod`

---

### 4C. Тесты

**Go**: Новые файлы `*_test.go` в `services/backend/handlers/`, `services/backend/database/`. Использовать `testing` + `httptest`. Тесты: auth, bot CRUD, ownership, chat.

**Python**: Новые файлы `services/python-ai/tests/test_routes.py`, `test_rag_service.py`. Использовать `pytest` + `httpx`. Мок llama.cpp.

**Frontend** (опционально): `vitest` + `@testing-library/react`.

---

## Фаза 5: Продвинутые фичи

### 5A. Fine-tuning моделей (самая сложная фича)

#### Ключевой принцип

Fine-tuning **отвязан от ботов**. Пользователь дообучает модель как самостоятельную сущность. Потом при создании/редактировании любого своего бота выбирает её из списка. Один контейнер дообученной модели может обслуживать несколько ботов одного владельца.

#### Архитектура

```
┌─────────────────────────────────────────────────────────┐
│  Страница "Мои модели" (отдельная от ботов)             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │ Qwen3-4B │  │ MyModel1 │  │ MyModel2 │              │
│  │  (base)  │  │(finetuned│  │(finetuned│              │
│  │ общий    │  │ owner=me)│  │ owner=me)│              │
│  └──────────┘  └────┬─────┘  └────┬─────┘              │
└─────────────────────┼─────────────┼─────────────────────┘
                      │             │
┌─────────────────────┼─────────────┼─────────────────────┐
│  Страница "Мои боты"│             │                     │
│  ┌─────┐ ┌─────┐   │  ┌─────┐   │                     │
│  │Bot A│ │Bot B│───┘  │Bot C│───┘                     │
│  │base │ │model1│      │model2│                         │
│  └──┬──┘ └──┬──┘      └──┬──┘                          │
└─────┼───────┼─────────────┼─────────────────────────────┘
      │       │             │
      ▼       ▼             ▼
 llama-cpp  llama-ft-1    llama-ft-2
 (default)  (контейнер)   (контейнер)
```

- Базовые модели → общий llama-cpp контейнер (из docker-compose)
- Каждая finetuned модель → свой llama-cpp контейнер (создаётся при deploy)
- Несколько ботов могут ссылаться на одну finetuned модель
- Finetuned модели видны ТОЛЬКО владельцу

#### Методы дообучения (на выбор пользователя)

| Метод | Описание | VRAM | Скорость | Когда использовать |
|-------|----------|------|----------|--------------------|
| **QLoRA** | 4-bit квантизация + LoRA адаптеры | ~6-8 GB | Средняя | По умолчанию. Лучший баланс качества и ресурсов |
| **LoRA** | Low-Rank Adaptation, 16-bit | ~12-16 GB | Средняя | Когда есть много VRAM и нужно макс. качество |
| **Prompt Tuning** | Обучаются только soft-prompt embeddings, модель заморожена | ~4-6 GB | Быстрая | Мало данных (<100 примеров), быстрый результат |
| **Adapter Tuning** | Вставка маленьких adapter-слоёв между слоями модели | ~6-8 GB | Быстрая | Компромисс: быстрее LoRA, качество чуть ниже |

**Библиотеки**:
- QLoRA/LoRA: `unsloth` + `peft` + `trl` (SFTTrainer)
- Prompt Tuning: `peft` (PromptTuningConfig)
- Adapter Tuning: `peft` (AdaptionPromptConfig) или `adapters` библиотека

#### Шаблон датасета для дообучения

Пользователь загружает файл в формате JSONL. На фронте — кнопка "Скачать шаблон" с примером.

**Файл `finetune_template.jsonl`** (поставляется с платформой):
```jsonl
{"instruction": "Что такое машинное обучение?", "input": "", "output": "Машинное обучение — это подраздел искусственного интеллекта, который позволяет системам автоматически учиться и улучшаться на основе опыта без явного программирования."}
{"instruction": "Переведи на английский", "input": "Привет, как дела?", "output": "Hello, how are you?"}
{"instruction": "Сократи текст до одного предложения", "input": "Нейронные сети — это вычислительные модели, вдохновлённые биологическими нейронными сетями мозга. Они состоят из слоёв узлов, каждый из которых выполняет простые вычисления.", "output": "Нейронные сети — вычислительные модели, имитирующие работу мозга через слои взаимосвязанных узлов."}
```

**Поля**:
- `instruction` (обязательно) — задание / вопрос пользователя
- `input` (опционально, может быть `""`) — дополнительный контекст / входной текст
- `output` (обязательно) — ожидаемый ответ модели

**Валидация при загрузке**:
- Каждая строка — валидный JSON
- Обязательные поля `instruction` и `output` присутствуют и не пусты
- Минимум 10 примеров для обучения
- Максимум 50 000 примеров
- Размер файла до 100 MB

#### Владение моделями (приватность)

**Принцип**: Дообученная модель принадлежит тому, кто запустил обучение. Никто другой не может её видеть или использовать.

В таблице `models` поле `owner_id` — FK на users. При запросе `GET /api/v1/models` backend возвращает:
- Все модели с `type = 'base'` (доступны всем)
- Только свои модели с `type = 'finetuned'` (WHERE `owner_id = current_user_id`)

При назначении модели боту (`bot.model_id`) — backend проверяет:
- Модель base → разрешено всем
- Модель finetuned → `model.owner_id == current_user_id`, иначе 403

#### Новые таблицы

```sql
-- Реестр моделей
CREATE TABLE IF NOT EXISTS models (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id INTEGER REFERENCES users(id) ON DELETE SET NULL, -- NULL для base моделей
    name VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL CHECK (type IN ('base', 'finetuned')),
    file_path VARCHAR(500) NOT NULL,
    base_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
    gguf_path VARCHAR(500),
    container_name VARCHAR(255),
    container_port INTEGER,
    endpoint_url VARCHAR(500),
    status VARCHAR(20) DEFAULT 'ready' CHECK (status IN ('ready', 'training', 'converting', 'deploying', 'running', 'stopped', 'error')),
    parameters JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_models_owner_id ON models(owner_id);

-- Задания на дообучение (НЕ привязаны к боту — модель дообучивается отдельно)
CREATE TABLE IF NOT EXISTS finetune_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    base_model_id UUID NOT NULL REFERENCES models(id),
    output_model_id UUID REFERENCES models(id), -- заполняется после успешного завершения
    method VARCHAR(30) NOT NULL DEFAULT 'qlora' CHECK (method IN ('lora', 'qlora', 'prompt_tuning', 'adapter_tuning')),
    name VARCHAR(255) NOT NULL, -- пользовательское имя для результирующей модели
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'validating', 'training', 'converting', 'deploying', 'completed', 'failed', 'cancelled')),
    progress DECIMAL(5,2) DEFAULT 0,
    hyperparameters JSONB DEFAULT '{}',
    error_message TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_finetune_jobs_user_id ON finetune_jobs(user_id);

-- Датасеты для дообучения
CREATE TABLE IF NOT EXISTS finetune_datasets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES finetune_jobs(id) ON DELETE CASCADE,
    filename VARCHAR(255) NOT NULL,
    format VARCHAR(20) NOT NULL DEFAULT 'jsonl' CHECK (format IN ('jsonl', 'json', 'csv')),
    file_path VARCHAR(500) NOT NULL,
    file_size BIGINT NOT NULL,
    num_examples INTEGER DEFAULT 0,
    validated BOOLEAN DEFAULT false,
    validation_errors JSONB DEFAULT '[]',
    uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Обновить bots
ALTER TABLE bots ADD COLUMN model_id UUID REFERENCES models(id) ON DELETE SET NULL;
```

#### Новый микросервис: `services/finetune-service/`

```
services/finetune-service/
  Dockerfile
  requirements.txt
  app/
    main.py           -- FastAPI, polling задач
    config.py
    api/routes.py     -- POST /jobs, GET /jobs/:id/status, POST /jobs/:id/cancel, GET /methods
    services/
      trainer.py      -- Диспетчер: выбирает стратегию по method
      strategies/
        qlora.py      -- QLoRA через unsloth
        lora.py       -- LoRA через unsloth
        prompt_tuning.py  -- Prompt Tuning через peft
        adapter_tuning.py -- Adapter Tuning через peft
      converter.py    -- Merge + конвертация в GGUF
      validator.py    -- Валидация датасетов (формат, поля, лимиты)
    models/schemas.py
    templates/
      finetune_template.jsonl  -- Шаблон для скачивания
```

**Пайплайн обучения** (`trainer.py` — общий для всех методов):
1. Загрузить и валидировать датасет (JSONL: `{instruction, input, output}`)
2. Загрузить базовую модель
3. Применить выбранный метод:
   - **QLoRA**: `unsloth` + 4-bit quantization, rank=16, alpha=32, SFTTrainer
   - **LoRA**: `unsloth` + 16-bit, rank=16, alpha=32, SFTTrainer
   - **Prompt Tuning**: `peft.PromptTuningConfig`, num_virtual_tokens=20, заморозить всю модель
   - **Adapter Tuning**: `peft` adapter layers между transformer-слоями
4. Обучить с progress callback (записывает прогресс для polling)
5. Merge/save адаптеры
6. Конвертировать в GGUF (`llama.cpp/convert_hf_to_gguf.py` или `gguf-py`)
7. Сохранить в shared volume `/models/finetuned/{model_id}/`
8. Уведомить backend через callback

**Эндпоинт описания методов** `GET /methods`:
```json
[
  {"id": "qlora", "name": "QLoRA", "description": "4-bit квантизация + LoRA. Лучший баланс.", "vram_gb": "6-8", "speed": "medium"},
  {"id": "lora", "name": "LoRA", "description": "Low-Rank Adaptation, 16-bit.", "vram_gb": "12-16", "speed": "medium"},
  {"id": "prompt_tuning", "name": "Prompt Tuning", "description": "Обучение soft-промптов. Самый быстрый.", "vram_gb": "4-6", "speed": "fast"},
  {"id": "adapter_tuning", "name": "Adapter Tuning", "description": "Adapter-слои между слоями модели.", "vram_gb": "6-8", "speed": "fast"}
]
```

**Библиотеки**: `unsloth`, `transformers`, `peft`, `trl`, `datasets`, `gguf`, `adapters`

#### Backend изменения

- `services/backend/database/models.go`: Структуры `Model` (с `OwnerID`), `FinetuneJob`, `FinetuneDataset`. Добавить `ModelID *string` в `Bot`.
- Новый `services/backend/database/model_repository.go`:
  - `GetAvailableModels(userID)` — base модели + finetuned модели WHERE `owner_id = userID`
  - `GetModelByID(modelID)` — с проверкой доступа
  - `CheckModelAccess(modelID, userID)` — base → true, finetuned → owner_id == userID
- Новый `services/backend/handlers/model_handler.go` — управление моделями (отдельно от ботов):
  - `GET /api/v1/models` — доступные модели (base для всех + свои finetuned)
  - `GET /api/v1/models/:id` — детали модели (с проверкой доступа)
  - `POST /api/v1/models/:id/deploy` — деплой (поднять контейнер). Проверка: `owner_id == userID`
  - `POST /api/v1/models/:id/stop` — остановить контейнер. Проверка: `owner_id == userID`
  - `DELETE /api/v1/models/:id` — удалить finetuned модель и контейнер. Проверка: `owner_id == userID`
- Новый `services/backend/handlers/finetune_handler.go` — управление дообучением:
  - `GET /api/v1/finetune/methods` — список методов дообучения
  - `GET /api/v1/finetune/template` — скачать шаблон JSONL файла
  - `POST /api/v1/finetune/jobs` — запуск (имя модели, base_model_id, датасет, метод, гиперпараметры)
  - `GET /api/v1/finetune/jobs` — мои задания на дообучение
  - `GET /api/v1/finetune/jobs/:id/status` — статус + прогресс
  - `POST /api/v1/finetune/jobs/:id/cancel` — отмена
- `services/backend/handlers/bot_handler.go`: При `PUT /api/v1/bots/:id` с `model_id` — проверять `CheckModelAccess(modelID, userID)`. Если модель finetuned и owner != текущий пользователь → 403.
- Новый `services/backend/services/container_manager.go`: Docker API через `github.com/docker/docker/client`
  - `StartLlamaContainer(modelID, ggufPath)` → создает контейнер llama.cpp с порт из пула 8100-8199
  - `StopLlamaContainer(containerName)`
  - `GetContainerStatus(containerName)`
  - Контейнеры в той же Docker network

#### Роутинг запросов к модели бота (как определяется нужный контейнер)

Полная цепочка от выбора модели до генерации ответа:

**Шаг 1 — Deploy модели (создание контейнера)**

Пользователь на странице "Мои модели" нажимает "Развернуть". `container_manager.go`:
1. Выбирает свободный порт из пула 8100-8199 (сканирует `models` таблицу на занятые порты)
2. Создаёт Docker контейнер через Docker API:
   - Image: `ghcr.io/ggml-org/llama.cpp:server`
   - Name: `chatbot-llama-ft-{model_id_short}` (например `chatbot-llama-ft-a1b2c3`)
   - Network: та же Docker network что и остальные сервисы (`chat-bot-platfrom_default`)
   - Volume: `./models:/models`
   - Command: `--model /models/finetuned/{model_id}.gguf --host 0.0.0.0 --port 8080 --ctx-size N_CTX --threads N_THREADS`
3. Записывает в БД:
   ```
   models.container_name = "chatbot-llama-ft-a1b2c3"
   models.container_port = 8103
   models.endpoint_url   = "http://chatbot-llama-ft-a1b2c3:8080"
   models.status          = "running"
   ```
   Контейнер доступен по имени внутри Docker network — внешний порт не нужен.

**Шаг 2 — Назначение модели боту**

При создании/редактировании бота (`POST /api/v1/bots` или `PUT /api/v1/bots/:id`):
- Пользователь выбирает модель из dropdown (base + свои finetuned)
- Backend проверяет `CheckModelAccess(modelID, userID)`:
  - `type = 'base'` → разрешено всем
  - `type = 'finetuned'` → только если `model.owner_id == userID`, иначе 403
- Сохраняет `bot.model_id = modelID`

**Шаг 3 — Генерация ответа в чате (роутинг)**

В `services/backend/handlers/handlers.go`, в `RAGChat` / `PublicRAGChat` / `streamRAGResponse`:

```go
// 1. Загружаем бота из БД
bot := botRepo.GetByID(botID)

// 2. Определяем LLM endpoint
llmEndpoint := ""  // пустая строка = использовать дефолтный llama-cpp

if bot.ModelID != nil {
    model := modelRepo.GetByID(*bot.ModelID)
    if model.Type == "finetuned" {
        if model.Status != "running" {
            // Ошибка: "Модель не запущена. Разверните её на странице Мои модели."
            return error 503
        }
        llmEndpoint = model.EndpointURL  // "http://chatbot-llama-ft-a1b2c3:8080"
    }
    // Если model.Type == "base" → llmEndpoint остаётся "" → AI service
    // использует дефолтный LLAMA_SERVER_URL
}

// 3. Передаём endpoint в запрос к AI service
genReq.LLMEndpoint = llmEndpoint
resp := h.client.StreamGeneration(h.cfg.Services.AIURL, genReq)
```

**Шаг 4 — AI service подставляет endpoint**

В `services/python-ai/app/models/schemas.py`:
```python
class AskRequest(BaseModel):
    # ... существующие поля ...
    llm_endpoint: Optional[str] = None  # НОВОЕ: override URL для llama.cpp
```

В `services/python-ai/app/services/model_service_gguf.py`:
```python
async def generate_response_stream(self, messages, params, llm_endpoint=None):
    # Если передан llm_endpoint → шлём туда, иначе → дефолтный LLAMA_SERVER_URL
    server_url = llm_endpoint or self.server_url
    url = f"{server_url}/v1/chat/completions"
    # ... далее как обычно ...
```

**Итого**: `bot.model_id` → `models.endpoint_url` → передаётся в AI service → AI service шлёт запрос в нужный контейнер llama.cpp.

**Несколько ботов — один контейнер**: Если три бота одного пользователя используют одну finetuned модель, все три шлют запросы в один и тот же контейнер `chatbot-llama-ft-a1b2c3`.

#### Docker изменения

В `docker-compose.yml` добавить:
```yaml
finetune-service:
  build:
    context: ./services/finetune-service
    dockerfile: Dockerfile
  container_name: chatbot-finetune
  restart: unless-stopped
  volumes:
    - ./models:/models
    - ./finetune-data:/finetune-data
  environment:
    BACKEND_CALLBACK_URL: http://backend:8080
    MODELS_DIR: /models
    DATASETS_DIR: /finetune-data
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: all
            capabilities: [gpu]
  ports:
    - "${FINETUNE_PORT:-8095}:8000"
```

Обновить `backend` сервис:
```yaml
backend:
  volumes:
    - /var/run/docker.sock:/var/run/docker.sock  # Docker API
    - ./models:/models
  environment:
    FINETUNE_SERVICE_URL: http://finetune-service:8000
    LLAMA_BASE_PORT: 8100
    LLAMA_IMAGE: ghcr.io/ggml-org/llama.cpp:server
```

Динамические контейнеры llama.cpp создаются runtime через Docker API (НЕ в docker-compose).

#### Frontend изменения

**Новая страница "Мои модели"** (отдельная от ботов):
- Новый `frontend/src/components/ModelsPage.jsx` + `ModelsPage.css`:
  - Список моделей: base (общие) + finetuned (только свои)
  - Карточка модели: имя, тип, статус (ready/running/training/stopped), base модель, дата
  - Действия: Deploy / Stop / Delete (только для finetuned)
  - Кнопка "Дообучить новую модель" → открывает форму дообучения
- Новый `frontend/src/components/FineTuneForm.jsx`:
  - Имя для новой модели
  - Выбор базовой модели (dropdown)
  - Выбор метода из `GET /finetune/methods` (карточки с описа��ием, VRAM, скоростью)
  - Загрузка датасета (JSONL) + кнопка "Скачать шаблон" → `GET /finetune/template`
  - Гиперпараметры (зависят от метода):
    - QLoRA/LoRA: epochs, learning rate, rank, alpha
    - Prompt Tuning: num_virtual_tokens, epochs, learning rate
    - Adapter Tuning: adapter_size, epochs, learning rate
  - Кнопка "Начать обучение"
- Новый `frontend/src/components/FineTuneStatus.jsx`: Прогресс-бар, логи, статус задачи (polling каждые 5с)
- `frontend/src/App.jsx`: Роут `/models`, ссылка в навигации

**Изменения в BotForm (выбор модели при создании бота)**:
- `frontend/src/components/BotForm.jsx`:
  - Новая секция "Модель" — dropdown из `GET /models` (base + свои finetuned, чужие НЕ видны)
  - Разделение в dropdown: "Базовые модели" / "Мои дообученные модели"
  - Если выбрана finetuned модель со статусом `stopped` — показать предупреждение "Модель не запущена"
- Новый `frontend/src/components/ModelSelector.jsx`: Переиспользуемый компонент выбора модели

**API клиент**:
- `frontend/src/api/client.js`:
  - `modelsAPI` (list, get, deploy, stop, delete)
  - `finetuneAPI` (startJob, listJobs, getStatus, cancel, listMethods, downloadTemplate)

#### Seed Data

При старте backend сканирует `/models/*.gguf` и регистрирует как base модели. Дефолтный llama.cpp контейнер → модель с `endpoint_url = http://llama-cpp:8080`.

---

## Порядок реализации и зависимости

```
Фаза 1 (неделя 1-2):
  1A: Stop Generation ──────── (независимо, 1-2 дня)
  1B: Chat History ─────────── (независимо, 3-4 дня)
  1C: Context Window ───────── (зависит от 1B, 1 день)

Фаза 2 (неделя 3-4):
  2A: Feedback ─────────────── (зависит от 1B, 2 дня)
  2B: Analytics ────────────── (зависит от 1B+2A, 2-3 дня)
  2C: Pagination ───────────── (независимо, 1-2 дня)
  2D: Search Bots ──────────── (независимо, 0.5 дня)
  2E: Export Chat ──────────── (зависит от 1B, 1-2 дня)

Фаза 3 (неделя 5-6):
  3B: Collaborative Access ─── (независимо, 2-3 дня)
  3C: Admin Panel ──────────── (независимо, 2-3 дня)

Фаза 4 (неделя 7):
  4A: Structured Logging ───── (независимо, 2 дня)
  4B: Swagger ──────────────── (после стабилизации API, 1-2 дня)
  4C: Tests ────────────────── (параллельно, 3-5 дней)

Фаза 5 (неделя 8-11):
  5A: Fine-tuning ──────────── (самая сложная фича)
    Неделя 1: Схема БД, реестр моделей, seed data, шаблон датасета
    Неделя 2: Finetune service (4 метода обучения + валидация)
    Неделя 3: Container orchestration (Docker API) + владение моделями
    Неделя 4: Frontend UI (выбор модели/метода, прогресс, деплой)
    Неделя 5-6: E2E тесты, GPU тесты, документация
```

---

## Риски и митигации

1. **Docker socket в backend контейнере** — ограничить Docker API usage конкретным image/network. Вариант: sidecar container orchestrator.
2. **Порты для динамических контейнеров** — пул 8100-8199, трекинг в БД, cleanup orphaned containers.
3. **GPU память для файнтюна** — QLoRA 4-bit на 4B модели ~6-8 GB VRAM. Очередь задач, только одна тренировка одновременно.
4. **Конвертация GGUF** — может не работать для всех архитектур. Тестировать с конкретными base моделями. Фиксировать версии библиотек.
5. **Рост таблицы messages** — индексы на `(conversation_id, created_at)`, политика архивации/очистки.

---

## Верификация

Для каждой фичи:
1. **Unit-тесты** (Go `*_test.go`, Python `pytest`)
2. **Ручное тестирование** через UI: создать бота → загрузить документ → начать чат → проверить фичу
3. **Docker**: `docker compose up -d --build`, проверить health всех сервисов
4. **Fine-tuning E2E**: подготовить тестовый JSONL (10-20 примеров), запустить обучение, дождаться деплоя, отправить запрос к дообученной модели
5. **Стоп генерации**: начать длинный ответ → нажать стоп → убедиться что стрим остановился и сообщение пометилось как cancelled
6. **Контекст**: отправить 3-4 сообщения → убедиться что бот помнит предыдущие
