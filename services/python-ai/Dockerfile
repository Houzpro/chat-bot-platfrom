FROM python:3.10-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

# Базовые зависимости для сборки некоторых пакетов и определения типов файлов
ARG APT_RETRIES=5
RUN set -eux; \
    apt-get -o Acquire::Retries=${APT_RETRIES} -o Acquire::http::Timeout=30 update; \
    apt-get -o Acquire::Retries=${APT_RETRIES} -o Acquire::http::Timeout=30 install -y --no-install-recommends \
        build-essential \
        libmagic1 \
    || apt-get -o Acquire::Retries=${APT_RETRIES} -o Acquire::http::Timeout=30 install -y --no-install-recommends --fix-missing \
        build-essential \
        libmagic1; \
    rm -rf /var/lib/apt/lists/*

COPY requirements.txt ./requirements.txt
RUN pip install --no-cache-dir --upgrade pip \
    && pip install --no-cache-dir -r requirements.txt

# Копируем код приложения
COPY app ./app
COPY start.sh ./start.sh
RUN chmod +x ./start.sh

EXPOSE 8000

# По умолчанию запускаем uvicorn через start.sh
CMD ["./start.sh"]
