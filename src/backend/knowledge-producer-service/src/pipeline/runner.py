"""Multi-step LLM pipeline: gather context → structure → dedup → save_draft.

Compatible with §8 stage 0 and the tool interface of agent services.
Uses an explicit pipeline (list of step functions) rather than LangGraph
for simplicity; can be migrated to LangGraph if needed.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

from ..config import settings
from ..logger.setup import get_logger
from ..schemas import (
    Draft,
    DraftKind,
    KnowledgeDraftContent,
    QuestionDraftContent,
    RawDocument,
    ReviewStatus,
)

logger = get_logger(__name__)


class PipelineRunner:
    """Orchestrates the LLM ingestion pipeline with configurable limits."""

    def __init__(
        self,
        llm_client: Any = None,
        knowledge_search_fn: Any = None,
        question_search_fn: Any = None,
    ):
        self._llm = llm_client
        self._search_kb = knowledge_search_fn
        self._search_q = question_search_fn
        self._max_llm_calls = settings.max_llm_calls_per_job
        self._llm_call_count = 0

    def _can_call_llm(self) -> bool:
        return self._llm is not None and self._llm_call_count < self._max_llm_calls

    async def _call_llm(self, prompt: str) -> str:
        if not self._can_call_llm():
            raise RuntimeError("LLM call limit reached or no LLM configured")
        self._llm_call_count += 1
        return await self._llm.generate(prompt)

    # ── Pipeline steps ───────────────────────────────────────────

    async def step_gather_context(
        self,
        raw: RawDocument,
        topic: str,
    ) -> Dict[str, Any]:
        """Step 1: Gather context from raw document and optional search."""
        context: Dict[str, Any] = {
            "source_uri": raw.source_uri,
            "mime": raw.mime,
            "text": raw.text[:50_000],
            "topic": topic,
            "metadata": raw.metadata,
        }

        if self._search_kb and topic:
            try:
                existing = await self._search_kb(topic)
                context["existing_knowledge"] = existing[:5] if existing else []
            except Exception as e:
                logger.warning("KB search failed during gather", error=str(e))

        return context

    async def step_structure(
        self,
        context: Dict[str, Any],
        kind: DraftKind,
    ) -> Dict[str, Any]:
        """Step 2: Use LLM to structure raw text into KB/question format."""
        if not self._can_call_llm():
            return self._fallback_structure(context, kind)

        topic = context.get("topic", "general")
        text_preview = context["text"][:8000]

        if kind == DraftKind.KNOWLEDGE:
            prompt = (
                f"Extract structured knowledge from the text below.\n"
                f"Topic: {topic}\n"
                f"Return JSON with fields: title, topic, complexity (1-3), "
                f"segments (array of {{type, content, order}}), "
                f"relations (array of {{target_id, relation_type}}).\n"
                f"Allowed segment types: {settings.allowed_segment_types}\n\n"
                f"Text:\n{text_preview}"
            )
        else:
            prompt = (
                f"Generate practice questions from the text below.\n"
                f"Topic: {topic}\n"
                f"Return JSON array, each item: {{content, ideal_answer, "
                f"question_type, complexity (1-3)}}.\n"
                f"Allowed question types: {settings.allowed_question_types}\n\n"
                f"Text:\n{text_preview}"
            )

        try:
            raw_response = await self._call_llm(prompt)
            import json
            structured = json.loads(raw_response)
            return {"structured": structured, "kind": kind.value}
        except Exception as e:
            logger.warning("LLM structuring failed, using fallback", error=str(e))
            return self._fallback_structure(context, kind)

    @staticmethod
    def _fallback_structure(context: Dict[str, Any], kind: DraftKind) -> Dict[str, Any]:
        """Deterministic fallback when LLM is unavailable."""
        topic = context.get("topic", "general")
        text = context.get("text", "")[:2000]

        if kind == DraftKind.KNOWLEDGE:
            return {
                "structured": {
                    "title": f"Material on {topic}",
                    "topic": topic,
                    "complexity": 2,
                    "segments": [{"type": "definition", "content": text, "order": 1}],
                    "relations": [],
                },
                "kind": kind.value,
            }
        return {
            "structured": [
                {
                    "content": f"Describe the key concepts of {topic}.",
                    "ideal_answer": "",
                    "question_type": "open",
                    "complexity": 2,
                }
            ],
            "kind": kind.value,
        }

    async def step_check_duplicates(
        self,
        structured: Dict[str, Any],
        topic: str,
    ) -> bool:
        """Step 3: Check for duplicates in CRUD. Returns True if duplicate found."""
        if not self._search_kb:
            return False

        try:
            title = ""
            data = structured.get("structured", {})
            if isinstance(data, dict):
                title = data.get("title", "")
            if not title:
                return False

            existing = await self._search_kb(topic)
            if not existing:
                return False

            from difflib import SequenceMatcher
            threshold = settings.similarity_threshold

            for item in existing[:settings.dedup_max_candidates]:
                existing_title = ""
                if isinstance(item, dict):
                    existing_title = item.get("title", "")
                if existing_title and SequenceMatcher(None, title.lower(), existing_title.lower()).ratio() > threshold:
                    return True
            return False

        except Exception as e:
            logger.warning("Duplicate check failed", error=str(e))
            return False

    async def step_save_draft(
        self,
        structured: Dict[str, Any],
        topic: str,
        source_uri: str,
        is_duplicate: bool,
    ) -> Draft:
        """Step 4: Persist as draft (not directly to production tables)."""
        kind_str = structured.get("kind", "knowledge")
        kind = DraftKind(kind_str)
        data = structured.get("structured", {})

        title = ""
        if isinstance(data, dict):
            title = data.get("title", f"Draft {topic}")
        elif isinstance(data, list) and data:
            title = f"Questions for {topic}"

        draft = Draft(
            draft_id=str(uuid.uuid4()),
            kind=kind,
            title=title,
            topic=topic,
            content=data if isinstance(data, dict) else {"items": data},
            source=source_uri,
            review_status=ReviewStatus.PENDING,
            duplicate_candidate=is_duplicate,
            created_at=datetime.now(timezone.utc),
        )

        logger.info(
            "Draft created by pipeline",
            draft_id=draft.draft_id,
            kind=kind.value,
            topic=topic,
            duplicate=is_duplicate,
        )
        return draft

    # ── Full pipeline ────────────────────────────────────────────

    async def run(
        self,
        raw: RawDocument,
        topic: str,
        kind: DraftKind = DraftKind.KNOWLEDGE,
    ) -> Draft:
        """Execute full pipeline: gather → structure → dedup → save_draft."""
        self._llm_call_count = 0

        logger.info("Pipeline started", source=raw.source_uri, topic=topic, kind=kind.value)

        context = await self.step_gather_context(raw, topic)
        structured = await self.step_structure(context, kind)
        is_dup = await self.step_check_duplicates(structured, topic)
        draft = await self.step_save_draft(structured, topic, raw.source_uri, is_dup)

        logger.info(
            "Pipeline completed",
            draft_id=draft.draft_id,
            llm_calls=self._llm_call_count,
            duplicate=is_dup,
        )
        return draft
