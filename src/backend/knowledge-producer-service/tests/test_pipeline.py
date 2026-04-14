"""Tests for src/pipeline/runner.py – PipelineRunner steps."""

from __future__ import annotations

import json
from unittest.mock import AsyncMock, patch

import pytest

from src.schemas import DraftKind, RawDocument, ReviewStatus


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _raw_doc(text: str = "Some knowledge text") -> RawDocument:
    return RawDocument(source_uri="file:///test.md", mime="text/markdown", text=text)


def _make_runner(llm_client=None, kb_fn=None, q_fn=None, max_calls: int = 5):
    from src.config import settings

    with patch.object(settings, "max_llm_calls_per_job", max_calls):
        from src.pipeline.runner import PipelineRunner
        return PipelineRunner(
            llm_client=llm_client,
            knowledge_search_fn=kb_fn,
            question_search_fn=q_fn,
        )


# ── _fallback_structure ──────────────────────────────────────────────────


class TestFallbackStructure:
    def test_knowledge_fallback_has_definition_segment(self):
        from src.pipeline.runner import PipelineRunner

        ctx = {"topic": "attention", "text": "Attention is all you need."}
        result = PipelineRunner._fallback_structure(ctx, DraftKind.KNOWLEDGE)

        assert result["kind"] == "knowledge"
        structured = result["structured"]
        assert structured["title"] == "Material on attention"
        assert structured["complexity"] == 2
        assert len(structured["segments"]) == 1
        assert structured["segments"][0]["type"] == "definition"

    def test_question_fallback_returns_open_question(self):
        from src.pipeline.runner import PipelineRunner

        ctx = {"topic": "cnn", "text": "Convolution."}
        result = PipelineRunner._fallback_structure(ctx, DraftKind.QUESTION)

        assert result["kind"] == "question"
        items = result["structured"]
        assert isinstance(items, list) and len(items) == 1
        assert items[0]["question_type"] == "open"
        assert "cnn" in items[0]["content"]

    def test_fallback_truncates_text_to_2000(self):
        from src.pipeline.runner import PipelineRunner

        long_text = "x" * 5000
        ctx = {"topic": "t", "text": long_text}
        result = PipelineRunner._fallback_structure(ctx, DraftKind.KNOWLEDGE)
        assert len(result["structured"]["segments"][0]["content"]) == 2000


# ── step_gather_context ──────────────────────────────────────────────────


class TestStepGatherContext:
    @pytest.mark.asyncio
    async def test_basic_context(self):
        runner = _make_runner()
        raw = _raw_doc("hello world")
        ctx = await runner.step_gather_context(raw, "topic")

        assert ctx["source_uri"] == raw.source_uri
        assert ctx["topic"] == "topic"
        assert ctx["text"] == "hello world"

    @pytest.mark.asyncio
    async def test_text_truncated_to_50k(self):
        runner = _make_runner()
        raw = _raw_doc("a" * 100_000)
        ctx = await runner.step_gather_context(raw, "t")
        assert len(ctx["text"]) == 50_000

    @pytest.mark.asyncio
    async def test_kb_search_appended(self):
        kb = AsyncMock(return_value=[{"title": "existing"}])
        runner = _make_runner(kb_fn=kb)
        raw = _raw_doc()
        ctx = await runner.step_gather_context(raw, "ml")
        assert ctx["existing_knowledge"] == [{"title": "existing"}]

    @pytest.mark.asyncio
    async def test_kb_search_failure_does_not_raise(self):
        kb = AsyncMock(side_effect=RuntimeError("boom"))
        runner = _make_runner(kb_fn=kb)
        ctx = await runner.step_gather_context(_raw_doc(), "t")
        assert "existing_knowledge" not in ctx


# ── step_structure ───────────────────────────────────────────────────────


class TestStepStructure:
    @pytest.mark.asyncio
    async def test_uses_llm_when_available(self, mock_llm_client):
        runner = _make_runner(llm_client=mock_llm_client)
        ctx = {"topic": "ml", "text": "data"}
        result = await runner.step_structure(ctx, DraftKind.KNOWLEDGE)
        assert result["kind"] == "knowledge"
        assert "title" in result["structured"]
        mock_llm_client.generate.assert_awaited_once()

    @pytest.mark.asyncio
    async def test_falls_back_when_no_llm(self):
        runner = _make_runner(llm_client=None)
        ctx = {"topic": "rnn", "text": "Recurrent."}
        result = await runner.step_structure(ctx, DraftKind.KNOWLEDGE)
        assert result["structured"]["title"] == "Material on rnn"

    @pytest.mark.asyncio
    async def test_falls_back_on_json_decode_error(self):
        bad_llm = AsyncMock()
        bad_llm.generate = AsyncMock(return_value="not json at all")
        runner = _make_runner(llm_client=bad_llm)
        ctx = {"topic": "x", "text": "y"}
        result = await runner.step_structure(ctx, DraftKind.KNOWLEDGE)
        assert result["structured"]["title"] == "Material on x"

    @pytest.mark.asyncio
    async def test_question_prompt_path(self, mock_llm_client):
        mock_llm_client.generate = AsyncMock(
            return_value='[{"content":"Q?","ideal_answer":"A","question_type":"open","complexity":1}]'
        )
        runner = _make_runner(llm_client=mock_llm_client)
        ctx = {"topic": "dl", "text": "Deep learning basics."}
        result = await runner.step_structure(ctx, DraftKind.QUESTION)
        assert result["kind"] == "question"
        assert isinstance(result["structured"], list)


