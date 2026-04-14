"""Planner tools for interview program assembly (§8 stage 2).

Each function is:
  1. An async callable the agent nodes invoke directly.
  2. Described in TOOL_DEFINITIONS (OpenAI function-calling JSON Schema)
     so the LLM can request them via tool_calls.

Clients are injected once at startup via set_tool_clients().
"""

from __future__ import annotations

import json
from typing import Any, Dict, List, Optional

from ..logger import get_logger

logger = get_logger(__name__)

# ── Dependency injection ──────────────────────────────────────────────────────

_questions_client = None
_knowledge_client = None


_results_client = None  # Optional ResultsClient for episodic memory (§10.6)


def set_tool_clients(questions_client, knowledge_client, results_client=None) -> None:
    global _questions_client, _knowledge_client, _results_client
    _questions_client = questions_client
    _knowledge_client = knowledge_client
    _results_client = results_client


# ── Tool implementations ──────────────────────────────────────────────────────


async def search_questions(
    topics: List[str],
    level: str = "middle",
    mode: str = "interview",
    limit: int = 30,
) -> List[Dict[str, Any]]:
    """Search for questions in the question bank by topics, level, and mode.

    Returns a list of question records. For *study* mode, theoretical questions
    are ranked first; for *training*, practical questions come first.
    """
    if not _questions_client:
        logger.warning("search_questions: questions client not initialised")
        return []

    from ..services.interview_builder_service import LEVEL_TO_COMPLEXITY, TOPIC_TO_TAGS, \
        _extract_question_data
    from ..config import settings

    complexity = LEVEL_TO_COMPLEXITY.get(level.lower(), 2)

    try:
        questions = await _questions_client.get_questions_by_filters(complexity=complexity)
        if not questions:
            questions = await _questions_client.get_questions_by_filters()

        # Mode-based type preference
        if mode == "study":
            preferred = {"theoretical", "open", "multiple_choice"}
        elif mode == "training":
            preferred = {"coding", "practical", "applied", "case"}
        else:
            preferred = None

        if preferred:
            def _type_score(q: Dict) -> int:
                d = q.get("Data") or q.get("data") or q
                c = d.get("content") or {}
                qt = c.get("question_type", "open") if isinstance(c, dict) else "open"
                return 0 if qt in preferred else 1
            questions = sorted(questions, key=_type_score)

        # Topic tag filtering via knowledge client
        if _knowledge_client and topics:
            all_tags: set = set()
            for t in topics:
                mapped = TOPIC_TO_TAGS.get(t.lower(), [t.lower()])
                all_tags.update(mapped)

            filtered: List[Dict] = []
            for q in questions:
                _, theory_id = _extract_question_data(q)
                if not theory_id:
                    filtered.append(q)
                    continue
                try:
                    kb = await _knowledge_client.get_knowledge_by_id(theory_id)
                    if not kb:
                        filtered.append(q)
                        continue
                    kd = kb.get("Data") or kb.get("data") or kb
                    kb_tags = set(kd.get("tags") or kd.get("metadata", {}).get("tags", []))
                    if kb_tags & all_tags:
                        filtered.append(q)
                except Exception:
                    filtered.append(q)
            questions = filtered

        return questions[:limit]

    except Exception as e:
        logger.error("search_questions failed", error=str(e))
        return []


