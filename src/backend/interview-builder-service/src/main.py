"""Main application entry point"""

import signal
import asyncio
from fastapi import FastAPI
from contextlib import asynccontextmanager

from .config import settings
from .logger import setup_logger, get_logger
from .services import InterviewBuilderService
from .kafka import KafkaConsumer
from .metrics import get_metrics_collector

logger = get_logger(__name__)

# Global instances
interview_builder_service: InterviewBuilderService = None
kafka_consumer: KafkaConsumer = None
running = True


def signal_handler(signum, frame):
    """Handle shutdown signals"""
    global running
    logger.info(f"Received signal {signum}, initiating shutdown...")
    running = False


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan context manager for startup and shutdown"""
    global interview_builder_service, kafka_consumer
    
    # Startup
    setup_logger()
    logger.info(f"Starting {settings.service_name} v{settings.service_version}")
    
    # Start metrics server
    metrics_collector = get_metrics_collector()
    metrics_collector.start_metrics_server()
    logger.info(f"Metrics server started on port {settings.metrics_port}")
    
    # Initialize services
    interview_builder_service = InterviewBuilderService()
    
    # Setup Kafka consumer
    kafka_consumer = KafkaConsumer()
    
    def handle_request(session_id: str, params: dict):
        """Handle interview build request - runs in separate thread with its own event loop"""
        # Run async function in new event loop
        try:
            asyncio.run(
                interview_builder_service.handle_interview_build_request(session_id, params)
            )
        except Exception as e:
            logger.error("Failed to handle interview build request",
                        session_id=session_id,
                        error=str(e),
                        exc_info=True)
    
    kafka_consumer.set_handler(handle_request)
    kafka_consumer.start()
    
    # Register signal handlers
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    logger.info("Application started successfully")
    
    yield
    
    # Shutdown
    logger.info("Shutting down...")
    if kafka_consumer:
        kafka_consumer.stop()
    if interview_builder_service:
        await interview_builder_service.close()
    logger.info("Shutdown complete")


app = FastAPI(
    title="Interview Builder Service",
    version=settings.service_version,
    lifespan=lifespan,
)


@app.get("/healthz")
async def health_check():
    """Health check endpoint"""
    return {"status": "ok"}


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
        host=settings.server_host,
        port=settings.server_port,
        log_config=None,
    )

