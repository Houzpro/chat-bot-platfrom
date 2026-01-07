"""
Сервис для работы с GGUF моделями через llama.cpp
Оптимизировано для CPU - в 10-20 раз быстрее PyTorch
"""
import threading
from pathlib import Path
from typing import Iterator, Optional, Any, List, Dict

try:
    from llama_cpp import Llama
    LLAMA_CPP_AVAILABLE = True
except ImportError:
    LLAMA_CPP_AVAILABLE = False
    Llama = None  # type: ignore

from app.config.settings import settings


class ModelServiceGGUF:
    """Сервис для работы с GGUF моделями (CPU оптимизированный)"""
    
    def __init__(self):
        self._model: Optional[Any] = None
        self._lock = threading.Lock()
        self._stop_sequences = settings.generation_stop_sequences
        
        if not LLAMA_CPP_AVAILABLE:
            print("⚠️  llama-cpp-python не установлен. Установите: pip install llama-cpp-python")
    
    def load_model(self) -> Any:
        """
        Загрузка GGUF модели (singleton)
        
        Returns:
            Llama model instance или None если не установлен llama-cpp-python
        """
        if not LLAMA_CPP_AVAILABLE:
            raise ImportError("llama-cpp-python не установлен")
        
        with self._lock:
            if self._model is not None:
                return self._model
            
            model_path = settings.gguf_model_path
            if model_path is None:
                raise ValueError("GGUF_MODEL_PATH is not configured")
            
            # Проверка существования модели
            if not Path(model_path).exists():
                raise FileNotFoundError(
                    f"GGUF модель не найдена: {model_path}\n"
                    f"Скачайте модель:\n"
                    f"cd models && curl -L -o qwen2.5-3b-instruct-q4_k_m.gguf https://huggingface.co/Qwen/Qwen2.5-3B-Instruct-GGUF/resolve/main/qwen2.5-3b-instruct-q4_k_m.gguf"
                )
            
            print(f"📦 Загрузка GGUF модели из {model_path}...")
            print(f"⚡ CPU оптимизация: {settings.n_threads} потоков")
            
            # Загрузка модели
            from llama_cpp import Llama as LlamaCpp
            self._model = LlamaCpp(
                model_path=model_path,
                n_ctx=settings.n_ctx,
                n_threads=settings.n_threads,
                n_gpu_layers=0,  # Для CPU всегда 0
                verbose=False,
                n_batch=512,
                use_mlock=True  # Предотвращает swapping
            )
            
            print(f"✅ GGUF модель загружена (контекст: {settings.n_ctx}, потоки: {settings.n_threads})")
            return self._model
    
    def generate_response(
        self,
        messages: List[Dict[str, str]],
        max_new_tokens: Optional[int] = None,
        temperature: Optional[float] = None,
        top_p: Optional[float] = None,
        top_k: Optional[int] = None,
        do_sample: Optional[bool] = None,
        behavior_instruction: Optional[str] = None,
        system_prompt: Optional[str] = None
    ) -> str:
        """
        Генерация ответа (синхронная)
        
        Args:
            messages: История сообщений
            max_new_tokens: Максимальное количество токенов
            temperature: Температура генерации
            top_p: Nucleus sampling
            top_k: Top-k sampling
            do_sample: Использовать sampling
            behavior_instruction: Дополнительная инструкция для модели
            system_prompt: Системный промпт
        
        Returns:
            Сгенерированный ответ
        """
        model = self.load_model()
        
        prompt = self._format_messages(messages, behavior_instruction, system_prompt)
        gen_kwargs = self._prepare_generation_kwargs(
            max_new_tokens=max_new_tokens,
            temperature=temperature,
            top_p=top_p,
            top_k=top_k,
            do_sample=do_sample
        )

        output = model(prompt, stream=False, stop=self._stop_sequences, **gen_kwargs)
        
        return output['choices'][0]['text'].strip()
    
    def generate_response_stream(
        self,
        messages: List[Dict[str, str]],
        max_new_tokens: Optional[int] = None,
        temperature: Optional[float] = None,
        top_p: Optional[float] = None,
        top_k: Optional[int] = None,
        do_sample: Optional[bool] = None,
        behavior_instruction: Optional[str] = None,
        system_prompt: Optional[str] = None
    ) -> Iterator[str]:
        """
        Потоковая генерация ответа
        
        Args:
            messages: История сообщений
            max_new_tokens: Максимальное количество токенов
            temperature: Температура генерации
            top_p: Nucleus sampling
            top_k: Top-k sampling
            do_sample: Использовать sampling
            behavior_instruction: Дополнительная инструкция для модели
            system_prompt: Системный промпт
        
        Yields:
            Токены ответа
        """
        model = self.load_model()
        
        # Debug logging
        if system_prompt:
            print(f"[AI Service] System prompt length: {len(system_prompt)} chars")
            print(f"[AI Service] System prompt preview: {system_prompt[:500]}...")
        print(f"[AI Service] User query: {messages[0].get('content', '') if messages else 'N/A'}")
        
        prompt = self._format_messages(messages, behavior_instruction, system_prompt)
        gen_kwargs = self._prepare_generation_kwargs(
            max_new_tokens=max_new_tokens,
            temperature=temperature,
            top_p=top_p,
            top_k=top_k,
            do_sample=do_sample
        )

        started = False  # Флаг для отслеживания начала реального контента
        for output in model(prompt, stream=True, stop=self._stop_sequences, **gen_kwargs):
            token = output['choices'][0]['text']
            # Удаляем открывающие/закрывающие теги, контент оставляем
            token = token.replace('<think>', '').replace('</think>', '')
            token = token.replace('<thought>', '').replace('</thought>', '')
            
            # Пропускаем пустые токены и переносы строк в начале ответа
            if not started:
                if not token or token.isspace():
                    continue
                started = True
            
            # Отдаем все токены после начала (важно для потоковой генерации)
            yield token
    
    def _format_messages(
        self,
        messages: List[Dict[str, str]],
        behavior_instruction: Optional[str] = None,
        system_prompt: Optional[str] = None
    ) -> str:
        """
        Форматирование сообщений в Qwen chat формат
        
        Args:
            messages: Список сообщений
            behavior_instruction: Дополнительная инструкция
            system_prompt: Системный промпт
        
        Returns:
            Отформатированный prompt
        """
        # Системный промпт
        if system_prompt:
            # User provided custom prompt - combine with base
            system_message = f"{system_prompt}\n\n{settings.generation_system_base_prompt}"
        else:
            # Use default prompts
            system_message = f"{settings.generation_user_prompt}\n\n{settings.generation_system_base_prompt}"
            if behavior_instruction:
                system_message = f"{behavior_instruction}\n{system_message}"
        
        # Qwen формат: <|im_start|>role\ncontent<|im_end|>
        prompt = f"<|im_start|>system\n{system_message}<|im_end|>\n"
        
        for msg in messages:
            role = msg.get("role", "user")
            content = msg.get("content", "")
            prompt += f"<|im_start|>{role}\n{content}<|im_end|>\n"
        
        prompt += "<|im_start|>assistant\n"
        return prompt

    def _prepare_generation_kwargs(
        self,
        max_new_tokens: Optional[int],
        temperature: Optional[float],
        top_p: Optional[float],
        top_k: Optional[int],
        do_sample: Optional[bool]
    ) -> Dict[str, Any]:
        """Приводит параметры генерации к значениям из settings при отсутствии входных."""
        max_tokens = settings.generation_max_new_tokens if max_new_tokens is None else max_new_tokens
        sample_flag = settings.generation_do_sample if do_sample is None else do_sample
        temperature_val = settings.generation_temperature if temperature is None else temperature
        top_p_val = settings.generation_top_p if top_p is None else top_p
        top_k_val = settings.generation_top_k if top_k is None else top_k

        return {
            "max_tokens": max_tokens,
            "temperature": 0.0 if not sample_flag else temperature_val,
            "top_p": 1.0 if not sample_flag else top_p_val,
            "top_k": -1 if not sample_flag else top_k_val
        }


# Singleton instance
model_service_gguf = ModelServiceGGUF()
