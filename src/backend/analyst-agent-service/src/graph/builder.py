"""LangGraph state machine for the analyst ReAct agent (§8 stage 4, §9 p.2.7).

Architecture: deterministic data fetch → ReAct agent loop → save.

    fetch_data          (deterministic: chat transcript + program from CRUD)
      → agent_init      (build system/user prompt, seed messages)
      → call_agent_llm  ┐ loop: LLM decides next tool_call OR terminates
      → execute_tools   ┘       (terminal tool = emit_report)
      → agent_cleanup   (strip transient fields)
      → save_results    (persist report + presets + progress)
      → END

The previous `generate_report → validate_report → retry` pipeline is removed:
the LLM now drives section generation, validation, and retry through the
`TOOL_DEFINITIONS` published in `tools.analyst_tools`.
"""

from langgraph.graph import END, StateGraph

from ..graph.nodes import (
    fetch_data,
    save_results,
    set_analyst_service,
)
from ..graph.agent_nodes import (
    agent_init,
    call_agent_llm,
    execute_tools,
    agent_cleanup,
    agent_route,
    set_analyst_service_ref,
)
from ..graph.state import AnalystState
from ..logger import get_logger

logger = get_logger(__name__)


def create_analyst_graph(analyst_service) -> "CompiledGraph":  # type: ignore[name-defined]
    """Build and compile the analyst ReAct agent graph."""
    set_analyst_service(analyst_service)
    set_analyst_service_ref(analyst_service)

    from ..tools.analyst_tools import set_analyst_tool_clients
    set_analyst_tool_clients(
        results_client=analyst_service.results_client,
        chat_client=analyst_service.chat_client,
        session_client=analyst_service.session_client,
        llm=analyst_service.llm,
    )

    workflow = StateGraph(AnalystState)

    # Deterministic preprocessing
    workflow.add_node("fetch_data", fetch_data)

    # Agent loop
    workflow.add_node("agent_init", agent_init)
    workflow.add_node("call_agent_llm", call_agent_llm)
    workflow.add_node("execute_tools", execute_tools)
    workflow.add_node("agent_cleanup", agent_cleanup)

    # Deterministic persistence
    workflow.add_node("save_results", save_results)

    workflow.set_entry_point("fetch_data")
    workflow.add_edge("fetch_data", "agent_init")
    workflow.add_edge("agent_init", "call_agent_llm")

    workflow.add_conditional_edges(
        "call_agent_llm",
        agent_route,
        {"continue": "execute_tools", "save": "agent_cleanup"},
    )
    workflow.add_conditional_edges(
        "execute_tools",
        agent_route,
        {"continue": "call_agent_llm", "save": "agent_cleanup"},
    )

    workflow.add_edge("agent_cleanup", "save_results")
    workflow.add_edge("save_results", END)

    graph = workflow.compile()
    logger.info("Analyst ReAct agent graph compiled (§8 stage 4)")
    return graph
