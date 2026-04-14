"""Service for building interview programs with deterministic pipeline.

Pipeline: filter → dedup → coverage → balance → sort.
Mode-specific profiles: interview, training, study.
"""

import hashlib
import time
from collections import defaultdict
from typing import Any, Dict, List, Optional, Tuple

from ..clients.knowledge_client import KnowledgeClient
from ..clients.questions_client import QuestionsClient
from ..config import settings
from ..kafka import KafkaProducer
from ..logger import get_logger
from ..metrics import get_metrics_collector
from ..schemas import ProgramMeta

logger = get_logger(__name__)

LEVEL_TO_COMPLEXITY = {"junior": 1, "middle": 2, "senior": 3}

TOPIC_TO_TAGS = {
    "classic_ml": ["machine_learning"],
    "ml": ["machine_learning"],
    "ml_basics": ["machine_learning"],
    "machine learning": ["machine_learning"],
    "deep learning": ["deep_learning"],
    "neural networks": ["neural_networks"],
    "natural language processing": ["nlp"],
    "computer vision": ["cv"],
    "data science": ["data_science"],
    "nlp": ["nlp"],
    "llm": ["llm"],
    "cv": ["cv"],
    "ds": ["data_science"],
}


def _extract_question_data(question: Dict[str, Any]) -> Tuple[Dict[str, Any], Optional[str]]:
    """Extract the inner data dict and theory_id from a question record.

    Handles Go struct serialization where fields may be capitalized.
    """
    if not isinstance(question, dict):
        return question, None

    for key in ("Data", "data"):
        val = question.get(key)
        if isinstance(val, dict):
            return val, val.get("theory_id")

    return question, question.get("theory_id")


def _extract_question_text(question_data: Dict[str, Any]) -> str:
    """Pull the question text from nested content structures."""
    if not isinstance(question_data, dict):
        return ""
    content = question_data.get("content", {})
    if isinstance(content, dict):
        text = content.get("question", "")
        if text:
            return text
    return question_data.get("question", "")


def _normalize_text(text: str) -> str:
    """Lowercase + strip for dedup comparison."""
    return " ".join(text.lower().split())


def _text_hash(text: str) -> str:
    return hashlib.md5(_normalize_text(text).encode()).hexdigest()


def _extract_knowledge_data(knowledge: Dict[str, Any]) -> Dict[str, Any]:
    for key in ("Data", "data"):
        val = knowledge.get(key)
        if isinstance(val, dict):
            return val
    return knowledge


