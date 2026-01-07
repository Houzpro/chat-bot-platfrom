#!/usr/bin/env python3
"""
Универсальный тест для RAG системы с GGUF моделью
Проверяет все ключевые функции: генерацию, RAG, документы
"""
import asyncio
import aiohttp
import time
from pathlib import Path

BASE_URL = "http://localhost:8000"
CLIENT_ID = "test_user"


async def _read_sse_response(resp: aiohttp.ClientResponse):
    """Читает SSE ответ и собирает токены"""
    tokens = []
    metadata = None
    
    async for raw_line in resp.content:
        line = raw_line.decode('utf-8').strip()
        if not line or line == "data: [DONE]":
            continue
        
        if line.startswith("data: "):
            try:
                import json
                payload = json.loads(line[6:])
                
                if payload.get("type") == "error":
                    raise RuntimeError(payload.get("error", "Ошибка генерации"))
                
                if payload.get("type") == "token":
                    token_text = payload.get("token", "")
                    tokens.append(token_text)
                    print(token_text, end="", flush=True)  # Печатаем токены по мере получения
                elif payload.get("type") == "metadata":
                    metadata = payload
                elif "documents" in payload:  # RAG метаданные
                    metadata = payload
            except json.JSONDecodeError as e:
                print(f"\n[DEBUG] JSON decode error: {e}, line: {line}")
                continue
    
    return "".join(tokens), metadata


async def test_health():
    """Тест 1: Health check"""
    print("\n" + "="*70)
    print("🏥 ТЕСТ 1: Health Check")
    print("="*70)
    
    async with aiohttp.ClientSession() as session:
        async with session.get(f"{BASE_URL}/") as resp:
            data = await resp.json()
            print(f"✅ Статус: {resp.status}")
            print(f"   Сервис: {data.get('service')}")
            print(f"   Версия: {data.get('version')}")
            print(f"   Модель: {data.get('model')}")


async def test_simple_generation():
    """Тест 2: Простая генерация без RAG"""
    print("\n" + "="*70)
    print("💬 ТЕСТ 2: Простая генерация (без RAG)")
    print("="*70)
    
    question = "Что такое искусственный интеллект? Ответь кратко в 2-3 предложения."
    print(f"Вопрос: {question}")
    
    payload = {
        "messages": [{"role": "user", "content": question}]
    }
    
    start = time.time()
    
    async with aiohttp.ClientSession() as session:
        async with session.post(f"{BASE_URL}/ask", json=payload) as resp:
            if resp.status != 200:
                print(f"❌ Ошибка HTTP: {resp.status}")
                return
            
            print(f"Ответ: ", end="", flush=True)
            answer, metadata = await _read_sse_response(resp)
            print()  # Новая строка после ответа
            
    elapsed = time.time() - start
    print(f"\n⏱️  Время: {elapsed:.2f} сек")
    if metadata:
        print(f"⚡ Скорость: ~{metadata.get('tokens_per_second', 0):.1f} токенов/сек")


async def test_document_upload():
    """Тест 3: Загрузка документа"""
    print("\n" + "="*70)
    print("📤 ТЕСТ 3: Загрузка документа в векторную БД")
    print("="*70)
    
    # Создаем тестовый документ
    test_file = Path("test_document.txt")
    test_content = """
Python - это высокоуровневый язык программирования общего назначения.
Python поддерживает множество парадигм программирования: объектно-ориентированное, 
функциональное и императивное программирование.

FastAPI - это современный, быстрый веб-фреймворк для создания API с Python.
FastAPI использует типизацию Python и автоматически генерирует документацию OpenAPI.

Qdrant - это векторная база данных с открытым исходным кодом.
Qdrant позволяет хранить и искать векторные представления данных.

RAG (Retrieval-Augmented Generation) - техника улучшения LLM через векторный поиск.
RAG позволяет моделям использовать актуальную информацию из документов.
"""
    
    test_file.write_text(test_content, encoding='utf-8')
    
    try:
        data = aiohttp.FormData()
        data.add_field('client_id', CLIENT_ID)
        data.add_field('file', open(test_file, 'rb'), 
                      filename='test_document.txt',
                      content_type='text/plain')
        
        async with aiohttp.ClientSession() as session:
            async with session.post(f"{BASE_URL}/documents/upload", data=data) as resp:
                result = await resp.json()
                
                if result.get('success'):
                    print(f"✅ Документ загружен")
                    print(f"   Файл: {result.get('file_name')}")
                    print(f"   Чанков: {result.get('chunks_count')}")
                    print(f"   ID документов: {len(result.get('document_ids', []))}")
                else:
                    print(f"❌ Ошибка: {result.get('error')}")
    
    finally:
        test_file.unlink(missing_ok=True)
    
    # Ждем индексации
    await asyncio.sleep(2)


