"""Event models for Kafka messages"""

from pydantic import BaseModel, Field, field_validator
from datetime import datetime
from typing import Optional, Dict, Any, List
from enum import Enum


class EventType(str, Enum):
    SESSION_COMPLETED = "session.completed"


class SessionKind(str, Enum):
    INTERVIEW = "interview"
    TRAINING = "training"
    STUDY = "study"


class KafkaEvent(BaseModel):
    """Base Kafka event envelope"""

    event_id: str = Field(..., description="Unique event ID (UUID)")
    event_type: EventType = Field(..., description="Event type")
    timestamp: datetime = Field(..., description="Event creation time (ISO 8601 UTC)")
    service: str = Field(..., description="Source service name")
    version: str = Field(..., description="Source service version")
    payload: Dict[str, Any] = Field(..., description="Event data")
    trace_id: Optional[str] = Field(None, description="Distributed trace ID for cross-service correlation")
    metadata: Optional[Dict[str, Any]] = Field(None, description="Additional metadata")

    class Config:
        json_encoders = {datetime: lambda v: v.isoformat()}
        use_enum_values = True


class SessionCompletedPayload(BaseModel):
    """Payload for session.completed events"""

    session_id: str
    session_kind: SessionKind
    user_id: str
    chat_id: str
    topics: List[str] = Field(default_factory=list)

    @field_validator("topics", mode="before")
    @classmethod
    def coerce_topics(cls, v: Any) -> List[str]:
        if v is None:
            return []
        return v
    level: Optional[str] = None
    started_at: Optional[datetime] = None
    completed_at: Optional[datetime] = None
    terminated_early: bool = Field(default=False, description="Whether the user terminated the interview early")
    answered_questions: Optional[int] = Field(default=None, description="Number of questions the user answered")
    total_questions: Optional[int] = Field(default=None, description="Total number of questions in the program")

    class Config:
        json_encoders = {datetime: lambda v: v.isoformat()}
        use_enum_values = True
