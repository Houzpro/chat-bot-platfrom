FROM python:3.10-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

# PyTorch CPU-only (для sentence-transformers)
RUN pip install --no-cache-dir torch --index-url https://download.pytorch.org/whl/cpu

COPY requirements.txt ./requirements.txt
RUN pip install --no-cache-dir --upgrade pip \
    && pip install --no-cache-dir -r requirements.txt

# NLTK данные при сборке
RUN python -c "import nltk; nltk.download('punkt_tab', quiet=True)"

# Embedding/reranker модели НЕ скачиваются в билде — они монтируются как volume
# из ./models/embedding (cache_folder) и ./models/hf-cache (HF default cache).
# При первом запуске reranker подтянется в hf-cache и больше качаться не будет.

COPY app ./app

EXPOSE 8000

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "1", "--timeout-keep-alive", "300", "--log-level", "info"]
