"""Tests for src/schemas.py – Pydantic models and validation rules."""

from __future__ import annotations

from datetime import datetime, timezone
from unittest.mock import patch

import pytest
from pydantic import ValidationError


# ---------------------------------------------------------------------------
# Module-level reload helpers
# ---------------------------------------------------------------------------
# ALLOWED_SEGMENT_TYPES / ALLOWED_QUESTION_TYPES are evaluated at import time
# from settings.  We patch settings *before* reloading the module so the sets
# pick up our test values.

def _reload_schemas(
    segment_types: str = "definition,explanation,example,analogy,code_sample,diagram_description,formula",
    question_types: str = "open,multiple_choice,coding,case,practical,applied,fill_blank,true_false",
):
    """Reload schemas module with custom allowed types."""
    import importlib
    from src.config import settings

    with patch.object(settings, "allowed_segment_types", segment_types), \
         patch.object(settings, "allowed_question_types", question_types):
        import src.schemas as _mod
        importlib.reload(_mod)
    return _mod


@pytest.fixture(autouse=True)
def schemas():
    """Provide a freshly-reloaded schemas module for every test."""
    mod = _reload_schemas()
    yield mod


# ── KnowledgeSegment validation ──────────────────────────────────────────


class TestKnowledgeSegment:
    def test_valid_segment_types(self, schemas):
        for t in ("definition", "explanation", "example", "formula"):
            seg = schemas.KnowledgeSegment(type=t, content="x")
            assert seg.type == t

    def test_whitespace_and_case_normalised(self, schemas):
        seg = schemas.KnowledgeSegment(type="  Definition  ", content="x")
        assert seg.type == "definition"

    def test_invalid_segment_type_raises(self, schemas):
        with pytest.raises(ValidationError, match="segment type"):
            schemas.KnowledgeSegment(type="haiku", content="x")


# ── QuestionDraftContent validation ──────────────────────────────────────


class TestQuestionDraftContent:
    def test_valid_question_types(self, schemas):
        for qt in ("open", "multiple_choice", "coding", "true_false"):
            q = schemas.QuestionDraftContent(content="What is ML?", question_type=qt)
            assert q.question_type == qt

    def test_invalid_question_type_raises(self, schemas):
        with pytest.raises(ValidationError, match="question_type"):
            schemas.QuestionDraftContent(content="What is ML?", question_type="essay")

    def test_content_min_length(self, schemas):
        with pytest.raises(ValidationError):
            schemas.QuestionDraftContent(content="Hi", question_type="open")

    def test_defaults(self, schemas):
        q = schemas.QuestionDraftContent(content="What is ML?")
        assert q.complexity == 2
        assert q.ideal_answer == ""
        assert q.theory_id is None


# ── Draft state machine ──────────────────────────────────────────────────


class TestDraftStateMachine:
    """Draft.review_status transitions follow: pending→approved→published,
    pending→rejected, but rejected→published should never happen in the
    business layer.  Here we verify the enum values and that the model
    correctly stores each state."""

    def _make_draft(self, schemas, status: str) -> "schemas.Draft":
        return schemas.Draft(
            draft_id="d-1",
            kind=schemas.DraftKind.KNOWLEDGE,
            title="Test",
            topic="ML",
            content={"title": "Test"},
            review_status=status,
        )

    def test_pending_to_approved(self, schemas):
        d = self._make_draft(schemas, "pending")
        assert d.review_status == schemas.ReviewStatus.PENDING
        d.review_status = schemas.ReviewStatus.APPROVED
        assert d.review_status == schemas.ReviewStatus.APPROVED

    def test_pending_to_rejected(self, schemas):
        d = self._make_draft(schemas, "pending")
        d.review_status = schemas.ReviewStatus.REJECTED
        assert d.review_status == schemas.ReviewStatus.REJECTED

    def test_rejected_to_published_not_a_valid_status(self, schemas):
        """There is no PUBLISHED enum member, so attempting it raises."""
        d = self._make_draft(schemas, "rejected")
        with pytest.raises(ValueError):
            d.review_status = schemas.ReviewStatus("published")

    def test_valid_enum_values(self, schemas):
        assert set(schemas.ReviewStatus) == {
            schemas.ReviewStatus.PENDING,
            schemas.ReviewStatus.APPROVED,
            schemas.ReviewStatus.REJECTED,
        }


# ── DraftKind ────────────────────────────────────────────────────────────


class TestDraftKind:
    def test_knowledge_and_question(self, schemas):
        assert schemas.DraftKind.KNOWLEDGE.value == "knowledge"
        assert schemas.DraftKind.QUESTION.value == "question"

    def test_invalid_kind_raises(self, schemas):
        with pytest.raises(ValueError):
            schemas.DraftKind("essay")


# ── RawDocument ──────────────────────────────────────────────────────────


class TestRawDocument:
    def test_minimal_creation(self, schemas):
        doc = schemas.RawDocument(
            source_uri="https://example.com/paper.pdf",
            mime="application/pdf",
            text="hello",
        )
        assert doc.metadata == {}
        assert doc.text == "hello"

    def test_metadata_is_optional(self, schemas):
        doc = schemas.RawDocument(source_uri="s", mime="m", text="t")
        assert isinstance(doc.metadata, dict)

    def test_full_creation(self, schemas):
        doc = schemas.RawDocument(
            source_uri="/tmp/f.md",
            mime="text/markdown",
            text="# title\nbody",
            metadata={"file_hash": "abc123", "file_name": "f.md"},
        )
        assert doc.metadata["file_hash"] == "abc123"


# ── KnowledgeDraftContent ───────────────────────────────────────────────


class TestKnowledgeDraftContent:
    def test_requires_at_least_one_segment(self, schemas):
        with pytest.raises(ValidationError):
            schemas.KnowledgeDraftContent(
                title="T", topic="ML", segments=[]
            )

    def test_defaults(self, schemas):
        kdc = schemas.KnowledgeDraftContent(
            title="T",
            topic="ML",
            segments=[schemas.KnowledgeSegment(type="definition", content="x")],
        )
        assert kdc.complexity == 2
        assert kdc.relations == []
        assert kdc.metadata.tags == []
