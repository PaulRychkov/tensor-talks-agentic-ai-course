"""Tests for analyst ReAct agent nodes (agent_nodes.py)."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from src.graph.agent_nodes import (
    agent_init,
    call_agent_llm,
    execute_tools,
    agent_cleanup,
    agent_route,
    set_analyst_service_ref,
    MAX_AGENT_ITERATIONS,
)


def _base_state(**overrides) -> dict:
    state = {
        "session_id": "sess-1",
        "session_kind": "interview",
        "user_id": "user-1",
        "chat_id": "chat-1",
        "topics": ["python", "ml"],
        "level": "middle",
        "terminated_early": False,
        "answered_questions": 5,
        "total_questions": 10,
        "chat_messages": [{"role": "user", "content": "hello"}],
        "program": None,
        "evaluations": [],
        "errors_by_topic": {},
        "report": None,
        "validation_result": None,
        "validation_attempts": 0,
        "max_validation_attempts": 3,
        "presets": None,
        "topic_progress": None,
        "error": None,
    }
    state.update(overrides)
    return state


# ===================================================================
# agent_init
# ===================================================================

class TestAgentInit:

    @pytest.mark.asyncio
    async def test_builds_messages(self):
        state = _base_state()
        result = await agent_init(state)
        assert "_agent_messages" in result
        assert len(result["_agent_messages"]) == 2
        assert result["_agent_messages"][0]["role"] == "system"
        assert result["_agent_messages"][1]["role"] == "user"
        assert result["_agent_iterations"] == 0
        assert result["_skip_agent_loop"] is False

    @pytest.mark.asyncio
    async def test_skip_on_no_messages_error(self):
        state = _base_state(error="no_messages")
        result = await agent_init(state)
        assert result["_skip_agent_loop"] is True

    @pytest.mark.asyncio
    async def test_system_prompt_contains_session_kind(self):
        state = _base_state(session_kind="study")
        result = await agent_init(state)
        system = result["_agent_messages"][0]["content"]
        assert "study" in system


# ===================================================================
# agent_route
# ===================================================================

class TestAgentRoute:

    def test_returns_save_when_skip(self):
        assert agent_route({"_skip_agent_loop": True}) == "save"

    def test_returns_continue_when_active(self):
        assert agent_route({"_skip_agent_loop": False}) == "continue"

    def test_returns_continue_when_missing(self):
        assert agent_route({}) == "continue"


# ===================================================================
# call_agent_llm
# ===================================================================

class TestCallAgentLlm:

    @pytest.mark.asyncio
    async def test_skips_when_flagged(self):
        state = _base_state(_skip_agent_loop=True, _agent_messages=[], _agent_iterations=0)
        result = await call_agent_llm(state)
        assert result.get("_skip_agent_loop") is True

    @pytest.mark.asyncio
    async def test_errors_if_no_service(self):
        set_analyst_service_ref(None)
        state = _base_state(_skip_agent_loop=False, _agent_messages=[
            {"role": "system", "content": "sys"},
            {"role": "user", "content": "go"},
        ], _agent_iterations=0)
        result = await call_agent_llm(state)
        assert result["error"] == "analyst_service not injected"
        assert result["_skip_agent_loop"] is True

    @pytest.mark.asyncio
    async def test_max_iterations_stops_loop(self):
        set_analyst_service_ref(MagicMock())
        state = _base_state(
            _skip_agent_loop=False,
            _agent_messages=[],
            _agent_iterations=MAX_AGENT_ITERATIONS,
        )
        result = await call_agent_llm(state)
        assert result["_skip_agent_loop"] is True
        assert result.get("error") == "max_iterations_exceeded"

    @pytest.mark.asyncio
    async def test_successful_call_with_tool_calls(self):
        mock_svc = AsyncMock()
        mock_svc.chat_with_tools = AsyncMock(return_value={
            "role": "assistant",
            "content": "",
            "tool_calls": [
                {"id": "tc1", "type": "function", "function": {"name": "get_evaluations", "arguments": "{}"}}
            ],
        })
        set_analyst_service_ref(mock_svc)
        state = _base_state(
            _skip_agent_loop=False,
            _agent_messages=[
                {"role": "system", "content": "sys"},
                {"role": "user", "content": "go"},
            ],
            _agent_iterations=0,
        )
        result = await call_agent_llm(state)
        assert result["_agent_iterations"] == 1
        assert result.get("_skip_agent_loop") is not True
        assert len(result["_agent_messages"]) == 3  # system + user + assistant

    @pytest.mark.asyncio
    async def test_no_tool_calls_ends_loop(self):
        mock_svc = AsyncMock()
        mock_svc.chat_with_tools = AsyncMock(return_value={
            "role": "assistant",
            "content": "Done.",
            "tool_calls": [],
        })
        set_analyst_service_ref(mock_svc)
        state = _base_state(
            _skip_agent_loop=False,
            _agent_messages=[
                {"role": "system", "content": "sys"},
                {"role": "user", "content": "go"},
            ],
            _agent_iterations=0,
        )
        result = await call_agent_llm(state)
        assert result["_skip_agent_loop"] is True


# ===================================================================
# execute_tools
# ===================================================================

class TestExecuteTools:

    @pytest.mark.asyncio
    async def test_skip_when_flagged(self):
        state = _base_state(_skip_agent_loop=True, _agent_messages=[])
        result = await execute_tools(state)
        assert result.get("_skip_agent_loop") is True

    @pytest.mark.asyncio
    async def test_emit_report_sets_report(self):
        report_data = {"summary": "Good", "score": 75}
        state = _base_state(
            _skip_agent_loop=False,
            _agent_messages=[
                {"role": "assistant", "content": "", "tool_calls": [
                    {"id": "tc1", "type": "function", "function": {
                        "name": "emit_report",
                        "arguments": json.dumps({"report": report_data}),
                    }}
                ]},
            ],
        )
        result = await execute_tools(state)
        assert result["report"] == report_data
        assert result["_skip_agent_loop"] is True

    @pytest.mark.asyncio
    async def test_unknown_tool_returns_error(self):
        state = _base_state(
            _skip_agent_loop=False,
            _agent_messages=[
                {"role": "assistant", "content": "", "tool_calls": [
                    {"id": "tc1", "type": "function", "function": {
                        "name": "nonexistent_tool",
                        "arguments": "{}",
                    }}
                ]},
            ],
        )
        result = await execute_tools(state)
        last_tool_msg = result["_agent_messages"][-1]
        assert "unknown tool" in last_tool_msg["content"]


# ===================================================================
# agent_cleanup
# ===================================================================

class TestAgentCleanup:

    @pytest.mark.asyncio
    async def test_removes_transient_fields(self):
        state = _base_state(
            _agent_messages=[{"role": "system", "content": "x"}],
            _agent_iterations=5,
            _skip_agent_loop=True,
        )
        result = await agent_cleanup(state)
        assert "_agent_messages" not in result
        assert "_agent_iterations" not in result
        assert "_skip_agent_loop" not in result
        assert result["session_id"] == "sess-1"
