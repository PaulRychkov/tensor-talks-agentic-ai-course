"""Main application entry point"""

import signal
from typing import Optional
from fastapi import FastAPI, File, HTTPException, Query, UploadFile
from fastapi.responses import JSONResponse
from pydantic import BaseModel, Field
from contextlib import asynccontextmanager

from .config import settings
from .logger import setup_logger, get_logger
from .services import ProducerService
from .metrics import get_metrics_collector

# Global LLM client (initialised in lifespan if LLM_API_KEY is set)
_llm_client = None


class CreateDraftRequest(BaseModel):
    title: str
    content: str
    topic: str
    source: str = ""


class ReviewDraftRequest(BaseModel):
    review_status: str = Field(..., pattern="^(approved|rejected)$")
    reviewed_by: str
    review_comment: Optional[str] = None


class PublishDraftRequest(BaseModel):
    override_duplicate: bool = False

logger = get_logger(__name__)

# Global service instance
producer_service: ProducerService = None


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Lifespan context manager for startup and shutdown"""
    global producer_service, _llm_client

    # Startup
    setup_logger()
    logger.info(f"Starting {settings.service_name} v{settings.service_version}")

    # Initialise LLM client if API key is configured
    if settings.llm_api_key:
        from .llm import LLMClient
        _llm_client = LLMClient()
        logger.info("LLM pipeline enabled (llm_workflow for KV ingestion, §8 stage 0)")
    else:
        logger.info("LLM pipeline disabled – set LLM_API_KEY to enable")
    
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


# ── Draft endpoints ──────────────────────────────────────────────────

@app.post("/drafts/knowledge", status_code=201)
async def create_knowledge_draft(body: CreateDraftRequest):
    """Create a new knowledge draft"""
    try:
        draft = await producer_service.create_draft(
            draft_type="knowledge",
            title=body.title,
            content=body.content,
            topic=body.topic,
            source=body.source,
        )
        return JSONResponse(content={"draft": draft}, status_code=201)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        logger.error("Failed to create knowledge draft", error=str(e), exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/drafts/questions", status_code=201)
async def create_question_draft(body: CreateDraftRequest):
    """Create a new question draft"""
    try:
        draft = await producer_service.create_draft(
            draft_type="question",
            title=body.title,
            content=body.content,
            topic=body.topic,
            source=body.source,
        )
        return JSONResponse(content={"draft": draft}, status_code=201)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        logger.error("Failed to create question draft", error=str(e), exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/drafts")
async def list_drafts(status: Optional[str] = Query(default=None)):
    """List all drafts, optionally filtered by review_status"""
    drafts = producer_service.list_drafts(status=status)
    return JSONResponse(content={"drafts": drafts, "total": len(drafts)})


@app.get("/drafts/{draft_id}")
async def get_draft(draft_id: str):
    """Get a single draft by ID"""
    draft = producer_service.get_draft(draft_id)
    if draft is None:
        raise HTTPException(status_code=404, detail=f"Draft {draft_id} not found")
    return JSONResponse(content={"draft": draft})


@app.put("/drafts/{draft_id}/review")
async def review_draft(draft_id: str, body: ReviewDraftRequest):
    """Approve or reject a draft"""
    try:
        draft = producer_service.review_draft(
            draft_id=draft_id,
            review_status=body.review_status,
            reviewed_by=body.reviewed_by,
            review_comment=body.review_comment,
        )
        return JSONResponse(content={"draft": draft})
    except KeyError:
        raise HTTPException(status_code=404, detail=f"Draft {draft_id} not found")
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))


class RejectDraftRequest(BaseModel):
    comment: Optional[str] = None
    reviewed_by: str = "admin"


@app.post("/drafts/{draft_id}/approve")
async def approve_draft(draft_id: str, reviewed_by: str = "admin"):
    """Approve a draft and publish it (convenience alias for review+publish)."""
    try:
        producer_service.review_draft(
            draft_id=draft_id,
            review_status="approved",
            reviewed_by=reviewed_by,
            review_comment=None,
        )
        result = await producer_service.publish_draft(draft_id=draft_id)
        return JSONResponse(content=result)
    except KeyError:
        raise HTTPException(status_code=404, detail=f"Draft {draft_id} not found")
    except PermissionError as e:
        raise HTTPException(status_code=403, detail=str(e))
    except Exception as e:
        logger.error("Failed to approve/publish draft", draft_id=draft_id, error=str(e), exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/drafts/{draft_id}/reject")
async def reject_draft(draft_id: str, body: RejectDraftRequest = RejectDraftRequest()):
    """Reject a draft with optional comment."""
    try:
        draft = producer_service.review_draft(
            draft_id=draft_id,
            review_status="rejected",
            reviewed_by=body.reviewed_by,
            review_comment=body.comment,
        )
        return JSONResponse(content={"draft": draft})
    except KeyError:
        raise HTTPException(status_code=404, detail=f"Draft {draft_id} not found")
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))


@app.post("/drafts/{draft_id}/publish")
async def publish_draft(draft_id: str, body: PublishDraftRequest = PublishDraftRequest()):
    """Publish an approved draft to the CRUD service"""
    try:
        result = await producer_service.publish_draft(
            draft_id=draft_id,
            override_duplicate=body.override_duplicate,
        )
        return JSONResponse(content=result)
    except KeyError:
        raise HTTPException(status_code=404, detail=f"Draft {draft_id} not found")
    except PermissionError as e:
        raise HTTPException(status_code=403, detail=str(e))
    except Exception as e:
        logger.error("Failed to publish draft", draft_id=draft_id, error=str(e), exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))


# ── Ingestion endpoints (§9 p.5.8) ──────────────────────────────


class IngestURLRequest(BaseModel):
    url: str
    topic: str = "general"
    kind: str = Field(default="knowledge", pattern="^(knowledge|question)$")


@app.post("/ingest/url")
async def ingest_from_url(body: IngestURLRequest):
    """Ingest content from URL, run LLM pipeline, save as draft."""
    from .pipeline.ingestion import ingest_url
    from .pipeline.runner import PipelineRunner
    from .schemas import DraftKind

    try:
        raw = await ingest_url(body.url)
    except Exception as e:
        raise HTTPException(status_code=400, detail=f"Failed to fetch URL: {e}")

    runner = PipelineRunner(llm_client=_llm_client)
    kind = DraftKind(body.kind)
    draft = await runner.run(raw, body.topic, kind)
    producer_service._drafts[draft.draft_id] = draft.model_dump(mode="json")
    return JSONResponse(content={"draft_id": draft.draft_id, "duplicate_candidate": draft.duplicate_candidate})


@app.post("/ingest/file")
async def ingest_from_file(
    file: UploadFile = File(...),
    topic: str = Query("general"),
    kind: str = Query(default="knowledge", pattern="^(knowledge|question)$"),
):
    """Ingest uploaded file (PDF, Markdown, JSON) — creates a draft for HITL review.

    Supported formats: .pdf, .md, .txt, .json
    """
    from .pipeline.ingestion import parse_uploaded_file
    from .pipeline.runner import PipelineRunner
    from .schemas import DraftKind

    filename = file.filename or "upload"
    content = await file.read()

    try:
        raw = parse_uploaded_file(filename, content)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        logger.error("Failed to parse uploaded file", filename=filename, error=str(e))
        raise HTTPException(status_code=400, detail=f"Failed to parse file: {e}")

    runner = PipelineRunner(llm_client=_llm_client)
    draft = await runner.run(raw, topic, DraftKind(kind))
    producer_service._drafts[draft.draft_id] = draft.model_dump(mode="json")
    return JSONResponse(content={"draft_id": draft.draft_id, "duplicate_candidate": draft.duplicate_candidate})


# ── Search endpoint (§9 p.5.9) ──────────────────────────────────


@app.get("/search/web")
async def search_web(query: str = Query(...), topic: str = Query("")):
    """Search the web using the configured provider."""
    from .pipeline.web_search import WebSearchService

    svc = WebSearchService()
    results = await svc.search(query, topic=topic or None)
    return JSONResponse(content={"results": results})


if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        app,
        host=settings.server_host,
        port=settings.server_port,
        log_config=None,
    )

