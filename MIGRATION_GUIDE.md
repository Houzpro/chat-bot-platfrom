# 🔄 Миграция на Multi-User архитектуру

## Что изменилось

Платформа была обновлена для поддержки:

1. **Аутентификация и регистрация пользователей**
2. **Управление ботами** - каждый пользователь может создавать и настраивать своих ботов
3. **Приватные и публичные чаты** - публичный доступ к боту по URL, но настройки только для владельца
4. **Bot-based collections** - документы в векторной БД теперь привязаны к `bot_id`, а не к `client_id`

---

## Архитектура изменений

### Новые компоненты

1. **PostgreSQL** - хранение пользователей и ботов
2. **JWT Authentication** - токены для авторизации
3. **Bot Management API** - CRUD операции для ботов
4. **Auth Handlers** - регистрация/логин

### Изменения в существующих сервисах

#### Backend Service
- ✅ Добавлена БД и репозитории (users, bots)
- ✅ JWT middleware для защиты endpoint'ов
- ✅ Новые эндпоинты для auth и bot management
- ✅ Обновлены зависимости (go.mod)

#### Vector DB Service
- ✅ Изменено: `client_id` → `bot_id` во всех моделях
- ✅ Коллекции теперь называются `bot_{uuid}` вместо `client_{id}`
- ✅ Обновлены API endpoints

#### Frontend (требуется реализация)
- ⏳ Страницы login/register
- ⏳ Dashboard с списком ботов
- ⏳ Форма создания/редактирования бота
- ⏳ Публичный URL для чата с ботом

---

## База данных

### Схема

```sql
-- users: пользователи системы
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW()
);

-- bots: боты пользователей
CREATE TABLE bots (
    id UUID PRIMARY KEY,
    owner_id INTEGER REFERENCES users(id),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    -- Параметры генерации
    temperature DECIMAL(3,2) DEFAULT 0.75,
    top_p DECIMAL(3,2) DEFAULT 0.92,
    top_k INTEGER DEFAULT 40,
    max_new_tokens INTEGER DEFAULT 512,
    do_sample BOOLEAN DEFAULT true,
    system_prompt TEXT,
    -- RAG настройки
    rag_top_k INTEGER DEFAULT 3,
    chunk_size INTEGER DEFAULT 2500,
    chunk_overlap INTEGER DEFAULT 500,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW()
);

-- bot_documents: метаданные загруженных документов
CREATE TABLE bot_documents (
    id SERIAL PRIMARY KEY,
    bot_id UUID REFERENCES bots(id),
    filename VARCHAR(255),
    file_type VARCHAR(50),
    file_size BIGINT,
    chunks_count INTEGER,
    uploaded_at TIMESTAMP DEFAULT NOW()
);
```

### Миграция выполняется автоматически

При первом запуске PostgreSQL выполнит `schema.sql` из `docker-entrypoint-initdb.d`.

---

## API Endpoints

### Публичные (без авторизации)

```bash
# Health check
GET /health

# Регистрация
POST /api/v1/auth/register
{
  "email": "user@example.com",
  "password": "securepassword",
  "name": "John Doe"
}

# Логин
POST /api/v1/auth/login
{
  "email": "user@example.com",
  "password": "securepassword"
}

# Получить информацию о боте (публично)
GET /api/v1/bots/:bot_id

# Публичный чат с ботом (без авторизации)
POST /api/v1/chat/public/:bot_id
{
  "query": "Что такое машинное обучение?"
}
```

### Защищенные (требуется JWT токен)

**Headers:** `Authorization: Bearer <token>`

```bash
# Получить информацию о текущем пользователе
GET /api/v1/auth/me

# Создать бота
POST /api/v1/bots
{
  "name": "My AI Assistant",
  "description": "Помощник по машинному обучению",
  "temperature": 0.7,
  "system_prompt": "You are an expert in ML"
}

# Получить список своих ботов
GET /api/v1/bots

# Обновить бота
PUT /api/v1/bots/:bot_id
{
  "name": "Updated name",
  "temperature": 0.8
}

# Удалить бота
DELETE /api/v1/bots/:bot_id

# Загрузить документ для бота
POST /api/v1/bots/:bot_id/documents/upload
Form-data: file=document.pdf

# Получить документы бота
GET /api/v1/bots/:bot_id/documents

# RAG чат с ботом
POST /api/v1/chat/rag
{
  "bot_id": "uuid-here",
  "query": "Вопрос"
}
```

---

## Запуск обновленной платформы

### 1. Обновить .env

Добавьте новые переменные:

