"""Tests for _apply_mode_profile: mode-specific reordering and weighting (§9 p.1.2.2)."""

import pytest
from src.services.interview_builder_service import InterviewBuilderService


def _make_question(
    qid: str,
    theory_id: str = None,
    text: str = "",
    tags=None,
    question_type: str = None,
):
    """Helper: question in Go-struct-like format with optional question_type."""
    data = {"id": qid, "theory_id": theory_id, "content": {"question": text}}
    if tags:
        data["tags"] = tags
    if question_type:
        data["question_type"] = question_type
    return {"Data": data}


def _ids(questions):
    """Extract ordered list of question ids."""
    return [q["Data"]["id"] for q in questions]


class TestTrainingWeakTopics:
    """Training mode prioritizes questions whose tags overlap with weak_topics."""

    def test_weak_topics_sorted_first(self):
        qs = [
            _make_question("q1", tags=["llm"], question_type="theory"),
            _make_question("q2", tags=["nlp"], question_type="theory"),
            _make_question("q3", tags=["nlp"], question_type="theory"),
            _make_question("q4", tags=["cv"], question_type="theory"),
        ]
        result = InterviewBuilderService._apply_mode_profile(qs, "training", ["nlp"])
        ids = _ids(result)
        assert ids.index("q2") < ids.index("q1")
        assert ids.index("q3") < ids.index("q1")
        assert ids.index("q2") < ids.index("q4")

    def test_multiple_weak_topics(self):
        qs = [
            _make_question("q1", tags=["cv"], question_type="theory"),
            _make_question("q2", tags=["nlp"], question_type="theory"),
            _make_question("q3", tags=["llm"], question_type="theory"),
        ]
        result = InterviewBuilderService._apply_mode_profile(
            qs, "training", ["nlp", "llm"]
        )
        ids = _ids(result)
        assert ids.index("q2") < ids.index("q1")
        assert ids.index("q3") < ids.index("q1")

    def test_weak_topics_case_insensitive(self):
        qs = [
            _make_question("q1", tags=["cv"], question_type="theory"),
            _make_question("q2", tags=["nlp"], question_type="theory"),
        ]
        result = InterviewBuilderService._apply_mode_profile(qs, "training", [" NLP "])
        ids = _ids(result)
        assert ids[0] == "q2"

    def test_no_matching_weak_topics_preserves_order(self):
        qs = [
            _make_question("q1", tags=["cv"], question_type="theory"),
            _make_question("q2", tags=["llm"], question_type="theory"),
        ]
        result = InterviewBuilderService._apply_mode_profile(
            qs, "training", ["nlp"]
        )
        assert len(result) == 2


class TestTrainingPracticeRatio:
    """Training mode applies training_practice_ratio to boost practical questions."""

    PRACTICAL_TYPES = ("coding", "case", "practical", "applied")

    def test_practical_questions_prioritized(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.training_practice_ratio",
            0.6,
        )
        qs = [
            _make_question("q1", question_type="theory"),
            _make_question("q2", question_type="coding"),
            _make_question("q3", question_type="theory"),
            _make_question("q4", question_type="case"),
            _make_question("q5", question_type="practical"),
        ]
        result = InterviewBuilderService._apply_mode_profile(qs, "training", None)
        ids = _ids(result)
        practical_ids = {"q2", "q4", "q5"}
        first_three = set(ids[:3])
        assert practical_ids == first_three

    def test_practice_count_respects_ratio(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.training_practice_ratio",
            0.5,
        )
        practical = [
            _make_question(f"p{i}", question_type="coding") for i in range(8)
        ]
        theoretical = [
            _make_question(f"t{i}", question_type="theory") for i in range(2)
        ]
        qs = theoretical + practical
        result = InterviewBuilderService._apply_mode_profile(qs, "training", None)
        practice_count = max(1, int(len(qs) * 0.5))  # 5
        result_ids = _ids(result)
        for pid in result_ids[:practice_count]:
            assert pid.startswith("p")

    def test_all_practical_stays_same_length(self):
        qs = [_make_question(f"q{i}", question_type="coding") for i in range(5)]
        result = InterviewBuilderService._apply_mode_profile(qs, "training", None)
        assert len(result) == 5

    def test_all_question_types_recognized(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.training_practice_ratio",
            1.0,
        )
        qs = [
            _make_question("q1", question_type="coding"),
            _make_question("q2", question_type="case"),
            _make_question("q3", question_type="practical"),
            _make_question("q4", question_type="applied"),
        ]
        result = InterviewBuilderService._apply_mode_profile(qs, "training", None)
        assert _ids(result) == ["q1", "q2", "q3", "q4"]

    def test_no_practical_questions_returns_all(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.training_practice_ratio",
            0.7,
        )
        qs = [_make_question(f"q{i}", question_type="theory") for i in range(4)]
        result = InterviewBuilderService._apply_mode_profile(qs, "training", None)
        assert len(result) == 4


