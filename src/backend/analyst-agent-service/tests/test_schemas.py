"""Tests for Pydantic models in analyst-agent-service."""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from src.models.llm_outputs import AnalystReport, TrainingPreset, ProgressDelta


# ===================================================================
# AnalystReport
# ===================================================================

class TestAnalystReport:

    def test_valid_report(self):
        r = AnalystReport(
            summary="Кандидат показал хорошие знания в Python и ML",
            score=72,
            errors_by_topic={"NLP": ["Не знает attention mechanism"]},
            strengths=["Python", "PyTorch"],
            preparation_plan=["Изучить NLP", "Повторить attention"],
            materials=["Jurafsky NLP book"],
        )
        assert r.score == 72
        assert "NLP" in r.errors_by_topic

    def test_score_clamped_to_100(self):
        r = AnalystReport(summary="x" * 10, score=150)
        assert r.score == 100

    def test_score_clamped_to_0(self):
        r = AnalystReport(summary="x" * 10, score=-5)
        assert r.score == 0

    def test_score_float_rounded(self):
        r = AnalystReport(summary="x" * 10, score=72.7)
        assert r.score == 72

    def test_summary_too_short_rejected(self):
        with pytest.raises(ValidationError):
            AnalystReport(summary="short", score=50)

    def test_defaults_for_list_fields(self):
        r = AnalystReport(summary="x" * 10, score=50)
        assert r.errors_by_topic == {}
        assert r.strengths == []
        assert r.preparation_plan == []
        assert r.materials == []

    def test_json_schema_generated(self):
        schema = AnalystReport.model_json_schema()
        assert "summary" in schema["properties"]
        assert "score" in schema["properties"]


# ===================================================================
# TrainingPreset
# ===================================================================

class TestTrainingPreset:

    def test_valid_preset(self):
        p = TrainingPreset(
            target_mode="training",
            topic="NLP",
            weak_topics=["attention", "transformers"],
            recommended_materials=["Vaswani 2017"],
            priority=1,
        )
        assert p.target_mode == "training"
        assert p.priority == 1

    def test_priority_clamped(self):
        with pytest.raises(ValidationError):
            TrainingPreset(target_mode="study", topic="ML", priority=5)

    def test_invalid_mode_rejected(self):
        with pytest.raises(ValidationError):
            TrainingPreset(target_mode="interview", topic="ML")


# ===================================================================
# ProgressDelta
# ===================================================================

class TestProgressDelta:

    def test_improved(self):
        d = ProgressDelta(
            topic="NLP",
            previous_score=0.3,
            current_score=0.7,
            delta=0.4,
            assessment="improved",
        )
        assert d.assessment == "improved"

    def test_declined(self):
        d = ProgressDelta(
            topic="CV", previous_score=0.8, current_score=0.5, delta=-0.3, assessment="declined"
        )
        assert d.delta == -0.3

    def test_invalid_assessment_rejected(self):
        with pytest.raises(ValidationError):
            ProgressDelta(topic="X", previous_score=0.5, current_score=0.5, delta=0, assessment="unknown")
