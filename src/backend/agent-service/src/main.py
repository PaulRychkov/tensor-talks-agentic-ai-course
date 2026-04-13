"""Main application entry point"""

import signal
import time
import asyncio
from fastapi import FastAPI
from contextlib import asynccontextmanager
from datetime import datetime

from .config import settings
from .logger import setup_logger, get_logger
from .metrics import get_metrics_collector
from .llm import LLMClient
from .clients import SessionServiceClient, RedisClient
from .kafka import KafkaProducer, create_kafka_consumer
from .graph.builder import create_agent_graph
from .graph.state import AgentState
from .models.events import KafkaEvent, MessageFullPayload, EventType
from .api.endpoints import router, set_agent_graph

logger = get_logger(__name__)

# Global instances
llm_client: LLMClient = None
session_client: SessionServiceClient = None
redis_client: RedisClient = None
kafka_producer: KafkaProducer = None
kafka_consumer = None
agent_graph = None
running = True


def _fix_mojibake(text: str) -> str:
    """Fix common UTF-8 mojibake (e.g., 'РњРѕ...' -> 'Можно')."""
    if not text:
        return text
    for source_encoding in ("latin1", "cp1251"):
        try:
            candidate = text.encode(source_encoding).decode("utf-8")
        except Exception:
            continue
        if candidate != text:
            return candidate
    return text


def signal_handler(signum, frame):
    """Handle shutdown signals"""
    global running
    logger.info(f"Received signal {signum}, initiating shutdown...")
    running = False


