"""Strict schemas for interview build request/response contracts"""

from __future__ import annotations

from enum import Enum
from typing import Any, Dict, List, Optional

from pydantic import BaseModel, Field, field_validator


class SessionMode(str, Enum):
    INTERVIEW = "interview"
    TRAINING = "training"
    STUDY = "study"


class SessionSource(str, Enum):
    MANUAL = "manual"
    PRESET = "preset"


SUPPORTED_LEVELS = {"junior", "middle", "senior"}


class BuildRequestParams(BaseModel):
    """Normalized parameters for interview build request."""

    mode: SessionMode = SessionMode.INTERVIEW
    source: SessionSource = SessionSource.MANUAL
    type: Optional[str] = Field(None, description="Specialty / discipline, e.g. ml, nlp, llm")
    level: str = "middle"
    topics: List[str] = Field(default_factory=list)
    subtopics: Optional[List[str]] = None
    preset_id: Optional[str] = None
    use_previous_results: bool = False
    user_id: Optional[str] = None
    num_questions: Optional[int] = Field(None, description="Override question count for interview mode (e.g. 5, 10, 15)")

    @field_validator("level")
    @classmethod
    def validate_level(cls, v: str) -> str:
        v = v.lower().strip()
        if v not in SUPPORTED_LEVELS:
            raise ValueError(f"Unsupported level '{v}', expected one of {SUPPORTED_LEVELS}")
        return v

    @field_validator("topics")
    @classmethod
    def validate_topics(cls, v: List[str]) -> List[str]:
        cleaned = [t.strip() for t in v if t and t.strip()]
        if not cleaned:
            raise ValueError("topics must contain at least one non-empty value")
        return cleaned


class BuildRequest(BaseModel):
    """Validated build request extracted from Kafka event payload."""

    session_id: str
    params: BuildRequestParams

    @field_validator("session_id")
    @classmethod
    def validate_session_id(cls, v: str) -> str:
        if not v or not v.strip():
            raise ValueError("session_id must not be empty")
        return v.strip()

    @classmethod
    def from_raw_payload(cls, payload: Dict[str, Any]) -> "BuildRequest":
        """Normalize raw Kafka event payload into a validated BuildRequest.

        Handles legacy field names (``type`` used as mode) and missing optional
        fields by applying safe defaults.
        """
        session_id = payload.get("session_id", "")
        raw_params = payload.get("params", {})
        if not isinstance(raw_params, dict):
            raw_params = {}

        mode = raw_params.get("mode")
        if mode is None:
            legacy_type = raw_params.get("type", "")
            if legacy_type in {m.value for m in SessionMode}:
                mode = legacy_type
            else:
                mode = SessionMode.INTERVIEW.value

        source = raw_params.get("source", SessionSource.MANUAL.value)

        topics_raw = raw_params.get("topics", [])
        if isinstance(topics_raw, str):
            topics_raw = [topics_raw]

        params = BuildRequestParams(
            mode=mode,
            source=source,
            type=raw_params.get("type"),
            level=raw_params.get("level", "middle"),
            topics=topics_raw,
            subtopics=raw_params.get("subtopics") or raw_params.get("weak_topics"),
            preset_id=raw_params.get("preset_id"),
            use_previous_results=bool(raw_params.get("use_previous_results", False)),
            user_id=raw_params.get("user_id"),
            num_questions=raw_params.get("num_questions"),
        )

        return cls(session_id=session_id, params=params)


class ProgramQuestion(BaseModel):
    """Single question inside the interview program."""

    id: Optional[str] = None
    question: str = ""
    theory: str = ""
    order: int = 0
    topic: Optional[str] = None
    # Study-mode hierarchy: subtopic → point → question.
    # For interview/training these remain empty/None.
    subtopic: Optional[str] = None
    point_id: Optional[str] = None
    point_title: Optional[str] = None
    point_theory: Optional[str] = None
    question_in_point: int = 0  # 0-based index of this question within its point


class ProgramMeta(BaseModel):
    """Technical metadata about program build quality."""

    validation_passed: bool = True
    coverage: Dict[str, int] = Field(default_factory=dict)
    fallback_reason: Optional[str] = None
    generator_version: str = "planner-1.0.0"


class BuildResponse(BaseModel):
    """Structured response for interview.build.response event."""

    session_id: str
    interview_program: Dict[str, Any]
    program_meta: ProgramMeta

    def to_kafka_payload(self) -> Dict[str, Any]:
        return {
            "session_id": self.session_id,
            "interview_program": self.interview_program,
            "program_meta": self.program_meta.model_dump(),
        }
