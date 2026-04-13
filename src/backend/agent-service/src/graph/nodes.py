"""LangGraph nodes for agent state machine"""

import json
import re
from datetime import datetime
from typing import Dict, Any
from uuid import uuid4

from ..graph.state import AgentState
from ..graph.utils import extract_session_id_from_state
from ..logger import get_logger
from ..metrics import get_metrics_collector
from ..llm import LLMClient
from ..llm.prompts import (
    build_off_topic_prompt,
    build_question_index_prompt,
    build_evaluation_prompt,
    build_clarification_prompt,
    build_hint_prompt,
    build_next_question_prompt,
    build_off_topic_reminder_prompt,
    build_final_evaluation_prompt,
)
from ..clients import SessionServiceClient, RedisClient
from ..kafka import KafkaProducer
from ..config import settings

logger = get_logger(__name__)


# Global instances (will be set during initialization)
llm_client: LLMClient = None
session_client: SessionServiceClient = None
redis_client: RedisClient = None
kafka_producer: KafkaProducer = None
metrics = get_metrics_collector()


def set_global_clients(
    llm: LLMClient,
    session: SessionServiceClient,
    redis: RedisClient,
    kafka: KafkaProducer,
):
    """Set global client instances"""
    global llm_client, session_client, redis_client, kafka_producer
    llm_client = llm
    session_client = session
    redis_client = redis
    kafka_producer = kafka


async def receive_message(state: AgentState) -> AgentState:
    """Node 1: Receive and parse message from Kafka"""
    logger.debug("NODE: receive_message", chat_id=state["chat_id"])
    state["processing_steps"].append("message_received")
    logger.info(
        "Message received",
        chat_id=state["chat_id"],
        message_id=state["message_id"],
    )
    return state


async def load_context(state: AgentState) -> AgentState:
    """Node 2: Load dialogue context (history, state, program)"""
    logger.debug("NODE: load_context", chat_id=state["chat_id"])
    chat_id = state["chat_id"]
    session_id = state.get("session_id")

    # Extract session_id from dialogue_state if not present
    if not session_id:
        session_id = extract_session_id_from_state(state)
        if session_id:
            state["session_id"] = session_id

    try:
        # Load history from Redis
        if settings.enable_redis_cache and redis_client:
            history = await redis_client.get_messages(chat_id, limit=50)
            state["dialogue_history"] = history
            logger.debug("History loaded from Redis", chat_id=chat_id, count=len(history))

            # Load dialogue state
            dialogue_state = await redis_client.get_dialogue_state(chat_id)
            if dialogue_state:
                state["dialogue_state"] = dialogue_state
                logger.debug("Dialogue state loaded from Redis", chat_id=chat_id)

        # Load interview program from Session Service
        if session_id and session_client:
            try:
                program = await session_client.get_program(session_id)
                if program:
                    state["interview_program"] = program
                    questions = program.get("questions", [])
                    state["total_questions"] = len(questions)
                    logger.info(
                        "Interview program loaded",
                        session_id=session_id,
                        questions_count=len(questions),
                    )
            except Exception as e:
                logger.warning(
                    "Failed to load program from Session Service, continuing without it",
                    session_id=session_id,
                    error=str(e),
                )
                # Create mock program for testing
                state["interview_program"] = {
                    "questions": [
                        {"order": 1, "question": "Что такое L1 регуляризация?", "theory": "L1 регуляризация добавляет сумму абсолютных значений весов к функции потерь."}
                    ]
                }
                state["total_questions"] = 1

        state["processing_steps"].append("context_loaded")
    except Exception as e:
        logger.error("Failed to load context", chat_id=chat_id, error=str(e), exc_info=True)
        # Don't set error state, allow processing to continue
        logger.warning("Continuing with minimal context", chat_id=chat_id)

    return state


async def check_off_topic(state: AgentState) -> AgentState:
    """Node 3: Check if message is off-topic"""
    logger.debug("NODE: check_off_topic", chat_id=state["chat_id"])
    user_message = state.get("user_message", "")
    
    # CRITICAL DEBUG: Log user_message state
    logger.info(
        "check_off_topic: Checking user_message",
        chat_id=state["chat_id"],
        user_message=user_message,
        user_message_length=len(user_message) if user_message else 0,
        user_message_repr=repr(user_message),
        user_message_type=type(user_message).__name__,
        is_empty=not user_message,
        is_empty_after_strip=not (user_message and user_message.strip()),
    )
    
    # Skip off-topic check for chat start (empty user_message means system message for chat start)
    # CRITICAL: This must be checked FIRST to prevent off_topic_reminder for chat start
    # Check both empty string and None, and also check if it's a system message by checking dialogue_context
    is_empty_message = not user_message or (isinstance(user_message, str) and user_message.strip() == "")
    
    # Also check if this is a chat start by checking dialogue_state or metadata
    dialogue_state = state.get("dialogue_state")
    history = state.get("dialogue_history", [])
    is_chat_start_from_state = False
    if dialogue_state and isinstance(dialogue_state, dict):
        status = dialogue_state.get("status")
        if status == "started" or status == "active":
            # Check if history is empty (chat just started)
            if len(history) == 0:
                is_chat_start_from_state = True
    
    # CRITICAL: Also check if history is empty and user_message is empty - this is definitely chat start
    if len(history) == 0 and is_empty_message:
        is_chat_start_from_state = True
    
    # CRITICAL: Force skip if empty message OR empty history (chat start)
    should_skip = is_empty_message or is_chat_start_from_state or (len(history) == 0 and not user_message)
    
    if should_skip:
        logger.info(
            "Skipping off-topic check for chat start",
            chat_id=state["chat_id"],
            user_message_length=len(user_message) if user_message else 0,
            user_message_repr=repr(user_message),
            is_empty_message=is_empty_message,
            is_chat_start_from_state=is_chat_start_from_state,
            dialogue_state_status=dialogue_state.get("status") if dialogue_state and isinstance(dialogue_state, dict) else None,
            history_length=len(history),
            should_skip=should_skip,
        )
        state["processing_steps"].append("skipped_off_topic_check_chat_start")
        # Ensure agent_decision is NOT off_topic_reminder for chat start
        if state.get("agent_decision") == "off_topic_reminder":
            logger.warning(
                "Removing incorrect off_topic_reminder decision for chat start",
                chat_id=state["chat_id"],
            )
            state["agent_decision"] = None
        return state
    
    recent_messages = state.get("dialogue_history", [])[-5:]  # Last 5 messages

    try:
        prompt = build_off_topic_prompt(user_message, recent_messages)
        response = await llm_client.generate(prompt, temperature=0.1)

        decision = response.strip().lower()
        if decision in ["off_topic", "comment"]:
            state["agent_decision"] = "off_topic_reminder"
            state["processing_steps"].append("off_topic_detected")
            logger.info("Off-topic message detected", chat_id=state["chat_id"])
        else:
            state["processing_steps"].append("on_topic")
            logger.debug("Message is on-topic", chat_id=state["chat_id"])

    except Exception as e:
        logger.error("Failed to check off-topic", error=str(e), exc_info=True)
        # Default to on-topic if check fails
        state["processing_steps"].append("on_topic")

    return state


