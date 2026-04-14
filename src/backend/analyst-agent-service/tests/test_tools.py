"""Tests for analyst agent tools (analyst_tools.py)."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from src.tools.analyst_tools import (
    group_errors_by_topic,
    validate_report,
    fetch_url,
    _TRUSTED_DOMAINS,
)


# ===================================================================
# group_errors_by_topic
# ===================================================================

class TestGroupErrorsByTopic:

    def test_groups_correctly(self):
        evaluations = [
            {"topic": "python", "question": "Q1", "score": 30},
            {"topic": "python", "question": "Q2", "score": 80},
            {"topic": "ml", "question": "Q3", "score": 20},
        ]
        result = group_errors_by_topic(evaluations)
        assert "python" in result
        assert "Q1" in result["python"]
        assert "Q2" not in result["python"]  # score 80 >= 60, not an error
        assert "ml" in result
        assert "Q3" in result["ml"]

    def test_empty_evaluations(self):
        result = group_errors_by_topic([])
        assert result == {}

    def test_all_passing(self):
        evaluations = [
            {"topic": "python", "question": "Q1", "score": 90},
        ]
        result = group_errors_by_topic(evaluations)
        assert result == {}


# ===================================================================
# validate_report
# ===================================================================

class TestValidateReport:

    def test_valid_report(self):
        report = {
            "summary": "Good performance overall in the interview",
            "score": 72,
            "errors_by_topic": {"NLP": ["Q1"]},
            "strengths": ["Python"],
            "preparation_plan": ["Study NLP"],
            "materials": ["Book A"],
        }
        result = validate_report(report, evaluations=[])
        assert result["validation_passed"] is True
        assert result["issues"] == []

    def test_missing_summary(self):
        report = {"score": 72}
        result = validate_report(report, evaluations=[])
        assert result["validation_passed"] is False
        assert any("summary" in i for i in result["issues"])

    def test_short_summary(self):
        report = {"summary": "ok", "score": 72}
        result = validate_report(report, evaluations=[])
        assert result["validation_passed"] is False

    def test_score_out_of_range_clamped(self):
        # AnalystReport.clamp_score clamps to [0,100], so 150 becomes 100 → passes
        report = {"summary": "x" * 15, "score": 150}
        result = validate_report(report, evaluations=[])
        assert result["validation_passed"] is True

    def test_score_negative_string_fails(self):
        report = {"summary": "x" * 15, "score": "not_a_number"}
        result = validate_report(report, evaluations=[])
        assert result["validation_passed"] is False


# ===================================================================
# fetch_url — domain allowlist
# ===================================================================

class TestFetchUrl:

    @pytest.mark.asyncio
    async def test_blocks_untrusted_domain(self):
        result = await fetch_url("https://evil.example.com/malware")
        assert "[blocked]" in result

    @pytest.mark.asyncio
    async def test_allows_trusted_domain(self):
        mock_response = MagicMock()
        mock_response.text = "Hello arxiv content"
        mock_response.headers = {"content-type": "text/plain"}

        mock_client = AsyncMock()
        mock_client.get = AsyncMock(return_value=mock_response)
        mock_client.__aenter__ = AsyncMock(return_value=mock_client)
        mock_client.__aexit__ = AsyncMock(return_value=False)

        with patch("httpx.AsyncClient", return_value=mock_client):
            result = await fetch_url("https://arxiv.org/abs/1234.5678")
            assert "Hello arxiv content" in result

    def test_trusted_domains_list_not_empty(self):
        assert len(_TRUSTED_DOMAINS) > 0
        assert "arxiv.org" in _TRUSTED_DOMAINS
