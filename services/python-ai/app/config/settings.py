"""
Конфигурация AI Service - только LLM генерация и эмбеддинги
Все значения берутся из переменных окружения без хардкод значений по умолчанию
"""
from pydantic_settings import BaseSettings
import os
import sys


class Settings(BaseSettings):
    """Настройки приложения"""
    
    # GGUF модель (CPU оптимизированная)
    gguf_model_path: str | None = os.getenv("GGUF_MODEL_PATH")
    n_ctx: int = int(os.getenv("N_CTX", "0"))
    n_threads: int = int(os.getenv("N_THREADS", "0"))

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
        "<|im_end|>",
        "<|endoftext|>",
        "\n\nUser:",
        "\n\nHuman:"
    ]
    
    # Embeddings для RAG
    embedding_model_name: str | None = os.getenv("EMBEDDING_MODEL_NAME")
    embedding_cache_folder: str | None = os.getenv("EMBEDDING_CACHE_FOLDER")
    
    # Reranker для точного переранжирования
    reranker_model_name: str = os.getenv("RERANKER_MODEL_NAME", "cross-encoder/ms-marco-MiniLM-L-6-v2")
    use_reranker: bool = os.getenv("USE_RERANKER", "true").lower() in {"true", "1", "yes", "on"}
    
    # Hybrid Search (Vector + BM25)
    use_hybrid_search: bool = os.getenv("USE_HYBRID_SEARCH", "true").lower() in {"true", "1", "yes", "on"}
    
    # Query Expansion (DEPRECATED - убрано из universal approach)
    use_query_expansion: bool = os.getenv("USE_QUERY_EXPANSION", "false").lower() in {"true", "1", "yes", "on"}
    query_expansion_count: int = int(os.getenv("QUERY_EXPANSION_COUNT", "2"))
    
    # Contextual Compression (DEPRECATED - используем полный контекст)
    use_contextual_compression: bool = os.getenv("USE_CONTEXTUAL_COMPRESSION", "false").lower() in {"true", "1", "yes", "on"}
    
    class Config:
        env_file = ".env"
    
    def validate_settings(self):
        """Валидация конфигурации при старте"""
        errors = []
        
        if not self.gguf_model_path:
            errors.append("GGUF_MODEL_PATH is required")
        if self.n_ctx <= 0:
            errors.append("N_CTX must be positive")
        if self.n_threads <= 0:
            errors.append("N_THREADS must be positive")
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
            print("❌ Configuration validation failed:", file=sys.stderr)
            for error in errors:
                print(f"  - {error}", file=sys.stderr)
            sys.exit(1)


settings = Settings()
settings.validate_settings()
