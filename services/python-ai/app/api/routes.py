"""
API endpoints for AI Service - LLM Generation and Embeddings only
"""
from fastapi import APIRouter, HTTPException, Body
from fastapi.responses import StreamingResponse
import json
from typing import TYPE_CHECKING

from app.models.schemas import AskRequest
from app.services.rag_service import rag_service
from app.config.settings import settings

if TYPE_CHECKING:
    from app.services.model_service_gguf import ModelServiceGGUF

# model_service инжектируется динамически из main.py
model_service: "ModelServiceGGUF" = None  # type: ignore

router = APIRouter()
SERVICE_INFO = {
    "service": "ai-service",
    "version": "2.0.0",
    "model": settings.gguf_model_path or "NOT CONFIGURED",
    "embedding_model": settings.embedding_model_name or "NOT CONFIGURED",
    "capabilities": ["llm_generation", "embeddings"]
}


@router.get("/")
def root():
    """Health check endpoint with basic service info"""
    return {"status": "ok", **SERVICE_INFO}


@router.get("/health")
def health():
    """Детальная информация о здоровье сервиса"""
    return {
        "status": "healthy",
        **SERVICE_INFO,
        "generation_defaults": {
            "max_new_tokens": settings.generation_max_new_tokens,
            "temperature": settings.generation_temperature,
            "top_p": settings.generation_top_p,
            "top_k": settings.generation_top_k,
            "do_sample": settings.generation_do_sample
        }
    }



@router.post("/ask")
def ask(request: AskRequest):
    """
    Потоковая генерация ответа без RAG.
    Возвращает Server-Sent Events, где каждый токен отправляется по мере генерации.
    """

    def generate():
        try:
            for chunk in model_service.generate_response_stream(
                messages=request.messages,
                max_new_tokens=request.max_new_tokens if request.max_new_tokens is not None else settings.generation_max_new_tokens,
                temperature=request.temperature if request.temperature is not None else settings.generation_temperature,
                top_p=request.top_p if request.top_p is not None else settings.generation_top_p,
                top_k=request.top_k if request.top_k is not None else settings.generation_top_k,
                do_sample=request.do_sample if request.do_sample is not None else settings.generation_do_sample,
                behavior_instruction=request.behavior_instruction,
                system_prompt=request.system_prompt
            ):
                yield f"data: {json.dumps({'type': 'token', 'token': chunk})}\n\n"

            yield "data: {\"type\": \"done\"}\n\n"
        except FileNotFoundError as e:
            error_msg = str(e).replace('\n', ' ')
            yield f"data: {json.dumps({'type': 'error', 'error': error_msg})}\n\n"
            yield "data: {\"type\": \"done\"}\n\n"
        except Exception as e:
            yield f"data: {json.dumps({'type': 'error', 'error': f'Ошибка генерации: {str(e)}'})}\n\n"
            yield "data: {\"type\": \"done\"}\n\n"

    return StreamingResponse(
        generate(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
        }
    )


@router.post("/generate")
def generate_endpoint(
    messages: list[dict] = Body(..., embed=True),
    max_new_tokens: int | None = Body(None),
    temperature: float | None = Body(None),
    top_p: float | None = Body(None),
    top_k: int | None = Body(None),
    do_sample: bool | None = Body(None),
    behavior_instruction: str | None = Body(None),
    system_prompt: str | None = Body(None),
):
    """Синхронная генерация ответа без RAG (для gateway)."""
    try:
        text = model_service.generate_response(
            messages=messages,
            max_new_tokens=max_new_tokens,
            temperature=temperature,
            top_p=top_p,
            top_k=top_k,
            do_sample=do_sample,
            behavior_instruction=behavior_instruction,
            system_prompt=system_prompt,
        )
        return {"text": text}
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Ошибка генерации: {str(e)}")


@router.post("/embeddings")
def embeddings_endpoint(payload: dict = Body(...)):
    """Создание эмбеддингов для списка текстов."""
    texts = payload.get("texts")
    is_query = payload.get("is_query", False)  # Для e5 моделей: query vs passage
    
    if not isinstance(texts, list) or not texts:
        raise HTTPException(status_code=400, detail="texts is required and must be a non-empty list")
    try:
        vectors = rag_service.create_embeddings(texts, is_query=is_query)
        return {"embeddings": vectors}
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Ошибка при создании embeddings: {str(e)}")


@router.post("/advanced-search")
def advanced_search_endpoint(payload: dict = Body(...)):
    """
    Agentic RAG Pipeline v2:
    - Agentic Router (query classification)
    - Tiered retrieval (vector -> hybrid -> full_document_read)
    - Cosine re-ranking (replaces RRF)
    - Self-correction loop with best_results tracking
    - Enumeration/global prompts
    - Language mismatch detection
    - Pipeline trace for debugging
    """
    bot_id = payload.get("bot_id")
    query = payload.get("query")
    vector_results = payload.get("vector_results", [])
    all_documents = payload.get("all_documents", [])
    top_k = payload.get("top_k", 30)

    if not bot_id or not query:
        raise HTTPException(status_code=400, detail="bot_id and query are required")

    try:
        pipeline_result = rag_service.advanced_search(
            bot_id, query, vector_results, top_k, all_documents
        )

        return {
            "results": pipeline_result["results"],
            "compressed_context": pipeline_result["compressed_context"],
            "num_results": pipeline_result["num_results"],
            "trace": pipeline_result["trace"],
            "prompt_addition": pipeline_result["prompt_addition"],
            "router_decision": pipeline_result["router_decision"],
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Advanced search error: {str(e)}")


@router.post("/split-document")
def split_document_endpoint(payload: dict = Body(...)):
    """
    Семантическое разбиение документа на чанки.
    """
    text = payload.get("text")
    chunk_size = payload.get("chunk_size", 1200)
    overlap = payload.get("overlap", 200)
    
    if not text:
        raise HTTPException(status_code=400, detail="text is required")
    
    try:
        chunks = rag_service.split_text_semantic(text, chunk_size=chunk_size, overlap=overlap)
        return {
            "chunks": chunks,
            "num_chunks": len(chunks),
            "total_chars": len(text),
            "avg_chunk_size": len(text) // len(chunks) if chunks else 0
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Document split error: {str(e)}")


