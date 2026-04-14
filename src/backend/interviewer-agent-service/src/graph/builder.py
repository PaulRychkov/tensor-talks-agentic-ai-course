"""LangGraph builder for the interviewer agent (§8 stage 3).

Architecture: deterministic preprocessing → ReAct agent loop → publish.

    receive_message
      → check_pii            [may short-circuit to publish on PII block]
      → load_context
      → check_off_topic
      → determine_question_index
      → agent_init           ← builds system/user prompt, resets iteration state
      → call_agent_llm       ┐ loop: LLM decides tool_call OR final message
      → execute_tools        ┘
      → finalize_response    ← sanity checks, advance question, metrics
      → publish_response
      → END

The hardcoded `evaluate_answer` → `make_decision` → `generate_response` pipeline
is removed: the LLM now drives those decisions via the emit_response tool
and `TOOL_DEFINITIONS` exposed in `tools.interviewer_tools`.
"""

from langgraph.graph import StateGraph, END

from ..graph.state import AgentState
from ..graph.nodes import (
    receive_message,
    check_pii,
    load_context,
    check_off_topic,
    determine_question_index,
    publish_response,
    set_global_clients,
)
from ..graph.agent_nodes import (
    agent_init,
    call_agent_llm,
    execute_tools,
    finalize_response,
    agent_route,
)
from ..tools import set_interviewer_tool_clients
from ..llm import LLMClient
from ..clients import SessionServiceClient, RedisClient, ChatCrudClient
from ..kafka import KafkaProducer
from ..logger import get_logger

logger = get_logger(__name__)


def create_agent_graph(
    llm_client: LLMClient,
    session_client: SessionServiceClient,
    redis_client: RedisClient,
    kafka_producer: KafkaProducer,
    chat_crud_client: ChatCrudClient = None,
) -> StateGraph:
    """Create and compile the interviewer ReAct agent graph."""
    set_global_clients(llm_client, session_client, redis_client, kafka_producer, chat_crud=chat_crud_client)
    set_interviewer_tool_clients(llm_client, session_client, redis_client)

    workflow = StateGraph(AgentState)

    # Deterministic preprocessing nodes
    workflow.add_node("receive_message", receive_message)
    workflow.add_node("check_pii", check_pii)
    workflow.add_node("load_context", load_context)
    workflow.add_node("check_off_topic", check_off_topic)
    workflow.add_node("determine_question_index", determine_question_index)

    # Agent loop nodes (§8 stage 3)
    workflow.add_node("agent_init", agent_init)
    workflow.add_node("call_agent_llm", call_agent_llm)
    workflow.add_node("execute_tools", execute_tools)
    workflow.add_node("finalize_response", finalize_response)

    workflow.add_node("publish_response", publish_response)

    workflow.set_entry_point("receive_message")
    workflow.add_edge("receive_message", "check_pii")

    # Short-circuit to publish if PII blocked the message.
    workflow.add_conditional_edges(
        "check_pii",
        lambda state: "publish_response" if state.get("agent_decision") == "blocked_pii" else "load_context",
        {"publish_response": "publish_response", "load_context": "load_context"},
    )

    workflow.add_edge("load_context", "check_off_topic")
    workflow.add_edge("check_off_topic", "determine_question_index")
    workflow.add_edge("determine_question_index", "agent_init")

    # ReAct loop: agent_init → call_agent_llm → execute_tools → call_agent_llm → ...
    workflow.add_edge("agent_init", "call_agent_llm")

    # After the LLM call, either continue to tool execution (there were tool_calls,
    # and none of them was the terminal emit_response) or finalize.
    workflow.add_conditional_edges(
        "call_agent_llm",
        agent_route,
        {"continue": "execute_tools", "finalize": "finalize_response"},
    )
    workflow.add_conditional_edges(
        "execute_tools",
        agent_route,
        {"continue": "call_agent_llm", "finalize": "finalize_response"},
    )

    workflow.add_edge("finalize_response", "publish_response")
    workflow.add_edge("publish_response", END)

    graph = workflow.compile()
    logger.info("Interviewer ReAct agent graph compiled (§8 stage 3)")
    return graph