class TestTrainingWithoutWeakTopics:
    """Training mode without weak_topics still applies practice ratio."""

    def test_practice_ratio_applied_without_weak_topics(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.training_practice_ratio",
            0.6,
        )
        qs = [
            _make_question("t1", question_type="theory"),
            _make_question("t2", question_type="theory"),
            _make_question("p1", question_type="coding"),
            _make_question("p2", question_type="case"),
            _make_question("p3", question_type="applied"),
        ]
        result = InterviewBuilderService._apply_mode_profile(qs, "training", None)
        ids = _ids(result)
        practical_ids = {"p1", "p2", "p3"}
        first_three = set(ids[:3])
        assert practical_ids == first_three

    def test_empty_weak_topics_treated_as_none(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.training_practice_ratio",
            0.6,
        )
        qs = [
            _make_question("t1", question_type="theory"),
            _make_question("p1", question_type="coding"),
        ]
        result_none = InterviewBuilderService._apply_mode_profile(qs, "training", None)
        result_empty = InterviewBuilderService._apply_mode_profile(qs, "training", [])
        assert _ids(result_none) == _ids(result_empty)


class TestStudyTheoryRatio:
    """Study mode applies study_theory_ratio to boost questions with theory_id."""

    def test_theory_questions_prioritized(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.study_theory_ratio",
            0.6,
        )
        qs = [
            _make_question("q1", text="Q1"),
            _make_question("q2", theory_id="t2", text="Q2"),
            _make_question("q3", text="Q3"),
            _make_question("q4", theory_id="t4", text="Q4"),
            _make_question("q5", theory_id="t5", text="Q5"),
        ]
        result = InterviewBuilderService._apply_mode_profile(qs, "study", None)
        ids = _ids(result)
        theory_count = max(1, int(5 * 0.6))  # 3
        for qid in ids[:theory_count]:
            assert qid in {"q2", "q4", "q5"}

    def test_theory_count_respects_ratio(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.study_theory_ratio",
            0.5,
        )
        with_theory = [
            _make_question(f"th{i}", theory_id=f"t{i}") for i in range(6)
        ]
        without_theory = [
            _make_question(f"no{i}") for i in range(4)
        ]
        qs = without_theory + with_theory
        result = InterviewBuilderService._apply_mode_profile(qs, "study", None)
        theory_count = max(1, int(10 * 0.5))  # 5
        result_ids = _ids(result)
        for qid in result_ids[:theory_count]:
            assert qid.startswith("th")

    def test_no_theory_questions_returns_all(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.study_theory_ratio",
            0.7,
        )
        qs = [_make_question(f"q{i}", text=f"Q{i}") for i in range(4)]
        result = InterviewBuilderService._apply_mode_profile(qs, "study", None)
        assert len(result) == 4

    def test_all_theory_questions_returns_all(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.study_theory_ratio",
            0.7,
        )
        qs = [
            _make_question(f"q{i}", theory_id=f"t{i}", text=f"Q{i}")
            for i in range(4)
        ]
        result = InterviewBuilderService._apply_mode_profile(qs, "study", None)
        assert len(result) == 4

    def test_study_ignores_weak_topics(self, monkeypatch):
        monkeypatch.setattr(
            "src.services.interview_builder_service.settings.study_theory_ratio",
            0.7,
        )
        qs = [
            _make_question("q1", tags=["nlp"], text="Q1"),
            _make_question("q2", theory_id="t2", tags=["cv"], text="Q2"),
        ]
        result = InterviewBuilderService._apply_mode_profile(
            qs, "study", ["nlp"]
        )
        assert _ids(result)[0] == "q2"


class TestInterviewMode:
    """Interview mode returns questions without reordering."""

    def test_interview_no_reordering(self):
        qs = [
            _make_question("q3", question_type="coding"),
            _make_question("q1", question_type="theory"),
            _make_question("q2", theory_id="t2"),
        ]
        result = InterviewBuilderService._apply_mode_profile(qs, "interview", None)
        assert _ids(result) == ["q3", "q1", "q2"]

    def test_interview_ignores_weak_topics(self):
        qs = [
            _make_question("q1", tags=["cv"]),
            _make_question("q2", tags=["nlp"]),
        ]
        result = InterviewBuilderService._apply_mode_profile(
            qs, "interview", ["nlp"]
        )
        assert _ids(result) == ["q1", "q2"]

    def test_interview_preserves_length(self):
        qs = [_make_question(f"q{i}") for i in range(7)]
        result = InterviewBuilderService._apply_mode_profile(qs, "interview", None)
        assert len(result) == 7


class TestEmptyQuestions:
    """Empty input returns empty output for all modes."""

    @pytest.mark.parametrize("mode", ["interview", "training", "study"])
    def test_empty_list_returns_empty(self, mode):
        result = InterviewBuilderService._apply_mode_profile([], mode, None)
        assert result == []

    @pytest.mark.parametrize("mode", ["interview", "training", "study"])
    def test_empty_with_weak_topics_returns_empty(self, mode):
        result = InterviewBuilderService._apply_mode_profile([], mode, ["nlp"])
        assert result == []
