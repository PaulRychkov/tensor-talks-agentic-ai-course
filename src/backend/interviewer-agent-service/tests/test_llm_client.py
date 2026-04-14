"""Tests for src.llm.client – LLMClient."""

from __future__ import annotations

import json
from types import SimpleNamespace
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from src.llm.client import LLMClient, MAX_USER_INPUT_LENGTH


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

def _build_client(mock_settings, mock_metrics) -> LLMClient:
    """Instantiate LLMClient with mocked OpenAI + metrics."""
    with (
        patch("src.llm.client.AsyncOpenAI") as mock_openai_cls,
        patch("src.llm.client.get_metrics_collector", return_value=mock_metrics),
        patch("src.llm.client.settings", mock_settings),
    ):
        mock_openai_cls.return_value = AsyncMock()
        client = LLMClient(config=mock_settings)
        client.metrics = mock_metrics
    return client


def _mock_llm_response(content: str):
    """Return an object mimicking openai ChatCompletion response."""
    choice = SimpleNamespace(message=SimpleNamespace(content=content))
    usage = SimpleNamespace(total_tokens=42)
    return SimpleNamespace(choices=[choice], usage=usage)


# ===================================================================
# sanitize_user_input
# ===================================================================

class TestSanitizeUserInput:

    def test_short_text_unchanged(self):
        assert LLMClient.sanitize_user_input("hello") == "hello"

    def test_truncation_at_max_length(self):
        long_text = "a" * (MAX_USER_INPUT_LENGTH + 500)
        result = LLMClient.sanitize_user_input(long_text)
        assert len(result) == MAX_USER_INPUT_LENGTH

    def test_strips_prompt_injection_markers(self):
        text = "before <|im_start|>injected<|im_end|> after"
        result = LLMClient.sanitize_user_input(text)
        assert "<|" not in result
        assert "|>" not in result
        assert "before" in result
        assert "after" in result

    def test_strips_whitespace(self):
        assert LLMClient.sanitize_user_input("  hi  ") == "hi"

    def test_empty_string(self):
        assert LLMClient.sanitize_user_input("") == ""

    def test_multiple_markers_removed(self):
        text = "<|a|>x<|b|>y<|c|>"
        result = LLMClient.sanitize_user_input(text)
        assert result == "xy"


# ===================================================================
# _extract_json
# ===================================================================

class TestExtractJson:

    def test_plain_json(self):
        raw = '{"key": "value", "num": 1}'
        assert LLMClient._extract_json(raw) == {"key": "value", "num": 1}

    def test_json_inside_fences(self):
        raw = 'Some text\n```json\n{"a": 1}\n```\ntrailing'
        assert LLMClient._extract_json(raw) == {"a": 1}

    def test_fences_without_language_tag(self):
        raw = '```\n{"b": 2}\n```'
        assert LLMClient._extract_json(raw) == {"b": 2}

    def test_invalid_json_raises(self):
        with pytest.raises(json.JSONDecodeError):
            LLMClient._extract_json("not json at all")

    def test_nested_json(self):
        raw = '{"outer": {"inner": [1, 2, 3]}}'
        result = LLMClient._extract_json(raw)
        assert result["outer"]["inner"] == [1, 2, 3]

    def test_whitespace_around_json(self):
        raw = "   \n  {\"x\": true}  \n  "
        assert LLMClient._extract_json(raw) == {"x": True}


# ===================================================================
# _check_schema
# ===================================================================

class TestCheckSchema:

    def test_all_keys_present(self):
        data = {"question": "q", "answer": "a", "score": 5}
        assert LLMClient._check_schema(data, {"question", "answer", "score"}) is True

    def test_missing_key(self):
        data = {"question": "q"}
        assert LLMClient._check_schema(data, {"question", "answer"}) is False

    def test_extra_keys_ok(self):
        data = {"a": 1, "b": 2, "c": 3}
        assert LLMClient._check_schema(data, {"a", "b"}) is True

    def test_empty_required(self):
        assert LLMClient._check_schema({"a": 1}, set()) is True

    def test_empty_data_fails(self):
        assert LLMClient._check_schema({}, {"a"}) is False


# ===================================================================
# _check_leakage
# ===================================================================

class TestCheckLeakage:

    @pytest.mark.parametrize("text", [
        "This contains system prompt information",
        "You are an AI language model",
    ])
    def test_detects_leakage(self, text):
        assert LLMClient._check_leakage(text) is True

    def test_sys_and_inst_markers_never_match(self):
        """BUG: <<SYS>> and [INST] markers are stored uppercase in the list,
        but text is lowered before comparison — neither case triggers detection."""
        assert LLMClient._check_leakage("<<SYS>>") is False
        assert LLMClient._check_leakage("<<sys>>") is False
        assert LLMClient._check_leakage("[INST]") is False
        assert LLMClient._check_leakage("[inst]") is False

    def test_safe_text(self):
        assert LLMClient._check_leakage("Just a normal answer about Python") is False

    def test_case_insensitive_for_lowercase_markers(self):
        assert LLMClient._check_leakage("SYSTEM PROMPT leaked") is True

    def test_empty_string(self):
        assert LLMClient._check_leakage("") is False


