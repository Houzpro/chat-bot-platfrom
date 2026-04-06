"""
RAG Service v2: Agentic Router + Self-Correction + Cosine Re-ranking + Enumeration Prompts

Based on techniques from Graph RAG article (Habr):
1. Agentic Router - classifies queries into types (simple/global/enumeration/multi_hop/relation)
2. Self-Correction loop - retries with rephrased queries, keeps best results
3. Cosine Re-ranking - replaces RRF for embedding-based signals (article finding #4)
4. Enumeration prompts - special format for "list all..." questions (finding #9)
5. Language matching - detects query/document language mismatch (finding #1, +29pp)
6. Relevance escalation tiers - vector -> hybrid -> full_document_read (finding #6)
7. Typed pipeline trace - structured trace for debugging (article Typed API)
"""
import re
import time
import threading
import uuid
import numpy as np
from typing import List, Dict, Any, Optional, Tuple
from dataclasses import dataclass, field, asdict

from sentence_transformers import SentenceTransformer, CrossEncoder
from langchain_text_splitters import RecursiveCharacterTextSplitter
from rank_bm25 import BM25Okapi

from app.config.settings import settings


# ═══════════════════════════════════════════════════════════════════
# Data models for Typed API (PipelineTrace)
# ═══════════════════════════════════════════════════════════════════

@dataclass
class RouterDecision:
    query_type: str  # simple, relation, multi_hop, global, enumeration
    suggested_tool: str  # vector_search, hybrid_search, full_document_read
    confidence: float
    method: str  # keyword, pattern
    detected_language: str  # ru, en, mixed
    is_enumeration: bool

@dataclass
class ToolStep:
    tool_name: str
    results_count: int
    relevance_score: float
    duration_ms: float

@dataclass
class EscalationStep:
    from_tool: str
    to_tool: str
    reason: str
    previous_score: float

@dataclass
class SelfCorrectionAttempt:
    attempt_number: int
    rephrased_query: str
    score: float
    results_count: int

@dataclass
class PipelineTrace:
    trace_id: str = field(default_factory=lambda: f"tr_{uuid.uuid4().hex[:12]}")
    router_step: Optional[RouterDecision] = None
    tool_steps: List[ToolStep] = field(default_factory=list)
    escalation_steps: List[EscalationStep] = field(default_factory=list)
    self_correction_attempts: List[SelfCorrectionAttempt] = field(default_factory=list)
    best_score: float = 0.0
    total_duration_ms: float = 0.0

    def to_dict(self) -> dict:
        return asdict(self)


# ═══════════════════════════════════════════════════════════════════
# Constants and keyword patterns for Agentic Router
# ═══════════════════════════════════════════════════════════════════

# Bilingual keywords for query classification (inspired by Mangle routing rules)
ENUMERATION_KEYWORDS_RU = [
    "перечисли", "перечислите", "список", "назови все", "какие есть",
    "все виды", "все типы", "все варианты", "сколько всего",
    "перечень", "укажи все", "укажите все", "все пункты",
    "полный список", "все элементы", "все категории",
]
ENUMERATION_KEYWORDS_EN = [
    "list all", "enumerate", "what are all", "name all",
    "how many", "all types", "all kinds", "all variants",
    "complete list", "all items", "all categories",
]

GLOBAL_KEYWORDS_RU = [
    "в целом", "общий", "обзор", "суммар", "итого", "весь документ",
    "о чём", "о чем", "основная мысль", "главная идея",
    "краткое содержание", "резюме", "вывод",
]
GLOBAL_KEYWORDS_EN = [
    "overall", "summary", "overview", "in general", "main idea",
    "what is about", "conclusion", "total", "brief",
]

MULTI_HOP_KEYWORDS_RU = [
    "как связан", "какая связь", "через что", "по цепочке",
    "если.*то", "последовательность", "цепь", "зависимость",
]
MULTI_HOP_KEYWORDS_EN = [
    "how.*related", "connection between", "chain of",
    "if.*then", "sequence", "dependency", "leads to",
]

RELATION_KEYWORDS_RU = [
    "сравни", "отличие", "разница", "versus", "между",
    "в отличие", "по сравнению", "чем отличается",
]
RELATION_KEYWORDS_EN = [
    "compare", "difference", "versus", "between",
    "unlike", "compared to", "differ",
]

# Relevance thresholds from settings
RELEVANCE_ESCALATION_THRESHOLD = settings.relevance_escalation_threshold
EMBEDDING_SIMILARITY_AUTOPASS = settings.embedding_similarity_autopass


