"""LangGraph builder for agent state machine"""

from langgraph.graph import StateGraph, END
from typing import Callable

from ..graph.state import AgentState
from ..graph.nodes import (
    receive_message,
    load_context,
    check_off_topic,
    determine_question_index,
    evaluate_answer,
    make_decision,
    generate_response,
    publish_response,
    set_global_clients,
)
from ..llm import LLMClient
from ..clients import SessionServiceClient, RedisClient
from ..kafka import KafkaProducer
from ..logger import get_logger

logger = get_logger(__name__)


def create_agent_graph(
    llm_client: LLMClient,
    session_client: SessionServiceClient,
    redis_client: RedisClient,
    kafka_producer: KafkaProducer,
) -> StateGraph:
    """Create and compile agent state graph"""
    # Set global clients for nodes
    set_global_clients(llm_client, session_client, redis_client, kafka_producer)

    # Create state graph
    workflow = StateGraph(AgentState)

    # Add nodes
    workflow.add_node("receive_message", receive_message)
    workflow.add_node("load_context", load_context)
    workflow.add_node("check_off_topic", check_off_topic)
    workflow.add_node("determine_question_index", determine_question_index)
    workflow.add_node("evaluate_answer", evaluate_answer)
    workflow.add_node("make_decision", make_decision)
    workflow.add_node("generate_response", generate_response)
    workflow.add_node("publish_response", publish_response)

    # Define flow
    workflow.set_entry_point("receive_message")

    workflow.add_edge("receive_message", "load_context")
    workflow.add_edge("load_context", "check_off_topic")
    
    # Always go to determine_question_index, but skip evaluation if off-topic
    workflow.add_edge("check_off_topic", "determine_question_index")
    
    # Sequential edges
    workflow.add_edge("determine_question_index", "evaluate_answer")
    workflow.add_edge("evaluate_answer", "make_decision")
    workflow.add_edge("make_decision", "generate_response")
    workflow.add_edge("generate_response", "publish_response")
    workflow.add_edge("publish_response", END)

    # Compile graph
    graph = workflow.compile()
    logger.info("Agent graph created and compiled")

    return graph
