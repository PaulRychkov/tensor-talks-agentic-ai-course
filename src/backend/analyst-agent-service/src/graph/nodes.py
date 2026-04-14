"""LangGraph nodes for the analyst agent (§8 stage 4, §9 p.2.7).

Graph flow:
  fetch_data → evaluate_answers → generate_report → validate_report
             → [pass]  → save_results → END
             → [fail, attempts < max] → generate_report (retry)
"""

from __future__ import annotations

import json
from typing import Any, Dict

from ..graph.state import AnalystState
from ..logger import get_logger

logger = get_logger(__name__)

# ── Global references (injected by builder) ───────────────────────────────────
_analyst_service = None


def set_analyst_service(analyst_service) -> None:
    global _analyst_service
    _analyst_service = analyst_service


# ── Nodes ─────────────────────────────────────────────────────────────────────


async def fetch_data(state: AnalystState) -> AnalystState:
    """Fetch chat transcript and session program from external services."""
    from ..tools.analyst_tools import get_evaluations

    if not _analyst_service:
        return {**state, "error": "AnalystService not injected"}

    try:
        messages = await _analyst_service.chat_client.get_messages(state["chat_id"])
        if not messages:
            logger.warning("No messages found", session_id=state["session_id"])
            return {**state, "chat_messages": [], "error": "no_messages"}

        program = None
        try:
            program = await _analyst_service.session_client.get_program(state["session_id"])
        except Exception as exc:
            logger.warning("Could not fetch program", error=str(exc))

        return {**state, "chat_messages": messages or [], "program": program, "error": None}

    except Exception as exc:
        logger.error("fetch_data failed", session_id=state["session_id"], error=str(exc))
        return {**state, "error": str(exc)}


async def evaluate_answers(state: AnalystState) -> AnalystState:
    """Extract per-question evaluations and group errors by topic."""
    from ..tools.analyst_tools import get_evaluations, group_errors_by_topic

    if state.get("error"):
        return state

    try:
        evaluations = await get_evaluations(state["session_id"])
        errors = group_errors_by_topic(evaluations)
        return {**state, "evaluations": evaluations, "errors_by_topic": errors}
    except Exception as exc:
        logger.warning("evaluate_answers failed (non-fatal)", error=str(exc))
        return {**state, "evaluations": [], "errors_by_topic": {}}


async def generate_report(state: AnalystState) -> AnalystState:
    """Call the LLM to produce the structured report JSON.

    On retry (validation_attempts > 0) the previous validation issues are
    appended to the prompt so the LLM knows what to fix.
    """
    if state.get("error") == "no_messages":
        return state  # skip – nothing to analyse

    if not _analyst_service:
        return {**state, "error": "AnalystService not injected"}

    try:
        from ..models.events import SessionCompletedPayload, SessionKind

        # Build a minimal payload object so we can reuse existing _generate_report
        payload = SessionCompletedPayload(
            session_id=state["session_id"],
            session_kind=state["session_kind"],
            user_id=state["user_id"],
            chat_id=state["chat_id"],
            topics=state.get("topics", []),
            level=state.get("level", "middle"),
            terminated_early=state.get("terminated_early", False),
            answered_questions=state.get("answered_questions"),
            total_questions=state.get("total_questions"),
        )

        messages = state.get("chat_messages", [])
        program = state.get("program")

        # If this is a retry, inject previous validation issues into the prompt context
        prev_issues = []
        if state.get("validation_attempts", 0) > 0 and state.get("validation_result"):
            prev_issues = state["validation_result"].get("issues", [])

        if prev_issues:
            extra = (
                "\n\nPREVIOUS VALIDATION FAILED. Fix these issues:\n"
                + "\n".join(f"- {i}" for i in prev_issues)
            )
            # Append a correction instruction to the last user message (cosmetic patch)
            if messages:
                messages = list(messages) + [{"role": "user", "content": extra}]

        report = await _analyst_service._generate_report(payload, messages, program)

        if state["session_kind"] == SessionKind.STUDY and program:
            try:
                _analyst_service._attach_study_details(report, program)
            except Exception as exc:
                logger.warning(
                    "Failed to attach study details in graph path",
                    session_id=state["session_id"],
                    error=str(exc),
                )

        logger.info(
            "Report generated (agent node)",
            session_id=state["session_id"],
            score=report.get("score"),
            attempt=state.get("validation_attempts", 0),
        )
        return {**state, "report": report}

    except Exception as exc:
        logger.error("generate_report node failed", session_id=state["session_id"], error=str(exc))
        return {**state, "error": str(exc)}


