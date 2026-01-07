"""
FastAPI приложение для работы с языковой моделью и RAG
"""
from fastapi import FastAPI
from contextlib import asynccontextmanager

from app.config.settings import settings
from app.services.rag_service import rag_service

# Используем только GGUF модели (CPU оптимизированные)
from app.services.model_service_gguf import model_service_gguf as model_service
print(f"🔧 Используется GGUF модель: {settings.gguf_model_path or 'NOT CONFIGURED'}")

# Инжектим model_service в routes
from app.api import routes
routes.model_service = model_service

from app.api.routes import router
from app.api.semantic_chunking import router as semantic_chunking_router


@asynccontextmanager
async def lifespan(app: FastAPI):
    """
    Lifespan события для предзагрузки моделей при старте приложения
    """
    # Startup: принудительно загружаем модель
    print("🚀 Предзагрузка модели...")
    model_service.load_model()
    rag_service.load_embedding_model()
    print("✅ Модель загружена и готова к работе")
    yield
    # Shutdown: здесь можно добавить код очистки, если нужно


# Создаем приложение FastAPI
app = FastAPI(
    title="AI Chat Bot Platform - Python Service",
    description="Микросервис для работы с языковой моделью и RAG",
    version="1.0.0",
    lifespan=lifespan
)

# Подключаем роутеры
app.include_router(router)
app.include_router(semantic_chunking_router)
