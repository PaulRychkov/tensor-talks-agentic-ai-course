"""Tests for schemas: validation, normalization, defaults (§9 p.1.5.1)"""

import pytest
from src.schemas import (
    BuildRequest,
    BuildRequestParams,
    BuildResponse,
    ProgramMeta,
    SessionMode,
    SessionSource,
)


class TestBuildRequestNormalization:
    """§9 p.1.5.1: input without mode/source gets defaults;
    empty topics or session_id gives predictable error."""

    def test_defaults_applied_when_missing(self):
        payload = {
            "session_id": "abc-123",
            "params": {"topics": ["nlp"], "level": "middle"},
        }
        req = BuildRequest.from_raw_payload(payload)
        assert req.params.mode == SessionMode.INTERVIEW
        assert req.params.source == SessionSource.MANUAL

    def test_mode_from_legacy_type_field(self):
        payload = {
            "session_id": "abc-123",
            "params": {"topics": ["nlp"], "level": "middle", "type": "training"},
        }
        req = BuildRequest.from_raw_payload(payload)
        assert req.params.mode == SessionMode.TRAINING

    def test_explicit_mode_takes_precedence(self):
        payload = {
            "session_id": "abc-123",
            "params": {
                "topics": ["nlp"],
                "level": "middle",
                "type": "ml",
                "mode": "study",
            },
        }
        req = BuildRequest.from_raw_payload(payload)
        assert req.params.mode == SessionMode.STUDY

    def test_empty_session_id_raises(self):
        payload = {"session_id": "", "params": {"topics": ["nlp"], "level": "middle"}}
        with pytest.raises(Exception):
            BuildRequest.from_raw_payload(payload)

    def test_empty_topics_raises(self):
        payload = {
            "session_id": "abc-123",
            "params": {"topics": [], "level": "middle"},
        }
        with pytest.raises(Exception):
            BuildRequest.from_raw_payload(payload)

    def test_whitespace_only_topics_raises(self):
        payload = {
            "session_id": "abc-123",
            "params": {"topics": ["", " "], "level": "middle"},
        }
        with pytest.raises(Exception):
            BuildRequest.from_raw_payload(payload)

    def test_unsupported_level_raises(self):
        payload = {
            "session_id": "abc-123",
            "params": {"topics": ["ml"], "level": "expert"},
        }
        with pytest.raises(Exception):
            BuildRequest.from_raw_payload(payload)

    def test_topics_string_coerced_to_list(self):
        payload = {
            "session_id": "abc-123",
            "params": {"topics": "nlp", "level": "middle"},
        }
        req = BuildRequest.from_raw_payload(payload)
        assert req.params.topics == ["nlp"]

    def test_full_valid_training_payload(self):
        payload = {
            "session_id": "9f1b66b7-6f4a-4f98-a4ee-1b307a428f4f",
            "params": {
                "type": "ml",
                "level": "middle",
                "topics": ["nlp"],
                "mode": "training",
                "source": "manual",
                "weak_topics": ["nlp"],
            },
        }
        req = BuildRequest.from_raw_payload(payload)
        assert req.params.mode == SessionMode.TRAINING
        assert req.params.weak_topics == ["nlp"]


class TestProgramMetaSerialization:
    """§9 p.1.5.3: program_meta always present in response;
    on failure validation_passed=false and meaningful fallback_reason."""

    def test_success_meta(self):
        meta = ProgramMeta(
            validation_passed=True,
            coverage={"nlp": 1, "llm": 1},
        )
        d = meta.model_dump()
        assert d["validation_passed"] is True
        assert d["fallback_reason"] is None
        assert d["generator_version"] == "planner-1.0.0"

    def test_failure_meta(self):
        meta = ProgramMeta(
            validation_passed=False,
            fallback_reason="missing_coverage:mlops",
        )
        d = meta.model_dump()
        assert d["validation_passed"] is False
        assert "mlops" in d["fallback_reason"]

    def test_build_response_always_has_meta(self):
        resp = BuildResponse(
            session_id="test-id",
            interview_program={"questions": []},
            program_meta=ProgramMeta(),
        )
        payload = resp.to_kafka_payload()
        assert "program_meta" in payload
        assert payload["program_meta"]["validation_passed"] is True
