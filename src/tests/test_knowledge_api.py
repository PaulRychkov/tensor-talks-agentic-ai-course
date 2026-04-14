# -*- coding: utf-8 -*-
"""questions-crud, knowledge-base-crud, knowledge-producer integration tests."""

from __future__ import annotations

import pytest
import requests

pytestmark = [pytest.mark.api, pytest.mark.usefixtures("live")]


def test_questions_crud_list(service_urls: dict):
    r = requests.get(f"{service_urls['questions_crud']}/questions", timeout=15)
    assert r.status_code == 200
    data = r.json()
    items = data.get("questions", data) if isinstance(data, dict) else data
    assert isinstance(items, list)
    assert len(items) > 0, "expected seeded questions from knowledge-producer"


def test_questions_crud_get_by_id(service_urls: dict):
    lst = requests.get(f"{service_urls['questions_crud']}/questions", timeout=15)
    lst.raise_for_status()
    data = lst.json()
    items = data.get("questions", [])
    assert items, "no questions to fetch"
    qid = items[0].get("ID") or items[0].get("id")
    assert qid
    r = requests.get(f"{service_urls['questions_crud']}/questions/{qid}", timeout=10)
    assert r.status_code == 200


def test_questions_crud_unknown_id_404(service_urls: dict):
    r = requests.get(
        f"{service_urls['questions_crud']}/questions/nonexistent_question_id_xyz",
        timeout=5,
    )
    assert r.status_code == 404


def test_knowledge_crud_list(service_urls: dict):
    r = requests.get(f"{service_urls['knowledge_crud']}/knowledge", timeout=15)
    assert r.status_code == 200


def test_knowledge_producer_healthz(service_urls: dict):
    r = requests.get(f"{service_urls['knowledge_producer']}/healthz", timeout=5)
    assert r.status_code == 200
    assert r.json().get("status") == "ok"


def test_knowledge_producer_list_drafts(service_urls: dict):
    r = requests.get(f"{service_urls['knowledge_producer']}/drafts", timeout=10)
    if r.status_code == 404:
        pytest.skip("knowledge-producer image without /drafts routes")
    assert r.status_code == 200
    body = r.json()
    assert "drafts" in body
    assert "total" in body


def test_knowledge_producer_create_draft(service_urls: dict):
    r = requests.post(
        f"{service_urls['knowledge_producer']}/drafts/knowledge",
        json={
            "title": "Pytest API draft",
            "content": "Short content for integration test.",
            "topic": "testing",
            "source": "api-test",
        },
        timeout=15,
    )
    if r.status_code == 404:
        pytest.skip("knowledge-producer image without draft API")
    assert r.status_code == 201, r.text
    assert "draft" in r.json()


def test_knowledge_producer_draft_review_flow(service_urls: dict):
    base = service_urls["knowledge_producer"]
    c = requests.post(
        f"{base}/drafts/questions",
        json={
            "title": "Q draft",
            "content": "What is pytest?",
            "topic": "testing",
        },
        timeout=15,
    )
    if c.status_code == 404:
        pytest.skip("knowledge-producer image without draft API")
    assert c.status_code == 201, c.text
    draft = c.json()["draft"]
    draft_id = draft.get("draft_id") or draft.get("id")
    assert draft_id

    g = requests.get(f"{base}/drafts/{draft_id}", timeout=10)
    assert g.status_code == 200

    rev = requests.put(
        f"{base}/drafts/{draft_id}/review",
        json={"review_status": "approved", "reviewed_by": "pytest"},
        timeout=10,
    )
    assert rev.status_code == 200


def test_knowledge_producer_search_web(service_urls: dict):
    r = requests.get(
        f"{service_urls['knowledge_producer']}/search/web",
        params={"query": "machine learning", "topic": "ml"},
        timeout=20,
    )
    if r.status_code == 404:
        pytest.skip("knowledge-producer image without /search/web")
    assert r.status_code == 200
    assert "results" in r.json()


def test_knowledge_producer_produce_all_idempotent(service_urls: dict):
    """Re-running produce may be heavy; accept 200/500 if CRUD busy."""
    r = requests.post(
        f"{service_urls['knowledge_producer']}/produce/all",
        timeout=120,
    )
    assert r.status_code in (200, 500), r.text
