"""Planner agent state for LangGraph (§9 p.1, §8 stage 2)."""

from typing import Any, Dict, List, Optional, TypedDict


class PlannerState(TypedDict):
    """Full mutable state for one planning invocation."""

    # ── Request parameters ────────────────────────────────────────
    session_id: str
    mode: str            # interview | training | study
    topics: List[str]
    level: str           # junior | middle | senior
    weak_topics: List[str]
    n_questions: int

    # ── LLM conversation (OpenAI message-list format) ─────────────
    messages: List[Dict[str, Any]]

    # ── Accumulated tool results ──────────────────────────────────
    candidate_questions: List[Dict[str, Any]]
    coverage_report: Optional[Dict[str, Any]]
    validation_report: Optional[Dict[str, Any]]
    knowledge_snippets: List[Dict[str, Any]]

    # ── Final output ──────────────────────────────────────────────
    final_program: Optional[List[Dict[str, Any]]]
    program_meta: Optional[Dict[str, Any]]

    # ── Control ───────────────────────────────────────────────────
    iteration: int
    max_iterations: int
    error: Optional[str]
