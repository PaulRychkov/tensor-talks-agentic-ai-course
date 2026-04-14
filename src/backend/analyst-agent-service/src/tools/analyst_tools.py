"""Analyst agent tools (§8 stage 4, §9 p.2.7).

Each function is an async callable used by LangGraph nodes.
Clients are injected once at startup via set_analyst_tool_clients().
"""

from __future__ import annotations

import json
from typing import Any, Dict, List, Optional

from pydantic import ValidationError

from ..logger import get_logger
from ..models.llm_outputs import AnalystReport

logger = get_logger(__name__)

# ── Dependency injection ──────────────────────────────────────────────────────

_results_client = None
_chat_client = None
_session_client = None
_llm = None       # AsyncOpenAI instance


def set_analyst_tool_clients(results_client, chat_client, session_client, llm) -> None:
    global _results_client, _chat_client, _session_client, _llm
    _results_client = results_client
    _chat_client = chat_client
    _session_client = session_client
    _llm = llm


# ── Tool implementations ──────────────────────────────────────────────────────


async def get_evaluations(session_id: str) -> List[Dict[str, Any]]:
    """Fetch per-question evaluations for a completed session.

    Returns a list of {question, answer, score, topic} dicts derived from
    the chat transcript. Falls back to an empty list on failure.
    """
    if not _chat_client or not _session_client:
        return []

    try:
        messages = await _chat_client.get_messages(session_id)
        program = None
        try:
            program = await _session_client.get_program(session_id)
        except Exception:
            pass

        evaluations: List[Dict] = []
        prog_questions: List[Dict] = []
        if program and isinstance(program, dict):
            prog_questions = program.get("questions", [])

        # Pair assistant questions with user answers
        pairs: List[tuple] = []
        last_q = ""
        for msg in (messages or []):
            role = msg.get("role", "")
            content = msg.get("content", "") or msg.get("data", {}).get("content", "")
            if role == "assistant" and content:
                last_q = content
            elif role == "user" and content and last_q:
                pairs.append((last_q, content))
                last_q = ""

        for idx, (question, answer) in enumerate(pairs):
            prog_q = prog_questions[idx] if idx < len(prog_questions) else {}
            evaluations.append({
                "question_index": idx,
                "question": question,
                "answer": answer,
                "question_id": prog_q.get("id", ""),
                "topic": prog_q.get("topic", ""),
                "score": None,  # set by LLM analysis
            })
        return evaluations
    except Exception as exc:
        logger.warning("get_evaluations failed", session_id=session_id, error=str(exc))
        return []


def group_errors_by_topic(evaluations: List[Dict[str, Any]]) -> Dict[str, List[str]]:
    """Group low-scoring answers by topic for the Errors section of the report.

    Deterministic: no LLM call required.
    A question is considered an error when score < 0.6 (or score is None).
    Returns {topic: [question_text, ...]} mapping.
    """
    errors: Dict[str, List[str]] = {}
    for ev in evaluations:
        score = ev.get("score")
        topic = ev.get("topic") or "general"
        question = ev.get("question", "")
        if score is None or (isinstance(score, (int, float)) and score < 60):
            errors.setdefault(topic, []).append(question)
    return errors


async def generate_report_section(
    section_type: str,
    data: Dict[str, Any],
) -> str:
    """Generate one section of the analysis report via LLM.

    section_type is one of: summary | errors | strengths | plan | materials
    Returns the generated text (markdown or plain text).
    """
    if not _llm:
        return f"[{section_type} section not available – no LLM configured]"

    from ..config import settings

    SECTION_PROMPTS = {
        "summary": (
            "Write a concise 2-4 sentence summary of this session.\n"
            "Data: {data}\nReturn plain text only."
        ),
        "errors": (
            "Describe the key errors grouped by topic in 1-2 sentences per topic.\n"
            "Errors: {data}\nReturn plain text only."
        ),
        "strengths": (
            "List the candidate's 3-5 key strengths based on the session.\n"
            "Data: {data}\nReturn a numbered list."
        ),
        "plan": (
            "Create a short preparation plan (3-5 action items) targeting the weak areas.\n"
            "Weak topics: {data}\nReturn a numbered list."
        ),
        "materials": (
            "Suggest 3-5 specific learning resources (books, docs, tutorials) "
            "for the weak topics.\n"
            "Topics: {data}\nReturn a bulleted list."
        ),
    }

    prompt_template = SECTION_PROMPTS.get(
        section_type,
        "Summarise the following data in 2-3 sentences.\nData: {data}",
    )
    prompt = prompt_template.format(data=json.dumps(data, ensure_ascii=False)[:4000])

    try:
        is_gpt5 = "gpt-5" in settings.llm_model.lower()
        params: Dict[str, Any] = {
            "model": settings.llm_model,
            "messages": [{"role": "user", "content": prompt}],
        }
        if not is_gpt5:
            params["temperature"] = settings.llm_temperature
            params["max_tokens"] = 800
        else:
            params["max_completion_tokens"] = 800

        resp = await _llm.chat.completions.create(**params)
        return resp.choices[0].message.content or ""
    except Exception as exc:
        logger.warning("generate_report_section failed", section=section_type, error=str(exc))
        return f"[{section_type} generation failed: {exc}]"