# ── step_check_duplicates ────────────────────────────────────────────────


class TestStepCheckDuplicates:
    @pytest.mark.asyncio
    async def test_no_search_fn_returns_false(self):
        runner = _make_runner(kb_fn=None)
        structured = {"structured": {"title": "Neural networks"}, "kind": "knowledge"}
        assert await runner.step_check_duplicates(structured, "ml") is False

    @pytest.mark.asyncio
    async def test_exact_duplicate_detected(self):
        kb = AsyncMock(return_value=[{"title": "Neural networks"}])
        runner = _make_runner(kb_fn=kb)
        structured = {"structured": {"title": "Neural networks"}, "kind": "knowledge"}
        assert await runner.step_check_duplicates(structured, "ml") is True

    @pytest.mark.asyncio
    async def test_similar_title_above_threshold(self):
        kb = AsyncMock(return_value=[{"title": "neural network basics"}])
        runner = _make_runner(kb_fn=kb)
        structured = {"structured": {"title": "neural network basic"}, "kind": "knowledge"}
        assert await runner.step_check_duplicates(structured, "ml") is True

    @pytest.mark.asyncio
    async def test_dissimilar_title_no_duplicate(self):
        kb = AsyncMock(return_value=[{"title": "quantum computing"}])
        runner = _make_runner(kb_fn=kb)
        structured = {"structured": {"title": "neural networks"}, "kind": "knowledge"}
        assert await runner.step_check_duplicates(structured, "ml") is False

    @pytest.mark.asyncio
    async def test_empty_title_returns_false(self):
        kb = AsyncMock(return_value=[{"title": "something"}])
        runner = _make_runner(kb_fn=kb)
        structured = {"structured": {"title": ""}, "kind": "knowledge"}
        assert await runner.step_check_duplicates(structured, "ml") is False

    @pytest.mark.asyncio
    async def test_search_failure_returns_false(self):
        kb = AsyncMock(side_effect=RuntimeError("db down"))
        runner = _make_runner(kb_fn=kb)
        structured = {"structured": {"title": "X"}, "kind": "knowledge"}
        assert await runner.step_check_duplicates(structured, "ml") is False


# ── step_save_draft ──────────────────────────────────────────────────────


class TestStepSaveDraft:
    @pytest.mark.asyncio
    async def test_knowledge_draft_created(self):
        runner = _make_runner()
        structured = {
            "structured": {"title": "Attention", "topic": "transformers"},
            "kind": "knowledge",
        }
        draft = await runner.step_save_draft(structured, "transformers", "file:///t.md", False)
        assert draft.kind == DraftKind.KNOWLEDGE
        assert draft.title == "Attention"
        assert draft.review_status == ReviewStatus.PENDING
        assert draft.duplicate_candidate is False

    @pytest.mark.asyncio
    async def test_question_draft_wraps_list_in_items(self):
        runner = _make_runner()
        structured = {
            "structured": [{"content": "Q?", "question_type": "open"}],
            "kind": "question",
        }
        draft = await runner.step_save_draft(structured, "ml", "file:///q.md", True)
        assert draft.kind == DraftKind.QUESTION
        assert "items" in draft.content
        assert draft.duplicate_candidate is True

    @pytest.mark.asyncio
    async def test_draft_has_uuid(self):
        import uuid

        runner = _make_runner()
        structured = {"structured": {"title": "T"}, "kind": "knowledge"}
        draft = await runner.step_save_draft(structured, "t", "s", False)
        uuid.UUID(draft.draft_id)  # raises if invalid


# ── LLM call limit enforcement ───────────────────────────────────────────


class TestLlmCallLimit:
    @pytest.mark.asyncio
    async def test_exceeding_limit_raises(self):
        llm = AsyncMock()
        llm.generate = AsyncMock(return_value="{}")
        runner = _make_runner(llm_client=llm, max_calls=2)

        await runner._call_llm("p1")
        await runner._call_llm("p2")
        with pytest.raises(RuntimeError, match="LLM call limit"):
            await runner._call_llm("p3")

    @pytest.mark.asyncio
    async def test_no_llm_raises(self):
        runner = _make_runner(llm_client=None)
        with pytest.raises(RuntimeError, match="no LLM configured"):
            await runner._call_llm("prompt")


# ── Full pipeline run ────────────────────────────────────────────────────


class TestFullPipelineRun:
    @pytest.mark.asyncio
    async def test_full_run_with_llm(self, mock_llm_client):
        runner = _make_runner(llm_client=mock_llm_client)
        raw = _raw_doc("Transformers use self-attention.")
        draft = await runner.run(raw, "transformers", DraftKind.KNOWLEDGE)

        assert draft.kind == DraftKind.KNOWLEDGE
        assert draft.review_status == ReviewStatus.PENDING
        assert draft.source == raw.source_uri
        assert runner._llm_call_count == 1

    @pytest.mark.asyncio
    async def test_full_run_without_llm_uses_fallback(self):
        runner = _make_runner(llm_client=None)
        raw = _raw_doc("Recurrent neural networks.")
        draft = await runner.run(raw, "rnn", DraftKind.KNOWLEDGE)

        assert draft.title == "Material on rnn"
        assert runner._llm_call_count == 0

    @pytest.mark.asyncio
    async def test_run_resets_call_count(self, mock_llm_client):
        runner = _make_runner(llm_client=mock_llm_client, max_calls=1)
        raw = _raw_doc("text")

        await runner.run(raw, "t")
        assert runner._llm_call_count == 1
        await runner.run(raw, "t")
        assert runner._llm_call_count == 1
