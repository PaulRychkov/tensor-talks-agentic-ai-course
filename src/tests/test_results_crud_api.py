# -*- coding: utf-8 -*-
"""results-crud-service HTTP API (direct) integration tests."""

from __future__ import annotations

import uuid

import pytest
import requests

pytestmark = [pytest.mark.api, pytest.mark.usefixtures("live")]


def test_results_healthz(service_urls: dict):
    r = requests.get(f"{service_urls['results_crud']}/healthz", timeout=5)
    assert r.status_code == 200


def test_results_create_and_get(service_urls: dict, auth_user: dict):
    base = service_urls["results_crud"]
    sid = str(uuid.uuid4())
    payload = {
        "session_id": sid,
        "user_id": auth_user["user_id"],
        "score": 82,
        "feedback": "API test result",
        "session_kind": "interview",
    }
    c = requests.post(f"{base}/results", json=payload, timeout=10)
    assert c.status_code == 201, c.text

    g = requests.get(f"{base}/results/{sid}", timeout=10)
    assert g.status_code == 200
    data = g.json()
    assert "result" in data
    assert data["result"]["score"] == 82


def test_results_get_many(service_urls: dict, auth_user: dict):
    base = service_urls["results_crud"]
    ids = [str(uuid.uuid4()) for _ in range(2)]
    for sid in ids:
        requests.post(
            f"{base}/results",
            json={
                "session_id": sid,
                "user_id": auth_user["user_id"],
                "score": 50,
                "feedback": "multi",
            },
            timeout=10,
        ).raise_for_status()

    r = requests.get(
        f"{base}/results",
        params={"session_ids": ",".join(ids)},
        timeout=10,
    )
    assert r.status_code == 200
    results = r.json().get("results", [])
    assert len(results) >= 2


def test_results_get_many_missing_param_400(service_urls: dict):
    r = requests.get(f"{service_urls['results_crud']}/results", timeout=5)
    assert r.status_code == 400


def test_results_create_missing_session_400(service_urls: dict):
    r = requests.post(
        f"{service_urls['results_crud']}/results",
        json={"score": 1, "feedback": "x"},
        timeout=5,
    )
    assert r.status_code == 400


def test_results_report_json_validation_400(service_urls: dict, auth_user: dict):
    base = service_urls["results_crud"]
    sid = str(uuid.uuid4())
    # Must be a JSON object at key report_json (not a string), or Go skips structure validation
    bad_report = {"summary": "only summary"}
    r = requests.post(
        f"{base}/results",
        json={
            "session_id": sid,
            "user_id": auth_user["user_id"],
            "score": 10,
            "feedback": "x",
            "report_json": bad_report,
        },
        timeout=10,
    )
    assert r.status_code == 400


def test_results_study_session_kind(service_urls: dict, auth_user: dict):
    base = service_urls["results_crud"]
    sid = str(uuid.uuid4())
    r = requests.post(
        f"{base}/results",
        json={
            "session_id": sid,
            "user_id": auth_user["user_id"],
            "score": 60,
            "feedback": "study",
            "session_kind": "study",
        },
        timeout=10,
    )
    assert r.status_code == 201


def test_preset_create_and_list(service_urls: dict, auth_user: dict):
    base = service_urls["results_crud"]
    uid = auth_user["user_id"]
    c = requests.post(
        f"{base}/presets",
        json={
            "user_id": uid,
            "target_mode": "study",
            "topics": ["nlp"],
            "materials": ["doc1"],
        },
        timeout=10,
    )
    if c.status_code == 404:
        pytest.skip("results-crud image without /presets routes")
    assert c.status_code in (200, 201), c.text

    lst = requests.get(f"{base}/presets", params={"user_id": uid}, timeout=10)
    assert lst.status_code == 200


def test_user_progress_upsert_and_get(service_urls: dict, auth_user: dict):
    base = service_urls["results_crud"]
    uid = auth_user["user_id"]
    r = requests.post(
        f"{base}/user-progress",
        json={
            "user_id": uid,
            "topic_id": "pytest_topic_ml",
        },
        timeout=10,
    )
    if r.status_code == 404:
        pytest.skip("results-crud image without /user-progress routes")
    assert r.status_code == 200

    g = requests.get(f"{base}/user-progress", params={"user_id": uid}, timeout=10)
    assert g.status_code == 200
    assert "progress" in g.json()


def test_user_progress_missing_user_id_400(service_urls: dict):
    r = requests.get(f"{service_urls['results_crud']}/user-progress", timeout=5)
    # Gin may return 404 for missing route vs 400 for handler — accept both
    assert r.status_code in (400, 404)