async def determine_question_index(state: AgentState) -> AgentState:
    """Node 4: Determine current question index from history"""
    logger.debug("NODE: determine_question_index", chat_id=state["chat_id"], decision=state.get("agent_decision"))
    # Для off-topic тоже нужно определить текущий вопрос, чтобы повторить его в напоминании
    # Не пропускаем этот узел даже для off-topic
    
    user_message = state.get("user_message", "")
    program = state.get("interview_program")
    history = state.get("dialogue_history", [])
    is_off_topic = state.get("agent_decision") == "off_topic_reminder"

    if not program or "questions" not in program:
        state["error"] = "Interview program not loaded"
        state["agent_decision"] = "error"
        return state

    current_question_id = state.get("current_question_id") or state.get("question_id")
    if not current_question_id:
        state["error"] = "Missing question_id for current message"
        state["agent_decision"] = "error"
        logger.error(
            "Missing question_id for current message",
            chat_id=state["chat_id"],
        )
        return state

    questions = program.get("questions", [])
    for q in questions:
        if q.get("id") == current_question_id:
            state["current_question_index"] = q.get("order")
            state["current_question"] = q
            state["current_theory"] = q.get("theory")
            state["processing_steps"].append(
                f"question_index_determined: {state['current_question_index']} (from_question_id)"
            )
            logger.info(
                "Question determined by question_id",
                chat_id=state["chat_id"],
                question_id=current_question_id,
                question_index=state["current_question_index"],
            )
            return state

    state["error"] = "question_id not found in program"
    state["agent_decision"] = "error"
    logger.error(
        "question_id not found in program",
        chat_id=state["chat_id"],
        question_id=current_question_id,
    )
    return state

    # For system messages (chat start), set first question index directly
    if not user_message or user_message.strip() == "":
        logger.info(
            "Chat start - setting first question index",
            chat_id=state["chat_id"],
        )
        questions = program.get("questions", [])
        if questions:
            first_question = questions[0]
            state["current_question_index"] = first_question.get("order", 1)
            state["current_question"] = first_question
            state["current_theory"] = first_question.get("theory")
            state["processing_steps"].append(
                f"question_index_determined: {state['current_question_index']} (chat_start)"
            )
            logger.info(
                "Question index determined (chat start)",
                chat_id=state["chat_id"],
                question_index=state["current_question_index"],
            )
            return state

    # Для off-topic используем упрощенную логику - ищем вопрос в истории или берем первый
    if is_off_topic:
        logger.info("Determining question for off-topic reminder", chat_id=state["chat_id"])
        # Сначала пытаемся найти вопрос в истории
        if history:
            for msg in reversed(history[-10:]):
                if msg and isinstance(msg, dict) and msg.get("role") == "assistant":
                    content = msg.get("content", "")
                    questions = program.get("questions", [])
                    for q in questions:
                        q_text = q.get("question", "")
                        # Проверяем, содержит ли сообщение вопрос из программы
                        if q_text in content or content in q_text or any(word in content.lower() for word in q_text.lower().split()[:3] if len(word) > 4):
                            state["current_question_index"] = q.get("order")
                            state["current_question"] = q
                            state["current_theory"] = q.get("theory")
                            logger.info(
                                "Question found in history for off-topic",
                                chat_id=state["chat_id"],
                                question_index=state["current_question_index"],
                            )
                            return state
        
        # Если не нашли в истории, берем первый вопрос
        questions = program.get("questions", [])
        if questions:
            first_q = questions[0]
            state["current_question_index"] = first_q.get("order", 1)
            state["current_question"] = first_q
            state["current_theory"] = first_q.get("theory")
            logger.info(
                "Using first question for off-topic reminder",
                chat_id=state["chat_id"],
                question_index=state["current_question_index"],
            )
        return state

    try:
        # CRITICAL: First, check dialogue_state for current_question_index - this is the most reliable source
        # This is updated when we move to next question, so it reflects the actual current question
        dialogue_state = state.get("dialogue_state")
        if dialogue_state and isinstance(dialogue_state, dict):
            stored_question_index = dialogue_state.get("current_question_index")
            if stored_question_index is not None:
                questions = program.get("questions", [])
                for q in questions:
                    if q.get("order") == stored_question_index:
                        state["current_question_index"] = stored_question_index
                        state["current_question"] = q
                        state["current_theory"] = q.get("theory")
                        logger.info(
                            "Using current_question_index from dialogue_state",
                            chat_id=state["chat_id"],
                            question_index=stored_question_index,
                            question_text=q_text[:50] if (q_text := q.get("question", "")) else None,
                        )
                        state["processing_steps"].append(
                            f"question_index_determined: {state['current_question_index']} (from_dialogue_state)"
                        )
                        return state
        
        # If dialogue_state doesn't have it, try to find the MOST RECENT question from history
        # This is more reliable than asking LLM, especially when user asks about a new question
        most_recent_question_index = None
        if history:
            # Look for the last assistant message that contains a question from program
            # CRITICAL: Check messages in reverse order to find the MOST RECENT question asked
            # IMPORTANT: Skip transition messages that contain multiple questions - look for the actual question asked
            for msg in reversed(history[-10:]):
                if msg and isinstance(msg, dict) and msg.get("role") == "assistant":
                    content = msg.get("content", "")
                    questions = program.get("questions", [])
                    # Check questions in REVERSE order (most recent questions first)
                    # This ensures we find the question that was asked most recently
                    # CRITICAL: Prefer questions that appear at the END of the message (new question)
                    # over questions that appear at the BEGINNING (old question in transition)
                    for q in reversed(questions):
                        q_text = q.get("question", "")
                        # More flexible matching - check if question text appears in content
                        # Also check if key terms from question appear in content
                        q_terms = [word.lower() for word in q_text.split() if len(word) > 4]
                        content_lower = content.lower()
                        # Check if question appears in the last 50% of the message (more likely to be the new question)
                        content_length = len(content)
                        question_position = content_lower.find(q_text.lower())
                        is_in_last_half = question_position >= content_length // 2 if question_position >= 0 else False
                        
                        if (q_text in content or 
                            any(word in content_lower for word in q_terms) or
                            any(word in content_lower for word in q_text.lower().split()[:5] if len(word) > 3)):
                            # If question appears in last half, prefer it; otherwise use first match
                            if is_in_last_half or most_recent_question_index is None:
                                most_recent_question_index = q.get("order")
                                logger.info(
                                    "Found most recent question from history",
                                    chat_id=state["chat_id"],
                                    question_index=most_recent_question_index,
                                    question_text=q_text[:50],
                                    matched_content=content[:100],
                                    is_in_last_half=is_in_last_half,
                                )
                                if is_in_last_half:
                                    break  # Prefer question in last half
                    # Break outer loop if we found a question in last half
                    if most_recent_question_index is not None:
                        # Check if we found a question in last half by checking the last matched question
                        if is_in_last_half:
                            break
        
        # If we found a question from history, use it
        if most_recent_question_index is not None:
            state["current_question_index"] = most_recent_question_index
            questions = program.get("questions", [])
            for q in questions:
                if q.get("order") == most_recent_question_index:
                    state["current_question"] = q
                    state["current_theory"] = q.get("theory")
                    break
        else:
            # Fallback to LLM if we couldn't find from history
            prompt = build_question_index_prompt(program, history)
            response = await llm_client.generate(prompt, temperature=0.1)
            
            # Try to parse JSON from response (may be wrapped in text or code blocks)
            try:
                # Remove markdown code blocks if present
                cleaned_response = response.strip()
                if cleaned_response.startswith("```"):
                    # Extract JSON from code block
                    lines = cleaned_response.split("\n")
                    cleaned_response = "\n".join(lines[1:-1]) if len(lines) > 2 else cleaned_response
                elif cleaned_response.startswith("```json"):
                    lines = cleaned_response.split("\n")
                    cleaned_response = "\n".join(lines[1:-1]) if len(lines) > 2 else cleaned_response
                
                result = json.loads(cleaned_response)
            except json.JSONDecodeError:
                # If not valid JSON, try to extract JSON object from text
                import re
                json_match = re.search(r'\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}', cleaned_response)
                if json_match:
                    result = json.loads(json_match.group())
                else:
                    raise
            state["current_question_index"] = result.get("current_question_order")

        # Set current question
        if state["current_question_index"] is not None and program:
            questions = program.get("questions", [])
            for q in questions:
                if q.get("order") == state["current_question_index"]:
                    state["current_question"] = q
                    state["current_theory"] = q.get("theory")
                    break
        else:
            # If question_index is None, try to infer from history
            # CRITICAL: Look for the MOST RECENT question asked (not the first one!)
            # This prevents going back to previous questions when user gives incomplete answer
            if history:
                # Look for last assistant message that might contain a question
                # Start from the most recent message and go backwards
                for msg in reversed(history[-10:]):
                    if msg and isinstance(msg, dict) and msg.get("role") == "assistant":
                        # Try to match content with program questions
                        content = msg.get("content", "")
                        questions = program.get("questions", [])
                        # Check questions in reverse order (most recent first)
                        for q in reversed(questions):
                            q_text = q.get("question", "")
                            # More flexible matching - check if question text appears in content
                            if q_text in content or any(word in content.lower() for word in q_text.lower().split()[:5] if len(word) > 3):
                                state["current_question_index"] = q.get("order")
                                state["current_question"] = q
                                state["current_theory"] = q.get("theory")
                                logger.info(
                                    "Inferred question index from history (most recent)",
                                    chat_id=state["chat_id"],
                                    question_index=state["current_question_index"],
                                    question_text=q_text[:50],
                                )
                                break
                        if state["current_question_index"] is not None:
                            break
            
            # CRITICAL: If still None, DON'T default to first question!
            # Instead, try to find the last question that was asked based on dialogue history
            # Only use first question as absolute last resort
            if state["current_question_index"] is None:
                # Try to find last question from dialogue_state if available
                dialogue_state = state.get("dialogue_state")
                if dialogue_state and isinstance(dialogue_state, dict):
                    last_question_index = dialogue_state.get("current_question_index")
                    if last_question_index is not None:
                        questions = program.get("questions", [])
                        for q in questions:
                            if q.get("order") == last_question_index:
                                state["current_question_index"] = last_question_index
                                state["current_question"] = q
                                state["current_theory"] = q.get("theory")
                                logger.info(
                                    "Using last question index from dialogue_state",
                                    chat_id=state["chat_id"],
                                    question_index=last_question_index,
                                )
                                break
                
                # Only if still None, use first question as last resort
                if state["current_question_index"] is None:
                    questions = program.get("questions", [])
                    if questions:
                        first_q = questions[0]
                        state["current_question_index"] = first_q.get("order", 1)
                        state["current_question"] = first_q
                        state["current_theory"] = first_q.get("theory")
                        logger.warning(
                            "Could not determine question index, defaulting to first (last resort)",
                            chat_id=state["chat_id"],
                        )
                    else:
                        state["current_question"] = None

        state["processing_steps"].append(
            f"question_index_determined: {state['current_question_index']}"
        )
        logger.info(
            "Question index determined",
            chat_id=state["chat_id"],
            question_index=state["current_question_index"],
        )

    except json.JSONDecodeError as e:
        logger.error("Failed to parse question index response", error=str(e))
        state["error"] = f"Failed to parse question index: {str(e)}"
        state["agent_decision"] = "error"
    except Exception as e:
        logger.error("Failed to determine question index", error=str(e), exc_info=True)
        state["error"] = f"Failed to determine question index: {str(e)}"
        state["agent_decision"] = "error"

    return state


