"""Shared fixtures for agent-service tests."""

from __future__ import annotations

import sys
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest


# ---------------------------------------------------------------------------
# Stub heavy third-party imports so tests run without real Kafka / Redis / etc.
# Must happen before any `from src.*` import.
# ---------------------------------------------------------------------------

def _ensure_stub(module_name: str) -> MagicMock:
    if module_name not in sys.modules:
        sys.modules[module_name] = MagicMock()
    return sys.modules[module_name]


for _mod in (
    "confluent_kafka",
    "structlog",
    "structlog.stdlib",
    "structlog.contextvars",
    "structlog.processors",
    "structlog.dev",
    "prometheus_client",
    "redis",
    "hiredis",
):
    _ensure_stub(_mod)


# ---------------------------------------------------------------------------
# Mock settings  (avoid reading .env / real env vars)
# ---------------------------------------------------------------------------

def _make_mock_settings() -> SimpleNamespace:
    return SimpleNamespace(
        service_name="interviewer-agent-service",
        service_version="1.0.0-test",
        llm_provider="openai",
        llm_base_url=None,
        llm_api_key="sk-test-key",
        llm_model="gpt-4",
        llm_temperature=0.7,
        llm_max_tokens=2000,
        llm_timeout=60,
        kafka_bootstrap_servers="localhost:9092",
        kafka_topic_messages_full="tensor-talks-messages.full.data",
        kafka_topic_generated="tensor-talks-generated.phrases",
        kafka_topic_session_completed="tensor-talks-session.completed",
        kafka_consumer_group="tensor-talks-agent-service-group",
        kafka_consumer_auto_offset_reset="earliest",
        kafka_consumer_enable_auto_commit=False,
        kafka_consumer_max_poll_records=100,
        kafka_consumer_session_timeout_ms=30000,
        kafka_consumer_heartbeat_interval_ms=10000,
        kafka_producer_acks="all",
        kafka_producer_retries=3,
        kafka_producer_compression_type="snappy",
        enable_prometheus=False,
        metrics_port=9092,
        # Confidence-aware thresholds (§10.7)
        strong_next_threshold=0.85,
        weak_next_threshold=0.75,
        next_confidence_threshold=0.7,
        strong_hint_threshold=0.4,
        confidence_re_evaluate_threshold=0.5,
        # Interview limits (§10.13)
        max_hints_per_question=2,
        max_attempts_per_question=3,
        max_tool_calls_per_step=5,
        max_clarifications=3,
        # Redis
        enable_redis_cache=True,
    )


@pytest.fixture()
def mock_settings():
    return _make_mock_settings()


# ---------------------------------------------------------------------------
# Mock metrics collector  (lightweight stand-in for Prometheus objects)
# ---------------------------------------------------------------------------

def _make_labelled_counter():
    """Return a mock that supports .labels(...).inc()."""
    counter = MagicMock()
    child = MagicMock()
    counter.labels.return_value = child
    return counter


def _make_histogram():
    """Return a mock Histogram with a .time() context-manager."""
    hist = MagicMock()
    hist.time.return_value = MagicMock(__enter__=MagicMock(return_value=None), __exit__=MagicMock(return_value=False))
    return hist


def _make_mock_metrics():
    m = SimpleNamespace(
        error_count=_make_labelled_counter(),
        llm_calls_total=_make_labelled_counter(),
        llm_call_duration=_make_histogram(),
        kafka_producer_duration=_make_histogram(),
        messages_processed_total=_make_labelled_counter(),
        processing_duration=_make_histogram(),
        active_dialogues=MagicMock(),
        current_question_index=MagicMock(),
        kafka_consumer_lag=MagicMock(),
        redis_operation_duration=_make_histogram(),
    )
    return m


@pytest.fixture()
def mock_metrics():
    return _make_mock_metrics()


# ---------------------------------------------------------------------------
# Auto-patch settings + metrics for every test (session-scoped patches)
# ---------------------------------------------------------------------------

@pytest.fixture(autouse=True)
def _patch_globals(mock_settings, mock_metrics):
    """Patch settings and get_metrics_collector globally so imports Just Work."""
    with (
        patch("src.config.settings", mock_settings),
        patch("src.metrics.collector.get_metrics_collector", return_value=mock_metrics),
        patch("src.metrics.get_metrics_collector", return_value=mock_metrics),
    ):
        yield
