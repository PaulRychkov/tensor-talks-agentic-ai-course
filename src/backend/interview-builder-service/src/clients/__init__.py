"""HTTP clients for CRUD services"""

from .questions_client import QuestionsClient
from .knowledge_client import KnowledgeClient

__all__ = ["QuestionsClient", "KnowledgeClient"]

