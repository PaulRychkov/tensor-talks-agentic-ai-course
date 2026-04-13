"""Service for building interview programs"""

import random
import sys
from typing import List, Dict, Any, Optional
from ..clients.questions_client import QuestionsClient
from ..clients.knowledge_client import KnowledgeClient
from ..kafka import KafkaProducer
from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector
import time

logger = get_logger(__name__)


class InterviewBuilderService:
    """Service for building interview programs from questions and knowledge"""

    def __init__(
        self,
        questions_client: Optional[QuestionsClient] = None,
        knowledge_client: Optional[KnowledgeClient] = None,
        kafka_producer: Optional[KafkaProducer] = None,
    ):
        # Store client factories, not instances, to avoid issues with async context
        self._questions_client_factory = lambda: questions_client or QuestionsClient()
        self._knowledge_client_factory = lambda: knowledge_client or KnowledgeClient()
        self.kafka_producer = kafka_producer or KafkaProducer()
        self.metrics = get_metrics_collector()
    
    def _get_questions_client(self) -> QuestionsClient:
        """Get questions client instance"""
        return self._questions_client_factory()
    
    def _get_knowledge_client(self) -> KnowledgeClient:
        """Get knowledge client instance"""
        return self._knowledge_client_factory()

    def _map_level_to_complexity(self, level: str) -> int:
        """Map level string to complexity integer"""
        mapping = {
            "junior": 1,
            "middle": 2,
            "senior": 3,
        }
        return mapping.get(level.lower(), 2)

    def _map_topics_to_tags(self, topics: List[str]) -> List[str]:
        """Map topic strings to knowledge tags"""
        tag_mapping = {
            "classic_ml": ["machine_learning"],
            "ml": ["machine_learning"],
            "ml basics": ["machine_learning"],  # Handle old format
            "ml_basics": ["machine_learning"],
            "nlp": ["nlp"],
            "llm": ["llm"],
        }
        
        tags = []
        for topic in topics:
            topic_lower = topic.lower().strip()
            if topic_lower in tag_mapping:
                tags.extend(tag_mapping[topic_lower])
            else:
                # Default fallback
                tags.append("machine_learning")
        
        return list(set(tags))  # Remove duplicates

    async def build_interview_program(
        self, session_id: str, params: Dict[str, Any]
    ) -> Dict[str, Any]:
        """Build interview program from parameters"""
        start_time = time.time()
        topics = params.get("topics", [])
        level = params.get("level", "middle")
        interview_type = params.get("type", "interview")

        logger.info("Building interview program",
                   session_id=session_id,
                   topics=topics,
                   level=level,
                   type=interview_type)
        
        try:
            # Map parameters to filters
            complexity = self._map_level_to_complexity(level)
            tags = self._map_topics_to_tags(topics)

            # Get questions matching filters
            # Create a new client for this request to avoid connection reuse issues
            questions_client = QuestionsClient()
            questions = None
            try:
                questions = await questions_client.get_questions_by_filters(
                    complexity=complexity,
                    question_type=None,  # Get all types
                )
                self.metrics.questions_fetched_total.labels(
                    service=settings.service_name,
                    status="success"
                ).inc()
                # Log first question structure for debugging
                if questions and len(questions) > 0:
                    first_q = questions[0]
                    logger.info("First question structure",
                               question_keys=list(first_q.keys())[:20] if isinstance(first_q, dict) else [],
                               has_data="Data" in first_q if isinstance(first_q, dict) else False,
                               has_lowercase_data="data" in first_q if isinstance(first_q, dict) else False,
                               first_question_str=str(first_q)[:500] if first_q else "")
            except Exception as e:
                self.metrics.questions_fetched_total.labels(
                    service=settings.service_name,
                    status="error"
                ).inc()
                raise
            finally:
                # Close client after request is complete
                if questions_client:
                    try:
                        await questions_client.close()
                    except Exception as e:
                        # Ignore errors during shutdown
                        logger.debug("Error closing questions client", error=str(e))

            # Filter questions by topic through related knowledge
            if tags and questions:
                filtered_questions = []
                for question in questions:
                    # Handle Go struct serialization: fields are capitalized (ID, TheoryID, Data)
                    # Structure: {"ID": "...", "TheoryID": "...", "Data": {"id": "...", "theory_id": "..."}}
                    theory_id = None
                    if isinstance(question, dict):
                        # Try capitalized "Data" field first (Go struct serialization)
                        if "Data" in question and isinstance(question.get("Data"), dict):
                            question_data = question["Data"]
                            theory_id = question_data.get("theory_id")
                        # Fallback to lowercase "data" field
                        elif "data" in question and isinstance(question.get("data"), dict):
                            question_data = question["data"]
                            theory_id = question_data.get("theory_id")
                        # If question is already the data structure
                        else:
                            question_data = question
                            theory_id = question_data.get("theory_id") if isinstance(question_data, dict) else None
                    else:
                        question_data = question
                        theory_id = question_data.get("theory_id") if isinstance(question_data, dict) else None
                    
                    if theory_id:
                        # Create a new client for this request to avoid connection reuse issues
                        knowledge_client = None
                        try:
                            knowledge_client = KnowledgeClient()
                            knowledge = await knowledge_client.get_knowledge_by_id(theory_id)
                            if knowledge:
                                # Handle both direct knowledge dict and nested data structure
                                # Go serializes with capitalized "Data" field
                                if "Data" in knowledge and isinstance(knowledge.get("Data"), dict):
                                    knowledge_data = knowledge["Data"]
                                elif "data" in knowledge and isinstance(knowledge.get("data"), dict):
                                    knowledge_data = knowledge["data"]
                                else:
                                    knowledge_data = knowledge
                                
                                knowledge_tags = knowledge_data.get("tags", [])
                                # Check if any tag matches
                                if any(tag in knowledge_tags for tag in tags):
                                    filtered_questions.append(question)
                        except Exception as e:
                            logger.warn("Failed to check knowledge tags", theory_id=theory_id, error=str(e))
                            # Include question if we can't check
                            filtered_questions.append(question)
                        finally:
                            if knowledge_client:
                                try:
                                    await knowledge_client.close()
                                except Exception as e:
                                    # Ignore errors during shutdown
                                    logger.debug("Error closing knowledge client", error=str(e))
                    else:
                        # Include questions without theory_id
                        filtered_questions.append(question)
                
                if filtered_questions:
                    questions = filtered_questions
                else:
                    logger.warn("No questions found after topic filtering", tags=tags)
                    # Fallback: use all questions

            if not questions:
                logger.warn("No questions found for filters", complexity=complexity, tags=tags)
                # Fallback: try without complexity filter
                questions_client_fallback = None
                try:
                    questions_client_fallback = QuestionsClient()
                    questions = await questions_client_fallback.get_questions_by_filters(
                        complexity=None,
                    )
                finally:
                    if questions_client_fallback:
                        try:
                            await questions_client_fallback.close()
                        except Exception as e:
                            # Ignore errors during shutdown
                            logger.debug("Error closing questions client", error=str(e))

            if not questions:
                raise ValueError("No questions available in database")

            # Select random questions (5 by default)
            num_questions = min(settings.questions_per_interview, len(questions))
            selected_questions = random.sample(questions, num_questions)
            logger.info("Selected questions", 
                       selected_count=len(selected_questions),
                       first_question_keys=list(selected_questions[0].keys())[:10] if selected_questions and isinstance(selected_questions[0], dict) else [])

            # Order questions by logic (group related questions together)
            ordered_questions = self._order_questions(selected_questions)
            logger.info("Ordered questions", 
                       ordered_count=len(ordered_questions))

            # Build program with theory
            program_questions = []
            # Force log output with print as well - this should always appear
            print(f"[DEBUG] Starting to build program questions, count={len(ordered_questions)}", flush=True, file=sys.stderr)
            if ordered_questions:
                first_q = ordered_questions[0]
                print(f"[DEBUG] First question type={type(first_q).__name__}, is_dict={isinstance(first_q, dict)}", flush=True, file=sys.stderr)
                if isinstance(first_q, dict):
                    print(f"[DEBUG] First question keys: {list(first_q.keys())[:20]}", flush=True, file=sys.stderr)
                    if "Data" in first_q:
                        print(f"[DEBUG] First question has 'Data' field", flush=True, file=sys.stderr)
                        data = first_q.get("Data", {})
                        if isinstance(data, dict):
                            print(f"[DEBUG] Data keys: {list(data.keys())[:20]}", flush=True, file=sys.stderr)
                            if "content" in data:
                                content = data.get("content", {})
                                if isinstance(content, dict):
                                    print(f"[DEBUG] Content keys: {list(content.keys())}, has_question={'question' in content}", flush=True, file=sys.stderr)
                                    if "question" in content:
                                        q_text = content.get("question", "")
                                        print(f"[DEBUG] Question text found! Length={len(q_text)}, preview={q_text[:100]}", flush=True, file=sys.stderr)
            logger.warn("Starting to build program questions - DEBUG", 
                       ordered_questions_count=len(ordered_questions),
                       first_question_type=type(ordered_questions[0]).__name__ if ordered_questions else None,
                       first_question_keys=list(ordered_questions[0].keys())[:20] if ordered_questions and isinstance(ordered_questions[0], dict) else [])
            
            for idx, question in enumerate(ordered_questions, start=1):
                print(f"[DEBUG] Processing question {idx}, type={type(question).__name__}, is_dict={isinstance(question, dict)}", flush=True, file=sys.stderr)
                if isinstance(question, dict):
                    print(f"[DEBUG] Question keys: {list(question.keys())[:20]}", flush=True, file=sys.stderr)
                logger.warn("Processing question - DEBUG", question_index=idx, 
                           question_type=type(question).__name__,
                           is_dict=isinstance(question, dict),
                           question_keys=list(question.keys())[:20] if isinstance(question, dict) else [],
                           question_str=str(question)[:500] if question else "")
                
                # Handle Go struct serialization: fields are capitalized (ID, TheoryID, Data)
                # Structure: {"ID": "...", "TheoryID": "...", "Data": {"id": "...", "content": {"question": "..."}}}
                question_data = None
                theory_id = None
                
                if isinstance(question, dict):
                    logger.info("Question is dict", keys=list(question.keys())[:10])
                    # Try capitalized "Data" field first (Go struct serialization)
                    # Go struct: Question{ID, TheoryID, Data: QuestionJSONB{content: QuestionContent{question: "..."}}}
                    if "Data" in question and isinstance(question.get("Data"), dict):
                        question_data = question["Data"]
                        theory_id = question_data.get("theory_id")
                        logger.info("Found Data field", 
                                   question_data_keys=list(question_data.keys())[:10],
                                   has_content="content" in question_data)
                    # Fallback to lowercase "data" field
                    elif "data" in question and isinstance(question.get("data"), dict):
                        question_data = question["data"]
                        theory_id = question_data.get("theory_id")
                        logger.info("Found data field (lowercase)", 
                                   question_data_keys=list(question_data.keys())[:10],
                                   has_content="content" in question_data)
                    # If question is already the data structure (shouldn't happen, but handle it)
                    else:
                        question_data = question
                        theory_id = question_data.get("theory_id")
                        logger.info("Using question as question_data directly",
                                   question_data_keys=list(question_data.keys())[:10],
                                   has_content="content" in question_data)
                else:
                    question_data = question
                    theory_id = question_data.get("theory_id") if isinstance(question_data, dict) else None
                    logger.info("Question is not dict, using as question_data")
                
                # Log structure for debugging
                if question_data:
                    print(f"[DEBUG] Question data extracted, has_content={'content' in question_data if isinstance(question_data, dict) else False}, keys={list(question_data.keys())[:20] if isinstance(question_data, dict) else []}", flush=True, file=sys.stderr)
                    if isinstance(question_data, dict) and "content" in question_data:
                        content = question_data.get("content", {})
                        print(f"[DEBUG] Content type={type(content).__name__}, has_question={'question' in content if isinstance(content, dict) else False}", flush=True, file=sys.stderr)
                        if isinstance(content, dict) and "question" in content:
                            q_text = content.get("question", "")
                            print(f"[DEBUG] Question text length={len(q_text)}, preview={q_text[:100]}", flush=True, file=sys.stderr)
                    logger.warn("Question data extracted - DEBUG",
                               question_id=question_data.get("id") if isinstance(question_data, dict) else None,
                               has_content="content" in question_data if isinstance(question_data, dict) else False,
                               question_data_keys=list(question_data.keys())[:20] if isinstance(question_data, dict) else [],
                               raw_keys=list(question.keys())[:20] if isinstance(question, dict) else [],
                               question_data_content=question_data.get("content", {}) if isinstance(question_data, dict) else {},
                               question_data_str=str(question_data)[:500] if question_data else "")
                
                # Get theory for question
                theory_text = ""
                if theory_id:
                    # Create a new client for this request to avoid connection reuse issues
                    knowledge_client = None
                    try:
                        knowledge_client = KnowledgeClient()
                        knowledge = await knowledge_client.get_knowledge_by_id(theory_id)
                        if knowledge:
                            self.metrics.knowledge_fetched_total.labels(
                                service=settings.service_name,
                                status="success"
                            ).inc()
                            # Handle both direct knowledge dict and nested data structure
                            # Go serializes with capitalized "Data" field
                            if "Data" in knowledge and isinstance(knowledge.get("Data"), dict):
                                knowledge_data = knowledge["Data"]
                            elif "data" in knowledge and isinstance(knowledge.get("data"), dict):
                                knowledge_data = knowledge["data"]
                            else:
                                knowledge_data = knowledge
                            
                            segments = knowledge_data.get("segments", [])
                            # Combine all segments into theory text
                            theory_parts = []
                            for segment in segments:
                                if isinstance(segment, dict) and segment.get("type") in ["definition", "intuition"]:
                                    theory_parts.append(segment.get("content", ""))
                            theory_text = " ".join(theory_parts)
                    except Exception as e:
                        self.metrics.knowledge_fetched_total.labels(
                            service=settings.service_name,
                            status="error"
                        ).inc()
                        logger.warn("Failed to get knowledge", theory_id=theory_id, error=str(e))
                    finally:
                        if knowledge_client:
                            try:
                                await knowledge_client.close()
                            except Exception as e:
                                # Ignore errors during shutdown
                                logger.debug("Error closing knowledge client", error=str(e))

                # Extract question text from content
                # Go serializes Question as: {"ID": "...", "TheoryID": "...", "Data": {"id": "...", "content": {"question": "..."}}}
                # question_data should be the Data field (QuestionJSONB) which contains content (QuestionContent) with question (string)
                question_text = ""
                
                # DEBUG: Log question_data structure
                print(f"[DEBUG] Extracting question text, question_data type={type(question_data).__name__}, is_dict={isinstance(question_data, dict)}", flush=True, file=sys.stderr)
                if isinstance(question_data, dict):
                    print(f"[DEBUG] question_data keys: {list(question_data.keys())[:20]}", flush=True, file=sys.stderr)
                    if "content" in question_data:
                        content_val = question_data.get("content")
                        print(f"[DEBUG] content type={type(content_val).__name__}, is_dict={isinstance(content_val, dict)}", flush=True, file=sys.stderr)
                        if isinstance(content_val, dict):
                            print(f"[DEBUG] content keys: {list(content_val.keys())}", flush=True, file=sys.stderr)
                            if "question" in content_val:
                                q_val = content_val.get("question")
                                print(f"[DEBUG] question value type={type(q_val).__name__}, value={str(q_val)[:100] if q_val else 'None'}", flush=True, file=sys.stderr)
                
                # Path 1: question_data["content"]["question"] (question_data is the Data field from Go struct)
                if question_data and isinstance(question_data, dict):
                    content = question_data.get("content", {})
                    if isinstance(content, dict):
                        question_text = content.get("question", "")
                        if question_text:
                            print(f"[DEBUG] Path 1 SUCCESS: question_text length={len(question_text)}, preview={question_text[:100]}", flush=True, file=sys.stderr)
                            logger.info("Question extracted via Path 1 (question_data['content']['question'])",
                                       question_text_preview=question_text[:100])
                        else:
                            print(f"[DEBUG] Path 1 FAILED: content.get('question') returned empty", flush=True, file=sys.stderr)
                    else:
                        print(f"[DEBUG] Path 1 FAILED: content is not dict, type={type(content).__name__}", flush=True, file=sys.stderr)
                else:
                    print(f"[DEBUG] Path 1 FAILED: question_data is not dict or None", flush=True, file=sys.stderr)
                
                # Path 2: Check if question is in raw question structure (question["Data"]["content"]["question"])
                # This is a fallback in case question_data wasn't set correctly
                if not question_text and isinstance(question, dict):
                    # Try capitalized "Data" first (Go struct serialization)
                    if "Data" in question:
                        data = question.get("Data", {})
                        if isinstance(data, dict):
                            data_content = data.get("content", {})
                            if isinstance(data_content, dict):
                                question_text = data_content.get("question", "")
                                if question_text:
                                    logger.info("Question extracted via Path 2 (question['Data']['content']['question'])",
                                               question_text_preview=question_text[:100])
                    # Fallback to lowercase "data"
                    if not question_text and "data" in question:
                        data = question.get("data", {})
                        if isinstance(data, dict):
                            data_content = data.get("content", {})
                            if isinstance(data_content, dict):
                                question_text = data_content.get("question", "")
                                if question_text:
                                    logger.info("Question extracted via Path 2 (question['data']['content']['question'])",
                                               question_text_preview=question_text[:100])
                
                # Path 3: If still empty, try direct access (fallback - shouldn't happen)
                if not question_text and question_data and isinstance(question_data, dict):
                    question_text = question_data.get("question", "")
                    if question_text:
                        logger.info("Question extracted via Path 3 (question_data['question'])",
                                   question_text_preview=question_text[:100])
                
                # Log for debugging
                if not question_text:
                    logger.warn("Empty question text found",
                              question_id=question_data.get("id") if question_data and isinstance(question_data, dict) else None,
                              content=question_data.get("content", {}) if question_data and isinstance(question_data, dict) else {},
                              question_data_keys=list(question_data.keys()) if question_data and isinstance(question_data, dict) else [],
                              raw_question_keys=list(question.keys()) if isinstance(question, dict) else [],
                              question_data_str=str(question_data)[:300] if question_data else str(question_data),
                              raw_question_str=str(question)[:300] if isinstance(question, dict) else str(question))
                else:
                    logger.info("Question text extracted successfully",
                              question_id=question_data.get("id") if question_data and isinstance(question_data, dict) else None,
                              question_text_length=len(question_text),
                              question_text_preview=question_text[:100] if question_text else "")
                
                program_questions.append({
                    "id": question_data.get("id") if isinstance(question_data, dict) else None,
                    "question": question_text,
                    "theory": theory_text,
                    "order": idx,
                })

            program = {
                "questions": program_questions,
            }

            # Log program details before sending - CRITICAL DEBUG INFO
            preview_list = []
            for q in program_questions[:3]:
                preview_list.append({
                    "order": q.get("order"),
                    "question_length": len(q.get("question", "")),
                    "question_preview": q.get("question", "")[:50] if q.get("question") else "",
                    "theory_length": len(q.get("theory", ""))
                })
            first_question_full = program_questions[0].get("question", "") if program_questions and len(program_questions) > 0 else ""
            all_questions_debug = [{"order": q.get("order"), "question": q.get("question", ""), "question_len": len(q.get("question", ""))} for q in program_questions]
            print(f"[DEBUG] Interview program built, questions_count={len(program_questions)}, first_question_length={len(first_question_full)}", flush=True, file=sys.stderr)
            print(f"[DEBUG] All questions: {all_questions_debug}", flush=True, file=sys.stderr)
            logger.warn("Interview program built - DEBUG",
                       session_id=session_id,
                       questions_count=len(program_questions),
                       program_questions_preview=preview_list,
                       first_question_full=first_question_full,
                       first_question_length=len(first_question_full),
                       all_questions=all_questions_debug)

            # Record metrics
            duration = time.time() - start_time
            self.metrics.program_building_duration.labels(
                service=settings.service_name
            ).observe(duration)
            self.metrics.interview_programs_built_total.labels(
                service=settings.service_name,
                status="success"
            ).inc()

            return program
        except Exception as e:
            self.metrics.interview_programs_built_total.labels(
                service=settings.service_name,
                status="error"
            ).inc()
            self.metrics.error_count.labels(
                error_type=type(e).__name__,
                service=settings.service_name
            ).inc()
            raise

    def _order_questions(self, questions: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Order questions by logic: group related questions together"""
        # Simple ordering: group by theory_id
        questions_by_theory = {}
        questions_without_theory = []

        for question in questions:
            # Handle Go struct serialization: fields are capitalized (ID, TheoryID, Data)
            # Structure: {"ID": "...", "TheoryID": "...", "Data": {"id": "...", "theory_id": "..."}}
            theory_id = None
            if isinstance(question, dict):
                # Try capitalized "Data" field first (Go struct serialization)
                if "Data" in question and isinstance(question.get("Data"), dict):
                    question_data = question["Data"]
                    theory_id = question_data.get("theory_id")
                # Fallback to lowercase "data" field
                elif "data" in question and isinstance(question.get("data"), dict):
                    question_data = question["data"]
                    theory_id = question_data.get("theory_id")
                # If question is already the data structure
                else:
                    question_data = question
                    theory_id = question_data.get("theory_id") if isinstance(question_data, dict) else None
            else:
                question_data = question
                theory_id = question_data.get("theory_id") if isinstance(question_data, dict) else None
            
            if theory_id:
                if theory_id not in questions_by_theory:
                    questions_by_theory[theory_id] = []
                questions_by_theory[theory_id].append(question)
            else:
                questions_without_theory.append(question)

        # Order: group questions by theory_id, then add questions without theory
        ordered = []
        for theory_id, theory_questions in questions_by_theory.items():
            ordered.extend(theory_questions)
        ordered.extend(questions_without_theory)

        return ordered

    async def handle_interview_build_request(
        self, session_id: str, params: Dict[str, Any]
    ):
        """Handle interview build request from Kafka"""
        try:
            program = await self.build_interview_program(session_id, params)
            # Log program before sending
            logger.info("Program ready to send",
                       session_id=session_id,
                       questions_count=len(program.get("questions", [])),
                       first_question_preview=program.get("questions", [{}])[0].get("question", "")[:100] if program.get("questions") else "")
            self.kafka_producer.send_interview_program(session_id, program)
            logger.info("Interview program sent to Kafka", session_id=session_id)
        except Exception as e:
            logger.error("Failed to build interview program",
                        session_id=session_id,
                        error=str(e),
                        exc_info=True)
            raise

    async def close(self):
        """Close clients"""
        # Clients are created per-request, so nothing to close here
        # Only close Kafka producer, HTTP clients are closed per-request
        self.kafka_producer.close()

