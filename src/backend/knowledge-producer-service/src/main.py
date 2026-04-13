"""Main application entry point"""

import signal
from fastapi import FastAPI, HTTPException
from fastapi.responses import JSONResponse
from contextlib import asynccontextmanager

from .config import settings
from .logger import setup_logger, get_logger
from .services import ProducerService
from .metrics import get_metrics_collector

logger = get_logger(__name__)

# Global service instance
producer_service: ProducerService = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan context manager for startup and shutdown"""
    global producer_service
    
    # Startup
    setup_logger()
    logger.info(f"Starting {settings.service_name} v{settings.service_version}")
    
    # Start metrics server
    metrics_collector = get_metrics_collector()
    metrics_collector.start_metrics_server()
    logger.info(f"Metrics server started on port {settings.metrics_port}")
    
    producer_service = ProducerService()
    
    # Auto-load data on startup if enabled
    if settings.auto_load_on_startup:
        import asyncio
        logger.info("Auto-loading data on startup enabled, waiting for CRUD services...")
        
        # Wait for CRUD services to be ready with retries
        retries = 0
        while retries < settings.startup_load_max_retries:
            try:
                # Try to produce all data
                result = await producer_service.produce_all()
                logger.info("Auto-load completed successfully", 
                          knowledge_total=result.get("knowledge", {}).get("total", 0),
                          questions_total=result.get("questions", {}).get("total", 0))
                break
            except Exception as e:
                retries += 1
                if retries >= settings.startup_load_max_retries:
                    logger.error("Failed to auto-load data after max retries", 
                               error=str(e), 
                               retries=retries,
                               exc_info=True)
                    # Don't fail startup, just log error
                else:
                    logger.warning("CRUD services not ready yet, retrying...", 
                                 error=str(e),
                                 retry=retries,
                                 max_retries=settings.startup_load_max_retries)
                    await asyncio.sleep(settings.startup_load_retry_delay)
    
    yield
    
    # Shutdown
    logger.info("Shutting down...")
    if producer_service:
        await producer_service.close()


app = FastAPI(
    title="Knowledge Producer Service",
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


@app.post("/produce/knowledge")
async def produce_knowledge():
    """Produce all knowledge from files"""
    try:
        result = await producer_service.produce_all_knowledge()
        return JSONResponse(content=result)
    except Exception as e:
        logger.error("Failed to produce knowledge", error=str(e), exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/produce/questions")
async def produce_questions():
    """Produce all questions from files"""
    try:
        result = await producer_service.produce_all_questions()
        return JSONResponse(content=result)
    except Exception as e:
        logger.error("Failed to produce questions", error=str(e), exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/produce/all")
async def produce_all():
    """Produce all knowledge and questions from files"""
    try:
        result = await producer_service.produce_all()
        return JSONResponse(content=result)
    except Exception as e:
        logger.error("Failed to produce all", error=str(e), exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        app,
        host=settings.server_host,
        port=settings.server_port,
        log_config=None,
    )

