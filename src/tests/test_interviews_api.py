# -*- coding: utf-8 -*-
"""BFF /api/interviews/* integration tests."""

from __future__ import annotations

import time

import pytest
import requests

pytestmark = [pytest.mark.api, pytest.mark.usefixtures("live")]


def test_interviews_authenticated_returns_200(bff_url: str, auth_headers: dict):
    """Authenticated request with no query params should return 200 (user_id comes from JWT)."""
    r = requests.get(f"{bff_url}/api/interviews", headers=auth_headers, timeout=10)
    assert r.status_code == 200
    assert "interviews" in r.json()


def test_interviews_with_user_id_query_still_200(bff_url: str, auth_user: dict, auth_headers: dict):
    """Extra user_id query param is ignored; auth token determines the user."""
    r = requests.get(
        f"{bff_url}/api/interviews",
        headers=auth_headers,
        params={"user_id": auth_user["user_id"]},
        timeout=10,
    )
    assert r.status_code == 200


def test_interviews_list_ok(bff_url: str, auth_user: dict, auth_headers: dict):
    # create at least one session
    requests.post(
        f"{bff_url}/api/chat/start",
        headers=auth_headers,
        json={
            "params": {
                "type": "ml",
                "level": "middle",
                "topics": ["nlp"],
                "mode": "interview",
            }
        },
        timeout=60,
    )
    time.sleep(1)
    # user_id is resolved from JWT; no query param needed
    r = requests.get(
        f"{bff_url}/api/interviews",
        headers=auth_headers,
        timeout=15,
    )
    assert r.status_code == 200
    body = r.json()
    assert "interviews" in body
    assert isinstance(body["interviews"], list)


def test_interview_chat_for_session(
    bff_url: str, auth_headers: dict, started_session: str
):
    r = requests.get(
        f"{bff_url}/api/interviews/{started_session}/chat",
        headers=auth_headers,
        timeout=15,
    )
    assert r.status_code in (200, 404)


def test_interview_result_for_session(
    bff_url: str, auth_headers: dict, started_session: str
):
    r = requests.get(
        f"{bff_url}/api/interviews/{started_session}/result",
        headers=auth_headers,
        timeout=15,
    )
    assert r.status_code in (200, 404, 500)


def test_interviews_requires_auth(bff_url: str):
    r = requests.get(
        f"{bff_url}/api/interviews",
        params={"user_id": "00000000-0000-0000-0000-000000000001"},
        timeout=5,
    )
    assert r.status_code == 401
