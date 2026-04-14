"""Agent state schema for LangGraph

Extended state with per-question counters, limits, and structured evaluations
(§9 p.2.2.1).
"""

from typing import TypedDict, List, Optional, Literal
from datetime import datetime

from pydantic import BaseModel, Field, field_validator

from ..config import settings

# Imported from config to allow env-override (§10.13)
MAX_HINTS_PER_QUESTION = settings.max_hints_per_question
MAX_ATTEMPTS_PER_QUESTION = settings.max_attempts_per_question
MAX_TOOL_CALLS_PER_STEP = settings.max_tool_calls_per_step


class QuestionEvaluation(BaseModel):
    """Structured evaluation for a single question (§9 p.2.2.2.3, §10.5).

    Migrated from TypedDict to Pydantic BaseModel for runtime validation.
    LangGraph state stores List[QuestionEvaluation]; each element is validated
    on creation via model_validate().
    """

    question_id: str = ""
    score: float = Field(default=0.0, ge=0.0, le=1.0)
    decision: str = ""
    attempts: int = Field(default=0, ge=0)
    hints_used: int = Field(default=0, ge=0)
    topic: Optional[str] = None
    decision_confidence: float = Field(default=1.0, ge=0.0, le=1.0)

    @field_validator("score", mode="before")
    @classmethod
    def clamp_score(cls, v: float) -> float:
        return max(0.0, min(1.0, float(v)))


class AgentState(TypedDict):
    """Agent state in LangGraph"""

    # Входные данные
    chat_id: str
    session_id: str
    user_id: str
    message_id: str
    user_message: str
    message_timestamp: datetime
    question_id: str

    # История диалога
    dialogue_history: List[dict]
    dialogue_state: Optional[dict]

    # Программа интервью
    interview_program: Optional[dict]
    session_mode: Optional[str]  # interview, training, study
    total_questions: int
    current_question_index: Optional[int]
    current_question_id: Optional[str]
    current_question: Optional[dict]
    current_theory: Optional[str]

    # Per-question counters (§9 p.2.2.1)
    attempts: int
    hints_given: int
    tool_calls: int

    # Study-mode point hierarchy counters (reset when moving to next question)
    clarifications_in_question: int

    # Accumulated evaluations across all questions
    evaluations: List[QuestionEvaluation]

    # Last decision for tracing
    last_decision: Optional[str]

    # Оценка ответа
    answer_evaluation: Optional[dict]

    # Решение агента (§9 p.2.2.2)
    agent_decision: Optional[
        Literal[
            "next",
            "hint",
            "clarify",
            "redirect",
            "skip",
            "ask_clarification",
            "give_hint",
            "next_question",
            "off_topic_reminder",
            "thank_you",
            "error",
            "blocked_pii",
        ]
    ]

    # Специальные флаги обработки
    hint_request: Optional[bool]
    skip_question_request: Optional[bool]

    # Сгенерированный ответ
    generated_response: Optional[str]
    generated_question: Optional[str]
    response_metadata: Optional[dict]

    # PII masking: masked version of user_message (fragments replaced with placeholders)
    pii_masked_content: Optional[str]

    # Episodic memory context (§10.6) — previous topic scores from past sessions.
    # Loaded during load_context when use_previous_results=True in session params.
    # None = not loaded yet; {} = loaded but no history found.
    previous_topic_scores: Optional[dict]

    # Метаданные обработки
    processing_steps: List[str]
    error: Optional[str]
    retry_count: int

    # ReAct agent loop transient fields (§8 stage 3) — MUST be declared in
    # TypedDict so LangGraph preserves them between nodes. Without these the
    # conditional `agent_route` never sees `_skip_agent_loop=True` and the loop
    # runs until the recursion limit (GraphRecursionError).
    _agent_messages: List[dict]
    _agent_iterations: int
    _skip_agent_loop: bool
    _last_eval_score: Optional[float]