async def evaluate_answer(state: AgentState) -> AgentState:
    """Node 5: Evaluate user's answer"""
    logger.debug("NODE: evaluate_answer", chat_id=state["chat_id"], decision=state.get("agent_decision"))
    
    # Skip if off-topic (already handled)
    if state.get("agent_decision") == "off_topic_reminder":
        state["processing_steps"].append("skipped_evaluation_off_topic")
        logger.debug("Skipping evaluation for off-topic", chat_id=state["chat_id"])
        return state
    
    user_message = state.get("user_message", "")
    current_question = state.get("current_question")
    current_theory = state.get("current_theory")
    current_question_id = state.get("current_question_id")

    # Skip evaluation for system messages (chat start) - no user message to evaluate
    if not user_message or user_message.strip() == "":
        logger.info(
            "Skipping evaluation for system message (chat start)",
            chat_id=state["chat_id"],
        )
        # Create a placeholder evaluation for chat start
        state["answer_evaluation"] = {
            "is_complete": True,  # Treat as complete to proceed to first question
            "overall_score": 0.0,
            "reason": "Chat start - no answer to evaluate",
        }
        state["processing_steps"].append("evaluation_skipped_chat_start")
        return state

    if not current_question:
        logger.warning(
            "Current question not determined, creating default evaluation",
            chat_id=state["chat_id"],
        )
        # Create a default evaluation indicating we can't evaluate without question
        state["answer_evaluation"] = {
            "is_complete": False,
            "overall_score": 0.0,
            "reason": "Question not determined - program may not be loaded",
        }
        state["processing_steps"].append("evaluation_skipped_no_question")
        return state

    try:
        # Collect attempts counts for the current question (no hardcoded thresholds)
        clarification_attempts = 0
        hint_attempts = 0
        dialogue_state = state.get("dialogue_state") or {}
        clarification_history = dialogue_state.get("clarification_history", {})
        if isinstance(clarification_history, dict) and current_question_id:
            entries = clarification_history.get(current_question_id, [])
            if isinstance(entries, list):
                clarification_attempts = len(entries)
        history = state.get("dialogue_history", []) or []
        if current_question_id and history:
            for msg in history:
                if not isinstance(msg, dict):
                    continue
                metadata = msg.get("metadata") or {}
                msg_qid = metadata.get("question_id") or msg.get("question_id")
                msg_kind = metadata.get("message_kind") or msg.get("message_kind")
                if msg_qid == current_question_id and msg_kind == "hint":
                    hint_attempts += 1

        prompt = build_evaluation_prompt(
            current_question.get("question", ""),
            current_theory or "",
            user_message,
            clarification_attempts=clarification_attempts,
            hint_attempts=hint_attempts,
        )

        response = await llm_client.generate(prompt, temperature=0.1)
        
        # Try to parse JSON from response (may be wrapped in text or code blocks)
        try:
            # Remove markdown code blocks if present
            cleaned_response = response.strip()
            if cleaned_response.startswith("```"):
                # Extract JSON from code block
                lines = cleaned_response.split("\n")
                cleaned_response = "\n".join(lines[1:-1]) if len(lines) > 2 else cleaned_response
            elif cleaned_response.startswith("```json"):
                lines = cleaned_response.split("\n")
                cleaned_response = "\n".join(lines[1:-1]) if len(lines) > 2 else cleaned_response
            
            evaluation = json.loads(cleaned_response)
        except json.JSONDecodeError:
            # If not valid JSON, try to extract JSON object from text
            json_match = re.search(r'\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}', cleaned_response)
            if json_match:
                evaluation = json.loads(json_match.group())
            else:
                raise

        if not isinstance(evaluation, dict):
            evaluation = {}
        errors = evaluation.get("errors") or []
        if not isinstance(errors, list):
            errors = [str(errors)]
        evaluation["errors"] = errors
        state["answer_evaluation"] = evaluation
        state["processing_steps"].append("answer_evaluated")
        logger.info(
            "Answer evaluated",
            chat_id=state["chat_id"],
            is_complete=evaluation.get("is_complete", False),
            overall_score=evaluation.get("overall_score", 0.0),
        )

    except json.JSONDecodeError as e:
        logger.error("Failed to parse evaluation response", error=str(e), response_preview=response[:200] if 'response' in locals() else None)
        state["error"] = f"Failed to parse evaluation response: {str(e)}"
        state["agent_decision"] = "error"
        state["processing_steps"].append("evaluation_parse_failed")
        return state
    except Exception as e:
        logger.error("Failed to evaluate answer", error=str(e), exc_info=True)
        state["error"] = f"Failed to evaluate answer: {str(e)}"
        state["agent_decision"] = "error"
        state["processing_steps"].append("evaluation_failed")
        return state

    return state


