"""HTTP client for Chat CRUD Service"""

import httpx
from typing import Optional, Dict, Any, List

from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector

logger = get_logger(__name__)


class ChatCrudClient:
    """Client for interacting with Chat CRUD Service"""

    def __init__(self, base_url: Optional[str] = None):
        self.base_url = base_url or settings.chat_crud_service_url
        self.client = httpx.AsyncClient(timeout=30.0)
        self.metrics = get_metrics_collector()
        logger.info("ChatCrudClient initialized", base_url=self.base_url)

    async def get_messages(self, chat_id: str) -> List[Dict[str, Any]]:
        """Get all messages for a chat session"""
        try:
            response = await self.client.get(
                f"{self.base_url}/messages/{chat_id}"
            )
            response.raise_for_status()
            self.metrics.http_requests_total.labels(
                service="chat-crud", method="GET", status="success"
            ).inc()
            data = response.json()
            messages = data.get("messages", data) if isinstance(data, dict) else data
            logger.info(
                "Messages fetched",
                chat_id=chat_id,
                message_count=len(messages) if isinstance(messages, list) else 0,
            )
            return messages if isinstance(messages, list) else []
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 404:
                logger.warning("Chat not found", chat_id=chat_id)
                return []
            self.metrics.http_requests_total.labels(
                service="chat-crud", method="GET", status="error"
            ).inc()
            logger.error(
                "Failed to get messages",
                chat_id=chat_id,
                status_code=e.response.status_code,
                error=str(e),
            )
            raise
        except Exception as e:
            self.metrics.http_requests_total.labels(
                service="chat-crud", method="GET", status="error"
            ).inc()
            logger.error("Failed to get messages", chat_id=chat_id, error=str(e))
            raise

    async def close(self):
        """Close HTTP client"""
        await self.client.aclose()
