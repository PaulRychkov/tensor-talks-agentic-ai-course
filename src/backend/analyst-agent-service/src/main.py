"""Main application entry point"""

import signal
from fastapi import FastAPI
from contextlib import asynccontextmanager

from .config import settings
from .logger import setup_logger, get_logger
from .metrics import get_metrics_collector
from .kafka import create_kafka_consumer
from .clients import ResultsCrudClient, SessionServiceClient, ChatCrudClient
from .services import AnalystService
from .models.events import KafkaEvent, SessionCompletedPayload, EventType

logger = get_logger(__name__)

results_client: ResultsCrudClient = None
session_client: SessionServiceClient = None
chat_client: ChatCrudClient = None
analyst_service: AnalystService = None
kafka_consumer = None
running = True


def signal_handler(signum, frame):
    """Handle shutdown signals"""
    global running
    logger.info(f"Received signal {signum}, initiating shutdown...")
    running = False


async def process_kafka_event(event: KafkaEvent):
    """Process a session.completed Kafka event"""
    logger.info(
        "Processing session.completed event",
        event_id=event.event_id,
        event_type=event.event_type,
    )
    try:
        if event.event_type != EventType.SESSION_COMPLETED:
            logger.debug("Skipping event", event_type=event.event_type)
            return

        try:
            payload = SessionCompletedPayload(**event.payload)
        except Exception as e:
            logger.error(
                "Failed to parse SessionCompletedPayload",
                event_id=event.event_id,
                error=str(e),
                payload_keys=list(event.payload.keys()) if isinstance(event.payload, dict) else None,
                exc_info=True,
            )
            return

        logger.info(
            "Parsed session.completed payload",
            event_id=event.event_id,
            session_id=payload.session_id,
            session_kind=payload.session_kind,
            user_id=payload.user_id,
            chat_id=payload.chat_id,
        )

        await analyst_service.analyze_session(payload)

    except Exception as e:
        logger.error(
            "Failed to process session.completed event",
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
    global results_client, session_client, chat_client, analyst_service, kafka_consumer

    setup_logger()
    logger.info(f"Starting {settings.service_name} v{settings.service_version}")

    metrics_collector = get_metrics_collector()
    metrics_collector.start_metrics_server()
    logger.info(f"Metrics server started on port {settings.metrics_port}")

    results_client = ResultsCrudClient()
    session_client = SessionServiceClient()
    chat_client = ChatCrudClient()

    analyst_service = AnalystService(
        results_client=results_client,
        session_client=session_client,
        chat_client=chat_client,
    )
    logger.info("Analyst service initialized")

    if settings.kafka_topic_session_completed:
        kafka_consumer = create_kafka_consumer(
            topic=settings.kafka_topic_session_completed,
            group_id=settings.kafka_consumer_group,
        )
        kafka_consumer.set_handler(process_kafka_event)
        kafka_consumer.start()
        logger.info("Kafka consumer started")

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    logger.info("Application started successfully")

    yield

    logger.info("Shutting down...")
    global running
    running = False

    if kafka_consumer:
        kafka_consumer.stop()
        kafka_consumer.close()

    if analyst_service:
        await analyst_service.close()

    logger.info("Shutdown complete")


app = FastAPI(
    title="Analyst Agent Service",
    version=settings.service_version,
    lifespan=lifespan,
)


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
        log_config=None,
    )
