"""REST API endpoints for agent service"""

from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from typing import Optional, Dict, Any
from datetime import datetime

from ..graph.state import AgentState
from ..graph.builder import create_agent_graph
from ..logger import get_logger
from ..llm import LLMClient
from ..clients import SessionServiceClient, RedisClient
from ..kafka import KafkaProducer
from ..config import settings

logger = get_logger(__name__)

router = APIRouter(prefix="/api/agent", tags=["agent"])

# Global graph instance (will be set during initialization)
agent_graph = None


def set_agent_graph(graph):
    """Set global agent graph instance"""
    global agent_graph
    agent_graph = graph


class ProcessRequest(BaseModel):
    """Request for processing message (Kafka event format)"""

    event_id: str
    event_type: str
    timestamp: str
    service: str
    version: str
    payload: Dict[str, Any]
    metadata: Optional[Dict[str, Any]] = None


class ProcessResponse(BaseModel):
    """Response for processing message"""

    success: bool
    event_id: str
    generated_response: Optional[str] = None
    agent_state: Optional[Dict[str, Any]] = None
    error: Optional[str] = None


@router.post("/process", response_model=ProcessResponse)
async def process_message(request: ProcessRequest):
    """Process message (for testing)"""
    if agent_graph is None:
        raise HTTPException(
            status_code=503, detail="Agent graph not initialized"
        )

    try:
        # Parse payload
        payload = request.payload
        metadata = payload.get("metadata", {})

        # Extract session_id from dialogue_context if available
        dialogue_context = metadata.get("dialogue_context", {})
        session_id = dialogue_context.get("session_id", "")

        # Convert timestamp
        try:
            message_timestamp = datetime.fromisoformat(
                payload["timestamp"].replace("Z", "+00:00")
            )
        except Exception:
            message_timestamp = datetime.utcnow()

        if not payload.get("question_id"):
            return ProcessResponse(
                success=False,
                event_id=request.event_id,
                generated_response=None,
                agent_state={},
                error="Missing question_id in payload",
            )

        # Initialize state
        initial_state: AgentState = {
            "chat_id": payload["chat_id"],
            "session_id": session_id,
            "user_id": metadata.get("user_id", ""),
            "message_id": payload["message_id"],
            "user_message": payload["content"],
            "message_timestamp": message_timestamp,
            "question_id": payload["question_id"],
            "dialogue_history": [],
            "dialogue_state": None,
            "interview_program": None,
            "total_questions": 0,
            "current_question_index": None,
            "current_question_id": payload["question_id"],
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

        # Run graph
        final_state = await agent_graph.ainvoke(initial_state)

        # Prepare response
        return ProcessResponse(
            success=True,
            event_id=request.event_id,
            generated_response=final_state.get("generated_response"),
            agent_state={
                "current_question_index": final_state.get("current_question_index"),
                "agent_decision": final_state.get("agent_decision"),
                "answer_evaluation": final_state.get("answer_evaluation"),
                "processing_steps": final_state.get("processing_steps"),
            },
            error=final_state.get("error"),
        )

    except Exception as e:
        logger.error("Failed to process message", error=str(e), exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))
