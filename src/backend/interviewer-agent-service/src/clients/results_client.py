"""HTTP client for results-crud-service (§10.6 episodic memory)."""

import asyncio
from typing import Any, Dict, Optional
import httpx

from ..config import settings
from ..logger import get_logger

logger = get_logger(__name__)


class ResultsClient:
    """Lightweight async client for results-crud-service episodic memory endpoints."""

    def __init__(self, base_url: Optional[str] = None):
        self.base_url = (base_url or "http://results-crud-service:8088").rstrip("/")

    async def get_topic_scores(
        self,
        user_id: str,
        topics: Optional[list] = None,
    ) -> Dict[str, float]:
        """Fetch average topic scores for a user from past sessions (§10.6).

        Returns a dict: {topic: avg_score (0.0-1.0)}.
        Returns {} on error — caller treats as no history.
        """
        params = {"user_id": user_id}
        if topics:
            params["topics"] = ",".join(topics)

        try:
            async with httpx.AsyncClient(timeout=3.0) as client:
                resp = await client.get(
                    f"{self.base_url}/results/topic-scores",
                    params=params,
                )
                if resp.status_code == 200:
                    data = resp.json()
                    return data.get("topic_scores", {})
                logger.debug(
                    "topic-scores returned non-200",
                    status=resp.status_code,
                    user_id=user_id,
                )
        except Exception as exc:
            logger.warning(
                "Failed to fetch topic scores from results-crud",
                user_id=user_id,
                error=str(exc),
            )
        return {}
