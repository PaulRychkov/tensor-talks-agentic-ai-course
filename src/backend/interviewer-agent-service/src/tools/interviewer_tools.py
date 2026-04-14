"""Interviewer agent tools (§8 stage 3, §9 p.2).

Each function formalises a capability already present in the LangGraph nodes
(graph/nodes.py) as a standalone async callable.  Nodes call these functions
directly; the same signatures also serve as the OpenAI function-calling
definitions should the agent ever need explicit tool-use mode.

Injected via set_interviewer_tool_clients() at startup.
"""

from __future__ import annotations

import json
import re
from typing import Any, Dict, List, Optional

from ..logger import get_logger

logger = get_logger(__name__)

# ── Dependency injection ──────────────────────────────────────────────────────

_llm_client = None
_session_client = None
_redis_client = None


def set_interviewer_tool_clients(llm_client, session_client, redis_client) -> None:
    global _llm_client, _session_client, _redis_client
    _llm_client = llm_client
    _session_client = session_client
    _redis_client = redis_client


# ── Tool implementations ──────────────────────────────────────────────────────


async def evaluate_answer(
    question: str,
    answer: str,
    theory: str,
    model: str = "",
) -> Dict[str, Any]:
    """Evaluate a candidate's answer against the expected theory.

    Returns {score (0-100), decision, feedback} where decision is one of:
    next | hint | clarify | redirect | skip.

    This is the core tool used by the interviewer to branch the dialogue
    (§8 stage 3 – evaluate_answer tool).
    """
    if not _llm_client:
        return {"score": 50, "decision": "next", "feedback": "LLM not configured"}

    from ..llm.prompts import build_evaluation_prompt

    prompt = build_evaluation_prompt(
        question=question,
        user_message=answer,
        theory=theory,
    )
    try:
        result = await _llm_client.generate_json(
            prompt=prompt,
            schema={
                "type": "object",
                "properties": {
                    "overall_score": {"type": "number"},
                    "is_complete": {"type": "boolean"},
                    "missing_points": {"type": "array", "items": {"type": "string"}},
                    "should_move_on": {"type": "boolean"},
                    "evaluation_reasoning": {"type": "string"},
                },
                "required": ["overall_score", "is_complete"],
            },
        )
        if not isinstance(result, dict):
            raise ValueError("Non-dict JSON response")
        # Normalize: prompt returns overall_score (0-1)
        raw_score = float(result.get("overall_score", result.get("score", 0.5)))
        if raw_score > 1.0:
            raw_score = raw_score / 100.0
        return {
            "score": raw_score,
            "feedback": result.get("evaluation_reasoning", result.get("feedback", "")),
            "missing_points": result.get("missing_points", []),
            "is_complete": result.get("is_complete", raw_score >= 0.75),
            "should_move_on": result.get("should_move_on", False),
        }
    except Exception as exc:
        logger.warning("evaluate_answer failed", error=str(exc))
        return {"score": 50, "decision": "next", "feedback": "evaluation error"}


async def search_knowledge_base(
    query: str,
    topic: str = "",
) -> List[Dict[str, Any]]:
    """Search the knowledge base for theory related to the current question.

    Used by the interviewer to find supplementary hints or clarification
    material when the candidate gives a partially correct answer
    (§8 stage 3 – search_knowledge_base tool).
    """
    if not _session_client:
        return []

    try:
        # Session service exposes /knowledge?query=...&topic=... in some deployments.
        # Gracefully return empty list if endpoint not available.
        kb_items = await _session_client.search_knowledge(query=query, topic=topic)
        return kb_items or []
    except Exception as exc:
        logger.debug("search_knowledge_base: no result", error=str(exc))
        return []


async def search_questions(
    related_to: str = "",
    topic: str = "",
    limit: int = 3,
) -> List[Dict[str, Any]]:
    """Find follow-up or clarifying sub-questions related to the current question.

    The interviewer uses this tool to generate a probing sub-question when the
    candidate's score is in the 40-80 range (§8 stage 3 – search_questions tool).
    """
    if not _session_client:
        return []

    try:
        related = await _session_client.search_questions(
            related_to=related_to, topic=topic, limit=limit
        )
        return related or []
    except Exception as exc:
        logger.debug("search_questions: no result", error=str(exc))
        return []


async def summarize_dialogue(
    messages: List[Dict[str, Any]],
    max_tokens: int = 400,
) -> str:
    """Summarise a long dialogue history to stay within the LLM context window.

    Returns a compact plain-text summary of the conversation so far.
    Called automatically when message history grows too long
    (§8 stage 3 – summarize_dialogue tool).
    """
    if not _llm_client or not messages:
        return ""

    from ..config import settings

    text = "\n".join(
        f"[{m.get('role', 'unknown').upper()}]: {m.get('content', '')}"
        for m in messages[-20:]  # last 20 messages
    )
    prompt = (
        "Summarise this conversation in 3-5 sentences, preserving key facts about "
        "which questions were asked and what the candidate said.\n\n"
        f"Conversation:\n{text[:6000]}"
    )
    try:
        return await _llm_client.generate(prompt)
    except Exception as exc:
        logger.warning("summarize_dialogue failed", error=str(exc))
        return ""


