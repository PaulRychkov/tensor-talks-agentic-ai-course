"""HTTP client for Knowledge Base CRUD Service"""

import httpx
from typing import List, Dict, Any, Optional
from ..config import settings
from ..logger import get_logger

logger = get_logger(__name__)


class KnowledgeClient:
    """Client for interacting with Knowledge Base CRUD Service"""

    def __init__(self, base_url: Optional[str] = None):
        self.base_url = base_url or settings.knowledge_base_crud_url
        self.client = httpx.AsyncClient(timeout=30.0)

    async def get_knowledge_by_id(self, knowledge_id: str) -> Optional[Dict[str, Any]]:
        """Get knowledge by ID"""
        try:
            response = await self.client.get(f"{self.base_url}/knowledge/{knowledge_id}")
            if response.status_code == 200:
                data = response.json()
                return data.get("knowledge")
            elif response.status_code == 404:
                return None
            else:
                response.raise_for_status()
        except Exception as e:
            logger.error("Failed to get knowledge", knowledge_id=knowledge_id, error=str(e))
            raise

    async def get_knowledge_by_filters(
        self,
        complexity: Optional[int] = None,
        concept: Optional[str] = None,
        parent_id: Optional[str] = None,
        tags: Optional[List[str]] = None,
    ) -> List[Dict[str, Any]]:
        """Get knowledge by filters"""
        try:
            params = {}
            if complexity is not None:
                params["complexity"] = complexity
            if concept:
                params["concept"] = concept
            if parent_id:
                params["parent_id"] = parent_id
            if tags:
                params["tags"] = tags

            response = await self.client.get(
                f"{self.base_url}/knowledge",
                params=params
            )
            response.raise_for_status()
            data = response.json()
            return data.get("knowledge", [])
        except Exception as e:
            logger.error("Failed to get knowledge by filters", error=str(e))
            raise

    async def close(self):
        """Close HTTP client"""
        await self.client.aclose()

