"""LangGraph nodes for the interview planner agent (§8 stage 2, §9 p.1)."""

from __future__ import annotations

import json
from typing import Any, Dict, List

from ..graph.state import PlannerState
from ..logger import get_logger

logger = get_logger(__name__)

# ── Global clients (injected at startup) ─────────────────────────────────────
_llm_client = None


def set_planner_clients(llm_client) -> None:
    global _llm_client
    _llm_client = llm_client


# ── System prompt template ────────────────────────────────────────────────────

_SYSTEM_PROMPT = """\
You are an expert interview program planner for a technical ML/AI learning platform.

Task: assemble a {n_questions}-question {mode} program for a {level}-level session \
on topics: {topics}.{weak_hint}

Use the provided tools in this order:
1. search_questions  – fetch candidate questions for the requested topics and level
2. check_topic_coverage – verify every topic has at least one question
3. If coverage is missing for a topic: call get_topic_relations to find related topics, \
   then search_questions again with expanded topics
4. For study mode: also call search_knowledge_base for each topic to find theory materials
5. validate_program – confirm no duplicates and sufficient question count

Once the program is valid, respond with ONLY this JSON (no markdown fences):
{{
  "questions": [
    {{"question": "<text>", "theory": "<theory_id or text>", \
"order": 1, "topic": "<topic>", "question_id": "<id>"}}
  ],
  "coverage": {{"<topic>": <count>}},
  "validation_passed": true
}}
"""


# ── Nodes ─────────────────────────────────────────────────────────────────────


async def initialize_planner(state: PlannerState) -> PlannerState:
    """Build the initial LLM message context from the session request."""
    weak_hint = ""
    subtopics = state.get("subtopics") or state.get("weak_topics") or []
    if subtopics:
        weak_hint = f"\nPrioritise these subtopics: {subtopics}"

    system_content = _SYSTEM_PROMPT.format(
        n_questions=state["n_questions"],
        mode=state["mode"],
        level=state["level"],
        topics=", ".join(state["topics"]),
        weak_hint=weak_hint,
    )

    messages: List[Dict[str, Any]] = [
        {"role": "system", "content": system_content},
        {
            "role": "user",
            "content": (
                f"Build a {state['n_questions']}-question {state['mode']} program "
                f"for {state['level']} level on {state['topics']}."
            ),
        },
    ]

    return {
        **state,
        "messages": messages,
        "iteration": 0,
        "candidate_questions": [],
        "knowledge_snippets": [],
        "coverage_report": None,
        "validation_report": None,
        "final_program": None,
        "program_meta": None,
        "error": None,
    }


async def call_planner_llm(state: PlannerState) -> PlannerState:
    """Ask the LLM which tool to call next (or output the final program)."""
    from ..tools.planner_tools import TOOL_DEFINITIONS

    if not _llm_client:
        return {**state, "error": "LLM client not configured"}

    iteration = state.get("iteration", 0)
    if iteration >= state.get("max_iterations", 8):
        logger.warning(
            "Planner max iterations reached, falling back",
            session_id=state.get("session_id"),
            iteration=iteration,
        )
        return {**state, "error": f"max_iterations ({state.get('max_iterations', 8)}) exceeded"}

    try:
        assistant_msg = await _llm_client.chat_with_tools(
            messages=state["messages"],
            tools=TOOL_DEFINITIONS,
        )
        logger.debug(
            "LLM response received",
            has_tool_calls=bool(assistant_msg.get("tool_calls")),
            session_id=state.get("session_id"),
            iteration=iteration,
        )
        return {
            **state,
            "messages": state["messages"] + [assistant_msg],
            "iteration": iteration + 1,
        }
    except Exception as e:
        logger.error("call_planner_llm failed", error=str(e), session_id=state.get("session_id"))
        return {**state, "error": str(e)}


