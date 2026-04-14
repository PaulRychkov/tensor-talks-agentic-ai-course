"""LLM client for interview planner agent (§9 p.1, §8 stage 2)."""

from __future__ import annotations

from openai import AsyncOpenAI

from ..config import settings
from ..logger import get_logger

logger = get_logger(__name__)

_instance: LLMClient | None = None


class LLMClient:
    """AsyncOpenAI wrapper with tool-calling support for the planner agent."""

    def __init__(self) -> None:
        api_key = settings.llm_api_key or "not-needed"
        self._client = AsyncOpenAI(
            api_key=api_key,
            base_url=settings.llm_base_url or None,
            timeout=settings.llm_timeout,
        )
        logger.info(
            "Planner LLM client initialised",
            model=settings.llm_model,
            base_url=settings.llm_base_url or "default",
        )

    async def chat_with_tools(
        self,
        messages: list,
        tools: list,
    ) -> dict:
        """Call the LLM with tool definitions (OpenAI function-calling).

        Returns the raw assistant message as a plain dict so it can be
        appended directly to the messages list in PlannerState.
        """
        is_gpt5 = "gpt-5" in settings.llm_model.lower()
        params: dict = {
            "model": settings.llm_model,
            "messages": messages,
            "tools": tools,
            "tool_choice": "auto",
        }
        if not is_gpt5:
            params["temperature"] = settings.llm_temperature
            params["max_tokens"] = settings.llm_max_tokens
        else:
            params["max_completion_tokens"] = settings.llm_max_tokens

        resp = await self._client.chat.completions.create(**params)
        msg = resp.choices[0].message

        return {
            "role": "assistant",
            "content": msg.content or "",
            "tool_calls": [
                {
                    "id": tc.id,
                    "type": "function",
                    "function": {
                        "name": tc.function.name,
                        "arguments": tc.function.arguments,
                    },
                }
                for tc in (msg.tool_calls or [])
            ],
        }

    async def chat_plain(self, messages: list) -> str:
        """Call the LLM without tools and return the assistant text content."""
        is_gpt5 = "gpt-5" in settings.llm_model.lower()
        params: dict = {
            "model": settings.llm_model,
            "messages": messages,
        }
        if not is_gpt5:
            params["temperature"] = settings.llm_temperature
            params["max_tokens"] = settings.llm_max_tokens
        else:
            params["max_completion_tokens"] = settings.llm_max_tokens

        resp = await self._client.chat.completions.create(**params)
        return resp.choices[0].message.content or ""


def get_llm_client() -> LLMClient | None:
    """Return a singleton LLMClient if LLM_API_KEY is configured."""
    global _instance
    if _instance is None and settings.llm_api_key:
        _instance = LLMClient()
    return _instance