async def run_validate_report(state: AnalystState) -> AnalystState:
    """Validate the current report draft using the validate_report tool."""
    from ..tools.analyst_tools import validate_report

    report = state.get("report") or {}
    evaluations = state.get("evaluations") or []
    result = validate_report(report, evaluations)

    logger.info(
        "Report validation",
        session_id=state["session_id"],
        passed=result["validation_passed"],
        issues=result.get("issues"),
        attempt=state.get("validation_attempts", 0),
    )
    return {
        **state,
        "validation_result": result,
        "validation_attempts": state.get("validation_attempts", 0) + 1,
    }


async def save_results(state: AnalystState) -> AnalystState:
    """Persist report, presets, and topic progress via the analyst service."""
    if not _analyst_service:
        return {**state, "error": "AnalystService not injected"}

    report = state.get("report") or {}
    if not report:
        logger.warning("save_results: empty report, skipping", session_id=state["session_id"])
        return state

    try:
        from ..models.events import SessionCompletedPayload, SessionKind

        payload = SessionCompletedPayload(
            session_id=state["session_id"],
            session_kind=state["session_kind"],
            user_id=state["user_id"],
            chat_id=state["chat_id"],
            topics=state.get("topics", []),
            level=state.get("level", "middle"),
            terminated_early=state.get("terminated_early", False),
            answered_questions=state.get("answered_questions"),
            total_questions=state.get("total_questions"),
        )

        # Always set total_questions from payload (authoritative source)
        if payload.total_questions is not None:
            report["total_questions"] = payload.total_questions

        # Early-termination metadata
        if payload.terminated_early:
            report["terminated_early"] = True
            if payload.answered_questions is not None:
                report["answered_questions"] = payload.answered_questions

        # For study sessions: deterministic follow-up (no LLM call needed)
        # For interview sessions: generate presets via LLM
        presets = None
        preset_training = None
        program = state.get("program")
        if state["session_kind"] == SessionKind.STUDY:
            try:
                preset_training = _analyst_service._maybe_build_study_followup(
                    payload, report, program,
                )
                if preset_training:
                    logger.info(
                        "Study follow-up preset generated (graph path)",
                        session_id=state["session_id"],
                        unmastered=preset_training.get("weak_topics"),
                    )
            except Exception as exc:
                logger.error("Study follow-up failed (non-fatal)", error=str(exc))
        elif state["session_kind"] == SessionKind.INTERVIEW:
            try:
                presets = await _analyst_service._generate_presets(payload, report)
                if presets:
                    weak_topics: list = []
                    for p in presets:
                        for t in p.get("weak_topics", []) + p.get("topics", []):
                            if t not in weak_topics:
                                weak_topics.append(t)
                    preset_training = {
                        "presets": presets,
                        "weak_topics": weak_topics,
                        "recommended_materials": report.get("materials", []),
                    }
            except Exception as exc:
                logger.error("Preset generation failed (non-fatal)", error=str(exc))

        await _analyst_service.results_client.save_report(
            session_id=payload.session_id,
            user_id=payload.user_id,
            session_kind=payload.session_kind,
            report_json=report,
            preset_training=preset_training,
        )

        topic_progress = None
        if state["session_kind"] == SessionKind.STUDY:
            try:
                topic_progress = _analyst_service._extract_topic_progress(report, payload)
                if topic_progress:
                    await _analyst_service.results_client.update_user_progress(
                        session_id=payload.session_id,
                        user_id=payload.user_id,
                        topic_progress=topic_progress,
                    )
            except Exception as exc:
                logger.error("Topic progress update failed (non-fatal)", error=str(exc))

        logger.info(
            "Results saved (agent graph)",
            session_id=state["session_id"],
            presets=len(presets) if presets else 0,
        )
        return {**state, "presets": presets, "topic_progress": topic_progress}

    except Exception as exc:
        logger.error("save_results failed", session_id=state["session_id"], error=str(exc))
        return {**state, "error": str(exc)}
