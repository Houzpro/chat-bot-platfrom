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

# Скачиваем embedding и reranker модели при сборке образа,
# чтобы контейнер работал полностью автономно без интернета
ARG EMBEDDING_MODEL=intfloat/multilingual-e5-base
ARG RERANKER_MODEL=cross-encoder/ms-marco-MiniLM-L-6-v2

RUN python -c "\
from sentence_transformers import SentenceTransformer, CrossEncoder; \
SentenceTransformer('${EMBEDDING_MODEL}', cache_folder='/app/models/embedding'); \
CrossEncoder('${RERANKER_MODEL}', max_length=512); \
print('Models downloaded successfully')"

COPY app ./app

EXPOSE 8000

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000", "--workers", "1", "--timeout-keep-alive", "300", "--log-level", "info"]
