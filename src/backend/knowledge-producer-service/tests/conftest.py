"""Shared fixtures for knowledge-producer-service tests."""

from __future__ import annotations

import sys
from pathlib import Path
from types import ModuleType
from unittest.mock import AsyncMock, MagicMock

import pytest

# ── Ensure src is importable ────────────────────────────────────────────
SERVICE_ROOT = Path(__file__).resolve().parent.parent
if str(SERVICE_ROOT) not in sys.path:
    sys.path.insert(0, str(SERVICE_ROOT))

# ── Stub out heavy optional deps that may not be installed locally ────────
# structlog is used by src.logger.setup; provide a lightweight stand-in so
# tests don't require the full dependency tree.
_structlog_stub = ModuleType("structlog")
_structlog_stub.get_logger = lambda *a, **kw: MagicMock()  # type: ignore[attr-defined]
_structlog_stub.contextvars = MagicMock()  # type: ignore[attr-defined]
_structlog_stub.stdlib = MagicMock()  # type: ignore[attr-defined]
_structlog_stub.processors = MagicMock()  # type: ignore[attr-defined]
_structlog_stub.dev = MagicMock()  # type: ignore[attr-defined]
_structlog_stub.configure = MagicMock()  # type: ignore[attr-defined]
_structlog_stub.BoundLogger = MagicMock()  # type: ignore[attr-defined]
sys.modules.setdefault("structlog", _structlog_stub)
sys.modules.setdefault("structlog.stdlib", _structlog_stub.stdlib)

# python-json-logger may also be missing
sys.modules.setdefault("python_json_logger", MagicMock())
sys.modules.setdefault("pythonjsonlogger", MagicMock())


@pytest.fixture()
def _patch_settings(monkeypatch):
    """Override settings attributes used across multiple test modules."""
    from src.config import settings

    monkeypatch.setattr(settings, "allowed_segment_types",
                        "definition,explanation,example,analogy,code_sample,diagram_description,formula")
    monkeypatch.setattr(settings, "allowed_question_types",
                        "open,multiple_choice,coding,case,practical,applied,fill_blank,true_false")
    monkeypatch.setattr(settings, "similarity_threshold", 0.8)
    monkeypatch.setattr(settings, "dedup_max_candidates", 10)
    monkeypatch.setattr(settings, "max_llm_calls_per_job", 5)
    monkeypatch.setattr(settings, "max_ingest_file_bytes", 10_000_000)
    monkeypatch.setattr(settings, "max_fetch_url_bytes", 5_000_000)
    monkeypatch.setattr(settings, "fetch_url_timeout", 15)
    monkeypatch.setattr(settings, "trusted_domain_patterns",
                        "arxiv.org,pytorch.org,docs.python.org")
    monkeypatch.setattr(settings, "denied_domain_patterns", "")
    monkeypatch.setattr(settings, "web_search_rate_limit_per_minute", 10)
    monkeypatch.setattr(settings, "max_web_search_per_job", 3)


@pytest.fixture()
def sample_raw_document():
    """Minimal RawDocument for pipeline tests."""
    from src.schemas import RawDocument

    return RawDocument(
        source_uri="file:///tmp/test.md",
        mime="text/markdown",
        text="# Transformers\n\nAttention is all you need.",
        metadata={"origin": "test"},
    )


@pytest.fixture()
def mock_llm_client():
    """AsyncMock that simulates an LLM client with a .generate() method."""
    client = AsyncMock()
    client.generate = AsyncMock(return_value='{"title":"T","topic":"t","complexity":2,'
                                             '"segments":[{"type":"definition","content":"c","order":1}],'
                                             '"relations":[]}')
    return client


@pytest.fixture()
def golden_md(tmp_path: Path) -> Path:
    """Create a small markdown file for ingestion tests."""
    md = tmp_path / "notes.md"
    md.write_text(
        "# Neural Networks\n\n"
        "A neural network is a computational model inspired by biological neurons.\n\n"
        "## Layers\n\n"
        "Networks consist of input, hidden, and output layers.\n",
        encoding="utf-8",
    )
    return md