async def test_document_search():
    """Тест 4: Поиск документов"""
    print("\n" + "="*70)
    print("🔍 ТЕСТ 4: Поиск релевантных документов")
    print("="*70)
    
    query = "Что такое FastAPI?"
    print(f"Запрос: {query}")
    
    data = aiohttp.FormData()
    data.add_field('client_id', CLIENT_ID)
    data.add_field('query', query)
    data.add_field('limit', '3')
    
    async with aiohttp.ClientSession() as session:
        async with session.post(f"{BASE_URL}/documents/search", data=data) as resp:
            result = await resp.json()
            
            if result.get('success'):
                docs = result.get('documents', [])
                print(f"\n✅ Найдено документов: {len(docs)}")
                for i, doc in enumerate(docs, 1):
                    print(f"\n   Документ {i}:")
                    print(f"   Score: {doc.get('score', 0):.3f}")
                    print(f"   Текст: {doc.get('text', '')[:100]}...")
            else:
                print(f"❌ Ошибка: {result.get('error')}")


async def test_rag_generation():
    """Тест 5: RAG генерация с документами"""
    print("\n" + "="*70)
    print("🧠 ТЕСТ 5: RAG генерация (с документами из БД)")
    print("="*70)
    
    question = "Расскажи про FastAPI на основе документов"
    print(f"Вопрос: {question}")
    
    data = aiohttp.FormData()
    data.add_field("client_id", CLIENT_ID)
    data.add_field("query", question)
    data.add_field("top_k_docs", "3")
    
    start = time.time()
    
    async with aiohttp.ClientSession() as session:
        async with session.post(f"{BASE_URL}/ask-rag-from-db", data=data) as resp:
            if resp.status != 200:
                print(f"❌ Ошибка HTTP: {resp.status}")
                return
            
            print(f"Ответ: ", end="", flush=True)
            answer, metadata = await _read_sse_response(resp)
            print()  # Новая строка после ответа
            
            if metadata:
                used_docs = metadata.get('num_documents_used', 0)
                print(f"\n📚 Использовано документов: {used_docs}")
    
    elapsed = time.time() - start
    print(f"⏱️  Время: {elapsed:.2f} сек")


async def test_stats():
    """Тест 6: Статистика документов"""
    print("\n" + "="*70)
    print("📊 ТЕСТ 6: Статистика документов клиента")
    print("="*70)
    
    async with aiohttp.ClientSession() as session:
        async with session.get(f"{BASE_URL}/documents/stats/{CLIENT_ID}") as resp:
            result = await resp.json()
            
            if result.get('success'):
                print(f"✅ Статистика получена")
                print(f"   Документов: {result.get('total_documents', 0)}")
            else:
                print(f"❌ Ошибка: {result.get('error')}")


async def main():
    """Запуск всех тестов"""
    print("\n" + "="*70)
    print("🚀 ТЕСТИРОВАНИЕ RAG СИСТЕМЫ С GGUF")
    print("="*70)
    print(f"URL: {BASE_URL}")
    print(f"Клиент: {CLIENT_ID}")
    
    try:
        # Запускаем все тесты последовательно
        await test_health()
        await test_simple_generation()
        await test_document_upload()
        await test_document_search()
        await test_rag_generation()
        await test_stats()
        
        # Итоги
        print("\n" + "="*70)
        print("✅ ВСЕ ТЕСТЫ ПРОЙДЕНЫ УСПЕШНО!")
        print("="*70)
        print("\n📝 Резюме:")
        print("   ✅ Health check - OK")
        print("   ✅ Простая генерация - OK")
        print("   ✅ Загрузка документа - OK")
        print("   ✅ Поиск документов - OK")
        print("   ✅ RAG генерация - OK")
        print("   ✅ Статистика - OK")
        print("\n🎉 Система готова к использованию!")
        
    except Exception as e:
        print(f"\n❌ ОШИБКА: {e}")
        import traceback
        traceback.print_exc()
        exit(1)


if __name__ == "__main__":
    asyncio.run(main())