def validate_report(
    report_draft: Dict[str, Any],
    evaluations: Optional[List[Dict[str, Any]]] = None,
) -> Dict[str, Any]:
    """Validate a report draft using the AnalystReport Pydantic model (§10.5).

    Replaces the manual required-keys check with full Pydantic v2 validation.
    Returns {validation_passed: bool, issues: [str]}.
    """
    try:
        AnalystReport.model_validate(report_draft)
        return {"validation_passed": True, "issues": []}
    except ValidationError as exc:
        issues = [f"{'.'.join(str(loc) for loc in e['loc'])}: {e['msg']}" for e in exc.errors()]
        return {"validation_passed": False, "issues": issues}


async def search_knowledge_base(
    query: str,
    topic: str = "",
) -> List[Dict[str, Any]]:
    """Search the knowledge base for materials relevant to the query.

    Returns a list of {title, topic, text_preview} dicts.
    Used by the analyst to enrich preparation_plan and materials sections.
    """
    # knowledge-base-crud is not directly wired to analyst-agent-service in the
    # current architecture; return empty list gracefully.
    logger.debug("search_knowledge_base called (no KB client wired)", query=query, topic=topic)
    return []


async def web_search(
    query: str,
    num_results: int = 5,
) -> List[Dict[str, Any]]:
    """Perform an external web search for relevant learning resources.

    Returns a list of {title, url, snippet} dicts.
    Falls back to an empty list when no search provider is configured.
    """
    try:
        import httpx
        # Use arXiv as a free, allowlisted provider for ML topics
        params = {
            "search_query": f"all:{query}",
            "start": 0,
            "max_results": num_results,
            "sortBy": "relevance",
        }
        async with httpx.AsyncClient(timeout=10) as client:
            resp = await client.get("http://export.arxiv.org/api/query", params=params)
            if resp.status_code != 200:
                return []

        import re
        results: List[Dict] = []
        entries = re.findall(r"<entry>(.*?)</entry>", resp.text, re.DOTALL)
        for entry in entries[:num_results]:
            title_m = re.search(r"<title>(.*?)</title>", entry, re.DOTALL)
            link_m = re.search(r"<id>(.*?)</id>", entry)
            summ_m = re.search(r"<summary>(.*?)</summary>", entry, re.DOTALL)
            results.append({
                "title": (title_m.group(1).strip() if title_m else ""),
                "url": (link_m.group(1).strip() if link_m else ""),
                "snippet": ((summ_m.group(1).strip()[:200] if summ_m else "")),
                "source": "arxiv",
            })
        return results
    except Exception as exc:
        logger.warning("web_search failed", query=query, error=str(exc))
        return []


_TRUSTED_DOMAINS = [
    "arxiv.org", "pytorch.org", "tensorflow.org", "scikit-learn.org",
    "huggingface.co", "docs.python.org", "numpy.org", "scipy.org",
    "developer.mozilla.org", "learn.microsoft.com",
]


async def fetch_url(url: str) -> str:
    """Fetch the plain-text content of a URL (up to 5 000 chars).

    Only trusted domains are fetched; others are rejected with an error message.
    Returns the extracted text or an empty string on failure.
    """
    if not any(domain in url for domain in _TRUSTED_DOMAINS):
        logger.warning("fetch_url: URL not in allowlist", url=url)
        return f"[blocked] Domain not in allowlist. Allowed: {', '.join(_TRUSTED_DOMAINS)}"

    try:
        import httpx, re
        async with httpx.AsyncClient(timeout=10, follow_redirects=True) as client:
            resp = await client.get(url)
            text = resp.text
            if "html" in resp.headers.get("content-type", ""):
                text = re.sub(r"<[^>]+>", " ", text)
                text = re.sub(r"\s+", " ", text).strip()
            return text[:5000]
    except Exception as exc:
        logger.warning("fetch_url failed", url=url, error=str(exc))
        return ""