def clean_text(text: str) -> str:
    """Remove control characters, preserve punctuation and unicode."""
    text = re.sub(r'[\x00-\x08\x0B\x0C\x0E-\x1F]', '', text)
    text = re.sub(r'[ \t]+', ' ', text)
    text = re.sub(r'\n{3,}', '\n\n', text)
    return text.strip()


def tokenize_for_bm25(text: str) -> List[str]:
    """Tokenize for BM25: lowercase, words 2+ chars."""
    return [w.lower() for w in re.findall(r'[A-Za-zА-Яа-яЁё0-9]+', text) if len(w) >= 2]


def detect_language(text: str) -> str:
    """Detect dominant language of text (ru/en/mixed)."""
    cyrillic = len(re.findall(r'[А-Яа-яЁё]', text))
    latin = len(re.findall(r'[A-Za-z]', text))
    total = cyrillic + latin
    if total == 0:
        return "mixed"
    ratio = cyrillic / total
    if ratio > 0.6:
        return "ru"
    elif ratio < 0.4:
        return "en"
    return "mixed"


class RAGService:
    """
    Advanced RAG Pipeline v2:
    1. Agentic Router — classify query type via keyword/pattern matching
    2. Tiered retrieval — vector_search -> hybrid_search -> full_document_read
    3. Cosine re-ranking — direct cosine similarity instead of RRF
    4. Self-correction loop — retry with rephrased query, keep best results
    5. Enumeration prompts — special format for global/list questions
    6. Language matching — detect and handle cross-language queries
    7. Pipeline trace — structured logging for debugging
    """

    def __init__(self):
        self._embedding_model = None
        self._reranker_model = None
        self._lock = threading.Lock()

    # ─── Model Loading ──────────────────────────────────────────────

    def load_embedding_model(self) -> SentenceTransformer:
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
                print(f"Embedding model loaded: {settings.embedding_model_name}")
            return self._embedding_model

    def load_reranker_model(self) -> Optional[CrossEncoder]:
        with self._lock:
            if self._reranker_model is None and settings.use_reranker:
                try:
                    model_name = settings.reranker_model_name or "cross-encoder/ms-marco-MiniLM-L-6-v2"
                    self._reranker_model = CrossEncoder(model_name, max_length=512)
                    print(f"Reranker loaded: {model_name}")
                except Exception as e:
                    print(f"Failed to load reranker: {e}")
                    return None
            return self._reranker_model

    # ─── Embeddings ─────────────────────────────────────────────────

    def create_embeddings(self, texts: List[str], is_query: bool = False) -> List[List[float]]:
        embedding_model = self.load_embedding_model()

        model_name = settings.embedding_model_name or ""
        if "e5" in model_name.lower():
            prefix = "query: " if is_query else "passage: "
            texts = [f"{prefix}{text}" for text in texts]

        embeddings = embedding_model.encode(
            texts,
            convert_to_numpy=True,
            normalize_embeddings=True,
            show_progress_bar=False
        )
        return embeddings.tolist()

    # ─── Chunking ──────────────────────────────────────────────────

    def split_text_semantic(self, text: str, chunk_size: int = 1200, overlap: int = 200) -> List[str]:
        """Split text using RecursiveCharacterTextSplitter from langchain."""
        text = clean_text(text)
        if not text:
            return []
        if len(text) <= chunk_size:
            return [text]

        splitter = RecursiveCharacterTextSplitter(
            chunk_size=chunk_size,
            chunk_overlap=overlap,
            length_function=len,
            is_separator_regex=False,
            separators=["\n\n", "\n", ". ", " ", ""],
        )

        chunks = splitter.split_text(text)
        return [c for c in chunks if c.strip()]

    # ═══════════════════════════════════════════════════════════════
    # AGENTIC ROUTER — Query Classification
    # ═══════════════════════════════════════════════════════════════

    def classify_query(self, query: str) -> RouterDecision:
        """
        Three-tier routing (from article):
        Tier 1: Keyword matching (65 bilingual keywords) — confidence 0.7
        Tier 2: Pattern/regex matching — confidence 0.5
        Fallback: simple query with vector_search
        """
        query_lower = query.lower().strip()
        detected_lang = detect_language(query)

        # Tier 1: Keyword matching (highest priority for enumeration/global)
        # Check enumeration first (most specific)
        for kw in ENUMERATION_KEYWORDS_RU + ENUMERATION_KEYWORDS_EN:
            if kw in query_lower:
                return RouterDecision(
                    query_type="enumeration",
                    suggested_tool="full_document_read",
                    confidence=0.7,
                    method="keyword",
                    detected_language=detected_lang,
                    is_enumeration=True,
                )

        # Check global questions
        for kw in GLOBAL_KEYWORDS_RU + GLOBAL_KEYWORDS_EN:
            if kw in query_lower:
                return RouterDecision(
                    query_type="global",
                    suggested_tool="full_document_read",
                    confidence=0.7,
                    method="keyword",
                    detected_language=detected_lang,
                    is_enumeration=False,
                )

        # Check multi-hop
        for kw in MULTI_HOP_KEYWORDS_RU + MULTI_HOP_KEYWORDS_EN:
            if re.search(kw, query_lower):
                return RouterDecision(
                    query_type="multi_hop",
                    suggested_tool="hybrid_search",
                    confidence=0.7,
                    method="keyword",
                    detected_language=detected_lang,
                    is_enumeration=False,
                )

        # Check relation/comparison
        for kw in RELATION_KEYWORDS_RU + RELATION_KEYWORDS_EN:
            if kw in query_lower:
                return RouterDecision(
                    query_type="relation",
                    suggested_tool="hybrid_search",
                    confidence=0.7,
                    method="keyword",
                    detected_language=detected_lang,
                    is_enumeration=False,
                )

        # Tier 2: Pattern matching
        # Questions with "?" and question words are likely simple factual
        question_pattern = re.compile(
            r'^(что|кто|где|когда|как|почему|зачем|какой|какая|какое|какие|'
            r'what|who|where|when|how|why|which)\b',
            re.IGNORECASE
        )
        if question_pattern.search(query_lower):
            return RouterDecision(
                query_type="simple",
                suggested_tool="vector_search",
                confidence=0.5,
                method="pattern",
                detected_language=detected_lang,
                is_enumeration=False,
            )

        # Fallback: simple vector search
        return RouterDecision(
            query_type="simple",
            suggested_tool="vector_search",
            confidence=0.5,
            method="pattern",
            detected_language=detected_lang,
            is_enumeration=False,
        )

    # ═══════════════════════════════════════════════════════════════
    # COSINE RE-RANKING (replaces RRF — article finding #4)
    # ═══════════════════════════════════════════════════════════════

    def cosine_rerank(
        self,
        query: str,
        documents: List[Dict[str, Any]],
        top_k: int = 30
    ) -> List[Dict[str, Any]]:
        """
        Cosine re-ranking using real embeddings.
        Article finding #4: cosine re-ranking beats RRF when both signals are embedding-based.
        """
        if not documents:
            return []

        # Get query embedding
        query_emb = self.create_embeddings([query], is_query=True)[0]
        query_vec = np.array(query_emb)

        scored_docs = []
        for doc in documents:
            doc_copy = doc.copy()

            # Use existing embedding if available, otherwise use text to create one
            doc_embedding = doc.get('embedding')
            if doc_embedding is not None:
                doc_vec = np.array(doc_embedding)
            else:
                # Fallback: use the original vector score if available
                doc_copy['cosine_score'] = float(doc.get('score', 0))
                scored_docs.append(doc_copy)
                continue

            # Compute cosine similarity (embeddings are already normalized)
            cosine_sim = float(np.dot(query_vec, doc_vec))
            doc_copy['cosine_score'] = cosine_sim
            scored_docs.append(doc_copy)

        # Sort by cosine similarity descending
        scored_docs.sort(key=lambda d: d.get('cosine_score', 0), reverse=True)

        print(f"  Cosine re-ranking: top scores = {[round(d.get('cosine_score', 0), 4) for d in scored_docs[:5]]}")
        return scored_docs[:top_k]

    # ═══════════════════════════════════════════════════════════════
    # BM25 KEYWORD SEARCH
    # ═══════════════════════════════════════════════════════════════

    def bm25_search(
        self,
        query: str,
        documents: List[Dict[str, Any]],
        top_k: int = 30
    ) -> List[Dict[str, Any]]:
        if not documents:
            return []

        texts = [doc.get('text', '') for doc in documents]
        tokenized_corpus = [tokenize_for_bm25(t) for t in texts]

        non_empty = [(i, tokens) for i, tokens in enumerate(tokenized_corpus) if tokens]
        if not non_empty:
            return documents[:top_k]

        indices, corpus = zip(*non_empty)
        bm25 = BM25Okapi(list(corpus))

        query_tokens = tokenize_for_bm25(query)
        if not query_tokens:
            return documents[:top_k]

        scores = bm25.get_scores(query_tokens)

        scored = []
        for orig_idx, score in zip(indices, scores):
            doc = documents[orig_idx].copy()
            doc['bm25_score'] = float(score)
            scored.append(doc)

        scored.sort(key=lambda d: d['bm25_score'], reverse=True)
        return scored[:top_k]

    # ─── Window Retrieval ──────────────────────────────────────────

    def apply_window_retrieval(
        self,
        documents: List[Dict[str, Any]],
        all_documents: List[Dict[str, Any]],
        window: int = 1
    ) -> List[Dict[str, Any]]:
        """
        Window Retrieval: for each found chunk, pull in neighboring chunks
        (chunk_index +/- window) from the same file.
        """
        if window <= 0:
            return documents

        doc_index: Dict[str, Dict[int, Dict[str, Any]]] = {}
        for doc in all_documents:
            fname = doc.get('file_name', 'unknown')
            try:
                cidx = int(doc.get('chunk_index', -1))
            except (ValueError, TypeError):
                continue
            if fname not in doc_index:
                doc_index[fname] = {}
            doc_index[fname][cidx] = doc

        result_ids = set()
        result = []
        for doc in documents:
            doc_id = str(doc.get('id', ''))
            if doc_id not in result_ids:
                result_ids.add(doc_id)
                result.append(doc)

        neighbors_to_add = []
        for doc in documents:
            fname = doc.get('file_name', 'unknown')
            try:
                cidx = int(doc.get('chunk_index', -1))
            except (ValueError, TypeError):
                continue

            if fname not in doc_index:
                continue

            for offset in range(-window, window + 1):
                if offset == 0:
                    continue
                neighbor_idx = cidx + offset
                if neighbor_idx in doc_index[fname]:
                    neighbor = doc_index[fname][neighbor_idx]
                    neighbor_id = str(neighbor.get('id', ''))
                    if neighbor_id and neighbor_id not in result_ids:
                        result_ids.add(neighbor_id)
                        neighbors_to_add.append(neighbor)

        result.extend(neighbors_to_add)
        result.sort(key=lambda d: (d.get('file_name', ''), int(d.get('chunk_index', 0) or 0)))

        print(f"  Window retrieval: {len(documents)} -> {len(result)} (+{len(neighbors_to_add)} neighbors)")
        return result

    # ─── Cross-Encoder Reranking ──────────────────────────────────

    def rerank_documents(
        self,
        query: str,
        documents: List[Dict[str, Any]],
        top_k: int = 20
    ) -> List[Dict[str, Any]]:
        if not documents:
            return []

        reranker = self.load_reranker_model()
        if reranker is None:
            return documents[:top_k]

        try:
            pairs = [(query, doc.get('text', '')[:2000] or "empty") for doc in documents]
            scores = reranker.predict(pairs, show_progress_bar=False)

            for i, doc in enumerate(documents):
                doc['rerank_score'] = float(scores[i]) if i < len(scores) else 0.0

            reranked = sorted(documents, key=lambda d: d.get('rerank_score', 0), reverse=True)

            print(f"  Reranking: {len(documents)} -> top {min(top_k, len(reranked))}")
            for i, doc in enumerate(reranked[:3]):
                score = doc.get('rerank_score', 0)
                preview = doc.get('text', '')[:60].replace('\n', ' ')
                print(f"    #{i+1}: score={score:.4f} | \"{preview}\"")

            return reranked[:top_k]
        except Exception as e:
            print(f"  Reranking failed: {e}")
            return documents[:top_k]

    # ═══════════════════════════════════════════════════════════════
    # LANGUAGE MATCHING (article finding #1, +29pp improvement)
    # ═══════════════════════════════════════════════════════════════

    def detect_language_mismatch(
        self,
        query: str,
        documents: List[Dict[str, Any]]
    ) -> bool:
        """
        Detect if query language doesn't match document language.
        Article finding #1: matching languages gives +29pp improvement.
        """
        if not documents:
            return False

        query_lang = detect_language(query)

        # Sample first few documents to detect their language
        doc_texts = " ".join(
            doc.get('text', '')[:200] for doc in documents[:5]
        )
        doc_lang = detect_language(doc_texts)

        mismatch = query_lang != doc_lang and query_lang != "mixed" and doc_lang != "mixed"
        if mismatch:
            print(f"  Language mismatch detected: query={query_lang}, docs={doc_lang}")
        return mismatch

    # ═══════════════════════════════════════════════════════════════
    # RELEVANCE SCORING
    # ═══════════════════════════════════════════════════════════════

    def compute_relevance_score(
        self,
        query: str,
        results: List[Dict[str, Any]]
    ) -> float:
        """
        Compute aggregate relevance score (0-5 scale).
        Uses embedding similarity between query and top results.
        Article: relevance < 2.0 triggers escalation to next tier.
        """
        if not results:
            return 0.0

        # Use existing scores (vector similarity, rerank, cosine)
        scores = []
        for doc in results[:10]:
            # Prefer rerank_score > cosine_score > score
            s = doc.get('rerank_score') or doc.get('cosine_score') or doc.get('score', 0)
            try:
                scores.append(float(s))
            except (ValueError, TypeError):
                continue

        if not scores:
            return 0.0

        # Normalize to 0-5 scale
        # rerank_score is typically -10 to 10, cosine is 0 to 1, vector score is 0 to 1
        avg_score = sum(scores) / len(scores)

        # Detect score range and normalize
        max_score = max(scores)
        if max_score > 1.0:
            # Rerank scores (-10 to 10 range)
            normalized = (avg_score + 10) / 4  # maps -10..10 to 0..5
        else:
            # Cosine/vector similarity (0..1)
            normalized = avg_score * 5  # maps 0..1 to 0..5

        return round(min(5.0, max(0.0, normalized)), 2)

    # ═══════════════════════════════════════════════════════════════
    # TIERED RETRIEVAL (vector -> hybrid -> full_document_read)
    # ═══════════════════════════════════════════════════════════════

    def vector_search_tier(
        self,
        query: str,
        vector_results: List[Dict[str, Any]],
        all_documents: List[Dict[str, Any]],
        top_k: int = 30,
        trace: Optional[PipelineTrace] = None,
    ) -> Tuple[List[Dict[str, Any]], float]:
        """Tier 1: Vector search + window retrieval + cosine re-ranking."""
        t0 = time.time()

        # Deduplicate
        seen = {}
        for doc in vector_results:
            doc_id = str(doc.get('id', ''))
            if doc_id and doc_id not in seen:
                seen[doc_id] = doc
        unique_docs = list(seen.values())

        if not unique_docs:
            return [], 0.0

        # Vector ranking by original score
        vector_ranked = sorted(unique_docs, key=lambda d: float(d.get('score', 0)), reverse=True)

        # Window retrieval
        top_for_window = vector_ranked[:15]
        windowed = self.apply_window_retrieval(top_for_window, all_documents or unique_docs, window=1)

        # Cosine re-ranking (article finding #4: beats RRF)
        results = self.cosine_rerank(query, windowed, top_k=top_k)

        duration = (time.time() - t0) * 1000
        relevance = self.compute_relevance_score(query, results)

        if trace:
            trace.tool_steps.append(ToolStep(
                tool_name="vector_search",
                results_count=len(results),
                relevance_score=relevance,
                duration_ms=round(duration, 1),
            ))

        print(f"  Tier 1 (vector): {len(results)} results, relevance={relevance}")
        return results, relevance

    def hybrid_search_tier(
        self,
        query: str,
        vector_results: List[Dict[str, Any]],
        all_documents: List[Dict[str, Any]],
        top_k: int = 30,
        trace: Optional[PipelineTrace] = None,
    ) -> Tuple[List[Dict[str, Any]], float]:
        """Tier 2: Hybrid search (vector + BM25 + cross-encoder reranking)."""
        t0 = time.time()

        # Deduplicate
        seen = {}
        for doc in vector_results:
            doc_id = str(doc.get('id', ''))
            if doc_id and doc_id not in seen:
                seen[doc_id] = doc
        unique_docs = list(seen.values())

        if not unique_docs:
            return [], 0.0

        all_docs_list = all_documents if all_documents else unique_docs

        # Window retrieval on top vector results
        vector_ranked = sorted(unique_docs, key=lambda d: float(d.get('score', 0)), reverse=True)
        windowed = self.apply_window_retrieval(vector_ranked[:15], all_docs_list, window=1)

        # Merge windowed + vector results (no dupes)
        windowed_ids = {str(d.get('id', '')) for d in windowed}
        all_candidates = list(windowed)
        for doc in unique_docs:
            if str(doc.get('id', '')) not in windowed_ids:
                all_candidates.append(doc)

        # BM25 keyword search
        bm25_ranked = self.bm25_search(query, all_candidates, top_k=len(all_candidates))

        # Cosine re-ranking on combined candidates (instead of RRF - article finding #4)
        cosine_ranked = self.cosine_rerank(query, all_candidates, top_k=top_k * 2)

        # Merge BM25 and cosine results with weighted scoring
        doc_scores: Dict[str, float] = {}
        doc_map: Dict[str, Dict[str, Any]] = {}

        for doc in cosine_ranked:
            doc_id = str(doc.get('id', ''))
            if not doc_id:
                continue
            cosine_s = doc.get('cosine_score', 0)
            doc_scores[doc_id] = doc_scores.get(doc_id, 0) + cosine_s * 0.7
            doc_map[doc_id] = doc

        for doc in bm25_ranked:
            doc_id = str(doc.get('id', ''))
            if not doc_id:
                continue
            bm25_s = doc.get('bm25_score', 0)
            # Normalize BM25 score
            max_bm25 = bm25_ranked[0].get('bm25_score', 1) if bm25_ranked else 1
            if max_bm25 > 0:
                norm_bm25 = bm25_s / max_bm25
            else:
                norm_bm25 = 0
            doc_scores[doc_id] = doc_scores.get(doc_id, 0) + norm_bm25 * 0.3
            if doc_id not in doc_map:
                doc_map[doc_id] = doc

        # Sort by combined score
        sorted_ids = sorted(doc_scores.keys(), key=lambda x: doc_scores[x], reverse=True)
        merged = []
        for doc_id in sorted_ids:
            doc = doc_map[doc_id].copy()
            doc['hybrid_score'] = doc_scores[doc_id]
            merged.append(doc)

        # Cross-encoder reranking on top candidates
        if settings.use_reranker:
            rerank_k = min(top_k * 2, len(merged))
            results = self.rerank_documents(query, merged[:rerank_k], top_k=top_k)
        else:
            results = merged[:top_k]

        duration = (time.time() - t0) * 1000
        relevance = self.compute_relevance_score(query, results)

        if trace:
            trace.tool_steps.append(ToolStep(
                tool_name="hybrid_search",
                results_count=len(results),
                relevance_score=relevance,
                duration_ms=round(duration, 1),
            ))

        print(f"  Tier 2 (hybrid): {len(results)} results, relevance={relevance}")
        return results, relevance

    def full_document_read_tier(
        self,
        query: str,
        all_documents: List[Dict[str, Any]],
        top_k: int = 50,
        trace: Optional[PipelineTrace] = None,
    ) -> Tuple[List[Dict[str, Any]], float]:
        """
        Tier 3: Full document read — return ALL chunks, sorted by file/index.
        Used for global/enumeration queries or when lower tiers fail.
        Article finding #6: cross-language global queries need full_document_read.
        """
        t0 = time.time()

        if not all_documents:
            return [], 0.0

        # Return all documents sorted by file and chunk index
        sorted_docs = sorted(
            all_documents,
            key=lambda d: (d.get('file_name', ''), int(d.get('chunk_index', 0) or 0))
        )

        results = sorted_docs[:top_k]

        duration = (time.time() - t0) * 1000
        # For full document read, relevance is always "high enough" since we return everything
        relevance = 4.0

        if trace:
            trace.tool_steps.append(ToolStep(
                tool_name="full_document_read",
                results_count=len(results),
                relevance_score=relevance,
                duration_ms=round(duration, 1),
            ))

        print(f"  Tier 3 (full_document_read): {len(results)} results")
        return results, relevance

    # ═══════════════════════════════════════════════════════════════
    # SELF-CORRECTION LOOP (article finding #8)
    # ═══════════════════════════════════════════════════════════════

    def generate_rephrased_queries(self, query: str) -> List[str]:
        """
        Generate rephrased versions of the query for self-correction attempts.
        Simple rule-based rephrasing (no LLM needed).
        """
        rephrased = []
        query_lang = detect_language(query)

        if query_lang == "ru":
            # Add explicit instruction prefix
            rephrased.append(f"Найди информацию: {query}")
            # Try making it more specific
            rephrased.append(f"Подробно ответь на вопрос: {query}")
        else:
            rephrased.append(f"Find information about: {query}")
            rephrased.append(f"Provide detailed answer: {query}")

        return rephrased

    def self_correction_search(
        self,
        query: str,
        vector_results: List[Dict[str, Any]],
        all_documents: List[Dict[str, Any]],
        router_decision: RouterDecision,
        top_k: int = 30,
        max_attempts: int = 3,
        trace: Optional[PipelineTrace] = None,
    ) -> List[Dict[str, Any]]:
        """
        Self-correction loop with best_results tracking.
        Article finding #8: each attempt must NOT overwrite previous best.
        Tracks best_results and best_score across all attempts.
        """
        best_results: List[Dict[str, Any]] = []
        best_score: float = 0.0
        current_query = query

        for attempt in range(max_attempts):
            print(f"\n  === Attempt {attempt + 1}/{max_attempts}: '{current_query[:60]}' ===")

            # Choose retrieval tier based on router decision and attempt number
            if router_decision.suggested_tool == "full_document_read" or attempt >= 2:
                results, relevance = self.full_document_read_tier(
                    current_query, all_documents, top_k=top_k, trace=trace
                )
            elif router_decision.suggested_tool == "hybrid_search" or attempt >= 1:
                results, relevance = self.hybrid_search_tier(
                    current_query, vector_results, all_documents, top_k=top_k, trace=trace
                )
            else:
                results, relevance = self.vector_search_tier(
                    current_query, vector_results, all_documents, top_k=top_k, trace=trace
                )

            if trace:
                trace.self_correction_attempts.append(SelfCorrectionAttempt(
                    attempt_number=attempt + 1,
                    rephrased_query=current_query,
                    score=relevance,
                    results_count=len(results),
                ))

            # Article finding #8: keep best results across attempts
            if relevance > best_score:
                best_score = relevance
                best_results = results
                print(f"  New best score: {best_score}")

            # Check if score is good enough (> escalation threshold)
            if relevance >= RELEVANCE_ESCALATION_THRESHOLD:
                print(f"  Score {relevance} >= threshold {RELEVANCE_ESCALATION_THRESHOLD}, stopping")
                break

            # Escalate: add escalation step to trace
            if trace and attempt < max_attempts - 1:
                current_tool = router_decision.suggested_tool
                if attempt == 0:
                    next_tool = "hybrid_search"
                else:
                    next_tool = "full_document_read"
                trace.escalation_steps.append(EscalationStep(
                    from_tool=current_tool,
                    to_tool=next_tool,
                    reason=f"Relevance {relevance} < threshold {RELEVANCE_ESCALATION_THRESHOLD}",
                    previous_score=relevance,
                ))

            # Generate rephrased query for next attempt
            rephrased = self.generate_rephrased_queries(current_query)
            if attempt < len(rephrased):
                current_query = rephrased[attempt]
            # After rephrasing, escalate the tool
            if attempt == 0:
                router_decision.suggested_tool = "hybrid_search"
            elif attempt == 1:
                router_decision.suggested_tool = "full_document_read"

        if trace:
            trace.best_score = best_score

        print(f"\n  Final best score: {best_score}, results: {len(best_results)}")
        return best_results

    # ═══════════════════════════════════════════════════════════════
    # ENUMERATION / GENERATION PROMPTS (article findings #9, #10)
    # ═══════════════════════════════════════════════════════════════

    def build_system_prompt(
        self,
        query: str,
        router_decision: RouterDecision,
        base_prompt: str = "",
    ) -> str:
        """
        Build system prompt based on query type.
        Article finding #9: enumeration needs special prompt format.
        Article finding #10: judge limit should be 2000 chars, not 500.
        """
        if router_decision.is_enumeration or router_decision.query_type == "enumeration":
            # Special enumeration prompt (article finding #9)
            enum_instruction = (
                "IMPORTANT: The user is asking for an enumeration/list. "
                "Output a NUMBERED LIST. "
                "Scan ALL provided context chunks thoroughly. "
                "Do NOT stop early — include every matching item. "
                "If there are many items, list them ALL. "
                "Format: 1. item\\n2. item\\n..."
            )
            if base_prompt:
                return f"{enum_instruction}\n\n{base_prompt}"
            return enum_instruction

        elif router_decision.query_type == "global":
            global_instruction = (
                "The user is asking a global/overview question about the document(s). "
                "Provide a comprehensive answer covering ALL relevant aspects. "
                "Scan ALL context chunks to ensure completeness. "
                "Do not omit important details."
            )
            if base_prompt:
                return f"{global_instruction}\n\n{base_prompt}"
            return global_instruction

        elif router_decision.query_type == "multi_hop":
            multi_hop_instruction = (
                "The user is asking a multi-hop question that requires connecting "
                "information from multiple parts of the document(s). "
                "Trace the logical chain step by step."
            )
            if base_prompt:
                return f"{multi_hop_instruction}\n\n{base_prompt}"
            return multi_hop_instruction

        return base_prompt or ""

    # ═══════════════════════════════════════════════════════════════
    # MAIN PIPELINE: advanced_search (replaces old pipeline)
    # ═══════════════════════════════════════════════════════════════

    def advanced_search(
        self,
        bot_id: str,
        query: str,
        vector_results: List[Dict[str, Any]],
        top_k: int = 30,
        all_documents: Optional[List[Dict[str, Any]]] = None,
    ) -> Dict[str, Any]:
        """
        Full agentic RAG pipeline:
        1. Router classifies query
        2. Language mismatch detection
        3. Self-correction loop with tiered retrieval
        4. Build context with appropriate prompt
        5. Return results + trace + system prompt additions
        """
        pipeline_start = time.time()
        trace = PipelineTrace()

        print(f"\n{'='*60}")
        print(f"  AGENTIC RAG PIPELINE: '{query[:80]}'")
        print(f"{'='*60}")

        # Step 1: Route the query
        router_decision = self.classify_query(query)
        trace.router_step = router_decision
        print(f"  Router: type={router_decision.query_type}, "
              f"tool={router_decision.suggested_tool}, "
              f"lang={router_decision.detected_language}, "
              f"enum={router_decision.is_enumeration}")

        # Step 2: Language mismatch detection (article finding #1)
        all_docs_list = all_documents or vector_results
        lang_mismatch = self.detect_language_mismatch(query, all_docs_list)
        if lang_mismatch and router_decision.query_type in ("global", "enumeration"):
            # Article finding #6: cross-language global query -> force full_document_read
            print(f"  Cross-language global query detected, forcing full_document_read")
            router_decision.suggested_tool = "full_document_read"

        # Step 3: Self-correction loop with tiered retrieval
        results = self.self_correction_search(
            query=query,
            vector_results=vector_results,
            all_documents=all_docs_list,
            router_decision=router_decision,
            top_k=top_k,
            max_attempts=3,
            trace=trace,
        )

        # Step 4: Build context
        max_chars = 120000
        context = self.build_context(query, results, max_chars)

        # Step 5: Build enhanced system prompt
        prompt_addition = self.build_system_prompt(query, router_decision)

        # Final timing
        trace.total_duration_ms = round((time.time() - pipeline_start) * 1000, 1)

        print(f"\n  Pipeline done: {len(results)} results, "
              f"{len(context)} chars context, "
              f"{trace.total_duration_ms}ms total")

        return {
            "results": results,
            "compressed_context": context,
            "num_results": len(results),
            "trace": trace.to_dict(),
            "prompt_addition": prompt_addition,
            "router_decision": asdict(router_decision),
        }

    # ─── Context Building ──────────────────────────────────────────

    def build_context(
        self,
        query: str,
        documents: List[Dict[str, Any]],
        max_chars: int = 120000,
        max_docs: int = 50
    ) -> str:
        """
        Build context for LLM.
        Groups chunks by document, sorts by chunk_index.
        Article finding #10: increased context limit (was 500, now 2000+).
        """
        if not documents:
            return ""

        # Group by file and sort by chunk_index
        file_groups: Dict[str, List[Dict[str, Any]]] = {}
        for doc in documents:
            fname = doc.get('file_name', 'unknown')
            if fname not in file_groups:
                file_groups[fname] = []
            file_groups[fname].append(doc)

        for fname in file_groups:
            file_groups[fname].sort(key=lambda d: int(d.get('chunk_index', 0) or 0))

        context_parts = []
        total_chars = 0
        seen_texts = set()
        doc_counter = 0

        # Order files by max relevance score
        def file_score(f):
            return max(
                float(d.get('rerank_score', 0) or d.get('cosine_score', 0) or d.get('hybrid_score', 0) or d.get('score', 0) or 0)
                for d in file_groups[f]
            )

        sorted_files = sorted(file_groups.keys(), key=file_score, reverse=True)

        for fname in sorted_files:
            chunks = file_groups[fname]

            for doc in chunks:
                text = doc.get('text', '').strip()
                if not text:
                    continue

                text_key = text[:500]
                if text_key in seen_texts:
                    continue
                seen_texts.add(text_key)

                if doc_counter >= max_docs:
                    break

                chunk_idx = doc.get('chunk_index', '')
                header = f"[{fname}"
                if chunk_idx:
                    header += f" chunk {chunk_idx}"
                header += "]"

                formatted = f"{header}\n{text}"

                if total_chars + len(formatted) > max_chars:
                    remaining = max_chars - total_chars
                    if remaining > 300:
                        context_parts.append(formatted[:remaining] + "...")
                    break

                context_parts.append(formatted)
                total_chars += len(formatted)
                doc_counter += 1

        context = '\n\n'.join(context_parts)
        print(f"  Context: {doc_counter} docs, {total_chars} chars")
        return context


# Singleton
rag_service = RAGService()