class InterviewBuilderService:
    """Interview program builder: LangGraph planner agent (with LLM) or
    deterministic fallback pipeline (when no LLM key is configured).

    Agent pipeline (§8 stage 2, §9 p.1):
      LLM orchestrates tools → search_questions → check_topic_coverage →
      validate_program → get_topic_relations → search_knowledge_base

    Deterministic fallback pipeline:
    1. Fetch questions by complexity
    2. Filter by topic tags (via knowledge)
    3. Dedup by question_id and text hash
    4. Ensure topic coverage (at least 1 per requested topic)
    5. Balance (cap per-topic ratio)
    6. Sort (group by theory, stable order)
    7. Enrich with theory text
    """

    def __init__(
        self,
        questions_client: Optional[QuestionsClient] = None,
        knowledge_client: Optional[KnowledgeClient] = None,
        kafka_producer: Optional[KafkaProducer] = None,
    ):
        self._questions_client_ext = questions_client
        self._knowledge_client_ext = knowledge_client
        self.kafka_producer = kafka_producer or KafkaProducer()
        self.metrics = get_metrics_collector()
        self._planner_graph = None
        self._qid_to_resolved: Dict[str, List[str]] = {}
        self._init_agent()

    # ── Agent initialisation ─────────────────────────────────────

    def _init_agent(self) -> None:
        """Build the planner LangGraph if LLM credentials are available."""
        try:
            from ..llm import get_llm_client
            from ..tools import set_tool_clients
            from ..graph import create_planner_graph

            llm = get_llm_client()
            if llm:
                set_tool_clients(
                    self._questions_client_ext or QuestionsClient(),
                    self._knowledge_client_ext or KnowledgeClient(),
                )
                self._planner_graph = create_planner_graph(llm)
                logger.info("Planner agent graph ready (LLM mode)")
            else:
                logger.info("No LLM_API_KEY – using deterministic pipeline")
        except Exception as exc:
            logger.warning("Agent init failed, falling back to deterministic", error=str(exc))
            self._planner_graph = None

    # ── Agent-based program building ──────────────────────────────

    async def _build_with_agent(
        self, session_id: str, params: Dict[str, Any], limit: int = 5
    ) -> Optional[tuple]:
        """Run the LangGraph planner agent and return (program, meta) or None on failure."""
        if not self._planner_graph:
            return None

        from ..graph.state import PlannerState

        initial_state: PlannerState = {
            "session_id": session_id,
            "mode": params.get("mode", "interview"),
            "topics": params.get("topics", []),
            "level": params.get("level", "middle"),
            "weak_topics": params.get("subtopics") or [],
            "n_questions": limit,
            "messages": [],
            "candidate_questions": [],
            "coverage_report": None,
            "validation_report": None,
            "knowledge_snippets": [],
            "final_program": None,
            "program_meta": None,
            "iteration": 0,
            "max_iterations": 8,
            "error": None,
        }

        try:
            final_state = await self._planner_graph.ainvoke(initial_state)
        except Exception as exc:
            logger.error("Planner graph execution failed", session_id=session_id, error=str(exc))
            return None

        program_questions = final_state.get("final_program")
        program_meta_raw = final_state.get("program_meta") or {}
        agent_error = final_state.get("error")

        if agent_error or not program_questions:
            logger.warning(
                "Agent did not produce a valid program, falling back",
                session_id=session_id,
                error=agent_error,
            )
            return None

        # Build a lookup from question text to real database records
        # (candidate_questions from tool calls have real IDs from the DB)
        candidate_questions = final_state.get("candidate_questions") or []
        cand_by_text: Dict[str, Dict[str, Any]] = {}
        for cq in candidate_questions:
            cqd, _ = _extract_question_data(cq)
            cq_text = _extract_question_text(cqd)
            if cq_text:
                cand_by_text[_normalize_text(cq_text)] = cq

        # Normalise to the format expected by downstream services
        # CRITICAL: ensure every question has a unique ID (duplicate IDs break question tracking)
        import re as _re
        normalised: List[Dict[str, Any]] = []
        used_ids: set = set()
        for idx, q in enumerate(program_questions, start=1):
            q_text = q.get("question", "")
            q_id = q.get("question_id") or q.get("id") or ""

            # Try to match against real DB records by text
            is_uuid = bool(_re.match(r'^[0-9a-f-]{30,}$', q_id or ""))
            if not is_uuid and q_text:
                matched_cq = cand_by_text.get(_normalize_text(q_text))
                if matched_cq:
                    cqd, _ = _extract_question_data(matched_cq)
                    q_id = matched_cq.get("ID") or matched_cq.get("id") or cqd.get("id") or q_id

            # Guarantee uniqueness: append order suffix if this ID was already used
            base_id = q_id or f"q_{idx}"
            final_id = base_id
            if final_id in used_ids:
                final_id = f"{base_id}_{idx}"
            used_ids.add(final_id)

            # Only resolve a finer subtopic when the user explicitly filtered by
            # subtopic (weak_topics). Otherwise keep the planner's topic as-is,
            # so the plan groups cleanly under the requested top-level topic.
            topic = q.get("topic")
            user_subtopics = params.get("subtopics") or []
            if user_subtopics and q_text:
                matched_cq = cand_by_text.get(_normalize_text(q_text))
                if matched_cq:
                    _, theory_id = _extract_question_data(matched_cq)
                    if theory_id:
                        resolved = await self._resolve_question_tag(theory_id)
                        finest = self._pick_finest_subtopic(resolved or [])
                        if finest:
                            topic = finest

            normalised.append({
                "id": final_id,
                "question": q_text,
                "theory": q.get("theory", ""),
                "order": q.get("order", idx),
                "topic": topic,
            })

        program = {"questions": normalised}
        meta = ProgramMeta(
            validation_passed=program_meta_raw.get("validation_passed", True),
            coverage=program_meta_raw.get("coverage", {}),
            fallback_reason=program_meta_raw.get("fallback_reason"),
            generator_version=program_meta_raw.get("generator_version", "planner-agent-1.0"),
        )
        return program, meta

    # ── Study-mode dedicated builder ──────────────────────────────
    # Hierarchy: subtopic → points (1-n) → questions (1-3 per point).
    # Each point carries its own adequate-size theory fragment and all questions
    # inside a point must be answerable from that fragment alone.

    async def _build_study_program(
        self, session_id: str, params: Dict[str, Any]
    ) -> Optional[Tuple[Dict[str, Any], ProgramMeta]]:
        """Generate a study program (points + questions) using LLM + knowledge base."""
        from ..llm import get_llm_client

        llm = get_llm_client()
        if not llm:
            logger.info("Study builder: no LLM available, falling back", session_id=session_id)
            return None

        subtopics = params.get("subtopics") or params.get("topics") or []
        if not subtopics:
            return None

        program_questions: List[Dict[str, Any]] = []
        order_counter = 1
        coverage: Dict[str, int] = {}
        fallback_reason: Optional[str] = None

        for subtopic in subtopics:
            theory_text = await self._collect_subtopic_theory(subtopic)
            if not theory_text:
                logger.warning(
                    "Study builder: no theory for subtopic, skipping",
                    subtopic=subtopic, session_id=session_id,
                )
                fallback_reason = fallback_reason or f"no_theory:{subtopic}"
                continue

            points_data = await self._llm_generate_points(llm, subtopic, theory_text)
            if not points_data:
                logger.warning(
                    "Study builder: LLM produced no points, skipping subtopic",
                    subtopic=subtopic, session_id=session_id,
                )
                fallback_reason = fallback_reason or f"llm_failed:{subtopic}"
                continue

            for p_idx, point in enumerate(points_data):
                point_title = (point.get("title") or "").strip()
                point_theory = (point.get("theory") or "").strip()
                questions = point.get("questions") or []
                if not point_title or not point_theory or not questions:
                    continue
                point_id = f"{subtopic}::p{p_idx}"
                for q_idx, q_text in enumerate(questions[:3]):
                    q_text = (q_text or "").strip()
                    if not q_text:
                        continue
                    program_questions.append({
                        "id": f"{point_id}::q{q_idx}",
                        "question": q_text,
                        "theory": point_theory,
                        "order": order_counter,
                        "topic": subtopic,
                        "subtopic": subtopic,
                        "point_id": point_id,
                        "point_title": point_title,
                        "point_theory": point_theory,
                        "question_in_point": q_idx,
                    })
                    order_counter += 1
            coverage[subtopic] = sum(
                1 for q in program_questions if q.get("subtopic") == subtopic
            )

        if not program_questions:
            return None

        meta = ProgramMeta(
            validation_passed=not fallback_reason,
            coverage=coverage,
            fallback_reason=fallback_reason,
            generator_version="study-builder-1.0",
        )
        logger.info(
            "Study program built",
            session_id=session_id,
            subtopics=subtopics,
            questions_count=len(program_questions),
        )
        return {"questions": program_questions}, meta

    async def _collect_subtopic_theory(self, subtopic: str) -> str:
        """Fetch and concatenate all knowledge-base theory for a subtopic tag."""
        client = self._new_knowledge_client()
        try:
            items = await client.get_knowledge_by_filters(tags=[subtopic])
        except Exception as exc:
            logger.warning("Study builder: knowledge fetch failed",
                           subtopic=subtopic, error=str(exc))
            return ""
        finally:
            await client.close()

        if not items:
            return ""

        parts: List[str] = []
        for item in items:
            kd = _extract_knowledge_data(item)
            title = (kd.get("title") or "").strip()
            segments = kd.get("segments") or []
            for seg in segments:
                if not isinstance(seg, dict):
                    continue
                content = (seg.get("content") or "").strip()
                if not content:
                    continue
                heading = seg.get("title") or ""
                if heading:
                    parts.append(f"**{heading.strip()}**\n{content}")
                else:
                    parts.append(content)
            if not segments:
                content = (kd.get("content") or "").strip()
                if content:
                    parts.append(f"**{title}**\n{content}" if title else content)
        return "\n\n".join(parts).strip()

    async def _llm_generate_points(
        self, llm, subtopic: str, theory_text: str
    ) -> Optional[List[Dict[str, Any]]]:
        """Ask the LLM to split theory into 1-n points with 1-3 questions each."""
        import json

        system = (
            "You are an expert ML tutor. Given a theory article about a subtopic, "
            "split it into 1-4 coherent learning POINTS. Each point must be a "
            "self-contained, SUBSTANTIVE theory fragment of 500-1000 characters "
            "(roughly 3-6 sentences, or 2-3 short paragraphs) covering ONE specific "
            "aspect of the subtopic in depth. The theory MUST include: a clear "
            "definition, a concrete mechanism or example, and any key terminology. "
            "Do NOT write terse one-sentence fragments — the student will read this "
            "as their primary learning material before answering. For each point, "
            "generate 1-3 STUDY QUESTIONS that can be answered using ONLY that point's "
            "theory fragment — never require knowledge beyond it. Questions must be "
            "strictly about the point's content, not tangential or more advanced topics."
        )
        user = f"""Subtopic: {subtopic}

Theory:
{theory_text}

Output ONLY valid JSON in this exact shape (no markdown fences, no commentary):
{{
  "points": [
    {{
      "title": "<short point title in Russian>",
      "theory": "<the exact theory fragment in Russian that this point covers>",
      "questions": ["<question 1 in Russian>", "<question 2 in Russian>", "<question 3 in Russian>"]
    }}
  ]
}}

Rules:
- 1 to 4 points total
- Each point's "theory" field: 500-1000 characters of substantive content (NOT a summary)
- Each point: 1 to 3 questions
- All questions must be answerable from the point's theory fragment alone
- Use Russian for all user-facing text
- Do NOT invent facts not present in the source theory above
- The "theory" field should LOOK like a rich learning paragraph, not a bullet point
"""
        try:
            raw = await llm.chat_plain([
                {"role": "system", "content": system},
                {"role": "user", "content": user},
            ])
        except Exception as exc:
            logger.warning("Study builder: LLM call failed",
                           subtopic=subtopic, error=str(exc))
            return None

        if not raw:
            return None

        cleaned = raw.strip()
        if cleaned.startswith("```"):
            # Strip markdown fence if the LLM ignored instructions
            cleaned = cleaned.strip("`")
            if cleaned.lower().startswith("json"):
                cleaned = cleaned[4:]
            cleaned = cleaned.strip()

        try:
            parsed = json.loads(cleaned)
        except json.JSONDecodeError as exc:
            logger.warning("Study builder: JSON parse failed",
                           subtopic=subtopic, error=str(exc),
                           raw_preview=cleaned[:200])
            return None

        points = parsed.get("points") if isinstance(parsed, dict) else None
        if not isinstance(points, list) or not points:
            return None
        return points

    # ── Common helpers ────────────────────────────────────────────

    def _new_questions_client(self) -> QuestionsClient:
        return self._questions_client_ext or QuestionsClient()

    def _new_knowledge_client(self) -> KnowledgeClient:
        return self._knowledge_client_ext or KnowledgeClient()

    # ── topic mapping ────────────────────────────────────────────

    @staticmethod
    def _map_topics_to_tags(topics: List[str]) -> List[str]:
        tags: set = set()
        for t in topics:
            key = t.lower().strip()
            mapped = TOPIC_TO_TAGS.get(key)
            if mapped:
                tags.update(mapped)
            else:
                # Normalize spaces to underscores to match knowledge-base tag format
                tags.add(key.replace(" ", "_"))
        return sorted(tags)

    # ── pipeline steps ───────────────────────────────────────────

    async def _fetch_questions(self, complexity: Optional[int] = None) -> List[Dict[str, Any]]:
        client = self._new_questions_client()
        try:
            questions = await client.get_questions_by_filters(complexity=complexity)
            self.metrics.questions_fetched_total.labels(
                service=settings.service_name, status="success"
            ).inc()
            return questions or []
        except Exception:
            self.metrics.questions_fetched_total.labels(
                service=settings.service_name, status="error"
            ).inc()
            raise
        finally:
            await client.close()

    async def _resolve_question_tag(
        self, theory_id: str
    ) -> Optional[List[str]]:
        """Fetch knowledge record and return its tags, or None on error."""
        client = self._new_knowledge_client()
        try:
            knowledge = await client.get_knowledge_by_id(theory_id)
            if not knowledge:
                return None
            kd = _extract_knowledge_data(knowledge)
            return kd.get("tags", [])
        except Exception as e:
            logger.warning("Failed to resolve knowledge tags",
                           theory_id=theory_id, error=str(e))
            return None
        finally:
            await client.close()

    async def _filter_by_topic(
        self, questions: List[Dict[str, Any]], tags: List[str], strict: bool = False
    ) -> Tuple[List[Dict[str, Any]], Dict[str, List[Dict[str, Any]]]]:
        """Return (filtered list, questions grouped by matched tag).

        Non-strict: questions without a theory_id or with an unresolved tag pass through.
        Strict: ONLY questions whose resolved tags intersect with `tags` are kept —
        used for study mode where the user picks a specific subtopic and must get
        exclusively those questions (never pollute with untagged/unresolved ones).

        Side effect: caches resolved tags per qid in self._qid_to_resolved
        so later program-build can pick a fine-grained subtopic for the plan.
        """
        tag_set = set(tags)
        filtered: List[Dict[str, Any]] = []
        by_tag: Dict[str, List[Dict[str, Any]]] = defaultdict(list)

        for q in questions:
            qd, theory_id = _extract_question_data(q)
            qid = qd.get("id") if isinstance(qd, dict) else None
            if not theory_id:
                if strict:
                    continue
                filtered.append(q)
                by_tag["_untagged"].append(q)
                continue

            resolved = await self._resolve_question_tag(theory_id)
            if resolved is None:
                if strict:
                    # In strict mode, still allow match by direct theory_id (e.g. user picked
                    # "theory_rag" → question with theory_id="theory_rag" matches even if
                    # the knowledge record is missing or has unrelated domain tags).
                    if theory_id in tag_set:
                        if qid:
                            self._qid_to_resolved[qid] = [theory_id]
                        filtered.append(q)
                        by_tag[theory_id].append(q)
                    continue
                filtered.append(q)
                by_tag["_unresolved"].append(q)
                continue

            # Include the theory_id itself in the resolved set so a subtopic request
            # like ["theory_rag"] matches questions with theory_id="theory_rag" even
            # when the knowledge record's tags are domain labels (["rag","nlp",...]).
            resolved_with_id = list(resolved)
            if theory_id and theory_id not in resolved_with_id:
                resolved_with_id.append(theory_id)
            if qid:
                self._qid_to_resolved[qid] = resolved_with_id

            matched = tag_set.intersection(resolved_with_id)
            if matched:
                filtered.append(q)
                for t in matched:
                    by_tag[t].append(q)

        return filtered, dict(by_tag)

    @staticmethod
    def _pick_finest_subtopic(resolved: List[str]) -> Optional[str]:
        """Pick the most specific subtopic tag (prefers theory_*/practice_*)
        over coarse top-level topic tags like 'llm'/'nlp'/'machine_learning'."""
        if not resolved:
            return None
        for t in resolved:
            if t.startswith("theory_") or t.startswith("practice_"):
                return t
        coarse = {"llm", "nlp", "cv", "ml", "machine_learning", "deep_learning",
                  "neural_networks", "data_science"}
        for t in resolved:
            if t not in coarse:
                return t
        return resolved[0]

    @staticmethod
    def _dedup(questions: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Remove duplicates by question_id and by text hash."""
        seen_ids: set = set()
        seen_hashes: set = set()
        result: List[Dict[str, Any]] = []

        for q in questions:
            qd, _ = _extract_question_data(q)
            qid = qd.get("id") if isinstance(qd, dict) else None
            text = _extract_question_text(qd) if isinstance(qd, dict) else ""
            h = _text_hash(text) if text else None

            if qid and qid in seen_ids:
                continue
            if h and h in seen_hashes:
                continue

            if qid:
                seen_ids.add(qid)
            if h:
                seen_hashes.add(h)
            result.append(q)

        return result

    @staticmethod
    def _ensure_coverage(
        questions: List[Dict[str, Any]],
        by_tag: Dict[str, List[Dict[str, Any]]],
        requested_tags: List[str],
        limit: int,
    ) -> Tuple[List[Dict[str, Any]], Dict[str, int]]:
        """Pick at least one question per requested tag (round-robin),
        then fill remaining slots. Returns (selected, coverage_map)."""
        selected_ids: set = set()
        selected: List[Dict[str, Any]] = []
        coverage: Dict[str, int] = {}

        for tag in requested_tags:
            candidates = by_tag.get(tag, [])
            for c in candidates:
                cd, _ = _extract_question_data(c)
                cid = cd.get("id") if isinstance(cd, dict) else id(c)
                if cid not in selected_ids:
                    selected.append(c)
                    selected_ids.add(cid)
                    coverage[tag] = coverage.get(tag, 0) + 1
                    break
            if tag not in coverage:
                coverage[tag] = 0

        for q in questions:
            if len(selected) >= limit:
                break
            qd, _ = _extract_question_data(q)
            qid = qd.get("id") if isinstance(qd, dict) else id(q)
            if qid not in selected_ids:
                selected.append(q)
                selected_ids.add(qid)

        return selected[:limit], coverage

    @staticmethod
    def _balance(
        questions: List[Dict[str, Any]],
        by_tag: Dict[str, List[Dict[str, Any]]],
        max_ratio: float,
        limit: int,
    ) -> List[Dict[str, Any]]:
        """Cap the share of any single tag to max_ratio of the program."""
        if not questions:
            return questions

        max_per_tag = max(1, int(limit * max_ratio))

        qid_to_tags: Dict[str, set] = {}
        for tag, qs in by_tag.items():
            for q in qs:
                qd, _ = _extract_question_data(q)
                qid = qd.get("id") if isinstance(qd, dict) else str(id(q))
                qid_to_tags.setdefault(qid, set()).add(tag)

        tag_counts: Dict[str, int] = defaultdict(int)
        result: List[Dict[str, Any]] = []

        for q in questions:
            qd, _ = _extract_question_data(q)
            qid = qd.get("id") if isinstance(qd, dict) else str(id(q))
            tags = qid_to_tags.get(qid, set())
            over = any(tag_counts[t] >= max_per_tag for t in tags if t not in ("_untagged", "_unresolved"))
            if over:
                continue
            result.append(q)
            for t in tags:
                tag_counts[t] += 1

        if len(result) < len(questions):
            result_ids = {
                (_extract_question_data(q)[0].get("id") if isinstance(_extract_question_data(q)[0], dict) else id(q))
                for q in result
            }
            for q in questions:
                if len(result) >= limit:
                    break
                qd, _ = _extract_question_data(q)
                qid = qd.get("id") if isinstance(qd, dict) else id(q)
                if qid not in result_ids:
                    result.append(q)
                    result_ids.add(qid)

        return result[:limit]

    @staticmethod
    def _sort_by_theory(questions: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Group by theory_id, preserving insertion order within groups."""
        groups: Dict[Optional[str], List[Dict[str, Any]]] = {}
        for q in questions:
            _, tid = _extract_question_data(q)
            groups.setdefault(tid, []).append(q)
        result: List[Dict[str, Any]] = []
        for qs in groups.values():
            result.extend(qs)
        return result

    async def _enrich_with_theory(
        self, question_data: Dict[str, Any], theory_id: Optional[str]
    ) -> str:
        """Fetch theory text for a question."""
        if not theory_id:
            return ""
        client = self._new_knowledge_client()
        try:
            knowledge = await client.get_knowledge_by_id(theory_id)
            if not knowledge:
                return ""
            self.metrics.knowledge_fetched_total.labels(
                service=settings.service_name, status="success"
            ).inc()
            kd = _extract_knowledge_data(knowledge)
            segments = kd.get("segments", [])
            # Include all substantive segment types so study theory blocks read
            # as a full mini-article, not just a one-sentence definition.
            wanted = (
                "definition",
                "intuition",
                "explanation",
                "detail",
                "example",
                "formula",
                "pitfall",
                "summary",
                "note",
            )
            ordered_parts: List[str] = []
            for seg in segments:
                if not isinstance(seg, dict):
                    continue
                seg_type = seg.get("type", "").lower()
                content = (seg.get("content") or "").strip()
                if not content:
                    continue
                if seg_type in wanted or not seg_type:
                    heading = seg.get("title") or ""
                    if heading:
                        ordered_parts.append(f"**{heading.strip()}**\n{content}")
                    else:
                        ordered_parts.append(content)
            if ordered_parts:
                return "\n\n".join(ordered_parts)
            # Fallback: whole content field if segments are empty
            return (kd.get("content") or "").strip()
        except Exception as e:
            self.metrics.knowledge_fetched_total.labels(
                service=settings.service_name, status="error"
            ).inc()
            logger.warning("Failed to fetch theory", theory_id=theory_id, error=str(e))
            return ""
        finally:
            await client.close()

    # ── mode profiles ────────────────────────────────────────────

    @staticmethod
    def _apply_mode_profile(
        questions: List[Dict[str, Any]],
        mode: str,
        weak_topics: Optional[List[str]],
    ) -> List[Dict[str, Any]]:
        """Reorder / weight questions according to the session mode.

        - interview: uniform coverage, progressive difficulty (default sort)
        - training: prioritize weak_topics, boost practical questions via
          ``settings.training_practice_ratio``
        - study: boost theoretical questions via ``settings.study_theory_ratio``
        """
        if not questions:
            return questions

        if mode == "training":
            if weak_topics:
                weak_set = {t.lower().strip() for t in weak_topics}

                def weak_priority(q: Dict[str, Any]) -> int:
                    qd, _ = _extract_question_data(q)
                    if isinstance(qd, dict):
                        tags = qd.get("tags", [])
                        if any(t in weak_set for t in tags):
                            return 0
                    return 1

                questions = sorted(questions, key=weak_priority)

            practice_ratio = settings.training_practice_ratio
            practice_count = max(1, int(len(questions) * practice_ratio))

            def is_practical(q: Dict[str, Any]) -> bool:
                qd, _ = _extract_question_data(q)
                if isinstance(qd, dict):
                    qt = qd.get("question_type", "")
                    return qt in ("coding", "case", "practical", "applied")
                return False

            practical = [q for q in questions if is_practical(q)]
            theoretical = [q for q in questions if not is_practical(q)]
            result = practical[:practice_count]
            remaining = theoretical + practical[practice_count:]
            result.extend(remaining[: len(questions) - len(result)])
            questions = result

        elif mode == "study":
            theory_ratio = settings.study_theory_ratio
            theory_count = max(1, int(len(questions) * theory_ratio))

            def has_theory(q: Dict[str, Any]) -> bool:
                _, tid = _extract_question_data(q)
                return bool(tid)

            with_theory = [q for q in questions if has_theory(q)]
            without_theory = [q for q in questions if not has_theory(q)]
            result = with_theory[:theory_count]
            remaining = without_theory + with_theory[theory_count:]
            result.extend(remaining[: len(questions) - len(result)])
            questions = result

        return questions

    # ── main build ───────────────────────────────────────────────

    async def _fetch_user_weak_topics(self, user_id: str, topics: List[str]) -> List[str]:
        """Fetch user's weak topics from results-crud (episodic memory §10.6).

        Returns a list of topic names where avg score < 0.5 across past sessions.
        Falls back to empty list on any error.
        """
        try:
            import httpx
            params: Dict[str, str] = {"user_id": user_id}
            if topics:
                params["topics"] = ",".join(topics)
            async with httpx.AsyncClient(timeout=5.0) as client:
                resp = await client.get(
                    f"{settings.results_crud_url}/results/user-summary",
                    params=params,
                )
                if resp.status_code == 200:
                    data = resp.json()
                    weak = data.get("weak_topics", [])
                    logger.info(
                        "Episodic memory fetched",
                        user_id=user_id,
                        weak_topics=weak,
                        total_sessions=data.get("total_sessions", 0),
                    )
                    return weak
        except Exception as exc:
            logger.warning("Episodic memory fetch failed", user_id=user_id, error=str(exc))
        return []

    async def build_interview_program(
        self, session_id: str, params: Dict[str, Any]
    ) -> Tuple[Dict[str, Any], ProgramMeta]:
        """Build interview program using the LangGraph planner agent when available,
        falling back to the deterministic pipeline otherwise."""
        start_time = time.time()
        # Reset per-request qid→tags cache so different sessions can't leak into each other.
        self._qid_to_resolved = {}
        topics = params.get("topics", [])
        level = params.get("level", "middle")
        mode = params.get("mode", "interview")
        weak_topics = params.get("subtopics")

        # For training: no question limit — use all available questions for selected subtopics.
        # For study: dynamic count per chosen subtopic (clamped to [min, max] from settings).
        #   N subtopics → up to N * STUDY_PER_SUBTOPIC_MAX questions; pipeline returns at most
        #   what exists in the DB. Must stay strictly on-topic — never pull unrelated questions.
        # For interview: respect explicit num_questions (from UI duration picker) or config default.
        if mode == "training":
            limit = 999  # effectively unlimited; pipeline will return all relevant questions
        elif mode == "study":
            n_subtopics = len(weak_topics) if weak_topics else max(1, len(topics))
            per_max = settings.study_per_subtopic_max
            per_min = settings.study_per_subtopic_min
            # Cap by per_max * n; final count clamps from below by per_min * n during selection.
            limit = max(per_min * n_subtopics, per_max * n_subtopics)
        else:
            explicit = params.get("num_questions")
            limit = int(explicit) if explicit and int(explicit) > 0 else settings.questions_per_interview

        # Episodic memory: for interview mode, fetch historically weak topics from past results
        if mode == "interview" and params.get("use_previous_results") and params.get("user_id"):
            history_weak = await self._fetch_user_weak_topics(params["user_id"], topics)
            if history_weak:
                existing = set(weak_topics or [])
                merged = list(existing | set(history_weak))
                weak_topics = merged
                logger.info(
                    "Merged episodic weak topics",
                    session_id=session_id,
                    weak_topics=weak_topics,
                )

        logger.info("Building interview program",
                     session_id=session_id, topics=topics,
                     level=level, mode=mode)

        # ── Study mode: dedicated LLM-based points/questions builder ────────
        if mode == "study":
            study_result = await self._build_study_program(session_id, params)
            if study_result is not None:
                program, meta = study_result
                duration = time.time() - start_time
                self.metrics.program_building_duration.labels(
                    service=settings.service_name
                ).observe(duration)
                self.metrics.interview_programs_built_total.labels(
                    service=settings.service_name,
                    status="success" if meta.validation_passed else "fallback",
                ).inc()
                logger.info(
                    "Study program built (dedicated)",
                    session_id=session_id,
                    questions_count=len(program.get("questions", [])),
                    duration_s=round(duration, 3),
                )
                return program, meta
            logger.warning(
                "Study builder returned nothing, falling back to agent/deterministic",
                session_id=session_id,
            )

        # ── Try LangGraph planner agent first (§8 stage 2) ──────────────────
        if self._planner_graph:
            agent_result = await self._build_with_agent(session_id, params, limit)
            if agent_result is not None:
                program, meta = agent_result
                duration = time.time() - start_time
                self.metrics.program_building_duration.labels(
                    service=settings.service_name
                ).observe(duration)
                self.metrics.interview_programs_built_total.labels(
                    service=settings.service_name,
                    status="success" if meta.validation_passed else "fallback",
                ).inc()
                logger.info(
                    "Program built (agent)",
                    session_id=session_id,
                    questions_count=len(program.get("questions", [])),
                    validation_passed=meta.validation_passed,
                    duration_s=round(duration, 3),
                )
                return program, meta
            logger.info(
                "Agent fallback – switching to deterministic pipeline",
                session_id=session_id,
            )
        # ── Deterministic pipeline ───────────────────────────────────────────

        complexity = LEVEL_TO_COMPLEXITY.get(level.lower(), 2)
        # Study mode: filter strictly by the selected subtopic (weak_topics),
        # e.g. ["theory_rag"] → only RAG questions. Fall back to topic tags if no subtopic.
        if mode == "study" and weak_topics:
            tags = list(weak_topics)
        else:
            tags = self._map_topics_to_tags(topics)

        fallback_reason: Optional[str] = None

        # 1. Fetch at requested complexity
        questions = await self._fetch_questions(complexity)
        if not questions:
            logger.warning("No questions for complexity, trying without filter",
                           complexity=complexity)
            questions = await self._fetch_questions()  # all complexities
            if not questions:
                fallback_reason = "no_questions_in_database"

        # 2. Filter by topic
        # Study mode: strict — only questions whose resolved tags match the selected subtopic.
        # Never silently expand to "all" or pass untagged/unresolved through.
        strict_filter = (mode == "study")
        if questions and tags:
            filtered, by_tag = await self._filter_by_topic(questions, tags, strict=strict_filter)
            if filtered:
                questions = filtered
            elif strict_filter:
                # Study mode with zero matches at this complexity — leave empty; the
                # supplement step below retries across all complexities (still strict).
                logger.warning("Study: zero questions matched subtopic at complexity, will retry across all complexities",
                               tags=tags, complexity=complexity)
                questions = []
                by_tag = {}
            else:
                logger.warning("No questions after topic filter, using all",
                               tags=tags)
                by_tag = {"_all": questions}
        else:
            by_tag = {"_all": questions} if questions else {}

        # 2b. If fewer questions than limit, supplement from other complexities (all)
        if tags and len(questions) < limit:
            logger.warning(
                "Not enough questions after topic filter, supplementing from other complexities",
                found=len(questions), limit=limit, tags=tags,
            )
            all_questions = await self._fetch_questions()  # no complexity filter = all
            if all_questions:
                extra_filtered, extra_by_tag = await self._filter_by_topic(all_questions, tags, strict=strict_filter)
                if extra_filtered:
                    # Merge: add questions not already in current set
                    existing_ids = {
                        (_extract_question_data(q)[0].get("id") if isinstance(_extract_question_data(q)[0], dict) else id(q))
                        for q in questions
                    }
                    for q in extra_filtered:
                        qd, _ = _extract_question_data(q)
                        qid = qd.get("id") if isinstance(qd, dict) else id(q)
                        if qid not in existing_ids:
                            questions.append(q)
                            existing_ids.add(qid)
                            # Merge by_tag entries
                            for tag, qs in extra_by_tag.items():
                                for eq in qs:
                                    eqd, _ = _extract_question_data(eq)
                                    if eqd.get("id") == qid:
                                        by_tag.setdefault(tag, []).append(eq)
                    fallback_reason = fallback_reason or "insufficient_questions_at_level"
                    logger.info(
                        "Supplemented questions from other complexities",
                        total=len(questions), tags=tags,
                    )

        # 2c. Still insufficient? Drop topic filter, use any questions from all complexities.
        # NEVER do this for study mode — must stay strictly on the selected topic.
        if len(questions) < limit and mode != "study":
            logger.warning(
                "Still not enough questions after topic supplement, dropping topic filter",
                found=len(questions), limit=limit, tags=tags,
            )
            all_questions = await self._fetch_questions()  # no complexity filter = all
            if all_questions:
                existing_ids = {
                    (_extract_question_data(q)[0].get("id") if isinstance(_extract_question_data(q)[0], dict) else id(q))
                    for q in questions
                }
                for q in all_questions:
                    if len(questions) >= limit:
                        break
                    qd, _ = _extract_question_data(q)
                    qid = qd.get("id") if isinstance(qd, dict) else id(q)
                    if qid not in existing_ids:
                        questions.append(q)
                        existing_ids.add(qid)
                        by_tag.setdefault("_other", []).append(q)
                fallback_reason = fallback_reason or "insufficient_topic_questions"
                logger.info(
                    "Added off-topic questions to meet limit",
                    total=len(questions), limit=limit,
                )

        if not questions:
            meta = ProgramMeta(
                validation_passed=False,
                fallback_reason=fallback_reason or "no_questions_available",
            )
            return {"questions": []}, meta

        # 3. Dedup
        questions = self._dedup(questions)

        # 4. Coverage
        questions, coverage = self._ensure_coverage(questions, by_tag, tags, limit)

        # 5. Balance
        questions = self._balance(questions, by_tag, settings.max_same_topic_ratio, limit)

        # 6. Mode profile
        questions = self._apply_mode_profile(questions, mode, weak_topics)

        # 7. Sort by theory grouping
        questions = self._sort_by_theory(questions)

        # 7b. Reverse-index: question_id → matched subtopic tag (for grouping in UI plan)
        qid_to_topic: Dict[str, str] = {}
        for tag, qs in by_tag.items():
            if tag.startswith("_"):
                continue
            for q in qs:
                qd_, _ = _extract_question_data(q)
                qid_ = qd_.get("id") if isinstance(qd_, dict) else None
                if qid_ and qid_ not in qid_to_topic:
                    qid_to_topic[qid_] = tag

        # 8. Build final program with enriched theory
        program_questions = []
        for idx, q in enumerate(questions, start=1):
            qd, theory_id = _extract_question_data(q)
            question_text = _extract_question_text(qd) if isinstance(qd, dict) else ""
            theory_text = await self._enrich_with_theory(qd, theory_id)
            qid = qd.get("id") if isinstance(qd, dict) else None
            # Only prefer the fine-grained subtopic when the user explicitly
            # filtered by subtopic. Otherwise keep the coarse matched tag,
            # so the plan groups cleanly (e.g. "LLM — 6 questions") instead of
            # scattering across 6 random subtopics.
            topic = None
            if weak_topics:
                resolved = self._qid_to_resolved.get(qid) if qid else None
                topic = self._pick_finest_subtopic(resolved or [])
            if not topic and qid:
                topic = qid_to_topic.get(qid)

            program_questions.append({
                "id": qid,
                "question": question_text,
                "theory": theory_text,
                "order": idx,
                "topic": topic,
            })

        program = {"questions": program_questions}

        uncovered = [t for t in tags if coverage.get(t, 0) == 0]
        if uncovered:
            fallback_reason = f"missing_coverage:{','.join(uncovered)}"

        meta = ProgramMeta(
            validation_passed=not fallback_reason,
            coverage=coverage,
            fallback_reason=fallback_reason,
        )

        duration = time.time() - start_time
        self.metrics.program_building_duration.labels(
            service=settings.service_name
        ).observe(duration)
        self.metrics.interview_programs_built_total.labels(
            service=settings.service_name,
            status="success" if meta.validation_passed else "fallback",
        ).inc()

        logger.info("Program built",
                     session_id=session_id,
                     questions_count=len(program_questions),
                     coverage=coverage,
                     validation_passed=meta.validation_passed,
                     duration_s=round(duration, 3))

        return program, meta

    async def handle_interview_build_request(
        self, session_id: str, params: Dict[str, Any]
    ):
        """Handle interview build request from Kafka."""
        try:
            program, meta = await self.build_interview_program(session_id, params)
            self.kafka_producer.send_interview_program(session_id, program, meta)
            logger.info("Interview program sent to Kafka", session_id=session_id)
        except Exception as e:
            logger.error("Failed to build interview program",
                         session_id=session_id, error=str(e), exc_info=True)
            try:
                self.kafka_producer.send_failure(session_id, str(e))
            except Exception as send_err:
                logger.error("Failed to send failure response",
                             session_id=session_id, error=str(send_err))

    async def close(self):
        self.kafka_producer.close()
