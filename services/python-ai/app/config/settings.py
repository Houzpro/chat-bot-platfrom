"""
Конфигурация AI Service - только LLM генерация и эмбеддинги
Все значения берутся из переменных окружения без хардкод значений по умолчанию
"""
from pydantic_settings import BaseSettings
import os
import sys


class Settings(BaseSettings):
    """Настройки приложения"""
    
    # llama.cpp server URL (модель хостится в отдельном контейнере)
    llama_server_url: str = os.getenv("LLAMA_SERVER_URL", "http://llama-cpp:8080")

    # GGUF модель (информационные поля для health endpoint)
    gguf_model_path: str | None = os.getenv("GGUF_MODEL_PATH")

    # Настройки генерации ответов
    generation_max_new_tokens: int = int(os.getenv("GEN_MAX_NEW_TOKENS", "0"))
    generation_temperature: float = float(os.getenv("GEN_TEMPERATURE", "0"))
    generation_top_p: float = float(os.getenv("GEN_TOP_P", "0"))
    generation_top_k: int = int(os.getenv("GEN_TOP_K", "0"))
    generation_do_sample: bool = os.getenv("GEN_DO_SAMPLE", "").lower() in {"true", "1", "yes", "on"}
    
    # System prompts
    generation_system_base_prompt: str = os.getenv("GEN_SYSTEM_BASE_PROMPT", "")
    generation_user_prompt: str = os.getenv("GEN_USER_PROMPT", "")
    generation_stop_sequences: list[str] = [
        s.strip() for s in os.getenv(
            "GEN_STOP_SEQUENCES",
            "<|im_end|>,<|endoftext|>"
        ).split(",") if s.strip()
    ]
    
    # Embeddings для RAG
    embedding_model_name: str | None = os.getenv("EMBEDDING_MODEL_NAME")
    embedding_cache_folder: str | None = os.getenv("EMBEDDING_CACHE_FOLDER")
    
    # Reranker для точного переранжирования
    reranker_model_name: str = os.getenv("RERANKER_MODEL_NAME", "cross-encoder/ms-marco-MiniLM-L-6-v2")
    use_reranker: bool = os.getenv("USE_RERANKER", "true").lower() in {"true", "1", "yes", "on"}
    
    # Relevance thresholds
    relevance_escalation_threshold: float = float(os.getenv("RELEVANCE_ESCALATION_THRESHOLD", "2.0"))
    embedding_similarity_autopass: float = float(os.getenv("EMBEDDING_SIMILARITY_AUTOPASS", "0.65"))
    
    class Config:
        env_file = ".env"
    
    def validate_settings(self):
        """Валидация конфигурации при старте"""
        errors = []

        if not self.llama_server_url:
            errors.append("LLAMA_SERVER_URL is required")
        if self.generation_max_new_tokens <= 0:
            errors.append("GEN_MAX_NEW_TOKENS must be positive")
        if self.generation_temperature < 0:
            errors.append("GEN_TEMPERATURE must be non-negative")
        if self.generation_top_p <= 0 or self.generation_top_p > 1:
            errors.append("GEN_TOP_P must be between 0 and 1")
        if self.generation_top_k <= 0:
            errors.append("GEN_TOP_K must be positive")
        if not self.embedding_model_name:
            errors.append("EMBEDDING_MODEL_NAME is required")
        if not self.embedding_cache_folder:
            errors.append("EMBEDDING_CACHE_FOLDER is required")

        if errors:
            print("Configuration validation failed:", file=sys.stderr)
            for error in errors:
                print(f"  - {error}", file=sys.stderr)
            sys.exit(1)


settings = Settings()
settings.validate_settings()