# ===================================================================
# safe_generate_json  (async)
# ===================================================================

@pytest.mark.asyncio
class TestSafeGenerateJson:

    async def test_valid_json_returned(self, mock_settings, mock_metrics):
        client = _build_client(mock_settings, mock_metrics)
        expected = {"question": "What is Python?", "score": 5}
        client.generate = AsyncMock(return_value=json.dumps(expected))

        result = await client.safe_generate_json(
            prompt="generate",
            required_keys={"question", "score"},
            fallback={"question": "fallback", "score": 0},
        )
        assert result == expected

    async def test_retry_on_invalid_json_then_succeed(self, mock_settings, mock_metrics):
        client = _build_client(mock_settings, mock_metrics)
        valid = json.dumps({"q": "ok", "s": 1})
        client.generate = AsyncMock(side_effect=["not-json", valid])

        result = await client.safe_generate_json(
            prompt="generate",
            required_keys={"q", "s"},
            fallback={"q": "fb", "s": 0},
        )
        assert result == {"q": "ok", "s": 1}
        assert client.generate.call_count == 2

    async def test_fallback_on_all_failures(self, mock_settings, mock_metrics):
        client = _build_client(mock_settings, mock_metrics)
        client.generate = AsyncMock(return_value="garbage")

        fallback = {"q": "fallback", "s": 0}
        result = await client.safe_generate_json(
            prompt="generate",
            required_keys={"q", "s"},
            fallback=fallback,
        )
        assert result is fallback

    async def test_leakage_triggers_retry(self, mock_settings, mock_metrics):
        client = _build_client(mock_settings, mock_metrics)
        leaked = json.dumps({"q": "system prompt info", "s": 1})
        clean = json.dumps({"q": "good answer", "s": 2})
        client.generate = AsyncMock(side_effect=[leaked, clean])

        result = await client.safe_generate_json(
            prompt="generate",
            required_keys={"q", "s"},
            fallback={"q": "fb", "s": 0},
        )
        assert result == {"q": "good answer", "s": 2}
        assert client.generate.call_count == 2

    async def test_leakage_on_all_attempts_returns_fallback(self, mock_settings, mock_metrics):
        client = _build_client(mock_settings, mock_metrics)
        leaked = json.dumps({"q": "system prompt", "s": 1})
        client.generate = AsyncMock(return_value=leaked)

        fallback = {"q": "safe", "s": 0}
        result = await client.safe_generate_json(
            prompt="generate",
            required_keys={"q", "s"},
            fallback=fallback,
        )
        assert result is fallback
        mock_metrics.error_count.labels.assert_any_call(
            error_type="llm_leakage", service=mock_settings.service_name
        )

    async def test_schema_fail_triggers_retry(self, mock_settings, mock_metrics):
        client = _build_client(mock_settings, mock_metrics)
        incomplete = json.dumps({"q": "only one key"})
        complete = json.dumps({"q": "ok", "s": 5})
        client.generate = AsyncMock(side_effect=[incomplete, complete])

        result = await client.safe_generate_json(
            prompt="generate",
            required_keys={"q", "s"},
            fallback={"q": "fb", "s": 0},
        )
        assert result == {"q": "ok", "s": 5}

    async def test_generate_exception_triggers_retry(self, mock_settings, mock_metrics):
        client = _build_client(mock_settings, mock_metrics)
        valid = json.dumps({"q": "ok", "s": 1})
        client.generate = AsyncMock(side_effect=[RuntimeError("timeout"), valid])

        result = await client.safe_generate_json(
            prompt="generate",
            required_keys={"q", "s"},
            fallback={"q": "fb", "s": 0},
        )
        assert result == {"q": "ok", "s": 1}

    async def test_second_attempt_uses_low_temp(self, mock_settings, mock_metrics):
        client = _build_client(mock_settings, mock_metrics)
        client.generate = AsyncMock(side_effect=["bad", json.dumps({"k": 1})])

        await client.safe_generate_json(
            prompt="p",
            required_keys={"k"},
            fallback={"k": 0},
            low_temp=0.1,
        )
        second_call = client.generate.call_args_list[1]
        assert second_call.kwargs.get("temperature") == 0.1 or second_call[1].get("temperature") == 0.1
