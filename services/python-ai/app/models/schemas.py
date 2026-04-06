"""
Pydantic модели для запросов и ответов
"""
from pydantic import BaseModel, Field
from typing import Optional

from app.config.settings import settings


class AskRequest(BaseModel):
    """Запрос для стандартной генерации ответа"""
    messages: list
    max_new_tokens: Optional[int] = Field(default=settings.generation_max_new_tokens, ge=1)
    temperature: Optional[float] = Field(default=settings.generation_temperature, ge=0.0, le=2.0)
    top_p: Optional[float] = Field(default=settings.generation_top_p, ge=0.0, le=1.0)
    top_k: Optional[int] = Field(default=settings.generation_top_k, ge=1)
    do_sample: Optional[bool] = Field(default=settings.generation_do_sample)
    behavior_instruction: Optional[str] = Field(default=None)
    system_prompt: Optional[str] = Field(default=None)


