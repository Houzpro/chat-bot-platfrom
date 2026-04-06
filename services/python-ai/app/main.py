"""
FastAPI приложение для RAG pipeline: эмбеддинги, чанкинг, advanced search.
LLM генерация делегируется llama.cpp server через OpenAI-совместимый API.
"""
from fastapi import FastAPI
from contextlib import asynccontextmanager

from app.config.settings import settings
from app.services.rag_service import rag_service
from app.services.model_service_gguf import model_service_gguf as model_service

# Инжектим model_service в routes
from app.api import routes
routes.model_service = model_service


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan события для предзагрузки моделей при старте"""
    print(f"llama.cpp server URL: {settings.llama_server_url}")
    model_service.load_model()
    rag_service.load_embedding_model()
    print("Embedding model loaded, service ready")
    yield


app = FastAPI(
    title="AI Chat Bot Platform - Python Service",
    description="Микросервис для RAG pipeline: эмбеддинги, чанкинг, advanced search, LLM генерация (через llama.cpp server)",
    version="2.0.0",
    lifespan=lifespan,
)

app.include_router(routes.router)