def make_decision(state: AgentState) -> AgentState:
    """Node 6: Make decision about next action"""
    logger.debug("NODE: make_decision", chat_id=state["chat_id"], decision=state.get("agent_decision"))
    
    user_message = state.get("user_message", "")
    
    # For system messages (chat start), ALWAYS generate greeting and first question
    # This check must come BEFORE off-topic check to ensure chat start is handled correctly
    # CRITICAL: This is the FIRST check and MUST override any previous decision
    if not user_message or user_message.strip() == "":
        logger.info(
            "Chat start detected in make_decision - generating greeting and first question",
            chat_id=state["chat_id"],
            user_message_length=len(user_message),
            user_message_repr=repr(user_message),
            previous_decision=state.get("agent_decision"),
        )
        # FORCE next_question decision, overriding any previous decision
        state["agent_decision"] = "next_question"
        state["processing_steps"].append("decision_made: next_question (chat_start)")
        logger.info(
            "Decision made (chat start) - FORCED next_question",
            chat_id=state["chat_id"],
            decision=state["agent_decision"],
        )
        return state
    
    # If off-topic, decision already made, just log it (but only for non-empty messages)
    if state.get("agent_decision") == "off_topic_reminder":
        state["processing_steps"].append(f"decision_made: {state['agent_decision']}")
        logger.info(
            "Decision made (off-topic)",
            chat_id=state["chat_id"],
            decision=state["agent_decision"],
        )
        return state

    evaluation = state.get("answer_evaluation")
    current_index = state.get("current_question_index")
    total = state.get("total_questions", 0)
    history = state.get("dialogue_history", [])

    if not evaluation:
        state["agent_decision"] = "error"
        return state

    # CRITICAL: Check if we just moved to next question in previous response
    # If the last assistant message contains "Следующий вопрос" or similar transition phrases,
    # and user is now answering, we should NOT ask clarification for the old question
    last_assistant_msg = None
    if history:
        for msg in reversed(history[-5:]):
            if msg and isinstance(msg, dict) and msg.get("role") == "assistant":
                last_assistant_msg = msg.get("content", "")
                break
    
    # Check if last message was a transition to next question
    is_transition_to_next = False
    if last_assistant_msg:
        transition_phrases = [
            "следующий вопрос",
            "перейдем к следующему",
            "переходим к следующему",
            "давайте перейдем",
        ]
        is_transition_to_next = any(phrase in last_assistant_msg.lower() for phrase in transition_phrases)
    
    # If we just transitioned to next question, and user is answering, 
    # we should evaluate their answer to the NEW question, not ask clarification for old one
    if is_transition_to_next and evaluation.get("is_complete", True):
        # User is answering the new question - treat as complete answer to move forward
        logger.info(
            "Detected transition to next question - treating answer as complete",
            chat_id=state["chat_id"],
            current_index=current_index,
        )
        if current_index is not None and current_index >= total:
            state["agent_decision"] = "thank_you"
        else:
            state["agent_decision"] = "next_question"
        state["processing_steps"].append("decision_made: next_question (after_transition)")
        return state
    
    # Use explicit clarification history from Redis state (no text heuristics)
    clarification_history = (state.get("dialogue_state") or {}).get("clarification_history", {})
    clarification_attempts = 0
    if isinstance(clarification_history, dict) and state.get("current_question_id"):
        entries = clarification_history.get(state["current_question_id"], [])
        if isinstance(entries, list):
            clarification_attempts = len(entries)

    # Count hint attempts from dialogue history (message_kind metadata)
    hint_attempts = 0
    history = state.get("dialogue_history", []) or []
    current_qid = state.get("current_question_id")
    if current_qid and history:
        for msg in history:
            if not isinstance(msg, dict):
                continue
            metadata = msg.get("metadata") or {}
            msg_qid = metadata.get("question_id") or msg.get("question_id")
            msg_kind = metadata.get("message_kind") or msg.get("message_kind")
            if msg_qid == current_qid and msg_kind == "hint":
                hint_attempts += 1

    total_attempts = clarification_attempts + hint_attempts

    # Store attempts count for LLM to make decision
    state["clarification_attempts"] = total_attempts
    
    # Respect explicit skip request from evaluation
    if evaluation.get("skip_question_request"):
        if current_index is not None and current_index >= total:
            state["agent_decision"] = "thank_you"
        else:
            state["agent_decision"] = "next_question"
        state["processing_steps"].append("decision_made: skip_question -> next_question")
        logger.info(
            "Decision made (skip question)",
            chat_id=state["chat_id"],
            decision=state["agent_decision"],
        )
        return state

    # If user explicitly asked for a hint, honor it immediately.
    if evaluation.get("wants_hint"):
        state["agent_decision"] = "give_hint"
        state["processing_steps"].append("decision_made: give_hint (explicit)")
        logger.info(
            "Decision made (hint request)",
            chat_id=state["chat_id"],
            decision=state["agent_decision"],
        )
        return state

    # If answer is incomplete, decide between clarification or moving on
    if not evaluation.get("is_complete", False):
        should_move_on = bool(evaluation.get("should_move_on"))
        has_missing = bool(evaluation.get("missing_points") or [])

        if should_move_on:
            # Если по мнению модели по этому вопросу уже достаточно попыток,
            # переходим к следующему вопросу ИЛИ завершаем интервью, если это последний вопрос программы.
            if current_index is not None and current_index >= total:
                state["agent_decision"] = "thank_you"
            else:
                state["agent_decision"] = "next_question"
        elif not has_missing:
            # Нет явно оставшихся аспектов — тоже двигаемся дальше по программе.
            if current_index is not None and current_index >= total:
                state["agent_decision"] = "thank_you"
            else:
                state["agent_decision"] = "next_question"
        else:
            # Есть конкретные недостающие аспекты — задаём уточнение.
            state["agent_decision"] = "ask_clarification"
            state["clarification_attempt"] = clarification_attempts + 1
    # If answer is complete, move to next question
    elif evaluation.get("is_complete", False):
        # Check if this is the last question
        if current_index is not None and current_index >= total:
            state["agent_decision"] = "thank_you"
        else:
            state["agent_decision"] = "next_question"
    else:
        state["agent_decision"] = "error"

    state["processing_steps"].append(f"decision_made: {state['agent_decision']}")
    logger.info(
        "Decision made",
        chat_id=state["chat_id"],
        decision=state["agent_decision"],
    )

    return state


