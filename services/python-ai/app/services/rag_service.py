"""
Универсальный RAG сервис для любых типов документов
Без хардкода, полагается на мощные embedding и reranking модели
"""
import threading
import re
from typing import List, Dict, Any, Optional

from sentence_transformers import SentenceTransformer, CrossEncoder
import nltk

from app.config.settings import settings


# Скачать необходимые данные для NLTK
try:
    nltk.data.find('tokenizers/punkt')
except LookupError:
    try:
        nltk.download('punkt', quiet=True)
    except Exception:
        pass


def clean_pdf_artifacts(text: str) -> str:
    """
    Удалить артефакты и мусор из распарсенного PDF
    """
    text = re.sub(r'[\x00-\x08\x0B\x0C\x0E-\x1F]', '', text)
    text = re.sub(r'[^\w\s\-\.,!?:;()&\'"\n]', '', text, flags=re.UNICODE)
    text = re.sub(r' +', ' ', text)
    text = re.sub(r'\n{3,}', '\n\n', text)
    return text.strip()



class RAGService:
    """
    Универсальный RAG сервис:
    - Работает с любыми типами документов
    - Без специфичного хардкода
    - Полагается на качество embedding и reranking моделей
    """
    
    def __init__(self):
        self._embedding_model = None
        self._reranker_model = None
        self._lock = threading.Lock()
    
    def load_embedding_model(self) -> SentenceTransformer:
        """Загрузить модель для embeddings (singleton)"""
        with self._lock:
            if self._embedding_model is None:
                if not settings.embedding_model_name:
                    raise ValueError("EMBEDDING_MODEL_NAME is not configured")
                if not settings.embedding_cache_folder:
                    raise ValueError("EMBEDDING_CACHE_FOLDER is not configured")
                
                self._embedding_model = SentenceTransformer(
                    settings.embedding_model_name,
                    cache_folder=settings.embedding_cache_folder
                )
                print(f"✅ Embedding model loaded: {settings.embedding_model_name}")
            return self._embedding_model
    
    def load_reranker_model(self) -> Optional[CrossEncoder]:
        """Загрузить Cross-Encoder reranker (singleton)"""
        with self._lock:
            if self._reranker_model is None and settings.use_reranker:
                try:
                    model_name = settings.reranker_model_name or "cross-encoder/ms-marco-MiniLM-L-6-v2"
                    self._reranker_model = CrossEncoder(model_name, max_length=512)
                    print(f"✅ Reranker loaded: {model_name}")
                except Exception as e:
                    print(f"⚠️ Failed to load reranker: {e}")
                    return None
            return self._reranker_model
    
    def create_embeddings(self, texts: List[str], is_query: bool = False) -> List[List[float]]:
        """
        Создать векторные представления для текстов
        
        Args:
            texts: Список текстов
            is_query: True если это запрос (для некоторых моделей добавляется префикс)
        """
        embedding_model = self.load_embedding_model()
        
        # Для multilingual-e5 моделей добавляем префикс
        model_name = settings.embedding_model_name or ""
        if "e5" in model_name.lower():
            if is_query:
                texts = [f"query: {text}" for text in texts]
            else:
                texts = [f"passage: {text}" for text in texts]
        
        embeddings = embedding_model.encode(
            texts,
            convert_to_numpy=True,
            normalize_embeddings=True,
            show_progress_bar=False
        )
        
        return embeddings.tolist()
    

    def split_text_semantic(self, text: str, chunk_size: int = 2500, overlap: int = 500) -> List[str]:
        """
        Универсальное семантическое разбиение:
        - Работает с любыми документами (не привязано к героям/специфике)
        - Держит контекст локальных заголовков/секций, но не смешивает секции
        - Делает overlap только внутри одной секции, чтобы не тянуть чужой контекст
        - Чистит PDF артефакты
        """
        text = clean_pdf_artifacts(text)
        if not text or len(text) <= chunk_size:
            return [text] if text else []

        lines = text.split('\n')
        chunks: List[str] = []
        current_chunk: List[str] = []
        current_chunk_len = 0
        last_header = ""

        def is_heading(line: str) -> bool:
            if len(line) > 180:
                return False
            if line.startswith('#'):
                return True
            # ALL CAPS short lines
            if line.isupper() and len(line.split()) <= 8:
                return True
            # Numbered / bullet headings
            if re.match(r"^(\d+\.|[ivxlcdm]+\.|[•\-*])\s+", line.strip(), flags=re.IGNORECASE):
                return len(line) <= 160
            # Title case short phrase
            if line and line[0].isupper() and len(line.split()) <= 6:
                return True
            return False

        for raw_line in lines:
            line = raw_line.strip()
            if not line:
                continue

            heading = is_heading(line)

            # Если новый заголовок и уже есть накопленный чанк — закрываем чанк
            if heading and current_chunk:
                if last_header and last_header not in ' '.join(current_chunk):
                    chunk_text = f"{last_header}\n{' '.join(current_chunk)}"
                else:
                    chunk_text = ' '.join(current_chunk)
                chunks.append(chunk_text)
                # Не переносим overlap между секциями
                current_chunk = []
                current_chunk_len = 0

            if heading:
                last_header = line
                sentences = [line]
            else:
                try:
                    sentences = nltk.sent_tokenize(line)
                except Exception:
                    sentences = [line]

            for sent in sentences:
                sent = sent.strip()
                if not sent or len(sent) < 3:
                    continue

                sent_with_context = sent
                if not heading and last_header:
                    sent_with_context = f"{last_header}: {sent}"

                new_len = current_chunk_len + len(sent_with_context) + 1

                if new_len > chunk_size and current_chunk:
                    if last_header and last_header not in ' '.join(current_chunk):
                        chunk_text = f"{last_header}\n{' '.join(current_chunk)}"
                    else:
                        chunk_text = ' '.join(current_chunk)

                    chunks.append(chunk_text)

                    overlap_buf: List[str] = []
                    # Overlap только внутри секции (не копим при заголовке)
                    if overlap > 0 and not heading:
                        tail = chunk_text[-overlap:]
                        overlap_buf.append(tail)

                    current_chunk = overlap_buf
                    current_chunk_len = sum(len(x) for x in overlap_buf)

                current_chunk.append(sent_with_context)
                current_chunk_len = new_len

        if current_chunk:
            if last_header and last_header not in ' '.join(current_chunk):
                chunk_text = f"{last_header}\n{' '.join(current_chunk)}"
            else:
                chunk_text = ' '.join(current_chunk)
            chunks.append(chunk_text)

        return chunks if chunks else [text]
    
    def rerank_documents(
        self,
        query: str,
        documents: List[Dict[str, Any]],
        top_k: int = 20
    ) -> List[Dict[str, Any]]:
        """
        Reranking с помощью CrossEncoder
        Улучшенная версия с лучшей диагностикой и обработкой ошибок
        
        Args:
            query: Запрос пользователя
            documents: Список документов для переранжирования
            top_k: Сколько лучших результатов вернуть
        """
        if not documents:
            return []
        
        reranker = self.load_reranker_model()
        if reranker is None:
            print("⚠️ Reranker not available, returning original order")
            return documents[:top_k]
        
        try:
            # Подготавливаем пары (query, document)
            pairs = []
            for doc in documents:
                doc_text = doc.get('text', '')[:2000]  # Первые 2000 символов
                if doc_text.strip():
                    pairs.append((query, doc_text))
                else:
                    # Если нет текста, используем метаданные
                    pairs.append((query, str(doc)))
            
            if not pairs:
                return documents[:top_k]
            
            # Вычисляем scores через CrossEncoder
            scores = reranker.predict(pairs, show_progress_bar=False)
            
            # Добавляем scores в документы
            for i, doc in enumerate(documents):
                if i < len(scores):
                    doc['rerank_score'] = float(scores[i])
                else:
                    doc['rerank_score'] = 0.0
            
            # Сортируем по rerank_score
            reranked = sorted(documents, key=lambda d: d.get('rerank_score', 0), reverse=True)
            
            # Выводим подробный отчет reranking'а
            print(f"\n📊 RERANKING RESULTS for query: '{query}'")
            print(f"   Total candidates: {len(documents)}")
            print(f"   Returning: {min(top_k, len(reranked))}")
            print(f"\n   Top results:")
            for i, doc in enumerate(reranked[:top_k]):
                score = doc.get('rerank_score', 0)
                text_preview = doc.get('text', '')[:100].replace('\n', ' ')
                file_name = doc.get('file_name', 'unknown')
                chunk_idx = doc.get('chunk_index', '?')
                print(f"   #{i+1}: score={score:7.4f} | {file_name}[{chunk_idx}] | \"{text_preview}...\"")
            
            return reranked[:top_k]
        
        except Exception as e:
            print(f"⚠️ Reranking failed: {e}")
            import traceback
            traceback.print_exc()
            # Fallback: вернём документы в исходном порядке
            return documents[:top_k]
    
    def advanced_search(
        self,
        bot_id: str,
        query: str,
        vector_results: List[Dict[str, Any]],
        top_k: int = 30
    ) -> List[Dict[str, Any]]:
        """
        Продвинутый поиск: dense retrieval + optional reranking
        
        Стратегия:
        1. Берём все уникальные векторные результаты
        2. Если reranker включен - переранжируем полный набор
        3. Возвращаем top-k лучших результатов
        
        Args:
            bot_id: ID бота для логирования
            query: Запрос пользователя
            vector_results: Результаты векторного поиска из Qdrant
            top_k: Сколько результатов вернуть
        """
        print(f"\n🔍 ADVANCED SEARCH")
        print(f"   Query: '{query}'")
        print(f"   Bot: {bot_id}")
        print(f"   Vector results received: {len(vector_results)}")
        
        # Дедубликация по ID
        all_candidates = {}
        for doc in vector_results:
            doc_id = str(doc.get('id', ''))
            if doc_id and doc_id not in all_candidates:
                all_candidates[doc_id] = doc
        
        candidates_list = list(all_candidates.values())
        print(f"   After dedup: {len(candidates_list)} unique candidates")
        
        if not candidates_list:
            print("⚠️ No candidates found after dedup")
            return []
        
        # Если reranker включен - используем его
        if settings.use_reranker:
            print(f"   Using reranker: {settings.reranker_model_name}")
            # Берём больше для реранжирования чтобы не потерять релевантные
            rerank_k = min(top_k * 3, len(candidates_list))
            print(f"   Reranking {rerank_k} candidates...")
            reranked = self.rerank_documents(query, candidates_list, top_k=rerank_k)
            print(f"   ✅ Returned {len(reranked[:top_k])} results after reranking")
            return reranked[:top_k]
        else:
            # Без reranker возвращаем просто top-k по исходному порядку (distance)
            print(f"   ⚠️ Reranker disabled, returning {min(top_k, len(candidates_list))} by distance")
            return candidates_list[:top_k]
    
    def build_context(
        self,
        query: str,
        documents: List[Dict[str, Any]],
        max_chars: int = 120000,
        max_docs: int = 30,
        min_docs: int = 8
    ) -> str:
        """
        Собрать контекст из документов для передачи в LLM
        
        Стратегия: отдаём ВСЁ что нашли, LLM сама выберет нужное
        
        Args:
            query: Запрос (не используется, оставлен для совместимости)
            documents: Отсортированные по релевантности документы
            max_chars: Максимальный размер контекста
        """
        if not documents:
            return ""
        def extract_keywords(text: str) -> List[str]:
            # Берём осмысленные токены длиной 4+ символов (рус/лат), чтобы отсечь стоп-слова
            return [w.lower() for w in re.findall(r"[A-Za-zА-Яа-яЁё']+", text) if len(w) >= 4]

        query_keywords = extract_keywords(query)

        main_header: Optional[str] = None
        first_text = documents[0].get('text', '')
        first_line = first_text.split('\n', 1)[0].strip()
        if first_line and len(first_line) <= 60:
            main_header = first_line

        filtered_docs: List[Dict[str, Any]] = []
        max_rerank = 0.0
        for doc in documents:
            score = float(doc.get('rerank_score', 0) or 0)
            if score > max_rerank:
                max_rerank = score

        for doc in documents:
            text = doc.get('text', '')
            lower_text = text.lower()
            score = float(doc.get('rerank_score', 0) or 0)

            keep = False
            if query_keywords and any(k in lower_text for k in query_keywords):
                keep = True
            if not keep and main_header and main_header.lower() in lower_text:
                keep = True
            if not filtered_docs:
                keep = True

            # Дополнительно фильтруем по rerank_score, но не режем первый документ
            if keep and max_rerank > 0 and filtered_docs:
                # Оставляем документы с приемлемым скором, чтобы убрать шум
                if score < max_rerank * 0.55:
                    keep = False

            if keep:
                filtered_docs.append(doc)

        # Если фильтр дал мало — добавим сверху оставшиеся (расширяем recall), сохраняя исходный порядок
        if len(filtered_docs) < min_docs:
            for doc in documents:
                if doc in filtered_docs:
                    continue
                filtered_docs.append(doc)
                if len(filtered_docs) >= min_docs:
                    break

        # Если совсем нет совпадений по ключевым словам и заголовку —fallback: берём топ min_docs из rerank
        if not filtered_docs:
            filtered_docs = documents[:min(min_docs, len(documents))]

        # Удаляем точные дубликаты текста, чтобы не раздувать контекст
        deduped: List[Dict[str, Any]] = []
        seen_texts = set()
        for doc in filtered_docs:
            text = doc.get('text', '')
            key = text.strip()
            if key in seen_texts:
                continue
            seen_texts.add(key)
            deduped.append(doc)
        filtered_docs = deduped

        print(f"🧹 Context filter: {len(filtered_docs)} kept of {len(documents)} (min_docs={min_docs})")

        context_parts = []
        total_chars = 0

        for doc in filtered_docs:
            text = doc.get('text', '').strip()
            if not text:
                continue

            # Пропускаем голые заголовки без содержания (если это не самый первый документ)
            if len(text) < 120 and '.' not in text and '\n' not in text and len(context_parts) > 0:
                continue

            if len(context_parts) >= max_docs:
                break

            if total_chars + len(text) > max_chars:
                remaining = max_chars - total_chars
                if remaining > 500:
                    text = text[:remaining] + "..."
                    context_parts.append(text)
                break

            context_parts.append(text)
            total_chars += len(text)
        
        context = '\n\n'.join(context_parts)
        print(f"📄 Context built: {len(context_parts)} documents, {total_chars} chars")
        
        return context
    
    def build_bm25_index(self, bot_id: str, documents: List[Dict[str, Any]]) -> None:
        """
        Placeholder для совместимости (BM25 больше не используется в новом pipeline)
        Современный подход использует только dense retrieval + reranking
        
        Args:
            bot_id: ID бота
            documents: Список документов (игнорируется)
        """
        print(f"⚠️ build_bm25_index called for bot {bot_id}, but BM25 is deprecated")
        print(f"   Using modern dense retrieval + reranking instead")
        pass  # No-op: современный pipeline не нуждается в BM25


# Singleton instance
rag_service = RAGService()
