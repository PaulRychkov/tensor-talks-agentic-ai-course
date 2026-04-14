"""LLM client for knowledge ingestion pipeline (§9 p.5.6)."""

from openai import AsyncOpenAI

from ..config import settings
from ..logger.setup import get_logger

logger = get_logger(__name__)


class LLMClient:
    """Thin AsyncOpenAI wrapper providing the generate(prompt)->str interface
    expected by PipelineRunner."""

    def __init__(self) -> None:
        api_key = settings.llm_api_key or "not-needed"
        self._client = AsyncOpenAI(
            api_key=api_key,
            base_url=settings.llm_base_url or None,
            timeout=settings.llm_timeout,
        )
        logger.info(
            "Knowledge-producer LLM client initialized",
            model=settings.llm_model,
            base_url=settings.llm_base_url or "default",
        )

    async def generate(self, prompt: str) -> str:
        """Generate a text completion from a plain-text prompt."""
        is_gpt5 = "gpt-5" in settings.llm_model.lower()
        params: dict = {
            "model": settings.llm_model,
            "messages": [{"role": "user", "content": prompt}],
        }
        if not is_gpt5:
            params["temperature"] = settings.llm_temperature
            params["max_tokens"] = settings.llm_max_tokens
        else:
            params["max_completion_tokens"] = settings.llm_max_tokens

        response = await self._client.chat.completions.create(**params)
        return response.choices[0].message.content or ""