async def save_draft_material(
    title: str,
    content: str,
    topic: str,
    material_type: str = "knowledge",
) -> Dict[str, Any]:
    """Save a draft material to the knowledge-producer-service for HITL review.

    New KB content discovered by the analyst is NEVER written directly to
    the production tables – it goes through the draft/review/publish workflow
    (§5 p.5.1, §8 stage 0).

    Returns {status, message} describing the outcome.
    """
    import httpx
    from ..config import settings

    logger.info(
        "save_draft_material called",
        title=title,
        topic=topic,
        type=material_type,
        content_len=len(content),
    )

    endpoint = "questions" if material_type == "questions" else "knowledge"
    url = f"{settings.knowledge_producer_service_url}/drafts/{endpoint}"

    try:
        async with httpx.AsyncClient(timeout=30.0) as client:
            resp = await client.post(url, json={
                "title": title,
                "content": content,
                "topic": topic,
                "material_type": material_type,
            })
            resp.raise_for_status()
            data = resp.json()
            logger.info(
                "Draft saved to knowledge-producer",
                draft_id=data.get("id"),
                status=data.get("status"),
            )
            return {
                "status": "pending_review",
                "draft_id": data.get("id"),
                "message": f"Draft '{title}' saved for HITL review (id={data.get('id')}).",
            }
    except Exception as exc:
        logger.warning("save_draft_material HTTP call failed", url=url, error=str(exc))
        return {
            "status": "error",
            "message": f"Failed to save draft: {exc}",
        }


# ── OpenAI function-calling definitions (§8 stage 4) ─────────────────────────
# Consumed by AnalystService.chat_with_tools(); the LLM drives the report
# generation loop instead of a fixed generate→validate→retry pipeline.

TOOL_DEFINITIONS: List[Dict[str, Any]] = [
    {
        "type": "function",
        "function": {
            "name": "get_evaluations",
            "description": (
                "Fetch per-question evaluations (question/answer/topic/score) for the session. "
                "Call this first to see the raw material."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "session_id": {"type": "string"},
                },
                "required": ["session_id"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "group_errors_by_topic",
            "description": (
                "Group low-score answers by topic. Returns {topic: [question_texts]}."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "evaluations": {"type": "array", "items": {"type": "object"}},
                },
                "required": ["evaluations"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "generate_report_section",
            "description": (
                "Generate one section of the report (summary|errors|strengths|plan|materials). "
                "Call separately per section or combine in the final emit_report."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "section_type": {
                        "type": "string",
                        "enum": ["summary", "errors", "strengths", "plan", "materials"],
                    },
                    "data": {"type": "object"},
                },
                "required": ["section_type", "data"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "validate_report",
            "description": (
                "Validate a report draft against AnalystReport schema. "
                "Returns {validation_passed, issues[]}. Call this before emit_report."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "report_draft": {"type": "object"},
                    "evaluations": {"type": "array", "items": {"type": "object"}},
                },
                "required": ["report_draft"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "search_knowledge_base",
            "description": (
                "Search the internal KB for materials on a topic. Use to enrich "
                "the 'materials' or 'plan' sections before emit_report."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {"type": "string"},
                    "topic": {"type": "string"},
                },
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "web_search",
            "description": (
                "Search arXiv / trusted ML sources for learning materials when the "
                "internal KB doesn't cover a weak topic."
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
            "description": "Fetch the text content of a URL returned by web_search.",
            "parameters": {
                "type": "object",
                "properties": {"url": {"type": "string"}},
                "required": ["url"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "save_draft_material",
            "description": (
                "Queue NEW KB content as a draft for HITL review. "
                "Never writes directly to the production KB."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "title": {"type": "string"},
                    "content": {"type": "string"},
                    "topic": {"type": "string"},
                    "material_type": {"type": "string"},
                },
                "required": ["title", "content", "topic"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "emit_report",
            "description": (
                "FINAL STEP — submit the assembled AnalystReport JSON. "
                "Call this only after validate_report returns passed=true "
                "(or after max_iterations reached). The report must match the "
                "AnalystReport schema: summary, score (0-100), errors_by_topic, "
                "strengths, preparation_plan, materials."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "report": {
                        "type": "object",
                        "description": "Complete AnalystReport JSON.",
                    },
                },
                "required": ["report"],
            },
        },
    },
]


# Dispatch map for non-terminal tools.
TOOL_MAP = {
    "get_evaluations": get_evaluations,
    "group_errors_by_topic": group_errors_by_topic,
    "generate_report_section": generate_report_section,
    "validate_report": validate_report,
    "search_knowledge_base": search_knowledge_base,
    "web_search": web_search,
    "fetch_url": fetch_url,
    "save_draft_material": save_draft_material,
}
