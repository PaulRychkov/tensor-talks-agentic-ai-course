"""HTTP client for Session Service"""

import httpx
from typing import Optional, Dict, Any
from ..config import settings
from ..logger import get_logger

logger = get_logger(__name__)


class SessionServiceClient:
    """Client for interacting with Session Service"""

    def __init__(self, base_url: Optional[str] = None):
        self.base_url = base_url or settings.session_service_url
        self.client = httpx.AsyncClient(timeout=30.0)
        logger.info("SessionServiceClient initialized", base_url=self.base_url)

    async def get_program(self, session_id: str) -> Optional[Dict[str, Any]]:
        """Get interview program for session"""
        try:
            response = await self.client.get(
                f"{self.base_url}/sessions/{session_id}/program"
            )
            response.raise_for_status()
            data = response.json()
            return data.get("program")
        except httpx.HTTPStatusError as e:
            if e.response.status_code == 404:
                logger.warning(
                    "Program not found for session",
                    session_id=session_id,
                    status_code=404,
                )
                return None
            logger.error(
                "Failed to get program",
                session_id=session_id,
                status_code=e.response.status_code,
                error=str(e),
            )
            raise
        except Exception as e:
            logger.error("Failed to get program", session_id=session_id, error=str(e))
            raise

    async def close_session(self, session_id: str) -> bool:
        """Close session"""
        try:
            response = await self.client.put(f"{self.base_url}/sessions/{session_id}/close")
            response.raise_for_status()
            return True
        except Exception as e:
            logger.error("Failed to close session", session_id=session_id, error=str(e))
            return False

    async def close(self):
        """Close HTTP client"""
        await self.client.aclose()
