"""HTTP client for Chat CRUD Service (message masking)"""

import httpx
from typing import Optional
from ..config import settings
from ..logger import get_logger

logger = get_logger(__name__)

_PII_REDACTED_PLACEHOLDER = "[Сообщение содержало конфиденциальную информацию и было отклонено]"


class ChatCrudClient:
    """Client for Chat CRUD Service — used to retroactively mask PII-containing messages."""

    def __init__(self, base_url: Optional[str] = None):
        self.base_url = base_url or settings.chat_crud_service_url
        self.client = httpx.AsyncClient(timeout=10.0)

    async def mask_message(self, message_id: int, placeholder: str = _PII_REDACTED_PLACEHOLDER) -> bool:
        """Replace message content with placeholder (§10.11 retroactive masking).

        Returns True on success, False if message not found or request failed.
        Errors are logged but never propagated — masking failure must not block agent flow.
        """
        if not message_id:
            return False
        try:
            response = await self.client.patch(
                f"{self.base_url}/messages/{message_id}/mask",
                json={"placeholder": placeholder},
            )
            if response.status_code == 200:
                logger.info("Message masked in chat-crud", message_id=message_id)
                return True
            if response.status_code == 404:
                logger.warning("Message not found for masking", message_id=message_id)
                return False
            logger.error(
                "Unexpected status from chat-crud mask endpoint",
                message_id=message_id,
                status_code=response.status_code,
            )
            return False
        except Exception as exc:
            logger.error("Failed to mask message in chat-crud", message_id=message_id, error=str(exc))
            return False

    async def close(self) -> None:
        await self.client.aclose()