async def execute_tool_calls(state: PlannerState) -> PlannerState:
    """Execute all tool_calls from the last LLM message and feed results back."""
    from ..tools.planner_tools import (
        search_questions,
        check_topic_coverage,
        validate_program,
        get_topic_relations,
        search_knowledge_base,
    )

    _TOOL_MAP = {
        "search_questions": search_questions,
        "check_topic_coverage": check_topic_coverage,
        "validate_program": validate_program,
        "get_topic_relations": get_topic_relations,
        "search_knowledge_base": search_knowledge_base,
    }

    last_msg = state["messages"][-1]
    tool_calls = last_msg.get("tool_calls", [])
    if not tool_calls:
        return state

    new_messages = list(state["messages"])
    new_candidates = list(state.get("candidate_questions", []))
    new_snippets = list(state.get("knowledge_snippets", []))
    coverage_report = state.get("coverage_report")
    validation_report = state.get("validation_report")

    for tc in tool_calls:
        fn_name = tc["function"]["name"]
        fn_args_raw = tc["function"]["arguments"]
        tc_id = tc["id"]

        try:
            args = json.loads(fn_args_raw) if isinstance(fn_args_raw, str) else fn_args_raw
        except json.JSONDecodeError:
            args = {}

        # Inject accumulated state for tools that can't receive full data from LLM
        # (LLM gets truncated search results so can't pass questions back)
        if fn_name == "check_topic_coverage" and "questions" not in args:
            args["questions"] = new_candidates
        if fn_name == "validate_program" and "questions" not in args:
            args["questions"] = new_candidates

        result: Any = {"error": f"Unknown tool: {fn_name}"}
        if fn_name in _TOOL_MAP:
            try:
                result = await _TOOL_MAP[fn_name](**args)
            except Exception as exc:
                result = {"error": str(exc)}
                logger.warning("Tool call raised", tool=fn_name, error=str(exc))

        # Update accumulated state
        if fn_name == "search_questions" and isinstance(result, list):
            seen_ids: set = set()
            merged: List[Dict] = []
            for q in new_candidates + result:
                qid = (q.get("ID") or q.get("id", "")) or id(q)
                if qid not in seen_ids:
                    seen_ids.add(qid)
                    merged.append(q)
            new_candidates = merged

        elif fn_name == "check_topic_coverage" and isinstance(result, dict):
            coverage_report = result

        elif fn_name == "validate_program" and isinstance(result, dict):
            validation_report = result

        elif fn_name == "search_knowledge_base" and isinstance(result, list):
            new_snippets.extend(result)

        # Summarise large results to save context window space
        result_str = json.dumps(result, ensure_ascii=False)
        if len(result_str) > 3000:
            if isinstance(result, list):
                result_str = (
                    f"[{len(result)} items returned. "
                    f"First: {json.dumps(result[0]) if result else 'none'}]"
                )
            else:
                result_str = result_str[:3000] + "...[truncated]"

        new_messages.append({
            "role": "tool",
            "tool_call_id": tc_id,
            "content": result_str,
        })

        logger.debug(
            "Tool executed",
            tool=fn_name,
            result_len=len(str(result)),
            session_id=state.get("session_id"),
        )

    return {
        **state,
        "messages": new_messages,
        "candidate_questions": new_candidates,
        "knowledge_snippets": new_snippets,
        "coverage_report": coverage_report,
        "validation_report": validation_report,
    }


async def assemble_program(state: PlannerState) -> PlannerState:
    """Parse the final JSON program from the last assistant message."""
    last_msg = state["messages"][-1] if state.get("messages") else {}
    content = last_msg.get("content", "")

    program: List[Dict] | None = None
    meta: Dict | None = None

    if content and not state.get("error"):
        try:
            parsed = json.loads(content.strip())
            if isinstance(parsed, dict) and "questions" in parsed:
                program = parsed["questions"]
                meta = {
                    "validation_passed": parsed.get("validation_passed", True),
                    "coverage": parsed.get("coverage", {}),
                    "fallback_reason": None,
                    "generator_version": "planner-agent-1.0",
                    "source": "llm_agent",
                }
                logger.info(
                    "Agent assembled program",
                    questions=len(program),
                    session_id=state.get("session_id"),
                )
        except (json.JSONDecodeError, Exception) as exc:
            logger.warning(
                "assemble_program: could not parse LLM output as program JSON",
                error=str(exc),
                session_id=state.get("session_id"),
            )
            meta = {
                "validation_passed": False,
                "fallback_reason": "llm_output_parse_error",
                "generator_version": "planner-agent-1.0",
                "source": "llm_agent",
            }

    return {**state, "final_program": program, "program_meta": meta}