async def generate_response(state: AgentState) -> AgentState:
    """Node 7: Generate response based on decision"""
    if not state:
        logger.error("State is None in generate_response")
        state = {"error": "State is None", "agent_decision": "error", "chat_id": "unknown", "processing_steps": []}
        return state
    
    user_message = state.get("user_message", "")
    decision = state.get("agent_decision")
    
    # CRITICAL: For chat start (empty user_message), ALWAYS use next_question prompt
    # This overrides any incorrect agent_decision that might have been set
    # This is the FINAL check before generating response
    if not user_message or user_message.strip() == "":
        logger.warning(
            "Chat start detected in generate_response - FORCING next_question decision",
            chat_id=state.get("chat_id", "unknown"),
            current_decision=decision,
            user_message_length=len(user_message),
            user_message_repr=repr(user_message),
        )
        decision = "next_question"
        state["agent_decision"] = "next_question"
        # Add explicit step to track this override
        state.setdefault("processing_steps", []).append("generate_response_chat_start_override")
    
    logger.debug("NODE: generate_response", chat_id=state.get("chat_id", "unknown"), decision=decision)

    try:
        if decision == "ask_clarification":
            # CRITICAL: Ensure we're using the CURRENT question for clarification, not previous
            current_question = state.get("current_question")
            current_index = state.get("current_question_index")
            logger.info(
                "Generating clarification for current question",
                chat_id=state.get("chat_id", "unknown"),
                current_question_index=current_index,
                current_question_text=current_question.get("question", "")[:100] if current_question else None,
            )
            prompt = build_clarification_prompt(state or {})
        elif decision == "give_hint":
            prompt = build_hint_prompt(state or {})
        elif decision == "give_hint":
            hint_prompt = build_hint_prompt(state or {})
            hint_response = await llm_client.generate(hint_prompt, temperature=0.7, max_retries=1)
            generated_response = hint_response.strip() if hint_response else ""
        elif decision == "next_question":
            prompt = build_next_question_prompt(state or {})
        elif decision == "off_topic_reminder":
            # Double-check: don't use off-topic prompt for chat start
            if not user_message or user_message.strip() == "":
                logger.warning(
                    "Prevented off_topic_reminder for chat start - using next_question instead",
                    chat_id=state.get("chat_id", "unknown"),
                )
                decision = "next_question"
                state["agent_decision"] = "next_question"
                prompt = build_next_question_prompt(state or {})
            else:
                prompt = build_off_topic_reminder_prompt(state or {})
        elif decision == "thank_you":
            prompt = build_final_evaluation_prompt(state or {})
        elif decision == "error":
            # Generate a generic error response
            logger.warning(
                "Generating error response",
                chat_id=state.get("chat_id", "unknown"),
                error=state.get("error"),
            )
            error_msg = state.get("error", "Произошла ошибка при обработке вашего ответа")
            prompt = f"Извините, {error_msg}. Пожалуйста, попробуйте ответить еще раз."
        else:
            logger.error(
                "Unknown decision in generate_response",
                chat_id=state.get("chat_id", "unknown"),
                decision=decision,
            )
            state["error"] = f"Unknown decision: {decision}"
            return state
    except AttributeError as e:
        logger.error("Error building prompt", error=str(e), decision=decision, state_type=type(state), exc_info=True)
        state["error"] = f"Error building prompt: {str(e)}"
        state["agent_decision"] = "error"
        return state

    try:
        logger.debug("Generating LLM response", chat_id=state["chat_id"], decision=decision)
        response = await llm_client.generate(prompt, temperature=0.7)
        response_text = response.strip()

        # If clarification, expect JSON with hint/question or move_on
        if decision == "ask_clarification":
            def _parse_clarification_json(raw: str) -> Dict[str, Any]:
                cleaned = raw.strip()
                if cleaned.startswith("```"):
                    lines = cleaned.split("\n")
                    cleaned = "\n".join(lines[1:-1]) if len(lines) > 2 else cleaned
                elif cleaned.startswith("```json"):
                    lines = cleaned.split("\n")
                    cleaned = "\n".join(lines[1:-1]) if len(lines) > 2 else cleaned
                try:
                    return json.loads(cleaned)
                except json.JSONDecodeError:
                    import re
                    json_match = re.search(r'\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}', cleaned)
                    if json_match:
                        return json.loads(json_match.group())
                    raise

            clarification_data = None
            try:
                clarification_data = _parse_clarification_json(response_text)
            except Exception:
                # One retry with strict JSON-only instruction
                strict_prompt = prompt + "\n\nОТВЕТЬ ТОЛЬКО JSON. Без текста."
                retry_response = await llm_client.generate(strict_prompt, temperature=0.3, max_retries=1)
                retry_text = retry_response.strip() if retry_response else ""
                try:
                    clarification_data = _parse_clarification_json(retry_text)
                    response_text = retry_text
                except Exception:
                    clarification_data = None

            if clarification_data:
                action = str(clarification_data.get("action", "")).strip().lower()
                question = (clarification_data.get("question") or "").strip()

                # If model chooses to move on, generate next question via prompt
                if action == "move_on":
                    state["agent_decision"] = "next_question"
                    prompt = build_next_question_prompt(state or {})
                    response = await llm_client.generate(prompt, temperature=0.7)
                    response_text = response.strip()
                elif action == "clarify" and question:
                    response_text = question
                else:
                    state["agent_decision"] = "next_question"
                    prompt = build_next_question_prompt(state or {})
                    response = await llm_client.generate(prompt, temperature=0.7)
                    response_text = response.strip()
        
        # If hint response looks like clarification, retry with stricter prompt.
        if decision == "give_hint":
            response_lower = response_text.lower()
            if "уточн" in response_lower or "уточнить" in response_lower:
                strict_prompt = build_hint_prompt(state or {}) + "\n\nОТВЕТЬ СТРОГО В ФОРМАТЕ: Подсказка: ... Вопрос: ..."
                retry_response = await llm_client.generate(strict_prompt, temperature=0.3, max_retries=1)
                if retry_response:
                    response_text = retry_response.strip()
            if "подсказка:" not in response_text.lower() or "вопрос:" not in response_text.lower():
                current_question = (state.get("current_question") or {}).get("question", "")
                current_theory = state.get("current_theory") or ""
                hint = ""
                if current_theory:
                    parts = re.split(r"[.!?]\s+", current_theory.strip())
                    if parts:
                        hint = parts[0].strip()
                if not hint:
                    hint = "Подумайте о ключевых терминах из вопроса"
                response_text = f"Подсказка: {hint}.\nВопрос: {current_question}"

        # CRITICAL: If LLM decided to move to next question in clarification response,
        # detect this and update decision accordingly (text-only fallback)
        if decision == "ask_clarification":
            response_lower = response_text.lower()
            # Check if LLM explicitly suggests moving to next question
            move_next_phrases = [
                "давайте перейдем к следующему вопросу",
                "перейдем к следующему",
                "переходим к следующему",
                "можно перейти к следующему",
                "следующий вопрос",
            ]
            if any(phrase in response_lower for phrase in move_next_phrases):
                logger.info(
                    "LLM decided to move to next question in clarification response",
                    chat_id=state.get("chat_id", "unknown"),
                    clarification_attempts=state.get("clarification_attempts", 0),
                )
                # Update decision to next_question
                current_index = state.get("current_question_index")
                total = state.get("total_questions", 0)
                if current_index is not None and current_index >= total:
                    state["agent_decision"] = "thank_you"
                else:
                    state["agent_decision"] = "next_question"
                state["processing_steps"].append("llm_decided_move_to_next_question")
                # Keep the generated response as is (it already contains the transition message)
        
        # Если это thank_you, парсим JSON с благодарностью и рекомендациями
        if decision == "thank_you":
            try:
                # Убираем markdown code blocks если есть
                cleaned_response = response_text
                if cleaned_response.startswith("```"):
                    lines = cleaned_response.split("\n")
                    cleaned_response = "\n".join(lines[1:-1]) if len(lines) > 2 else cleaned_response
                elif cleaned_response.startswith("```json"):
                    lines = cleaned_response.split("\n")
                    cleaned_response = "\n".join(lines[1:-1]) if len(lines) > 2 else cleaned_response

                thank_you_data = json.loads(cleaned_response)
                state["generated_response"] = thank_you_data.get("thank_you_message", response_text)

                # Сохраняем рекомендации в response_metadata
                recommendations = thank_you_data.get("recommendations", [])
                if recommendations:
                    if not state.get("response_metadata"):
                        state["response_metadata"] = {}
                    state["response_metadata"]["recommendations"] = recommendations
                    logger.info(
                        "Final recommendations generated",
                        chat_id=state["chat_id"],
                        recommendations_count=len(recommendations),
                    )
            except json.JSONDecodeError:
                # Если не удалось распарсить JSON, никогда не отправляем сырой JSON в чат.
                # Вместо этого пытаемся вытащить JSON-фрагмент, а если не получилось —
                # генерируем отдельное благодарственное сообщение текстом.
                import re

                cleaned_response = response_text.strip()
                json_match = re.search(r'\{[^{}]*(?:\{[^{}]*\}[^{}]*)*\}', cleaned_response)
                if json_match:
                    try:
                        thank_you_data = json.loads(json_match.group())
                        state["generated_response"] = thank_you_data.get("thank_you_message", response_text)
                        recommendations = thank_you_data.get("recommendations", [])
                        if recommendations:
                            if not state.get("response_metadata"):
                                state["response_metadata"] = {}
                            state["response_metadata"]["recommendations"] = recommendations
                        logger.warning(
                            "Final evaluation JSON parsed from fragment after decode error",
                            chat_id=state.get("chat_id", "unknown"),
                        )
                    except Exception:
                        # Падаем в текстовый фоллбек ниже
                        pass

                if not state.get("generated_response"):
                    # Текстовый фоллбек: просим модель отдельно сформировать благодарность
                    logger.warning(
                        "Failed to parse final evaluation JSON, using text-only thank_you fallback",
                        chat_id=state.get("chat_id", "unknown"),
                    )
                    fallback_prompt = """Ты - интервьюер. Интервью завершено.

Сформулируй вежливое финальное сообщение благодарности кандидату за участие.

Формат ответа: только текст благодарности, без JSON, без списков, без пояснений."""
                    try:
                        fallback_resp = await llm_client.generate(fallback_prompt, temperature=0.4, max_retries=1)
                        state["generated_response"] = (fallback_resp or "").strip()
                    except Exception:
                        # Самый жёсткий фоллбек — статичное сообщение, чтобы не ломать UI
                        state["generated_response"] = (
                            "Спасибо за участие в интервью! Результаты будут доступны в разделе результатов."
                        )
        else:
            state["generated_response"] = response_text
        
        # Update current question based on IDs for next_question
        if state.get("agent_decision") == "next_question":
            program = state.get("interview_program") or {}
            questions = program.get("questions", []) if program else []
            current_index = state.get("current_question_index")
            user_message = state.get("user_message", "")
            is_chat_start = not user_message or user_message.strip() == ""
            next_question = None
            if is_chat_start and questions:
                next_question = questions[0]
            elif current_index is not None:
                for q in questions:
                    if q and isinstance(q, dict) and q.get("order") == current_index + 1:
                        next_question = q
                        break
            if next_question:
                state["current_question"] = next_question
                state["current_question_index"] = next_question.get("order")
                state["current_question_id"] = next_question.get("id")
        
        # CRITICAL: Validate and sanitize generated response
        # 1. Check for multiple questions and extract only the last one
        # 2. Filter aggressive/negative language
        generated_response = state.get("generated_response", "")
        if generated_response:
            # Count question marks to detect multiple questions
            question_count = generated_response.count("?")
            if question_count > 1:
                logger.warning(
                    "Multiple questions detected in response - extracting last question",
                    chat_id=state["chat_id"],
                    question_count=question_count,
                    response_preview=generated_response[:200],
                )
                # Find the last question mark
                last_question_pos = generated_response.rfind("?")
                if last_question_pos > 0:
                    # Find the start of the last question (look for sentence boundaries)
                    # Start from last question mark and go backwards to find sentence start
                    text_before_last_q = generated_response[:last_question_pos]
                    # Try to find sentence boundary (., !, or newline) before last question
                    sentence_end_pos = max(
                        text_before_last_q.rfind("."),
                        text_before_last_q.rfind("!"),
                        text_before_last_q.rfind("\n"),
                    )
                    if sentence_end_pos > 0:
                        # Keep everything from sentence start to end
                        state["generated_response"] = generated_response[sentence_end_pos + 1:last_question_pos + 1].strip()
                    else:
                        # No sentence boundary found, take everything from start to last question
                        state["generated_response"] = generated_response[:last_question_pos + 1].strip()
                    state["processing_steps"].append("response_sanitized_multiple_questions")
            
            # Check for aggressive/negative language
            aggressive_keywords = [
                "неправильно", "ошибка", "неверно", "плохо", "ужасно",
                "вы должны", "обязаны", "нельзя", "запрещено",
                "не понимаете", "не знаете", "не умеете",
            ]
            response_lower = generated_response.lower()
            has_aggressive = any(keyword in response_lower for keyword in aggressive_keywords)
            
            if has_aggressive:
                logger.warning(
                    "Aggressive language detected in response - sanitizing",
                    chat_id=state["chat_id"],
                    response_preview=generated_response[:200],
                )
                # Replace aggressive phrases with neutral ones
                sanitized = generated_response
                replacements = {
                    "неправильно": "можно улучшить",
                    "ошибка": "неточность",
                    "неверно": "не совсем так",
                    "плохо": "можно дополнить",
                    "вы должны": "рекомендую",
                    "обязаны": "желательно",
                    "нельзя": "лучше не",
                    "не понимаете": "возможно, стоит уточнить",
                    "не знаете": "если не помните",
                    "не умеете": "если не знакомы",
                }
                for aggressive, neutral in replacements.items():
                    sanitized = sanitized.replace(aggressive, neutral)
                    sanitized = sanitized.replace(aggressive.capitalize(), neutral.capitalize())
                
                state["generated_response"] = sanitized
                state["processing_steps"].append("response_sanitized_aggressive_language")
        
        if not state.get("response_metadata"):
            state["response_metadata"] = {}
        if decision == "ask_clarification":
            state["response_metadata"]["message_kind"] = "clarification"
        elif decision == "give_hint":
            state["response_metadata"]["message_kind"] = "hint"
            state["response_metadata"]["score_delta"] = -0.05
        elif decision == "next_question":
            state["response_metadata"]["message_kind"] = "question"
        else:
            state["response_metadata"]["message_kind"] = "system"

        state["processing_steps"].append("response_generated")
        logger.info(
            "Response generated",
            chat_id=state["chat_id"],
            decision=decision,
            response_length=len(state["generated_response"]),
        )
    except Exception as e:
        logger.error("Failed to generate response", error=str(e), exc_info=True)
        # Instead of hardcoded fallback, try to generate using simplified prompt
        decision = state.get("agent_decision", "error")
        chat_id = state.get("chat_id", "unknown")
        
        try:
            # Build minimal prompt based on decision
            if decision == "ask_clarification":
                current_question = state.get("current_question", {})
                question_text = current_question.get("question", "") if current_question else ""
                simplified_prompt = f"""Ты - интервьюер. Кандидат дал неполный ответ на вопрос: {question_text}

Сгенерируй JSON:
{{
  "action": "clarify" | "move_on",
  "question": "<уточняющий вопрос>"
}}

Ответь ТОЛЬКО JSON."""
            elif decision == "give_hint":
                simplified_prompt = build_hint_prompt(state or {})
            elif decision == "next_question":
                program = state.get("interview_program", {})
                questions = program.get("questions", []) if program else []
                current_index = state.get("current_question_index")
                user_message = state.get("user_message", "")
                is_chat_start = not user_message or user_message.strip() == ""
                
                if is_chat_start and questions:
                    first_q = questions[0]
                    simplified_prompt = f"""Ты - интервьюер. Интервью начинается.

Сформулируй короткое приветствие (1-2 предложения) и задай этот вопрос: {first_q.get('question', '')}

Формат: только текст (приветствие + вопрос)."""
                elif current_index is not None and current_index < len(questions) - 1:
                    # Есть следующий вопрос в программе — двигаемся к нему
                    next_q = questions[current_index + 1]
                    simplified_prompt = f"""Ты - интервьюер. Кандидат дал полный ответ.

Сформулируй краткое подтверждение (1 предложение) и задай следующий вопрос: {next_q.get('question', '')}

Формат: только текст (подтверждение + вопрос)."""
                elif current_index is not None and current_index >= len(questions) - 1 and questions:
                    # Программа вопросов уже пройдена — это действительно конец интервью.
                    # Важно: сигнализируем остальному пайплайну, что это thank_you, чтобы
                    # корректно сохранить результаты и завершить сессию.
                    state["agent_decision"] = "thank_you"
                    decision = "thank_you"
                    simplified_prompt = build_final_evaluation_prompt(state or {})
                else:
                    simplified_prompt = """Ты - интервьюер. Переходим к следующему вопросу.

Сформулируй краткое сообщение о переходе.

Формат: только текст."""
            elif decision == "thank_you":
                simplified_prompt = """Ты - интервьюер. Интервью завершено.

Сформулируй благодарность кандидату и сообщи, что результаты будут доступны.

Формат: только текст."""
            else:
                simplified_prompt = """Ты - интервьюер. Произошла техническая проблема.

Вежливо попроси кандидата попробовать ответить еще раз.

Формат: только текст."""
            
            # Try to generate with simplified prompt
            logger.info(
                "Attempting to generate response with simplified prompt after error",
                chat_id=chat_id,
                decision=decision,
            )
            response = await llm_client.generate(simplified_prompt, temperature=0.7, max_retries=2)
            if response and response.strip():
                # If clarification, parse JSON and compose question only
                if decision == "ask_clarification":
                    try:
                        clarification_data = json.loads(response.strip())
                        action = str(clarification_data.get("action", "")).strip().lower()
                        question = (clarification_data.get("question") or "").strip()
                        if action == "move_on" or not question:
                            state["agent_decision"] = "next_question"
                            next_prompt = build_next_question_prompt(state or {})
                            next_response = await llm_client.generate(next_prompt, temperature=0.7, max_retries=1)
                            state["generated_response"] = next_response.strip() if next_response else ""
                        else:
                            state["generated_response"] = question
                    except Exception:
                        state["agent_decision"] = "next_question"
                        next_prompt = build_next_question_prompt(state or {})
                        next_response = await llm_client.generate(next_prompt, temperature=0.7, max_retries=1)
                        state["generated_response"] = next_response.strip() if next_response else ""
                else:
                    state["generated_response"] = response.strip()
                state["processing_steps"].append("response_generated_simplified_prompt")
                logger.info(
                    "Successfully generated response with simplified prompt",
                    chat_id=chat_id,
                    decision=decision,
                )
            else:
                raise ValueError("Empty response from simplified prompt")
        except Exception as e2:
            # If even simplified prompt fails, set error state
            logger.error(
                "Failed to generate response even with simplified prompt",
                error=str(e2),
                chat_id=chat_id,
                decision=decision,
                exc_info=True,
            )
            state["error"] = f"Failed to generate response: {str(e2)}"
            state["generated_response"] = None
            state["processing_steps"].append("response_generation_failed")

    return state


