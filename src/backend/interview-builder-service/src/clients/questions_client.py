"""HTTP client for Questions CRUD Service"""

import httpx
from typing import List, Dict, Any, Optional
from ..config import settings
from ..logger import get_logger

logger = get_logger(__name__)


class QuestionsClient:
    """Client for interacting with Questions CRUD Service"""

    def __init__(self, base_url: Optional[str] = None):
        self.base_url = base_url or settings.questions_crud_url

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

            async with httpx.AsyncClient(timeout=30.0) as client:
                response = await client.get(
                    f"{self.base_url}/questions",
                    params=params
                )
            response.raise_for_status()
            data = response.json()
            questions = data.get("questions", [])
            # Debug: log first question structure
            if questions and len(questions) > 0:
                first_q = questions[0]
                logger.warn("QuestionsClient: First question structure",
                           question_type=type(first_q).__name__,
                           is_dict=isinstance(first_q, dict),
                           keys=list(first_q.keys())[:20] if isinstance(first_q, dict) else [],
                           has_data="Data" in first_q if isinstance(first_q, dict) else False,
                           has_lowercase_data="data" in first_q if isinstance(first_q, dict) else False)
                if isinstance(first_q, dict) and "Data" in first_q:
                    data_field = first_q.get("Data", {})
                    if isinstance(data_field, dict):
                        logger.warn("QuestionsClient: Data field structure",
                                   data_keys=list(data_field.keys())[:20],
                                   has_content="content" in data_field)
                        if "content" in data_field:
                            content = data_field.get("content", {})
                            if isinstance(content, dict):
                                logger.warn("QuestionsClient: Content field structure",
                                           content_keys=list(content.keys()),
                                           has_question="question" in content)
            return questions
        except Exception as e:
            logger.error("Failed to get questions by filters", error=str(e))
            raise

    async def close(self):
        """No-op: connections are created per-request"""
        pass

