"""Shared fixtures for analyst-agent-service tests."""

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
    "langgraph",
    "langgraph.graph",
):
    _ensure_stub(_mod)


# ---------------------------------------------------------------------------
# Mock settings
# ---------------------------------------------------------------------------

def _make_mock_settings() -> SimpleNamespace:
    return SimpleNamespace(
        service_name="analyst-agent-service",
        service_version="1.0.0-test",
        llm_provider="openai",
        llm_base_url=None,
        llm_api_key="sk-test-key",
        llm_model="gpt-4",
        llm_model_mini="gpt-4-mini",
        llm_model_nano="gpt-4-nano",
        llm_temperature=0.4,
        llm_max_tokens=4000,
        llm_timeout=120,
        kafka_bootstrap_servers="localhost:9092",
        kafka_topic_session_completed="tensor-talks-session.completed",
        kafka_consumer_group="tensor-talks-analyst-agent-service-group",
        kafka_consumer_auto_offset_reset="earliest",
        kafka_consumer_enable_auto_commit=False,
        kafka_consumer_session_timeout_ms=30000,
        kafka_consumer_heartbeat_interval_ms=10000,
        results_crud_service_url="http://results-crud:8088",
        session_service_url="http://session-service:8083",
        chat_crud_service_url="http://chat-crud:8087",
        knowledge_producer_service_url="http://knowledge-producer:8095",
        max_retries=3,
        processing_timeout=180,
        log_level="DEBUG",
        log_format="json",
        enable_prometheus=False,
        metrics_port=9094,
    )


@pytest.fixture()
def mock_settings():
    return _make_mock_settings()


# ---------------------------------------------------------------------------
# Mock metrics
# ---------------------------------------------------------------------------

def _make_labelled_counter():
    counter = MagicMock()
    child = MagicMock()
    counter.labels.return_value = child
    return counter


def _make_histogram():
    hist = MagicMock()
    hist.time.return_value = MagicMock(
        __enter__=MagicMock(return_value=None),
        __exit__=MagicMock(return_value=False),
    )
    return hist


def _make_mock_metrics():
    return SimpleNamespace(
        error_count=_make_labelled_counter(),
        llm_calls_total=_make_labelled_counter(),
        llm_call_duration=_make_histogram(),
        http_requests_total=_make_labelled_counter(),
        http_request_duration=_make_histogram(),
        active_analyses=MagicMock(),
        topic_progress_updates=MagicMock(),
    )


@pytest.fixture()
def mock_metrics():
    return _make_mock_metrics()


# ---------------------------------------------------------------------------
# Auto-patch settings + metrics for every test
# ---------------------------------------------------------------------------

@pytest.fixture(autouse=True)
def _patch_globals(mock_settings, mock_metrics):
    with (
        patch("src.config.settings", mock_settings),
        patch("src.metrics.collector.get_metrics_collector", return_value=mock_metrics),
        patch("src.metrics.get_metrics_collector", return_value=mock_metrics),
    ):
        yield
