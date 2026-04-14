"""LLM client for OpenAI and local models"""

import json
import re
import time
from openai import AsyncOpenAI
from typing import Any, Dict, Optional, Literal, Set, Type, TypeVar
from pydantic import BaseModel, ValidationError
from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector

T = TypeVar("T", bound=BaseModel)

MAX_USER_INPUT_LENGTH = 4000

# PII masking patterns (email, phone, credit card, long digit sequences)
_PII_PATTERNS = [
    (re.compile(r"[\w\.+-]+@[\w\.-]+\.\w+"), "[EMAIL]"),
    (re.compile(r"\+?\d[\d\-\s\(\)]{8,}\d"), "[PHONE]"),
    (re.compile(r"\b(?:\d[ -]?){13,19}\b"), "[CARD]"),
]

logger = get_logger(__name__)


class LLMClient:
    """Universal LLM client with support for OpenAI and local models"""

    def __init__(self, config=None):
        self.config = config or settings
        self.client = None
        self.metrics = get_metrics_collector()
        # Runtime capability flags — flipped off if the model/proxy rejects them.
        self._supports_json_object = True
        self._supports_json_schema = True
        self._initialize_client()

    @staticmethod
    def _is_response_format_unsupported(exc: Exception) -> bool:
        msg = str(exc).lower()
        return (
            "response_format" in msg
            and any(m in msg for m in ("not support", "unsupported", "invalid", "unknown"))
        )

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

        # Enable response_format={"type":"json_object"} when the caller asked for JSON
        # and the model hasn't rejected it in a previous call (_supports_json_object).
        response_format_param = None
        if response_format == "json" and self._supports_json_object:
            response_format_param = {"type": "json_object"}

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
                    # If the proxy/model rejected response_format, disable it and let
                    # the outer retry loop re-attempt without the param.
                    if response_format_param is not None and self._is_response_format_unsupported(e):
                        logger.warning(
                            "response_format unsupported by model, disabling and retrying",
                            model=self.config.llm_model,
                        )
                        self._supports_json_object = False
                        response_format_param = None
                        continue
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
                        self.metrics.error_count.labels(
                            error_type="llm_call_error", service=settings.service_name
                        ).inc()
                        logger.error("LLM call failed after all retries", error=str(e), exc_info=True)
                        raise last_error

    # ── safe JSON generation with schema check + fallback ────────

    @staticmethod
    def sanitize_user_input(text: str) -> str:
        """Truncate, strip prompt-injection markers, mask PII."""
        if not text:
            return ""
        text = text[:MAX_USER_INPUT_LENGTH]
        text = re.sub(r"<\|.*?\|>", "", text)
        for pattern, replacement in _PII_PATTERNS:
            text = pattern.sub(replacement, text)
        return text.strip()

    @staticmethod
    def _extract_json(raw: str) -> Dict[str, Any]:
        """Try to parse JSON from raw LLM output (handles ```json fences)."""
        raw = raw.strip()
        fence = re.search(r"```(?:json)?\s*\n?(.*?)```", raw, re.DOTALL)
        if fence:
            raw = fence.group(1).strip()
        return json.loads(raw)

    @staticmethod
    def _check_schema(data: Dict[str, Any], required_keys: Set[str]) -> bool:
        """Return True if all required keys present in data."""
        return required_keys.issubset(data.keys())

    @staticmethod
    def _check_leakage(text: str) -> bool:
        """Return True if response leaks system instructions."""
        markers = ["system prompt", "<<SYS>>", "[INST]", "you are an ai"]
        lower = text.lower()
        return any(m in lower for m in markers)

    async def safe_generate_json(
        self,
        prompt: str,
        required_keys: Set[str],
        fallback: Dict[str, Any],
        low_temp: float = 0.2,
    ) -> Dict[str, Any]:
        """Generate → parse JSON → validate schema → retry once → fallback.

        Steps: pre-check input (PII masking + prompt-injection strip) → call LLM →
        JSON parse → schema check → leakage check → on failure retry with low
        temperature → deterministic fallback.
        """
        prompt = self.sanitize_user_input(prompt)
        for attempt in range(2):
            temp = None if attempt == 0 else low_temp
            try:
                raw = await self.generate(prompt, temperature=temp, max_retries=2)

                if self._check_leakage(raw):
                    self.metrics.error_count.labels(
                        error_type="llm_leakage", service=settings.service_name
                    ).inc()
                    logger.warning("LLM response leaked system instructions, using fallback")
                    continue

                data = self._extract_json(raw)

                if not self._check_schema(data, required_keys):
                    missing = required_keys - data.keys()
                    self.metrics.error_count.labels(
                        error_type="schema_fail", service=settings.service_name
                    ).inc()
                    logger.warning(
                        "Schema check failed",
                        missing_keys=list(missing),
                        attempt=attempt + 1,
                    )
                    continue

                return data

            except json.JSONDecodeError:
                self.metrics.error_count.labels(
                    error_type="schema_fail", service=settings.service_name
                ).inc()
                logger.warning("Failed to parse LLM JSON", attempt=attempt + 1)
                continue
            except Exception as e:
                self.metrics.error_count.labels(
                    error_type="llm_call_error", service=settings.service_name
                ).inc()
                logger.warning("LLM call error in safe_generate_json", error=str(e), attempt=attempt + 1)
                continue

        self.metrics.error_count.labels(
            error_type="fallback_used", service=settings.service_name
        ).inc()
        logger.warning("Using deterministic fallback after failed LLM attempts")
        return fallback

    async def generate_structured(
        self,
        prompt: str,
        response_model: Type[T],
        fallback: T,
        low_temp: float = 0.2,
    ) -> T:
        """Generate a typed Pydantic model response from LLM (§10.5).

        Tries native json_schema response_format first; falls back to
        text-injection + manual parse if the model rejects it.
        """
        schema = response_model.model_json_schema()
        is_gpt5 = "gpt-5" in self.config.llm_model.lower()

        # ── Attempt 1: native json_schema (structured outputs) ──
        if self._supports_json_schema:
            try:
                params: Dict[str, Any] = {
                    "model": self.config.llm_model,
                    "messages": [{"role": "user", "content": prompt}],
                    "response_format": {
                        "type": "json_schema",
                        "json_schema": {
                            "name": response_model.__name__,
                            "schema": schema,
                            "strict": True,
                        },
                    },
                }
                if is_gpt5:
                    params["max_completion_tokens"] = self.config.llm_max_tokens
                else:
                    params["temperature"] = self.config.llm_temperature
                    params["max_tokens"] = self.config.llm_max_tokens

                call_start = time.perf_counter()
                resp = await self.client.chat.completions.create(**params)
                duration_ms = round((time.perf_counter() - call_start) * 1000, 2)

                raw = resp.choices[0].message.content or ""
                if self._check_leakage(raw):
                    raise ValueError("leakage detected")

                data = json.loads(raw)
                result = response_model.model_validate(data)

                self.metrics.llm_calls_total.labels(
                    provider=self.config.llm_provider,
                    model=self.config.llm_model,
                    status="success",
                ).inc()
                logger.info(
                    "generate_structured via json_schema",
                    model_type=response_model.__name__,
                    duration_ms=duration_ms,
                )
                return result

            except Exception as e:
                if self._is_response_format_unsupported(e):
                    self._supports_json_schema = False
                    logger.warning(
                        "json_schema response_format unsupported, falling back to text injection",
                        model=self.config.llm_model,
                    )
                else:
                    logger.warning(
                        "generate_structured json_schema attempt failed",
                        model_type=response_model.__name__,
                        error=str(e),
                    )

        # ── Attempt 2-3: text-injection fallback ──
        schema_str = json.dumps(schema, ensure_ascii=False, indent=2)
        augmented_prompt = (
            f"{prompt}\n\n"
            f"Respond ONLY with valid JSON matching this schema:\n{schema_str}"
        )
        augmented_prompt = self.sanitize_user_input(augmented_prompt)

        for attempt in range(2):
            temp = None if attempt == 0 else low_temp
            try:
                raw = await self.generate(augmented_prompt, temperature=temp, max_retries=2)

                if self._check_leakage(raw):
                    self.metrics.error_count.labels(
                        error_type="llm_leakage", service=settings.service_name
                    ).inc()
                    logger.warning("LLM response leaked system instructions, retrying")
                    continue

                data = self._extract_json(raw)
                result = response_model.model_validate(data)

                logger.debug(
                    "generate_structured succeeded (text fallback)",
                    model_type=response_model.__name__,
                    attempt=attempt + 1,
                )
                return result

            except (json.JSONDecodeError, ValidationError) as e:
                self.metrics.error_count.labels(
                    error_type="schema_fail", service=settings.service_name
                ).inc()
                logger.warning(
                    "generate_structured validation failed",
                    model_type=response_model.__name__,
                    error=str(e),
                    attempt=attempt + 1,
                )
                continue
            except Exception as e:
                self.metrics.error_count.labels(
                    error_type="llm_call_error", service=settings.service_name
                ).inc()
                logger.warning(
                    "generate_structured LLM call error",
                    model_type=response_model.__name__,
                    error=str(e),
                    attempt=attempt + 1,
                )
                continue

        self.metrics.error_count.labels(
            error_type="fallback_used", service=settings.service_name
        ).inc()
        logger.warning(
            "generate_structured using fallback",
            model_type=response_model.__name__,
        )
        return fallback

    async def chat_with_tools(
        self,
        messages: list,
        tools: list,
        tool_choice: str = "auto",
    ) -> Dict[str, Any]:
        """Call the LLM with OpenAI function-calling tool definitions (§8 stage 3).

        Returns the assistant message as a plain dict (role, content, tool_calls)
        so the agent loop can append it directly to the messages list.
        """
        is_gpt5 = "gpt-5" in self.config.llm_model.lower()
        params: Dict[str, Any] = {
            "model": self.config.llm_model,
            "messages": messages,
            "tools": tools,
            "tool_choice": tool_choice,
        }
        if not is_gpt5:
            params["temperature"] = self.config.llm_temperature
            params["max_tokens"] = self.config.llm_max_tokens
        else:
            params["max_completion_tokens"] = self.config.llm_max_tokens

        call_start = time.perf_counter()
        try:
            resp = await self.client.chat.completions.create(**params)
            duration_ms = round((time.perf_counter() - call_start) * 1000, 2)
            self.metrics.llm_calls_total.labels(
                provider=self.config.llm_provider,
                model=self.config.llm_model,
                status="success",
            ).inc()
            logger.info(
                "LLM tool-call completed",
                model=self.config.llm_model,
                duration_ms=duration_ms,
                tokens=resp.usage.total_tokens if resp.usage else None,
            )
        except Exception as exc:
            self.metrics.llm_calls_total.labels(
                provider=self.config.llm_provider,
                model=self.config.llm_model,
                status="error",
            ).inc()
            logger.error("chat_with_tools failed", error=str(exc))
            raise

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
