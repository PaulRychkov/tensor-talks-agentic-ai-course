"""Guardrail layer formalization (§10.8).

Centralizes all pre-call and post-call checks into a single pipeline.
Pre-call guardrails run before the LLM is invoked; post-call after.

This module is the single entry point for all content safety checks.
"""

import re
import time
from dataclasses import dataclass, field
from typing import List, Optional

from ..logger import get_logger
from ..metrics import get_metrics_collector
from .pii_filter import check_pii_regex, PIIFilterResult, PIICategory

logger = get_logger(__name__)

# ── Constants ─────────────────────────────────────────────────────────────────

MAX_USER_INPUT_LENGTH = 4000

# Prompt injection markers that should be stripped from user input
_INJECTION_MARKERS = [
    r"<\|.*?\|>",
    r"\[INST\].*?\[/INST\]",
    r"<<SYS>>.*?<</SYS>>",
    r"###\s*System\s*:",
    r"###\s*Assistant\s*:",
]

# System prompt leakage markers to detect in LLM output
_LEAKAGE_MARKERS = [
    "system prompt",
    "<<sys>>",
    "[inst]",
    "you are an ai",
    "i am a language model",
    "as an ai assistant",
]

# Tone violation keywords (agent should not use these about the person)
_TONE_VIOLATIONS = {
    "неправильно": "можно улучшить",
    "ошибка": "неточность",
    "неверно": "не совсем так",
    "плохо": "можно дополнить",
    "вы должны": "рекомендую",
    "обязаны": "желательно",
    "нельзя": "лучше не",
    "не понимаете": "возможно, стоит уточнить",
    "не знаете": "если не помните",
    "не умеете": "если не знакомы",
}


@dataclass
class GuardrailResult:
    """Result of a guardrail check."""
    passed: bool
    violations: List[str] = field(default_factory=list)
    sanitized_text: Optional[str] = None  # cleaned version of input/output


# ── Pre-call guardrails ────────────────────────────────────────────────────────

def pre_call_sanitize(text: str) -> GuardrailResult:
    """Pre-call: Sanitize user input (truncation, injection strip, PII masking).

    NOTE: PII rejection should happen BEFORE calling this (see pii_filter.py).
    This sanitizer handles residual PII masking for the LLM prompt only.
    """
    if not text:
        return GuardrailResult(passed=True, sanitized_text="")

    metrics = get_metrics_collector()
    start = time.perf_counter()
    violations = []

    # 1. Truncate
    if len(text) > MAX_USER_INPUT_LENGTH:
        text = text[:MAX_USER_INPUT_LENGTH]
        violations.append("truncated")

    # 2. Strip prompt injection markers
    for marker_re in _INJECTION_MARKERS:
        original = text
        text = re.sub(marker_re, "", text, flags=re.DOTALL | re.IGNORECASE)
        if text != original:
            violations.append("injection_stripped")
            metrics.pii_filter_triggered_total.labels(
                level="pre_call", category="injection"
            ).inc()

    # 3. Mask residual PII patterns for LLM prompt (email, phone, card)
    pii_result = check_pii_regex(text)
    if pii_result.detected:
        cat = pii_result.category.value if pii_result.category else "unknown"
        violations.append(f"pii_masked:{cat}")
        metrics.pii_filter_triggered_total.labels(level="pre_call", category=cat).inc()

    elapsed = time.perf_counter() - start
    metrics.pii_filter_latency_seconds.labels(level="pre_call").observe(elapsed)

    return GuardrailResult(
        passed=True,  # pre-call sanitize never blocks; just cleans
        violations=violations,
        sanitized_text=text.strip(),
    )


# ── Post-call guardrails ───────────────────────────────────────────────────────

def post_call_check_leakage(text: str) -> GuardrailResult:
    """Post-call: Detect system prompt leakage in LLM output."""
    metrics = get_metrics_collector()
    start = time.perf_counter()
    lower = text.lower()
    violations = []

    for marker in _LEAKAGE_MARKERS:
        if marker in lower:
            violations.append(f"leakage:{marker}")
            metrics.pii_filter_triggered_total.labels(level="post_call", category="leakage").inc()
            break

    elapsed = time.perf_counter() - start
    metrics.pii_filter_latency_seconds.labels(level="post_call").observe(elapsed)

    return GuardrailResult(
        passed=len(violations) == 0,
        violations=violations,
        sanitized_text=text,
    )


def post_call_sanitize_tone(text: str) -> str:
    """Post-call: Replace aggressive/negative language with neutral alternatives.

    Returns the sanitized text.
    """
    for aggressive, neutral in _TONE_VIOLATIONS.items():
        text = text.replace(aggressive, neutral)
        text = text.replace(aggressive.capitalize(), neutral.capitalize())
    return text


def post_call_check_schema(data: dict, required_keys: set) -> GuardrailResult:
    """Post-call: Verify LLM output dict contains all required keys."""
    missing = required_keys - set(data.keys())
    if missing:
        return GuardrailResult(
            passed=False,
            violations=[f"missing_keys:{','.join(sorted(missing))}"],
        )
    return GuardrailResult(passed=True)