async def web_search(
    query: str,
    num_results: int = 3,
) -> List[Dict[str, Any]]:
    """Search the web for information about a framework or concept.

    Used ONLY when the candidate mentions a very new or obscure framework
    that is not in the knowledge base (§8 stage 3 – web_search tool).
    Results are filtered against a trusted domain allowlist.
    """
    TRUSTED = [
        "arxiv.org", "pytorch.org", "tensorflow.org", "scikit-learn.org",
        "huggingface.co", "docs.python.org", "numpy.org", "scipy.org",
    ]
    try:
        import httpx

        params = {
            "search_query": f"all:{query}",
            "start": 0,
            "max_results": num_results,
            "sortBy": "relevance",
        }
        async with httpx.AsyncClient(timeout=8) as client:
            resp = await client.get("http://export.arxiv.org/api/query", params=params)
            if resp.status_code != 200:
                return []

        results: List[Dict] = []
        entries = re.findall(r"<entry>(.*?)</entry>", resp.text, re.DOTALL)
        for entry in entries[:num_results]:
            title_m = re.search(r"<title>(.*?)</title>", entry, re.DOTALL)
            link_m = re.search(r"<id>(.*?)</id>", entry)
            summ_m = re.search(r"<summary>(.*?)</summary>", entry, re.DOTALL)
            url = link_m.group(1).strip() if link_m else ""
            if not any(t in url for t in TRUSTED):
                continue
            results.append({
                "title": (title_m.group(1).strip() if title_m else ""),
                "url": url,
                "snippet": (summ_m.group(1).strip()[:200] if summ_m else ""),
                "source": "arxiv",
            })
        return results
    except Exception as exc:
        logger.debug("web_search failed", query=query, error=str(exc))
        return []


async def fetch_url(url: str, max_chars: int = 3000) -> str:
    """Fetch and extract plain text from a URL.

    Used in conjunction with web_search when the interviewer needs the full
    content of a found article (§8 stage 3 – fetch_url tool).
    """
    TRUSTED = [
        "arxiv.org", "pytorch.org", "tensorflow.org", "scikit-learn.org",
        "huggingface.co", "docs.python.org",
    ]
    if not any(t in url for t in TRUSTED):
        logger.warning("fetch_url: URL not in allowlist", url=url)
        return ""

    try:
        import httpx

        async with httpx.AsyncClient(timeout=8, follow_redirects=True) as client:
            resp = await client.get(url)
            text = resp.text
            if "html" in resp.headers.get("content-type", ""):
                text = re.sub(r"<[^>]+>", " ", text)
                text = re.sub(r"\s+", " ", text).strip()
            return text[:max_chars]
    except Exception as exc:
        logger.debug("fetch_url failed", url=url, error=str(exc))
        return ""


# ── OpenAI function-calling definitions (§8 stage 3) ─────────────────────────
# The LLM receives these schemas and decides which tool to call at each step of
# the ReAct loop instead of relying on hardcoded score thresholds.

TOOL_DEFINITIONS: List[Dict[str, Any]] = [
    {
        "type": "function",
        "function": {
            "name": "evaluate_answer",
            "description": (
                "Evaluate the candidate's answer against the expected theory. "
                "Returns {score 0-1, decision, feedback, missing_points}. "
                "Call this FIRST when the candidate has answered a question."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "question": {"type": "string", "description": "The question text"},
                    "answer": {"type": "string", "description": "The candidate's answer"},
                    "theory": {"type": "string", "description": "Expected theory/reference"},
                },
                "required": ["question", "answer", "theory"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "search_knowledge_base",
            "description": (
                "Search the internal knowledge base for theory related to a topic. "
                "Use when the candidate gave a partial answer and you need "
                "supplementary theory to form a clarifying question or hint."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search query"},
                    "topic": {"type": "string", "description": "Topic tag"},
                },
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "search_questions",
            "description": (
                "Find follow-up / clarifying sub-questions related to the current "
                "question. Use to generate a probing sub-question when the "
                "candidate's answer is partially correct."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "related_to": {"type": "string"},
                    "topic": {"type": "string"},
                    "limit": {"type": "integer", "description": "Max results (default 3)"},
                },
                "required": [],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "web_search",
            "description": (
                "Search the web (arxiv + trusted ML domains) for information about a "
                "NEW/OBSCURE framework that the candidate mentioned and that is not "
                "in the internal KB. Use sparingly."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {"type": "string"},
                    "num_results": {"type": "integer"},
                },
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "fetch_url",
            "description": (
                "Fetch the text content of a URL found via web_search. "
                "Only trusted ML domains are allowed; returns plain text."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "url": {"type": "string"},
                },
                "required": ["url"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "summarize_dialogue",
            "description": (
                "Summarise long dialogue history into 3-5 sentences to save context. "
                "Use when the history is very long."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "messages": {"type": "array", "items": {"type": "object"}},
                },
                "required": ["messages"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "emit_response",
            "description": (
                "FINAL STEP — produce the assistant message to send to the candidate "
                "AND declare the session-level action taken. Call this LAST after "
                "you've gathered enough information via other tools. "
                "Valid actions: 'ask_clarification' (probe missing points), "
                "'give_hint' (partial hint + re-ask), 'next_question' (move on), "
                "'thank_you' (interview finished), 'off_topic_reminder' (redirect), "
                "'skip' (skip current question)."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "action": {
                        "type": "string",
                        "enum": [
                            "ask_clarification",
                            "give_hint",
                            "next_question",
                            "thank_you",
                            "off_topic_reminder",
                            "skip",
                        ],
                    },
                    "message": {
                        "type": "string",
                        "description": "The exact text to send to the candidate (Russian).",
                    },
                    "rationale": {
                        "type": "string",
                        "description": "Brief explanation why this action was chosen (for traces).",
                    },
                    "score": {
                        "type": "number",
                        "description": "Optional evaluation score 0-1 to record for this answer.",
                    },
                },
                "required": ["action", "message"],
            },
        },
    },
]


# Dispatch map: tool_name → async callable
TOOL_MAP = {
    "evaluate_answer": evaluate_answer,
    "search_knowledge_base": search_knowledge_base,
    "search_questions": search_questions,
    "web_search": web_search,
    "fetch_url": fetch_url,
    "summarize_dialogue": summarize_dialogue,
}
