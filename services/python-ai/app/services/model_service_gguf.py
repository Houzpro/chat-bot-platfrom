"""
Сервис для работы с LLM через llama.cpp server (OpenAI-совместимый API).
Модель хостится в отдельном Docker контейнере (ghcr.io/ggml-org/llama.cpp:server).
"""
import httpx
from typing import Iterator, Optional, List, Dict

from app.config.settings import settings

# URL llama.cpp сервера (задаётся через переменную окружения)
LLAMA_SERVER_URL = settings.llama_server_url


class ModelServiceGGUF:
    """Клиент для llama.cpp server API (OpenAI-compatible)"""

    def __init__(self):
        self._stop_sequences = settings.generation_stop_sequences

    def load_model(self):
        """
        Проверка доступности llama.cpp сервера.
        Модель загружается контейнером автоматически при старте.
        """
        try:
            resp = httpx.get(f"{LLAMA_SERVER_URL}/health", timeout=30)
            if resp.status_code == 200:
                print(f"llama.cpp server is ready at {LLAMA_SERVER_URL}")
            else:
                print(f"llama.cpp server health check returned {resp.status_code}, waiting for model load...")
        except httpx.ConnectError:
            print(f"llama.cpp server not yet available at {LLAMA_SERVER_URL}, will retry on first request")

    def _build_messages(
        self,
        messages: List[Dict[str, str]],
        behavior_instruction: Optional[str] = None,
        system_prompt: Optional[str] = None,
    ) -> List[Dict[str, str]]:
        """Формирует список сообщений в формате OpenAI Chat API."""
        result = []

        # Системный промпт
        if system_prompt:
            system_message = f"{system_prompt}\n\n{settings.generation_system_base_prompt}"
        else:
            system_message = f"{settings.generation_user_prompt}\n\n{settings.generation_system_base_prompt}"
            if behavior_instruction:
                system_message = f"{behavior_instruction}\n{system_message}"

        result.append({"role": "system", "content": system_message})
        result.extend(messages)
        return result

    def _build_params(
        self,
        max_new_tokens: Optional[int],
        temperature: Optional[float],
        top_p: Optional[float],
        top_k: Optional[int],
        do_sample: Optional[bool],
    ) -> dict:
        """Формирует параметры генерации."""
        sample_flag = settings.generation_do_sample if do_sample is None else do_sample
        temperature_val = settings.generation_temperature if temperature is None else temperature
        top_p_val = settings.generation_top_p if top_p is None else top_p
        top_k_val = settings.generation_top_k if top_k is None else top_k
        max_tokens = settings.generation_max_new_tokens if max_new_tokens is None else max_new_tokens

        return {
            "max_tokens": max_tokens,
            "temperature": 0.0 if not sample_flag else temperature_val,
            "top_p": 1.0 if not sample_flag else top_p_val,
            "top_k": -1 if not sample_flag else top_k_val,
            "stop": self._stop_sequences,
        }

    def _resolve_server_url(self, llm_endpoint: Optional[str]) -> str:
        """Pick the llama.cpp base URL for this request.

        The platform default (settings.llama_server_url) serves base models.
        When a bot is bound to a finetuned model, the backend forwards that
        model's container URL via llm_endpoint, and we send the request there
        instead. Returning the same string both times keeps callers simple —
        they don't need to know which mode they're in.
        """
        if llm_endpoint:
            return llm_endpoint.rstrip("/")
        return LLAMA_SERVER_URL.rstrip("/")

    def generate_response(
        self,
        messages: List[Dict[str, str]],
        max_new_tokens: Optional[int] = None,
        temperature: Optional[float] = None,
        top_p: Optional[float] = None,
        top_k: Optional[int] = None,
        do_sample: Optional[bool] = None,
        behavior_instruction: Optional[str] = None,
        system_prompt: Optional[str] = None,
        llm_endpoint: Optional[str] = None,
    ) -> str:
        """Синхронная генерация ответа через llama.cpp server."""
        chat_messages = self._build_messages(messages, behavior_instruction, system_prompt)
        params = self._build_params(max_new_tokens, temperature, top_p, top_k, do_sample)
        server_url = self._resolve_server_url(llm_endpoint)

        payload = {
            "messages": chat_messages,
            "stream": False,
            **params,
        }

        resp = httpx.post(
            f"{server_url}/v1/chat/completions",
            json=payload,
            timeout=300,
        )
        resp.raise_for_status()
        data = resp.json()
        return data["choices"][0]["message"]["content"].strip()

    def generate_response_stream(
        self,
        messages: List[Dict[str, str]],
        max_new_tokens: Optional[int] = None,
        temperature: Optional[float] = None,
        top_p: Optional[float] = None,
        top_k: Optional[int] = None,
        do_sample: Optional[bool] = None,
        behavior_instruction: Optional[str] = None,
        system_prompt: Optional[str] = None,
        llm_endpoint: Optional[str] = None,
    ) -> Iterator[str]:
        """Потоковая генерация ответа через llama.cpp server (SSE)."""
        chat_messages = self._build_messages(messages, behavior_instruction, system_prompt)
        params = self._build_params(max_new_tokens, temperature, top_p, top_k, do_sample)
        server_url = self._resolve_server_url(llm_endpoint)

        if system_prompt:
            print(f"[AI Service] System prompt length: {len(system_prompt)} chars")
        print(f"[AI Service] User query: {messages[0].get('content', '') if messages else 'N/A'}")
        if llm_endpoint:
            print(f"[AI Service] Using override LLM endpoint: {server_url}")

        payload = {
            "messages": chat_messages,
            "stream": True,
            **params,
        }

        started = False
        with httpx.stream(
            "POST",
            f"{server_url}/v1/chat/completions",
            json=payload,
            timeout=300,
        ) as response:
            response.raise_for_status()
            for line in response.iter_lines():
                if not line or not line.startswith("data: "):
                    continue
                data_str = line[6:]
                if data_str.strip() == "[DONE]":
                    break

                import json
                try:
                    chunk = json.loads(data_str)
                except json.JSONDecodeError:
                    continue

                delta = chunk.get("choices", [{}])[0].get("delta", {})
                token = delta.get("content", "")
                if not token:
                    continue

                # Remove thinking tags
                token = token.replace("<think>", "").replace("</think>", "")
                token = token.replace("<thought>", "").replace("</thought>", "")

                # Skip empty/whitespace tokens at the beginning
                if not started:
                    if not token or token.isspace():
                        continue
                    started = True

                yield token


# Singleton instance
model_service_gguf = ModelServiceGGUF()
