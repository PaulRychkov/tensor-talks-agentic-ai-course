"""Tests for the interview builder pipeline (§9 p.1.5.2).

Verifies dedup, coverage, balance, and stable ordering.
"""

import pytest
from src.services.interview_builder_service import InterviewBuilderService


def _make_question(qid: str, theory_id: str = None, text: str = "", tags=None):
    """Helper to create a question in Go-struct-like format."""
    data = {"id": qid, "theory_id": theory_id, "content": {"question": text}}
    if tags:
        data["tags"] = tags
    return {"Data": data}


class TestDedup:
    """§9 p.1.5.2: after dedup no duplicates by id or text."""

    def test_dedup_by_id(self):
        qs = [
            _make_question("q1", text="What is ML?"),
            _make_question("q1", text="What is ML?"),
            _make_question("q2", text="What is NLP?"),
        ]
        result = InterviewBuilderService._dedup(qs)
        assert len(result) == 2

    def test_dedup_by_text_hash(self):
        qs = [
            _make_question("q1", text="What is machine learning?"),
            _make_question("q2", text="What is machine learning?"),
        ]
        result = InterviewBuilderService._dedup(qs)
        assert len(result) == 1

    def test_dedup_preserves_different_questions(self):
        qs = [
            _make_question("q1", text="What is ML?"),
            _make_question("q2", text="What is NLP?"),
            _make_question("q3", text="What is LLM?"),
        ]
        result = InterviewBuilderService._dedup(qs)
        assert len(result) == 3


class TestCoverage:
    """§9 p.1.5.2: each declared topic gets at least one slot
    (or explicit fallback_reason)."""

    def test_each_topic_gets_one(self):
        by_tag = {
            "nlp": [_make_question("q1", text="NLP q")],
            "llm": [_make_question("q2", text="LLM q")],
        }
        all_qs = by_tag["nlp"] + by_tag["llm"]

        selected, coverage = InterviewBuilderService._ensure_coverage(
            all_qs, by_tag, ["nlp", "llm"], limit=5
        )
        assert coverage.get("nlp", 0) >= 1
        assert coverage.get("llm", 0) >= 1

    def test_missing_topic_gets_zero_coverage(self):
        by_tag = {"nlp": [_make_question("q1", text="NLP q")]}
        all_qs = by_tag["nlp"]

        selected, coverage = InterviewBuilderService._ensure_coverage(
            all_qs, by_tag, ["nlp", "cv"], limit=5
        )
        assert coverage.get("cv", 0) == 0


class TestBalance:
    """§9 p.1.5.2: limit the share of a single topic."""

    def test_single_topic_capped(self):
        qs = [_make_question(f"q{i}", text=f"NLP question {i}") for i in range(10)]
        by_tag = {"nlp": qs}

        result = InterviewBuilderService._balance(qs, by_tag, max_ratio=0.6, limit=5)
        assert len(result) <= 5


class TestStableOrder:
    """§9 p.1.2.3.4: repeated run on same input gives same order."""

    def test_sort_is_deterministic(self):
        qs = [
            _make_question("q1", theory_id="t1", text="Q1"),
            _make_question("q2", theory_id="t2", text="Q2"),
            _make_question("q3", theory_id="t1", text="Q3"),
        ]
        sorted1 = InterviewBuilderService._sort_by_theory(qs)
        sorted2 = InterviewBuilderService._sort_by_theory(qs)
        ids1 = [q["Data"]["id"] for q in sorted1]
        ids2 = [q["Data"]["id"] for q in sorted2]
        assert ids1 == ids2