async def process_kafka_event(event: KafkaEvent):
    """Process Kafka event through agent graph"""
    logger.info(
        "Processing Kafka event",
        event_id=event.event_id,
        event_type=event.event_type,
    )
    try:
        # Only process message.full events
        if event.event_type != EventType.MESSAGE_FULL:
            logger.debug("Skipping event", event_type=event.event_type)
            return

        # Parse payload
        try:
            payload = MessageFullPayload(**event.payload)
        except Exception as e:
            logger.error(
                "Failed to parse MessageFullPayload",
                event_id=event.event_id,
                error=str(e),
                payload_keys=list(event.payload.keys()) if isinstance(event.payload, dict) else None,
                exc_info=True,
            )
            return

        # Pydantic with use_enum_values=True returns string, not enum
        role_str = str(payload.role)
        
        logger.info(
            "Parsed payload",
            event_id=event.event_id,
            chat_id=payload.chat_id,
            role=role_str,
            content_length=len(payload.content),
        )

        # Process user messages and system messages for chat start
        metadata = payload.metadata
        dialogue_context = metadata.get("dialogue_context", {})
        is_chat_start = dialogue_context.get("status") == "started"
        
        logger.info(
            "Checking message type",
            event_id=event.event_id,
            role=role_str,
            is_chat_start=is_chat_start,
            dialogue_context=dialogue_context,
        )
        
        if role_str == "user":
            # Process user messages normally
            logger.info("Processing user message", chat_id=payload.chat_id, event_id=event.event_id)
        elif role_str == "system" and is_chat_start:
            # Process system messages for chat start (generate greeting and first question)
            logger.info(
                "Processing chat start system message",
                chat_id=payload.chat_id,
                event_id=event.event_id,
            )
        else:
            logger.debug(
                "Skipping non-user/system message",
                role=role_str,
                is_chat_start=is_chat_start,
                event_id=event.event_id,
            )
            return

        # Extract session_id from metadata (already extracted above)
        session_id = dialogue_context.get("session_id", "")

        # Determine user_message: for system messages with chat start, ensure it's empty
        user_message_content = ""
        if role_str == "user":
            user_message_content = payload.content
            logger.info(
                "User message - setting user_message from payload.content",
                chat_id=payload.chat_id,
                event_id=event.event_id,
                content_length=len(payload.content),
            )
        elif role_str == "system" and is_chat_start:
            # For chat start system messages, user_message MUST be empty
            # Even if payload.content contains something, we ignore it for chat start
            user_message_content = ""
            logger.info(
                "Chat start system message - FORCING empty user_message",
                chat_id=payload.chat_id,
                event_id=event.event_id,
                original_content=payload.content,
                content_length=len(payload.content),
                content_repr=repr(payload.content),
                user_message_content=user_message_content,
                user_message_content_repr=repr(user_message_content),
            )
        elif role_str == "system":
            # For other system messages (like resume), also set empty
            user_message_content = ""
            logger.debug(
                "System message (non-chat-start) - setting empty user_message",
                chat_id=payload.chat_id,
                event_id=event.event_id,
                original_content=payload.content,
            )

        # Fix common mojibake for user messages (e.g., UTF-8 decoded as Latin-1).
        if role_str == "user" and user_message_content:
            fixed_message = _fix_mojibake(user_message_content)
            if fixed_message != user_message_content:
                logger.info(
                    "Fixed mojibake in user message",
                    chat_id=payload.chat_id,
                    event_id=event.event_id,
                    before=user_message_content,
                    after=fixed_message,
                )
            user_message_content = fixed_message
        
        if not payload.question_id:
            logger.error(
                "Missing question_id in message.full payload",
                chat_id=payload.chat_id,
                event_id=event.event_id,
                message_id=payload.message_id,
            )
            raise ValueError("Missing question_id in message.full payload")

        # Initialize state
        initial_state: AgentState = {
            "chat_id": payload.chat_id,
            "session_id": session_id,
            "user_id": metadata.get("user_id", ""),
            "message_id": payload.message_id,
            "user_message": user_message_content,
            "message_timestamp": payload.timestamp,
            "question_id": payload.question_id,
            "dialogue_history": [],
            "dialogue_state": None,
            "interview_program": None,
            "total_questions": 0,
            "current_question_index": None,
            "current_question_id": payload.question_id,
            "current_question": None,
            "current_theory": None,
            "answer_evaluation": None,
            "agent_decision": None,
            "generated_response": None,
            "generated_question": None,
            "response_metadata": None,
            "processing_steps": [],
            "error": None,
            "retry_count": 0,
        }
        
        # CRITICAL DEBUG: Log initial state user_message
        logger.info(
            "Initial state created with user_message",
            chat_id=payload.chat_id,
            event_id=event.event_id,
            user_message=initial_state["user_message"],
            user_message_length=len(initial_state["user_message"]) if initial_state["user_message"] else 0,
            user_message_repr=repr(initial_state["user_message"]),
            user_message_type=type(initial_state["user_message"]).__name__,
            is_empty=not initial_state["user_message"],
            is_empty_after_strip=not (initial_state["user_message"] and initial_state["user_message"].strip()),
        )

        # Run graph
        processing_start = time.perf_counter()
        logger.info(
            "Invoking agent graph",
            chat_id=payload.chat_id,
            event_id=event.event_id,
            session_id=session_id,
        )
        with get_metrics_collector().processing_duration.time():
            final_state = await agent_graph.ainvoke(initial_state)
        processing_duration_ms = round((time.perf_counter() - processing_start) * 1000, 2)

        # Update metrics
        decision = final_state.get("agent_decision", "unknown")
        status = "success" if not final_state.get("error") else "error"
        get_metrics_collector().messages_processed_total.labels(
            status=status, decision=decision
        ).inc()

        if final_state.get("current_question_index") is not None:
            get_metrics_collector().current_question_index.observe(
                final_state["current_question_index"]
            )

        logger.info(
            "Message processed",
            chat_id=payload.chat_id,
            message_id=payload.message_id,
            event_id=event.event_id,
            decision=decision,
            success=status == "success",
            has_error=bool(final_state.get("error")),
            error=final_state.get("error"),
            generated_response_length=len(final_state.get("generated_response") or "") if final_state.get("generated_response") else 0,
            response_published="response_published" in final_state.get("processing_steps", []),
            processing_duration_ms=processing_duration_ms,
        )

    except Exception as e:
        logger.error(
            "Failed to process Kafka event",
            event_id=event.event_id,
            error=str(e),
            exc_info=True,
        )
        get_metrics_collector().error_count.labels(
            error_type="kafka_event_processing", service=settings.service_name
        ).inc()


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan context manager for startup and shutdown"""
    global llm_client, session_client, redis_client, kafka_producer, kafka_consumer, agent_graph

    # Startup
    setup_logger()
    logger.info(f"Starting {settings.service_name} v{settings.service_version}")

    # Start metrics server
    metrics_collector = get_metrics_collector()
    metrics_collector.start_metrics_server()
    logger.info(f"Metrics server started on port {settings.metrics_port}")

    # Initialize clients
    logger.info("Initializing clients...")
    llm_client = LLMClient()
    session_client = SessionServiceClient()
    redis_client = RedisClient()
    await redis_client.connect()

    kafka_producer = KafkaProducer()

    # Create agent graph
    logger.info("Creating agent graph...")
    agent_graph = create_agent_graph(
        llm_client, session_client, redis_client, kafka_producer
    )
    set_agent_graph(agent_graph)

    # Setup Kafka consumer
    if settings.kafka_topic_messages_full:
        kafka_consumer = create_kafka_consumer(
            topic=settings.kafka_topic_messages_full,
            group_id=settings.kafka_consumer_group,
        )
        kafka_consumer.set_handler(process_kafka_event)
        kafka_consumer.start()
        logger.info("Kafka consumer started")

    # Register signal handlers
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    logger.info("Application started successfully")

    yield

    # Shutdown
    logger.info("Shutting down...")
    global running
    running = False

    if kafka_consumer:
        kafka_consumer.stop()
        kafka_consumer.close()

    if kafka_producer:
        kafka_producer.close()

    if redis_client:
        await redis_client.close()

    if session_client:
        await session_client.close()

    logger.info("Shutdown complete")


app = FastAPI(
    title="Agent Service",
    version=settings.service_version,
    lifespan=lifespan,
)

# Include API routes
if settings.enable_rest_api:
    app.include_router(router)
    logger.info("REST API enabled")


@app.get("/health")
async def health_check():
    """Health check endpoint"""
    return {"status": "ok", "service": settings.service_name}


@app.get("/metrics")
async def metrics():
    """Prometheus metrics endpoint"""
    from prometheus_client import generate_latest, CONTENT_TYPE_LATEST
    from fastapi.responses import Response

    return Response(content=generate_latest(), media_type=CONTENT_TYPE_LATEST)


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        app,
        host=settings.rest_api_host,
        port=settings.rest_api_port,
        log_config=None,  # Use our structured logging
    )
