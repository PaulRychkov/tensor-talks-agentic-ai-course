"""HTTP client for Questions CRUD Service"""

import httpx
from typing import Dict, Any, List, Optional
from ..config import settings
from ..logger import get_logger

logger = get_logger(__name__)


class QuestionsClient:
    """Client for interacting with Questions CRUD Service"""

    def __init__(self, base_url: Optional[str] = None):
        self.base_url = base_url or settings.questions_crud_url
        self.client = httpx.AsyncClient(timeout=30.0)

    async def get_question_by_id(self, question_id: str) -> Optional[Dict[str, Any]]:
        """Get question by ID"""
        try:
            response = await self.client.get(f"{self.base_url}/questions/{question_id}")
            if response.status_code == 200:
                data = response.json()
                return data.get("question")
            elif response.status_code == 404:
                return None
            else:
                response.raise_for_status()
        except Exception as e:
            logger.error("Failed to get question", question_id=question_id, error=str(e))
            raise

    async def create_question(self, question_data: Dict[str, Any]) -> Dict[str, Any]:
        """Create new question"""
        try:
            response = await self.client.post(
                f"{self.base_url}/questions",
                json=question_data
            )
            response.raise_for_status()
            data = response.json()
            return data.get("question")
        except Exception as e:
            logger.error("Failed to create question", question_id=question_data.get("id"), error=str(e))
            raise

    async def update_question(self, question_id: str, question_data: Dict[str, Any]) -> Dict[str, Any]:
        """Update existing question"""
        try:
            response = await self.client.put(
                f"{self.base_url}/questions/{question_id}",
                json=question_data
            )
            response.raise_for_status()
            data = response.json()
            return data.get("question")
        except Exception as e:
            logger.error("Failed to update question", question_id=question_id, error=str(e))
            raise

    async def get_questions_by_filters(
        self,
        complexity: Optional[int] = None,
        theory_id: Optional[str] = None,
        question_type: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """Get questions by filters"""
        try:
            params = {}
            if complexity is not None:
                params["complexity"] = complexity
            if theory_id:
                params["theory_id"] = theory_id
            if question_type:
                params["question_type"] = question_type

            response = await self.client.get(
                f"{self.base_url}/questions",
                params=params
            )
            response.raise_for_status()
            data = response.json()
            return data.get("questions", [])
        except Exception as e:
            logger.error("Failed to get questions by filters", error=str(e))
            raise

    async def close(self):
        """Close HTTP client"""
        await self.client.aclose()

