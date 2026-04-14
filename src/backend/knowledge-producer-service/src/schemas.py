"""Pydantic models for draft knowledge and question items (§9 p.5.7).

These mirror the Go CRUD models (KnowledgeJSONB, QuestionJSONB) and
generate JSON Schema for CI validation.
"""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Any, Dict, List, Optional

from pydantic import BaseModel, Field, field_validator

from .config import settings

ALLOWED_SEGMENT_TYPES = set(settings.allowed_segment_types.split(","))
ALLOWED_QUESTION_TYPES = set(settings.allowed_question_types.split(","))


class ReviewStatus(str, Enum):
    PENDING = "pending"
    APPROVED = "approved"
    REJECTED = "rejected"


class DraftKind(str, Enum):
    KNOWLEDGE = "knowledge"
    QUESTION = "question"


# ── Knowledge draft ──────────────────────────────────────────────


class KnowledgeSegment(BaseModel):
    type: str
    content: str
    order: int = 0

    @field_validator("type")
    @classmethod
    def validate_segment_type(cls, v: str) -> str:
        v = v.strip().lower()
        if v not in ALLOWED_SEGMENT_TYPES:
            raise ValueError(f"segment type '{v}' not in {ALLOWED_SEGMENT_TYPES}")
        return v


class KnowledgeRelation(BaseModel):
    target_id: str
    relation_type: str = "related"


class KnowledgeMetadata(BaseModel):
    source: Optional[str] = None
    version: Optional[str] = None
    tags: List[str] = Field(default_factory=list)


class KnowledgeDraftContent(BaseModel):
    """Mirrors Go KnowledgeJSONB."""

    title: str
    topic: str
    complexity: int = 2
    segments: List[KnowledgeSegment] = Field(default_factory=list, min_length=1)
    relations: List[KnowledgeRelation] = Field(default_factory=list)
    metadata: KnowledgeMetadata = Field(default_factory=KnowledgeMetadata)


# ── Question draft ───────────────────────────────────────────────


class QuestionMetadata(BaseModel):
    tags: List[str] = Field(default_factory=list)
    source: Optional[str] = None


class QuestionDraftContent(BaseModel):
    """Mirrors Go QuestionJSONB."""

    content: str = Field(..., min_length=5)
    ideal_answer: str = ""
    question_type: str = "open"
    complexity: int = 2
    theory_id: Optional[str] = None
    metadata: QuestionMetadata = Field(default_factory=QuestionMetadata)

    @field_validator("question_type")
    @classmethod
    def validate_question_type(cls, v: str) -> str:
        v = v.strip().lower()
        if v not in ALLOWED_QUESTION_TYPES:
            raise ValueError(f"question_type '{v}' not in {ALLOWED_QUESTION_TYPES}")
        return v


# ── Draft envelope ───────────────────────────────────────────────


class Draft(BaseModel):
    draft_id: str
    kind: DraftKind
    title: str
    topic: str
    content: Dict[str, Any]
    source: str = ""
    review_status: ReviewStatus = ReviewStatus.PENDING
    review_comment: Optional[str] = None
    reviewed_by: Optional[str] = None
    reviewed_at: Optional[datetime] = None
    published_at: Optional[datetime] = None
    duplicate_candidate: bool = False
    created_at: datetime = Field(default_factory=datetime.utcnow)


# ── Raw document (ingestion) ────────────────────────────────────


class RawDocument(BaseModel):
    """Intermediate representation after parsing any format."""

    source_uri: str
    mime: str
    text: str
    metadata: Dict[str, Any] = Field(default_factory=dict)
