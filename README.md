RAG Chat Platform — обзор и инструкции

Репозиторий содержит микросервисную платформу для Retrieval-Augmented Generation (RAG): парсинг документов, индексация в векторную БД, вычисление эмбеддингов и генерация ответов через локальную LLM-службу.

Ниже — актуальное описание структуры, конфигурации и команд для развёртывания и разработки.

## Содержание

- Обзор сервисов
- Быстрый запуск (Docker Compose)
- Локальная разработка
- Важные файлы и конфигурация
- Порты и эндпоинты
- Проверка работоспособности

## Обзор сервисов

- Frontend: React + Vite (папка `frontend`) — статический UI.
- Backend Gateway: Go + Fiber (папка `services/backend`) — API и RAG pipeline.
- Document Parser: Go (папка `services/document-parser-service`) — извлечение текста.
- Vector DB Service: Go (папка `services/vector-db-service`) — интеграция с Qdrant.
- AI Service: Python + FastAPI (папка `services/python-ai`) — LLM inference и эмбеддинги.
- Qdrant: векторная база данных (контейнер `qdrant`).
- PostgreSQL: контейнер `postgres` для метаданных.

## Быстрый запуск (Docker Compose)

1. Отредактируйте файл `.env` в корне репозитория при необходимости.
2. Запустите все сервисы:

```bash
docker-compose up -d --build
```

Доступ по умолчанию:

- Frontend: http://localhost:3000
- Backend API: http://localhost:8080
- AI Service: http://localhost:8000
- Document Parser: http://localhost:8081
- Vector DB Service: http://localhost:8082
- Qdrant UI: http://localhost:6333/dashboard

Если порты конфликтуют, измените `.env` и `docker-compose.yml`.

## Локальная разработка и тестирование

Примеры ручного запуска сервисов (альтернатива Docker):

```bash
# Запуск Qdrant (локально)
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant

# Backend
cd services/backend
export $(cat ../../.env | xargs)  # при необходимости
go run main.go

# Document parser
cd ../document-parser-service
go run main.go

# Vector DB service
cd ../vector-db-service
go run main.go

# AI service
cd ../python-ai
./start.sh

# Frontend (dev)
cd ../../frontend
npm install
npm run dev
```

## Важные файлы и конфигурация

- `.env` — единый конфиг для всех сервисов.
- `docker-compose.yml` — описывает контейнеры и зависимости.
- `services/python-ai/models/` — место для GGUF моделей и кешей эмбеддингов (файлы больших размеров исключены из git).
- `services/backend/database/schema.sql` — SQL-схема для инициализации PostgreSQL.

## Основные порты (по умолчанию)

- Frontend: `3000`
- Backend: `8080`
- AI Service: `8000`
- Document Parser: `8081`
- Vector DB Service: `8082`
- Qdrant REST: `6333`, gRPC: `6334`
- Postgres: `5432`

## Проверка работоспособности

Примеры запросов для проверки:

```bash
# Health check backend
curl http://localhost:8080/health

# Health check AI service
curl http://localhost:8000/health

# Отправка файла на парсер (через backend)
curl -X POST http://localhost:8080/api/v1/documents/upload \
  -F "file=@document.pdf" \
  -F "client_id=test"

# Пример RAG-запроса (через backend)
curl -X POST http://localhost:8080/api/v1/chat/rag \
  -H "Content-Type: application/json" \
  -d '{"bot_id":"<bot_id>","query":"Что такое машинное обучение?"}'
```

## Заметки по разработке

- Большие файлы моделей (`*.gguf`) и бинарные веса исключены из репозитория — используйте LFS или внешнее хранение.
- Параметры генерации модели задаются переменными окружения (`GEN_*`), см. `CONFIGURATION.md`.
- Backend содержит реализацию аутентификации JWT, управление ботами и endpoints для загрузки документов.

## Документация проекта

- [PLATFORM_GUIDE.md](PLATFORM_GUIDE.md)
- [CONFIGURATION.md](CONFIGURATION.md)
- [DEPLOYMENT.md](DEPLOYMENT.md)

---

Если нужно, могу добавить пример `.env.local`, скрипты для проверки статуса контейнеров или краткий раздел с часто встречающимися ошибками и логами.
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