async def publish_response(state: AgentState) -> AgentState:
    """Node 8: Publish response to Kafka"""
    logger.debug("NODE: publish_response", chat_id=state["chat_id"], has_response=bool(state.get("generated_response")))
    generated_response = state.get("generated_response")
    if not generated_response:
        # Generate fallback response instead of failing
        decision = state.get("agent_decision", "error")
        current_question = state.get("current_question", {})
        question_text = current_question.get("question", "") if current_question else ""
        
        if decision == "ask_clarification":
            # Generate a proper clarification question without hints
            try:
                evaluation = state.get("answer_evaluation", {})
                missing_points = evaluation.get("missing_points", [])
                user_message = state.get("user_message", "")
                
                # Build minimal prompt for clarification
                minimal_clarification_prompt = f"""Ты - интервьюер. Кандидат дал неполный ответ: "{user_message[:100]}"

Вопрос: {question_text[:200]}
Недостающие аспекты: {', '.join(missing_points) if missing_points else 'Не указаны'}

Сгенерируй JSON:
{{
  "action": "clarify" | "move_on",
  "question": "<уточняющий вопрос>"
}}

Ответь ТОЛЬКО JSON."""
                
                # Try to generate with minimal retries
                fallback_response = await llm_client.generate(minimal_clarification_prompt, temperature=0.7, max_retries=1)
                if fallback_response and fallback_response.strip():
                    try:
                        clarification_data = json.loads(fallback_response.strip())
                        action = str(clarification_data.get("action", "")).strip().lower()
                        question = (clarification_data.get("question") or "").strip()
                        if action == "move_on" or not question:
                            state["agent_decision"] = "next_question"
                            next_prompt = build_next_question_prompt(state or {})
                            next_response = await llm_client.generate(next_prompt, temperature=0.7, max_retries=1)
                            generated_response = next_response.strip() if next_response else ""
                        else:
                            generated_response = question
                    except Exception:
                        state["agent_decision"] = "next_question"
                        next_prompt = build_next_question_prompt(state or {})
                        next_response = await llm_client.generate(next_prompt, temperature=0.7, max_retries=1)
                        generated_response = next_response.strip() if next_response else ""
            except Exception as e:
                logger.error("Failed to generate clarification fallback", error=str(e))
                # Last resort: move on via next question prompt
                state["agent_decision"] = "next_question"
                next_prompt = build_next_question_prompt(state or {})
                next_response = await llm_client.generate(next_prompt, temperature=0.7, max_retries=1)
                generated_response = next_response.strip() if next_response else ""
        elif decision == "next_question":
            # CRITICAL: Check if this is chat start - must have greeting + first question
            user_message = state.get("user_message", "")
            is_chat_start = not user_message or user_message.strip() == ""
            
            program = state.get("interview_program", {})
            questions = program.get("questions", []) if program else []
            current_index = state.get("current_question_index")
            
            if is_chat_start and questions:
                # Chat start: greeting + first question
                first_q = questions[0]
                generated_response = f"Здравствуйте! Рад вас видеть на интервью. Давайте начнём с первого вопроса: {first_q.get('question', '')}"
            elif current_index is not None and current_index + 1 < len(questions):
                # Regular transition: confirmation + next question
                next_q = questions[current_index + 1]
                generated_response = f"Спасибо за ответ. Следующий вопрос: {next_q.get('question', '')}"
            else:
                # No more questions
                generated_response = "Спасибо за ответ. Переходим к следующему вопросу."
        elif decision == "thank_you":
            generated_response = "Спасибо за участие в интервью! Результаты будут доступны в разделе результатов."
        else:
            generated_response = "Извините, возникла техническая проблема. Пожалуйста, попробуйте ответить еще раз."
        
        state["generated_response"] = generated_response
        state["processing_steps"].append("response_generated_fallback_publish")
        logger.warning(
            "Generated fallback response in publish_response",
            chat_id=state["chat_id"],
            decision=decision,
        )

    try:
        # Формируем метаданные с рекомендациями, если они есть
        payload_metadata = {
            "current_question_index": state.get("current_question_index"),
            "agent_decision": state.get("agent_decision"),
            "evaluation": state.get("answer_evaluation") or {},
            "question_id": state.get("current_question_id"),
        }
        
        # Добавляем рекомендации, если они были сгенерированы (при thank_you)
        response_metadata = state.get("response_metadata", {})
        if response_metadata and "message_kind" in response_metadata:
            payload_metadata["message_kind"] = response_metadata["message_kind"]
        if response_metadata and "score_delta" in response_metadata:
            payload_metadata["score_delta"] = response_metadata["score_delta"]
        if response_metadata and "recommendations" in response_metadata:
            payload_metadata["recommendations"] = response_metadata["recommendations"]
        
        event = {
            "event_id": str(uuid4()),
            "event_type": "phrase.agent.generated",
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "service": settings.service_name,
            "version": settings.service_version,
            "payload": {
                "chat_id": state["chat_id"],
                "message_id": str(uuid4()),
                "question_id": state.get("current_question_id"),
                "generated_text": generated_response,
                "confidence": (state.get("answer_evaluation") or {}).get("overall_score", 0.0),
                "intermediate_steps": state.get("processing_steps", []),
                "metadata": payload_metadata,
                "timestamp": datetime.utcnow().isoformat() + "Z",
            },
            "metadata": {
                "correlation_id": state.get("message_id"),
            },
        }

        kafka_producer.publish(settings.kafka_topic_generated, event)
        state["processing_steps"].append("response_published")
        logger.info(
            "Response published to Kafka",
            chat_id=state["chat_id"],
            topic=settings.kafka_topic_generated,
        )

    except Exception as e:
        logger.error("Failed to publish response", error=str(e), exc_info=True)
        state["error"] = f"Failed to publish response: {str(e)}"

    return state
