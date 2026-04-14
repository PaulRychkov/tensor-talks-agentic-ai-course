"""Analyst agent state for LangGraph (§9 p.2.7, §8 stage 4)."""

from typing import Any, Dict, List, Optional, TypedDict


class AnalystState(TypedDict):
    """Full mutable state for one analyst graph invocation."""

    # ── Session metadata ──────────────────────────────────────────
    session_id: str
    session_kind: str   # interview | training | study
    user_id: str
    chat_id: str
    topics: List[str]
    level: str
    terminated_early: bool
    answered_questions: Optional[int]
    total_questions: Optional[int]

    # ── Fetched data ──────────────────────────────────────────────
    chat_messages: List[Dict[str, Any]]     # raw chat transcript
    program: Optional[Dict[str, Any]]       # session interview program
    evaluations: List[Dict[str, Any]]       # per-question evaluations

    # ── Intermediate analysis ─────────────────────────────────────
    errors_by_topic: Dict[str, List[str]]   # topic → [question texts with errors]

    # ── Report draft & validation ─────────────────────────────────
    report: Optional[Dict[str, Any]]        # LLM-generated report JSON
    validation_result: Optional[Dict[str, Any]]
    validation_attempts: int
    max_validation_attempts: int

    # ── Final outputs ─────────────────────────────────────────────
    presets: Optional[List[Dict[str, Any]]]
    topic_progress: Optional[List[Dict[str, Any]]]

    # ── Control ───────────────────────────────────────────────────
    error: Optional[str]

    # ── ReAct agent loop transient fields ─────────────────────────
    # MUST be declared so LangGraph preserves them between nodes.
    _agent_messages: List[Dict[str, Any]]
    _agent_iterations: int
    _skip_agent_loop: bool
