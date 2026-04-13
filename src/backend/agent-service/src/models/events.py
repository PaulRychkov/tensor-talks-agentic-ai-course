"""Event models for Kafka messages"""

from pydantic import BaseModel, Field
from datetime import datetime
from typing import Optional, Dict, Any, List
from enum import Enum


class EventType(str, Enum):
    """Типы событий"""

    MESSAGE_FULL = "message.full"
    PHRASE_AGENT_GENERATED = "phrase.agent.generated"


class KafkaEvent(BaseModel):
    """Базовая модель события Kafka"""

    event_id: str = Field(..., description="Уникальный ID события (UUID)")
    event_type: EventType = Field(..., description="Тип события")
    timestamp: datetime = Field(..., description="Время создания события (ISO 8601 UTC)")
    service: str = Field(..., description="Имя сервиса, создавшего событие")
    version: str = Field(..., description="Версия сервиса")
    payload: Dict[str, Any] = Field(..., description="Данные события")
    metadata: Optional[Dict[str, Any]] = Field(
        None, description="Дополнительные метаданные"
    )

    class Config:
        json_encoders = {datetime: lambda v: v.isoformat()}
        use_enum_values = True


class MessageRole(str, Enum):
    """Роли сообщений"""

    USER = "user"
    ASSISTANT = "assistant"
    SYSTEM = "system"


class MessageFullPayload(BaseModel):
    """Payload для messages.full.data"""

    chat_id: str
    message_id: str
    question_id: str
    role: MessageRole
    content: str
    metadata: Dict[str, Any]
    embeddings: Optional[List[float]] = None
    source: str
    timestamp: datetime
    processed_at: datetime

    class Config:
        json_encoders = {datetime: lambda v: v.isoformat()}
        use_enum_values = True


class PhraseAgentGeneratedPayload(BaseModel):
    """Payload для phrase.agent.generated"""

    chat_id: str
    message_id: str
    question_id: str
    generated_text: str
    confidence: Optional[float] = None
    intermediate_steps: Optional[List[Dict[str, Any]]] = None
    metadata: Optional[Dict[str, Any]] = None
    timestamp: datetime

    class Config:
        json_encoders = {datetime: lambda v: v.isoformat()}
