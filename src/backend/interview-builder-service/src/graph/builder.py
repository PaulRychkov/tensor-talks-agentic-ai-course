"""LangGraph state machine for the interview planner agent (§9 p.1, §8 stage 2).

Graph topology:
  START → initialize → call_llm → [tool_calls?] → execute_tools → call_llm (loop)
                                → [no calls]    → assemble → END
"""

from langgraph.graph import END, StateGraph

from ..graph.nodes import (
    assemble_program,
    call_planner_llm,
    execute_tool_calls,
    initialize_planner,
    set_planner_clients,
)
from ..graph.state import PlannerState
from ..logger import get_logger

logger = get_logger(__name__)


def _route_after_llm(state: PlannerState) -> str:
    """Conditional edge: delegate to tools or move to final assembly."""
    if state.get("error"):
        return "assemble"  # surface the error gracefully
    last_msg = state["messages"][-1] if state.get("messages") else {}
    if last_msg.get("tool_calls"):
        return "execute_tools"
    return "assemble"


def create_planner_graph(llm_client) -> "CompiledGraph":  # type: ignore[name-defined]
    """Build and compile the planner LangGraph.

    Args:
        llm_client: An LLMClient instance (or None – graph still compiles but
                    will fall back when executed without a key).
    """
    set_planner_clients(llm_client)

    workflow = StateGraph(PlannerState)

    workflow.add_node("initialize", initialize_planner)
    workflow.add_node("call_llm", call_planner_llm)
    workflow.add_node("execute_tools", execute_tool_calls)
    workflow.add_node("assemble", assemble_program)

    workflow.set_entry_point("initialize")
    workflow.add_edge("initialize", "call_llm")
    workflow.add_conditional_edges(
        "call_llm",
        _route_after_llm,
        {"execute_tools": "execute_tools", "assemble": "assemble"},
    )
    workflow.add_edge("execute_tools", "call_llm")
    workflow.add_edge("assemble", END)

    graph = workflow.compile()
    logger.info("Planner agent graph compiled")
    return graph
