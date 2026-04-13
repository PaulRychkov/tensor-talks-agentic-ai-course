"""Service for producing knowledge and questions from JSON files"""

import json
import os
from pathlib import Path
from typing import List, Dict, Any, Optional
from ..clients import KnowledgeClient, QuestionsClient
from ..logger import get_logger
from ..config import settings
from ..metrics import get_metrics_collector
import time

logger = get_logger(__name__)


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

