#!/bin/bash
# Оптимизированный запуск с увеличенными таймаутами для длинных ответов
uvicorn app.main:app \
  --host 0.0.0.0 \
  --port 8000 \
  --workers 1 \
  --limit-concurrency 200 \
  --timeout-keep-alive 300 \
  --timeout-graceful-shutdown 30 \
  --backlog 2048 \
  --log-level info