```bash
# PostgreSQL
POSTGRES_PASSWORD=secure_password_change_me

# JWT Authentication
JWT_SECRET=your_very_secure_jwt_secret_key_here
```

### 2. Обновить docker-compose.yml

Замените старый `docker-compose.yml` на `docker-compose-new.yml`:

```bash
mv docker-compose.yml docker-compose-old.yml
mv docker-compose-new.yml docker-compose.yml
```

### 3. Обновить backend main.go

```bash
cd services/backend
mv main.go main_old.go
mv main_new.go main.go
```

### 4. Обновить зависимости backend

```bash
cd services/backend
go mod tidy
go mod download
```

### 5. Пересобрать и запустить

```bash
docker-compose down -v  # Остановить старые контейнеры
docker-compose up -d --build
```

### 6. Проверить логи

```bash
docker-compose logs -f backend
docker-compose logs -f postgres
```

---

## Тестирование

### 1. Регистрация пользователя

```bash
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123",
    "name": "Test User"
  }'
```

Ответ:
```json
{
  "token": "eyJhbGc...",
  "user": {
    "id": 1,
    "email": "test@example.com",
    "name": "Test User"
  }
}
```

### 2. Создать бота

```bash
TOKEN="<token из предыдущего шага>"

curl -X POST http://localhost:8080/api/v1/bots \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "ML Assistant",
    "description": "Помощник по машинному обучению",
    "temperature": 0.7,
    "system_prompt": "You are an expert in machine learning."
  }'
```

Ответ:
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "owner_id": 1,
  "name": "ML Assistant",
  "temperature": 0.7,
  ...
}
```

### 3. Загрузить документ

```bash
BOT_ID="<id из предыдущего шага>"

curl -X POST http://localhost:8080/api/v1/bots/$BOT_ID/documents/upload \
  -H "Authorization: Bearer $TOKEN" \
  -F "file=@document.pdf"
```

### 4. Публичный чат (без токена)

```bash
curl -X POST http://localhost:8080/api/v1/chat/public/$BOT_ID \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Что такое машинное обучение?"
  }'
```

---

## Frontend изменения (TODO)

Необходимо реализовать:

### 1. Страницы аутентификации

- `/login` - форма входа
- `/register` - форма регистрации

### 2. Dashboard (`/dashboard`)

```jsx
// Компоненты:
- BotList - список ботов пользователя
- BotCard - карточка бота с кнопками Edit/Delete/Open
- CreateBotButton - кнопка создания нового бота
```

### 3. Форма создания/редактирования бота

```jsx
// /dashboard/bots/new
// /dashboard/bots/:id/edit

- Поля: name, description
- Параметры модели: temperature, top_p, top_k, etc.
- System prompt
- RAG настройки: top_k, chunk_size
```

### 4. Страница бота (`/bots/:id`)

```jsx
// Компоненты:
- BotInfo - информация о боте
- DocumentUpload - загрузка документов
- ChatArea - чат с ботом
```

### 5. Публичный чат (`/chat/:bot_id`)

```jsx
// Минималистичный интерфейс только с чатом
// Без кнопок настроек и загрузки документов
```

---

## Безопасность

### Реализовано

✅ JWT токены с истечением срока (24h)  
✅ Bcrypt хеширование паролей  
✅ Middleware для проверки авторизации  
✅ Проверка прав владельца бота  
✅ Rate limiting на уровне API Gateway  

### Рекомендации для продакшн

- 🔒 Использовать HTTPS
- 🔐 Сменить `JWT_SECRET` на случайный сложный ключ
- 🔑 Настроить firewall для PostgreSQL
- 📊 Добавить логирование попыток авторизации
- ⏰ Настроить автоматическую очистку expired токенов

---

## Миграция данных (если были старые данные)

Если у вас были документы в старых коллекциях `client_*`, их нужно перенести:

1. Экспортировать данные из старых коллекций
2. Создать ботов через API
3. Загрузить документы для каждого бота

**Скрипт миграции будет предоставлен отдельно.**

---

## Troubleshooting

### PostgreSQL не запускается

```bash
docker-compose logs postgres
# Проверить права доступа к volume
docker volume inspect chat-bot-platfrom_postgres_data
```

### Backend не подключается к БД

```bash
# Проверить DATABASE_URL в .env
# Убедиться что postgres healthy
docker-compose ps
```

### JWT токены не работают

```bash
# Проверить JWT_SECRET в .env
# Убедиться что токен передается в headers:
# Authorization: Bearer <token>
```

---

## Контакты

При возникновении проблем создавайте issue в репозитории.
