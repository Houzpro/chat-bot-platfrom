"""Fire N parallel chat requests at the public RAG endpoint, time each one,
print per-request and total wall-clock so you can see how throughput
changes vs a single sequential request.

Usage:
    python scripts/bench_parallel.py <bot_id> [num_parallel]
Example:
    python scripts/bench_parallel.py 01221e70-9935-45bd-b710-6de81597cd36 2

No external deps — only the standard library, so it runs on a stock
Python install on any platform.
"""
from __future__ import annotations

import json
import os
import sys
import threading
import time
import urllib.request

HOST = os.environ.get("HOST", "http://localhost:8080")

# Slightly different prompts so caching can't help us. Each is long enough
# that response time is dominated by generation, not setup.
PROMPTS = [
    "Расскажи подробно историю развития машинного обучения, начиная с перцептрона Розенблатта в 1958 году. Опиши ключевые вехи и прорывы.",
    "Объясни шаг за шагом, как работает алгоритм обратного распространения ошибки в нейронных сетях. Приведи математические формулы.",
    "Опиши архитектуру трансформеров. Что такое механизм внимания? Чем отличается self-attention от cross-attention?",
    "Расскажи о современных языковых моделях: GPT, BERT, T5. Чем они отличаются по архитектуре и применению?",
    "Что такое LoRA и QLoRA? Как они работают, какие у них преимущества при дообучении больших моделей?",
    "Объясни принципы работы RAG (Retrieval-Augmented Generation). Какие компоненты входят в типичную RAG-систему?",
    "Что такое квантизация моделей? Чем отличаются Q4_K_M, Q5_K_M, Q8_0? Где используется GGUF формат?",
    "Расскажи про контекстное окно языковых моделей. Что такое KV-кэш и почему он важен для производительности?",
    "Опиши техники few-shot и zero-shot prompting. В чём отличие от fine-tuning, когда что лучше использовать?",
    "Что такое semantic search? Как работают векторные эмбеддинги, какую роль играют модели типа sentence-transformers?",
]


def fire(slot: int, bot_id: str, prompt: str, results: list) -> None:
    """One streaming POST. We consume the SSE stream until [DONE] so the
    timing reflects how long the model actually took to finish, not just
    how long the first byte took to arrive.

    `results[slot]` is filled with a dict so the main thread can print
    everything in deterministic order after join().
    """
    body = json.dumps({"query": prompt, "limit": 10}).encode("utf-8")
    req = urllib.request.Request(
        f"{HOST}/api/v1/chat/public/{bot_id}",
        data=body,
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    start = time.perf_counter()
    bytes_seen = 0
    tokens_seen = 0  # rough: count "data: {...token...}" frames
    error: str | None = None
    try:
        with urllib.request.urlopen(req, timeout=600) as resp:
            # Stream line by line; SSE frames are "data: ..."
            for raw in resp:
                bytes_seen += len(raw)
                line = raw.decode("utf-8", errors="replace").rstrip()
                if not line.startswith("data: "):
                    continue
                payload = line[len("data: "):]
                if payload == "[DONE]":
                    break
                # Don't bother parsing JSON for every frame; just count
                # frames as a stand-in for tokens (close enough for
                # comparing one run to another).
                if '"type":"token"' in payload or '"type": "token"' in payload:
                    tokens_seen += 1
    except Exception as e:
        error = str(e)

    elapsed_ms = int((time.perf_counter() - start) * 1000)
    results[slot] = {
        "elapsed_ms": elapsed_ms,
        "bytes": bytes_seen,
        "tokens": tokens_seen,
        "error": error,
    }


def main() -> int:
    if len(sys.argv) < 2:
        print("Usage: python scripts/bench_parallel.py <bot_id> [num_parallel]", file=sys.stderr)
        return 1
    bot_id = sys.argv[1]
    n = int(sys.argv[2]) if len(sys.argv) > 2 else 2

    print(f"Firing {n} parallel requests against {HOST} (bot={bot_id})...\n")

    results: list = [None] * n
    threads: list[threading.Thread] = []

    overall_start = time.perf_counter()

    # Launch all N at (close to) the same instant — start() returns fast
    # so the threads enter their HTTP send within microseconds of each other.
    for i in range(n):
        prompt = PROMPTS[i % len(PROMPTS)]
        t = threading.Thread(target=fire, args=(i, bot_id, prompt, results), daemon=True)
        threads.append(t)
        t.start()

    for t in threads:
        t.join()

    overall_ms = int((time.perf_counter() - overall_start) * 1000)

    print("Per-request timings:")
    total_individual = 0
    total_tokens = 0
    for i, r in enumerate(results):
        if r is None:
            print(f"  slot {i}: <no result>")
            continue
        if r["error"]:
            print(f"  slot {i}: ERROR after {r['elapsed_ms']} ms — {r['error']}")
            continue
        print(
            f"  slot {i}: {r['elapsed_ms']:6d} ms  "
            f"({r['tokens']:4d} tokens, {r['bytes']:6d} bytes)"
        )
        total_individual += r["elapsed_ms"]
        total_tokens += r["tokens"]

    print()
    print(f"Wall clock (max of all): {overall_ms} ms")
    if total_individual:
        print(f"Sum of individual times: {total_individual} ms")
        print(f"Average per request:     {total_individual // n} ms")
    if total_tokens and overall_ms:
        # Aggregate tokens-per-second across all slots — useful to compare
        # parallel=1 vs parallel=2 throughput at a glance.
        agg_tps = total_tokens * 1000 / overall_ms
        print(f"Aggregate throughput:    {agg_tps:.1f} tok/s ({total_tokens} tokens)")
    print()
    print("Compare to a single request to see throughput cost:")
    print(f"  python scripts/bench_parallel.py {bot_id} 1")
    return 0


if __name__ == "__main__":
    sys.exit(main())
