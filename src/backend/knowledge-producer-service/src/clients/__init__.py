"""HTTP clients for CRUD services"""

from .knowledge_client import KnowledgeClient
from .questions_client import QuestionsClient

__all__ = ["KnowledgeClient", "QuestionsClient"]

