"""LLM client for OpenAI and local models"""

import time
from openai import AsyncOpenAI
from typing import Optional, Literal
from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector

logger = get_logger(__name__)


class LLMClient:
    """Universal LLM client with support for OpenAI and local models"""

    def __init__(self, config=None):
        self.config = config or settings
        self.client = None
        self.metrics = get_metrics_collector()
        self._initialize_client()

    def _initialize_client(self):
        """Initialize client based on provider"""
        if self.config.llm_provider == "openai":
            self.client = AsyncOpenAI(
                api_key=self.config.llm_api_key,
                base_url=self.config.llm_base_url or None,
                timeout=self.config.llm_timeout,
            )
            logger.info(
                "LLM client initialized (OpenAI)",
                model=self.config.llm_model,
                base_url=self.config.llm_base_url or "default",
            )
        elif self.config.llm_provider == "local":
            # For local models (e.g., Ollama, vLLM) or proxy APIs
            # Use API key if provided, otherwise use "not-needed" for truly local models
            api_key = self.config.llm_api_key or "not-needed"
            self.client = AsyncOpenAI(
                base_url=self.config.llm_base_url or "http://localhost:11434/v1",
                api_key=api_key,
                timeout=self.config.llm_timeout,
            )
            logger.info(
                "LLM client initialized (local/proxy)",
                model=self.config.llm_model,
                base_url=self.config.llm_base_url,
                has_api_key=bool(self.config.llm_api_key),
            )
        else:
            raise ValueError(f"Unsupported LLM provider: {self.config.llm_provider}")

    async def generate(
        self,
        prompt: str,
        temperature: Optional[float] = None,
        response_format: Optional[Literal["json", "text"]] = "text",
        max_retries: int = 3,
    ) -> str:
        """Generate response from LLM with retry logic"""
        temperature = temperature or self.config.llm_temperature
        is_gpt5 = "gpt-5" in self.config.llm_model.lower()
        # Proxy for gpt-5-mini only supports default temperature (=1); omit otherwise.
        if is_gpt5 and temperature != 1:
            temperature = None

        # Note: Some models don't support response_format, so we'll parse JSON from text if needed
        # Disabled response_format for now - model doesn't support it
        response_format_param = None

        last_error = None
        for attempt in range(max_retries):
            with self.metrics.llm_call_duration.time():
                try:
                    # Build request parameters
                    base_params = {
                        "model": self.config.llm_model,
                        "messages": [{"role": "user", "content": prompt}],
                    }
                    if temperature is not None:
                        base_params["temperature"] = temperature
                    # Only add response_format if it's not None (model supports it)
                    if response_format_param is not None:
                        base_params["response_format"] = response_format_param

                    # Try token params in order based on model family
                    token_param_order = ["max_completion_tokens", "max_tokens"] if is_gpt5 else ["max_tokens", "max_completion_tokens"]

                    response = None
                    last_token_error = None
                    for token_param in token_param_order:
                        request_params = dict(base_params)
                        request_params[token_param] = self.config.llm_max_tokens
                        try:
                            call_start = time.perf_counter()
                            response = await self.client.chat.completions.create(**request_params)
                            call_duration_ms = round((time.perf_counter() - call_start) * 1000, 2)
                            break
                        except Exception as e:
                            msg = str(e)
                            last_token_error = e
                            call_duration_ms = round((time.perf_counter() - call_start) * 1000, 2)
                            # Only retry if error indicates unsupported token param
                            if (
                                ("max_completion_tokens" in token_param and "max_completion_tokens" in msg) or
                                ("max_tokens" in token_param and "max_tokens" in msg and "not supported" in msg)
                            ):
                                continue
                            raise

                    if response is None and last_token_error is not None:
                        raise last_token_error

                    content = response.choices[0].message.content
                    if not content or not content.strip():
                        raise ValueError("Empty response from LLM")
                    
                    self.metrics.llm_calls_total.labels(
                        provider=self.config.llm_provider,
                        model=self.config.llm_model,
                        status="success",
                    ).inc()

                    logger.debug(
                        "LLM response generated",
                        model=self.config.llm_model,
                        tokens_used=response.usage.total_tokens if response.usage else None,
                        attempt=attempt + 1,
                    )
                    logger.info(
                        "LLM call completed",
                        model=self.config.llm_model,
                        attempt=attempt + 1,
                        duration_ms=call_duration_ms,
                    )

                    return content

                except Exception as e:
                    last_error = e
                    self.metrics.llm_calls_total.labels(
                        provider=self.config.llm_provider,
                        model=self.config.llm_model,
                        status="error",
                    ).inc()
                    logger.warning(
                        "LLM call failed, retrying",
                        error=str(e),
                        attempt=attempt + 1,
                        max_retries=max_retries,
                    )
                    if attempt < max_retries - 1:
                        # Wait before retry (exponential backoff)
                        import asyncio
                        await asyncio.sleep(2 ** attempt)
                        continue
                    else:
                        # Last attempt failed
                        self.metrics.error_count.labels(
                            error_type="llm_call_error", service=settings.service_name
                        ).inc()
                        logger.error("LLM call failed after all retries", error=str(e), exc_info=True)
                        raise last_error
