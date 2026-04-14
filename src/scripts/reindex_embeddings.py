#!/usr/bin/env python3
"""Batch re-indexing script for knowledge base and question embeddings (§10.3).

Recalculates embeddings for all records when the embedding model changes.
Updates the embedding_metadata table to track the current model.

Usage:
  python scripts/reindex_embeddings.py \\
    --db-url "postgresql://user:pass@host/db" \\
    --embedding-api-url "https://api.openai.com/v1" \\
    --embedding-api-key "$OPENAI_API_KEY" \\
    --model "text-embedding-3-small" \\
    --dimension 1536 \\
    --service knowledge  # or 'questions' or 'all'
"""

import argparse
import asyncio
import json
import sys
import time
from typing import Any, Dict, List, Optional

try:
    import asyncpg
    import httpx
except ImportError:
    print("ERROR: Install dependencies: pip install asyncpg httpx", file=sys.stderr)
    sys.exit(1)


async def get_embedding(
    client: httpx.AsyncClient,
    text: str,
    api_url: str,
    api_key: str,
    model: str,
) -> Optional[List[float]]:
    """Call embedding API and return vector."""
    try:
        resp = await client.post(
            f"{api_url.rstrip('/')}/embeddings",
            json={"input": text, "model": model},
            headers={"Authorization": f"Bearer {api_key}"},
            timeout=30.0,
        )
        resp.raise_for_status()
        data = resp.json()
        return data["data"][0]["embedding"]
    except Exception as exc:
        print(f"  WARNING: embedding failed: {exc}")
        return None


async def reindex_knowledge(
    conn: "asyncpg.Connection",
    http_client: httpx.AsyncClient,
    args: argparse.Namespace,
) -> None:
    """Reindex all knowledge_segments in knowledge-base-crud-service."""
    rows = await conn.fetch(
        "SELECT id, content FROM knowledge_base_crud.knowledge_segments WHERE status = 'published'"
    )
    print(f"Reindexing {len(rows)} knowledge segments...")
    success = 0
    for i, row in enumerate(rows, 1):
        embedding = await get_embedding(
            http_client, row["content"], args.embedding_api_url, args.embedding_api_key, args.model
        )
        if embedding:
            await conn.execute(
                "UPDATE knowledge_base_crud.knowledge_segments SET embedding = $1 WHERE id = $2",
                embedding, row["id"],
            )
            success += 1
        if i % 10 == 0:
            pct = i / len(rows) * 100
            print(f"  Progress: {i}/{len(rows)} ({pct:.1f}%)")
        await asyncio.sleep(0.05)  # rate limiting
    print(f"Knowledge reindex complete: {success}/{len(rows)} successful")


async def reindex_questions(
    conn: "asyncpg.Connection",
    http_client: httpx.AsyncClient,
    args: argparse.Namespace,
) -> None:
    """Reindex all questions in questions-crud-service."""
    rows = await conn.fetch(
        "SELECT id, question_text, ideal_answer FROM questions_crud.questions WHERE status = 'published'"
    )
    print(f"Reindexing {len(rows)} questions...")
    success = 0
    for i, row in enumerate(rows, 1):
        q_embedding = await get_embedding(
            http_client, row["question_text"], args.embedding_api_url, args.embedding_api_key, args.model
        )
        a_embedding = None
        if row["ideal_answer"]:
            a_embedding = await get_embedding(
                http_client, row["ideal_answer"], args.embedding_api_url, args.embedding_api_key, args.model
            )
        if q_embedding:
            await conn.execute(
                "UPDATE questions_crud.questions SET embedding = $1, ideal_answer_embedding = $2 WHERE id = $3",
                q_embedding, a_embedding, row["id"],
            )
            success += 1
        if i % 10 == 0:
            pct = i / len(rows) * 100
            print(f"  Progress: {i}/{len(rows)} ({pct:.1f}%)")
        await asyncio.sleep(0.05)
    print(f"Questions reindex complete: {success}/{len(rows)} successful")


async def update_embedding_metadata(
    conn: "asyncpg.Connection",
    model: str,
    dimension: int,
) -> None:
    """Mark all previous embedding models as not current, then insert new record."""
    await conn.execute(
        "UPDATE knowledge_base_crud.embedding_metadata SET is_current = FALSE"
    )
    await conn.execute(
        """INSERT INTO knowledge_base_crud.embedding_metadata
           (model_name, model_version, dimension, is_current)
           VALUES ($1, $2, $3, TRUE)""",
        model, "v1", dimension,
    )
    print(f"Embedding metadata updated: model={model}, dimension={dimension}")


async def main() -> int:
    parser = argparse.ArgumentParser(description="Reindex embeddings for pgvector search")
    parser.add_argument("--db-url", required=True, help="PostgreSQL connection URL")
    parser.add_argument("--embedding-api-url", required=True, help="Embedding API base URL")
    parser.add_argument("--embedding-api-key", required=True, help="Embedding API key")
    parser.add_argument("--model", default="text-embedding-3-small", help="Embedding model name")
    parser.add_argument("--dimension", type=int, default=1536, help="Embedding dimension")
    parser.add_argument(
        "--service",
        choices=["knowledge", "questions", "all"],
        default="all",
        help="Which service to reindex",
    )
    args = parser.parse_args()

    conn = await asyncpg.connect(args.db_url)
    try:
        async with httpx.AsyncClient() as http_client:
            if args.service in ("knowledge", "all"):
                await reindex_knowledge(conn, http_client, args)
            if args.service in ("questions", "all"):
                await reindex_questions(conn, http_client, args)
            if args.service == "all":
                await update_embedding_metadata(conn, args.model, args.dimension)
    finally:
        await conn.close()

    print("Reindexing complete.")
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
