"""Service for producing knowledge and questions from JSON files"""

import json
import os
import uuid
from difflib import SequenceMatcher
from datetime import datetime, timezone
from pathlib import Path
from typing import List, Dict, Any, Optional
from ..clients import KnowledgeClient, QuestionsClient
from ..logger import get_logger
from ..config import settings
from ..metrics import get_metrics_collector
import time

logger = get_logger(__name__)

VALID_DRAFT_TYPES = {"knowledge", "question"}
VALID_REVIEW_STATUSES = {"pending", "approved", "rejected"}


def _text_similarity(a: str, b: str) -> float:
    """Compute similarity ratio between two strings using SequenceMatcher."""
    if not a or not b:
        return 0.0
    return SequenceMatcher(None, a.lower(), b.lower()).ratio()


class ProducerService:
    """Service for loading and producing knowledge and questions"""

    def __init__(
        self,
        knowledge_client: Optional[KnowledgeClient] = None,
        questions_client: Optional[QuestionsClient] = None,
    ):
        self.knowledge_client = knowledge_client or KnowledgeClient()
        self.questions_client = questions_client or QuestionsClient()
        self.metrics = get_metrics_collector()
        self._drafts: Dict[str, Dict[str, Any]] = {}

    async def load_knowledge_from_files(self) -> List[Dict[str, Any]]:
        """Load all knowledge JSON files from directory"""
        knowledge_path = Path(settings.knowledge_data_path)
        if not knowledge_path.exists():
            logger.warning("Knowledge data path does not exist", path=str(knowledge_path))
            return []

        knowledge_list = []
        for json_file in knowledge_path.glob("*.json"):
            try:
                with open(json_file, "r", encoding="utf-8") as f:
                    knowledge_data = json.load(f)
                    knowledge_list.append(knowledge_data)
                    logger.debug("Loaded knowledge file", file=str(json_file), id=knowledge_data.get("id"))
            except Exception as e:
                logger.error("Failed to load knowledge file", file=str(json_file), error=str(e))

        logger.info("Loaded knowledge files", count=len(knowledge_list))
        return knowledge_list

    async def load_questions_from_files(self) -> List[Dict[str, Any]]:
        """Load all question JSON files from directory"""
        questions_path = Path(settings.questions_data_path)
        if not questions_path.exists():
            logger.warning("Questions data path does not exist", path=str(questions_path))
            return []

        questions_list = []
        for json_file in questions_path.glob("*.json"):
            try:
                with open(json_file, "r", encoding="utf-8") as f:
                    question_data = json.load(f)
                    questions_list.append(question_data)
                    logger.debug("Loaded question file", file=str(json_file), id=question_data.get("id"))
            except Exception as e:
                logger.error("Failed to load question file", file=str(json_file), error=str(e))

        logger.info("Loaded question files", count=len(questions_list))
        return questions_list

    async def produce_knowledge(self, knowledge_data: Dict[str, Any]) -> Dict[str, Any]:
        """Produce knowledge: check for duplicates and create/update"""
        knowledge_id = knowledge_data.get("id")
        if not knowledge_id:
            raise ValueError("Knowledge data must have 'id' field")

        # Check if knowledge already exists
        existing = await self.knowledge_client.get_knowledge_by_id(knowledge_id)

        if existing:
            logger.info("Knowledge already exists, updating", knowledge_id=knowledge_id)
            # Check if version is newer
            # Version can be at top level or in data.metadata.version
            existing_version = existing.get("version") or existing.get("data", {}).get("metadata", {}).get("version", "1.0")
            new_version = knowledge_data.get("metadata", {}).get("version", "1.0")
            
            if new_version > existing_version:
                logger.info("Updating knowledge with newer version", 
                          knowledge_id=knowledge_id,
                          old_version=existing_version,
                          new_version=new_version)
                result = await self.knowledge_client.update_knowledge(knowledge_id, knowledge_data)
                self.metrics.knowledge_produced_total.labels(
                    service=settings.service_name,
                    status="success",
                    operation="update"
                ).inc()
                return result
            else:
                logger.info("Knowledge exists with same or newer version, skipping",
                          knowledge_id=knowledge_id,
                          existing_version=existing_version,
                          new_version=new_version)
                self.metrics.knowledge_produced_total.labels(
                    service=settings.service_name,
                    status="skipped",
                    operation="skip"
                ).inc()
                return existing
        else:
            logger.info("Creating new knowledge", knowledge_id=knowledge_id)
            result = await self.knowledge_client.create_knowledge(knowledge_data)
            self.metrics.knowledge_produced_total.labels(
                service=settings.service_name,
                status="success",
                operation="create"
            ).inc()
            return result

    async def produce_question(self, question_data: Dict[str, Any]) -> Dict[str, Any]:
        """Produce question: check for duplicates and create/update"""
        question_id = question_data.get("id")
        if not question_id:
            raise ValueError("Question data must have 'id' field")

        # Check if question already exists
        existing = await self.questions_client.get_question_by_id(question_id)

        if existing:
            logger.info("Question already exists, updating", question_id=question_id)
            # Check if version is newer
            # Version can be at top level or in data.metadata.version
            existing_version = existing.get("version") or existing.get("data", {}).get("metadata", {}).get("version", "1.0")
            new_version = question_data.get("metadata", {}).get("version", "1.0")
            
            if new_version > existing_version:
                logger.info("Updating question with newer version",
                          question_id=question_id,
                          old_version=existing_version,
                          new_version=new_version)
                result = await self.questions_client.update_question(question_id, question_data)
                self.metrics.questions_produced_total.labels(
                    service=settings.service_name,
                    status="success",
                    operation="update"
                ).inc()
                return result
            else:
                logger.info("Question exists with same or newer version, skipping",
                          question_id=question_id,
                          existing_version=existing_version,
                          new_version=new_version)
                self.metrics.questions_produced_total.labels(
                    service=settings.service_name,
                    status="skipped",
                    operation="skip"
                ).inc()
                return existing
        else:
            logger.info("Creating new question", question_id=question_id)
            result = await self.questions_client.create_question(question_data)
            self.metrics.questions_produced_total.labels(
                service=settings.service_name,
                status="success",
                operation="create"
            ).inc()
            return result

    async def produce_all_knowledge(self) -> Dict[str, Any]:
        """Load and produce all knowledge from files"""
        start_time = time.time()
        knowledge_list = await self.load_knowledge_from_files()
        
        created = 0
        updated = 0
        skipped = 0
        errors = 0

        for knowledge_data in knowledge_list:
            try:
                existing = await self.knowledge_client.get_knowledge_by_id(knowledge_data.get("id"))
                if existing:
                    # Check version
                    # Version can be at top level or in data.metadata.version
                    existing_version = existing.get("version") or existing.get("data", {}).get("metadata", {}).get("version", "1.0")
                    new_version = knowledge_data.get("metadata", {}).get("version", "1.0")
                    if new_version > existing_version:
                        await self.produce_knowledge(knowledge_data)
                        updated += 1
                    else:
                        skipped += 1
                else:
                    await self.produce_knowledge(knowledge_data)
                    created += 1
            except Exception as e:
                self.metrics.knowledge_produced_total.labels(
                    service=settings.service_name,
                    status="error",
                    operation="error"
                ).inc()
                self.metrics.error_count.labels(
                    error_type=type(e).__name__,
                    service=settings.service_name
                ).inc()
                logger.error("Failed to produce knowledge", 
                           knowledge_id=knowledge_data.get("id"), 
                           error=str(e))
                errors += 1

        duration = time.time() - start_time
        self.metrics.production_duration.labels(
            service=settings.service_name,
            type="knowledge"
        ).observe(duration)

        result = {
            "total": len(knowledge_list),
            "created": created,
            "updated": updated,
            "skipped": skipped,
            "errors": errors,
        }
        logger.info("Knowledge production completed", **result)
        return result

    async def produce_all_questions(self) -> Dict[str, Any]:
        """Load and produce all questions from files"""
        start_time = time.time()
        questions_list = await self.load_questions_from_files()
        
        created = 0
        updated = 0
        skipped = 0
        errors = 0

        for question_data in questions_list:
            try:
                existing = await self.questions_client.get_question_by_id(question_data.get("id"))
                if existing:
                    # Check version
                    # Version can be at top level or in data.metadata.version
                    existing_version = existing.get("version") or existing.get("data", {}).get("metadata", {}).get("version", "1.0")
                    new_version = question_data.get("metadata", {}).get("version", "1.0")
                    if new_version > existing_version:
                        await self.produce_question(question_data)
                        updated += 1
                    else:
                        skipped += 1
                else:
                    await self.produce_question(question_data)
                    created += 1
            except Exception as e:
                self.metrics.questions_produced_total.labels(
                    service=settings.service_name,
                    status="error",
                    operation="error"
                ).inc()
                self.metrics.error_count.labels(
                    error_type=type(e).__name__,
                    service=settings.service_name
                ).inc()
                logger.error("Failed to produce question",
                           question_id=question_data.get("id"),
                           error=str(e))
                errors += 1

        duration = time.time() - start_time
        self.metrics.production_duration.labels(
            service=settings.service_name,
            type="questions"
        ).observe(duration)

        result = {
            "total": len(questions_list),
            "created": created,
            "updated": updated,
            "skipped": skipped,
            "errors": errors,
        }
        logger.info("Questions production completed", **result)
        return result

    async def produce_all(self) -> Dict[str, Any]:
        """Produce all knowledge and questions"""
        knowledge_result = await self.produce_all_knowledge()
        questions_result = await self.produce_all_questions()
        
        return {
            "knowledge": knowledge_result,
            "questions": questions_result,
        }

    async def close(self):
        """Close clients"""
        await self.knowledge_client.close()
        await self.questions_client.close()

    # ── Draft lifecycle ──────────────────────────────────────────────

    def _make_draft(
        self,
        draft_type: str,
        title: str,
        content: str,
        topic: str,
        source: str = "",
    ) -> Dict[str, Any]:
        now = datetime.now(timezone.utc).isoformat()
        return {
            "draft_id": str(uuid.uuid4()),
            "draft_type": draft_type,
            "title": title,
            "content": content,
            "topic": topic,
            "source": source,
            "review_status": "pending",
            "review_comment": None,
            "reviewed_by": None,
            "reviewed_at": None,
            "published_at": None,
            "duplicate_candidate": False,
            "created_at": now,
        }

    def _find_similar_drafts(self, draft: Dict[str, Any]) -> List[Dict[str, Any]]:
        """Find existing drafts similar to the given one by title+content."""
        candidates = []
        incoming_text = f"{draft['title']} {draft['content']}"

        for existing in self._drafts.values():
            if existing["draft_id"] == draft["draft_id"]:
                continue
            if existing["draft_type"] != draft["draft_type"]:
                continue
            if existing["topic"] != draft["topic"]:
                continue

            existing_text = f"{existing['title']} {existing['content']}"
            score = _text_similarity(incoming_text, existing_text)
            if score >= settings.similarity_threshold:
                candidates.append({**existing, "_similarity": round(score, 4)})

        candidates.sort(key=lambda c: c["_similarity"], reverse=True)
        return candidates[: settings.dedup_max_candidates]

    async def create_draft(
        self,
        draft_type: str,
        title: str,
        content: str,
        topic: str,
        source: str = "",
    ) -> Dict[str, Any]:
        if draft_type not in VALID_DRAFT_TYPES:
            raise ValueError(f"draft_type must be one of {VALID_DRAFT_TYPES}")

        draft = self._make_draft(draft_type, title, content, topic, source)

        similar = self._find_similar_drafts(draft)
        if similar:
            draft["duplicate_candidate"] = True
            logger.warning(
                "Duplicate candidate detected",
                draft_id=draft["draft_id"],
                similar_count=len(similar),
                top_similarity=similar[0]["_similarity"],
            )

        self._drafts[draft["draft_id"]] = draft
        logger.info("Draft created", draft_id=draft["draft_id"], draft_type=draft_type)
        return draft

    def get_draft(self, draft_id: str) -> Optional[Dict[str, Any]]:
        return self._drafts.get(draft_id)

    def list_drafts(self, status: Optional[str] = None) -> List[Dict[str, Any]]:
        drafts = list(self._drafts.values())
        if status:
            drafts = [d for d in drafts if d["review_status"] == status]
        drafts.sort(key=lambda d: d["created_at"], reverse=True)
        return drafts

    def review_draft(
        self,
        draft_id: str,
        review_status: str,
        reviewed_by: str,
        review_comment: Optional[str] = None,
    ) -> Dict[str, Any]:
        draft = self._drafts.get(draft_id)
        if draft is None:
            raise KeyError(f"Draft {draft_id} not found")

        if review_status not in {"approved", "rejected"}:
            raise ValueError("review_status must be 'approved' or 'rejected'")

        now = datetime.now(timezone.utc).isoformat()
        draft["review_status"] = review_status
        draft["reviewed_by"] = reviewed_by
        draft["reviewed_at"] = now
        draft["review_comment"] = review_comment

        logger.info(
            "Draft reviewed",
            draft_id=draft_id,
            review_status=review_status,
            reviewed_by=reviewed_by,
        )
        return draft

    async def publish_draft(
        self,
        draft_id: str,
        override_duplicate: bool = False,
    ) -> Dict[str, Any]:
        draft = self._drafts.get(draft_id)
        if draft is None:
            raise KeyError(f"Draft {draft_id} not found")

        logger.info(
            "Publish attempt",
            draft_id=draft_id,
            review_status=draft["review_status"],
            reviewed_by=draft.get("reviewed_by"),
        )

        if draft["review_status"] == "rejected":
            raise PermissionError(
                f"Draft rejected: {draft.get('review_comment', 'no comment')}"
            )

        if draft["review_status"] != "approved":
            raise PermissionError(
                f"Cannot publish draft with status '{draft['review_status']}'. "
                "Only approved drafts can be published."
            )

        if draft["duplicate_candidate"] and not override_duplicate:
            raise PermissionError(
                "Draft is flagged as duplicate candidate. "
                "Set override_duplicate=true to publish anyway."
            )

        if draft["draft_type"] == "knowledge":
            payload = {
                "id": draft["draft_id"],
                "title": draft["title"],
                "content": draft["content"],
                "topic": draft["topic"],
                "source": draft["source"],
                "metadata": {"created_via": "draft", "draft_id": draft["draft_id"]},
            }
            result = await self.knowledge_client.create_knowledge(payload)
        else:
            payload = {
                "id": draft["draft_id"],
                "title": draft["title"],
                "content": draft["content"],
                "topic": draft["topic"],
                "source": draft["source"],
                "metadata": {"created_via": "draft", "draft_id": draft["draft_id"]},
            }
            result = await self.questions_client.create_question(payload)

        draft["published_at"] = datetime.now(timezone.utc).isoformat()
        logger.info("Draft published", draft_id=draft_id, draft_type=draft["draft_type"])
        return {"draft": draft, "published": result}