async def check_topic_coverage(
    questions: List[Dict[str, Any]],
    required_topics: List[str],
) -> Dict[str, Any]:
    """Check whether the selected questions cover all required topics.

    Returns a dict with covered/missing topic lists and per-topic counts.
    """
    if not required_topics:
        return {"covered": [], "missing": [], "coverage_ratio": 1.0, "per_topic": {}}

    if not questions:
        return {
            "covered": [],
            "missing": list(required_topics),
            "coverage_ratio": 0.0,
            "per_topic": {t: 0 for t in required_topics},
        }

    from ..services.interview_builder_service import TOPIC_TO_TAGS, _extract_question_data

    per_topic: Dict[str, int] = {t: 0 for t in required_topics}

    if _knowledge_client:
        topic_tag_map = {t: set(TOPIC_TO_TAGS.get(t.lower(), [t.lower()])) for t in required_topics}
        for q in questions:
            _, theory_id = _extract_question_data(q)
            if not theory_id:
                continue
            try:
                kb = await _knowledge_client.get_knowledge_by_id(theory_id)
                if not kb:
                    continue
                kd = kb.get("Data") or kb.get("data") or kb
                kb_tags = set(kd.get("tags") or kd.get("metadata", {}).get("tags", []))
                for topic, tags in topic_tag_map.items():
                    if tags & kb_tags:
                        per_topic[topic] += 1
            except Exception:
                pass
    else:
        # Even distribution fallback
        n = max(1, len(questions) // max(1, len(required_topics)))
        per_topic = {t: n for t in required_topics}

    covered = [t for t, c in per_topic.items() if c > 0]
    missing = [t for t, c in per_topic.items() if c == 0]
    ratio = len(covered) / len(required_topics) if required_topics else 1.0

    return {"covered": covered, "missing": missing, "coverage_ratio": ratio, "per_topic": per_topic}


async def validate_program(questions: List[Dict[str, Any]]) -> Dict[str, Any]:
    """Validate the assembled program for duplicates, balance, and minimum count.

    Returns a dict with validation_passed flag and a list of issues found.
    """
    from ..services.interview_builder_service import _extract_question_data, \
        _extract_question_text, _text_hash
    from ..config import settings

    issues: List[str] = []

    if not questions:
        return {"validation_passed": False, "issues": ["Empty program"], "question_count": 0}

    seen_ids: set = set()
    seen_hashes: set = set()
    for q in questions:
        d, _ = _extract_question_data(q)
        qid = (d.get("id") or q.get("ID") or q.get("id", "")) if isinstance(d, dict) else ""
        text = _extract_question_text(d) if isinstance(d, dict) else ""
        h = _text_hash(text) if text else ""

        if qid and qid in seen_ids:
            issues.append(f"Duplicate question id: {qid}")
        if h and h in seen_hashes:
            issues.append("Duplicate question text detected")
        if qid:
            seen_ids.add(qid)
        if h:
            seen_hashes.add(h)

    if len(questions) < settings.questions_per_interview:
        issues.append(
            f"Too few questions: {len(questions)} (need {settings.questions_per_interview})"
        )

    return {
        "validation_passed": len(issues) == 0,
        "issues": issues,
        "question_count": len(questions),
    }


async def get_topic_relations(topic: str) -> List[str]:
    """Return related topics from the knowledge base for the given topic.

    Falls back to a static map when the knowledge client is unavailable.
    """
    _STATIC_RELATIONS: Dict[str, List[str]] = {
        "nlp": ["llm", "classic_ml"],
        "llm": ["nlp", "classic_ml"],
        "classic_ml": ["nlp", "cv", "ds"],
        "cv": ["classic_ml", "ds"],
        "ds": ["classic_ml"],
    }

    if not _knowledge_client:
        return _STATIC_RELATIONS.get(topic.lower(), [])

    try:
        items = await _knowledge_client.get_knowledge_by_filters(tags=[topic])
        related: set = set()
        for item in (items or [])[:10]:
            kd = item.get("Data") or item.get("data") or item
            tags = kd.get("tags") or kd.get("metadata", {}).get("tags", [])
            for t in (tags or []):
                if t != topic:
                    related.add(t)
        return list(related)[:5] or _STATIC_RELATIONS.get(topic.lower(), [])
    except Exception as e:
        logger.warning("get_topic_relations failed", topic=topic, error=str(e))
        return _STATIC_RELATIONS.get(topic.lower(), [])


async def search_knowledge_base(
    query: str,
    topic: str = "",
    limit: int = 5,
) -> List[Dict[str, Any]]:
    """Search for theory materials in the knowledge base.

    Essential for *study* mode to locate relevant learning content.
    """
    if not _knowledge_client:
        return []

    try:
        items = await _knowledge_client.get_knowledge_by_filters(tags=[topic] if topic else None)
        if not items:
            return []

        if query:
            ql = query.lower()

            def _relevance(item: Dict) -> int:
                kd = item.get("Data") or item.get("data") or item
                title = str(kd.get("title", "")).lower()
                segs = kd.get("segments", [])
                body = " ".join(
                    str(s.get("content", "")) for s in segs if isinstance(s, dict)
                ).lower()
                return int(ql in title) * 2 + int(ql in body)

            items = sorted(items, key=_relevance, reverse=True)

        result: List[Dict] = []
        for item in items[:limit]:
            kd = item.get("Data") or item.get("data") or item
            segs = kd.get("segments", [])
            preview = ""
            if segs and isinstance(segs[0], dict):
                preview = str(segs[0].get("content", ""))[:500]
            result.append({
                "id": item.get("ID") or item.get("id", ""),
                "title": kd.get("title", ""),
                "topic": topic or kd.get("topic", ""),
                "text_preview": preview,
            })
        return result

    except Exception as e:
        logger.warning("search_knowledge_base failed", error=str(e))
        return []


async def get_user_history(
    user_id: str,
    topics: Optional[List[str]] = None,
    limit: int = 5,
) -> Dict[str, Any]:
    """Fetch aggregated history of past sessions for a user (§10.6 episodic memory).

    Returns weak/strong topics and per-topic scores for personalization.
    """
    if not _results_client:
        logger.debug("get_user_history: results client not initialised")
        return {"user_id": user_id, "total_sessions": 0, "weak_topics": [], "strong_topics": [], "topic_scores": {}}

    try:
        import httpx
        params: Dict[str, str] = {"user_id": user_id, "limit": str(limit)}
        if topics:
            params["topics"] = ",".join(topics)

        base_url = getattr(_results_client, "base_url", "http://results-crud-service:8088")
        async with httpx.AsyncClient(timeout=5.0) as client:
            resp = await client.get(f"{base_url}/results/user-summary", params=params)
            if resp.status_code == 200:
                return resp.json()
    except Exception as exc:
        logger.warning("get_user_history failed", user_id=user_id, error=str(exc))

    return {"user_id": user_id, "total_sessions": 0, "weak_topics": [], "strong_topics": [], "topic_scores": {}}


# ── OpenAI function-calling tool definitions ──────────────────────────────────

TOOL_DEFINITIONS: List[Dict[str, Any]] = [
    {
        "type": "function",
        "function": {
            "name": "search_questions",
            "description": (
                "Search for interview/training/study questions in the question bank. "
                "Filter by topics, difficulty level, and session mode. "
                "Returns a list of question records."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "topics": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Topics to search, e.g. ['nlp', 'llm']",
                    },
                    "level": {
                        "type": "string",
                        "enum": ["junior", "middle", "senior"],
                        "description": "Difficulty level",
                    },
                    "mode": {
                        "type": "string",
                        "enum": ["interview", "training", "study"],
                        "description": "Session mode – affects question-type preferences",
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Maximum questions to return (default 30)",
                    },
                },
                "required": ["topics"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "check_topic_coverage",
            "description": (
                "Check whether the selected questions adequately cover all required topics. "
                "Returns per-topic question counts and a list of uncovered topics."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "questions": {
                        "type": "array",
                        "items": {"type": "object"},
                        "description": "Question records to evaluate",
                    },
                    "required_topics": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Topics that must be covered",
                    },
                },
                "required": ["questions", "required_topics"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "validate_program",
            "description": (
                "Validate the assembled program: check for duplicate questions, "
                "topic balance, and required question count."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "questions": {
                        "type": "array",
                        "items": {"type": "object"},
                        "description": "Assembled question list",
                    },
                },
                "required": ["questions"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "get_topic_relations",
            "description": (
                "Return related topics from the knowledge base for a given topic. "
                "Use this when a topic has too few questions to meet coverage requirements."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "topic": {
                        "type": "string",
                        "description": "Topic to expand",
                    },
                },
                "required": ["topic"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "search_knowledge_base",
            "description": (
                "Search for theory/learning materials in the knowledge base. "
                "Especially important for study mode to find relevant content."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Search query",
                    },
                    "topic": {
                        "type": "string",
                        "description": "Optional topic filter",
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max results (default 5)",
                    },
                },
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "get_user_history",
            "description": (
                "Fetch the user's session history and topic scores from past interviews. "
                "Returns weak_topics (avg score < 0.5) and strong_topics (avg score >= 0.8). "
                "Use this when building a personalized program to avoid repeating mastered topics "
                "and prioritize areas where the user struggled. Only useful when user_id is provided."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "user_id": {
                        "type": "string",
                        "description": "User ID for which to fetch history",
                    },
                    "topics": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Optional list of topics to filter scores by",
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max number of past sessions to consider (default 5)",
                    },
                },
                "required": ["user_id"],
            },
        },
    },
]
