# -*- coding: utf-8 -*-
"""BFF /api/chat/* integration tests."""

from __future__ import annotations

import time
import uuid

import pytest
import requests

pytestmark = [pytest.mark.api, pytest.mark.usefixtures("live")]


def test_chat_start_interview_201(bff_url: str, auth_headers: dict):
    r = requests.post(
        f"{bff_url}/api/chat/start",
        headers=auth_headers,
        json={
            "params": {
                "type": "ml",
                "level": "middle",
                "topics": ["nlp", "cv"],
                "mode": "interview",
            }
        },
        timeout=60,
    )
    assert r.status_code == 201, r.text
    body = r.json()
    assert body.get("ready") is True
    assert body.get("session_id")


@pytest.mark.parametrize(
    "mode,extra",
    [
        ("training", {"weak_topics": ["nlp"]}),
        ("study", {}),
    ],
)
def test_chat_start_modes_201(bff_url: str, auth_headers: dict, mode: str, extra: dict):
    params = {
        "type": "ml",
        "level": "middle",
        "topics": ["nlp"],
        "mode": mode,
        **extra,
    }
    r = requests.post(
        f"{bff_url}/api/chat/start",
        headers=auth_headers,
        json={"params": params},
        timeout=60,
    )
    assert r.status_code == 201, r.text


def test_chat_start_requires_auth_401(bff_url: str):
    r = requests.post(
        f"{bff_url}/api/chat/start",
        json={"params": {"type": "ml", "level": "middle", "topics": ["nlp"]}},
        timeout=10,
    )
    assert r.status_code == 401


def test_chat_start_invalid_body_400(bff_url: str, auth_headers: dict):
    r = requests.post(
        f"{bff_url}/api/chat/start",
        headers=auth_headers,
        json={},
        timeout=20,
    )
    assert r.status_code == 400


def test_chat_message_and_question(
    bff_url: str, auth_headers: dict, started_session: str
):
    msg = requests.post(
        f"{bff_url}/api/chat/message",
        headers=auth_headers,
        json={
            "session_id": started_session,
            "content": "Ready to answer technical questions.",
        },
        timeout=20,
    )
    assert msg.status_code in (200, 201, 202), msg.text

    time.sleep(2)
    q = requests.get(
        f"{bff_url}/api/chat/{started_session}/question",
        headers=auth_headers,
        timeout=15,
    )
    assert q.status_code in (200, 204, 404)


def test_chat_results_endpoint(bff_url: str, auth_headers: dict, started_session: str):
    r = requests.get(
        f"{bff_url}/api/chat/{started_session}/results",
        headers=auth_headers,
        timeout=15,
    )
    assert r.status_code in (200, 404)


def test_chat_message_empty_content_rejected_or_ok(
    bff_url: str, auth_headers: dict, started_session: str
):
    r = requests.post(
        f"{bff_url}/api/chat/message",
        headers=auth_headers,
        json={"session_id": started_session, "content": ""},
        timeout=15,
    )
    assert r.status_code in (200, 400, 422)


def test_chat_message_missing_session_400(bff_url: str, auth_headers: dict):
    r = requests.post(
        f"{bff_url}/api/chat/message",
        headers=auth_headers,
        json={"content": "hello"},
        timeout=10,
    )
    assert r.status_code == 400


def test_chat_resume_invalid_uuid_400(bff_url: str, auth_headers: dict):
    r = requests.post(
        f"{bff_url}/api/chat/not-a-uuid/resume",
        headers=auth_headers,
        timeout=10,
    )
    assert r.status_code == 400


def test_chat_terminate_random_session(
    bff_url: str, auth_headers: dict
):
    sid = str(uuid.uuid4())
    r = requests.post(
        f"{bff_url}/api/chat/{sid}/terminate",
        headers=auth_headers,
        timeout=15,
    )
    # May be 404/400/200 depending on session existence
    assert r.status_code in (200, 400, 404, 500)
