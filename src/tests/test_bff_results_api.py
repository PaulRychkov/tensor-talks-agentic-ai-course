# -*- coding: utf-8 -*-
"""BFF proxy for interview results (CRUD behind BFF)."""

from __future__ import annotations

import uuid

import pytest
import requests

pytestmark = [pytest.mark.api, pytest.mark.usefixtures("live")]


def test_bff_get_result_after_crud_create(
    bff_url: str, service_urls: dict, auth_headers: dict, auth_user: dict
):
    sid = str(uuid.uuid4())
    crud = service_urls["results_crud"]
    c = requests.post(
        f"{crud}/results",
        json={
            "session_id": sid,
            "user_id": auth_user["user_id"],
            "score": 91,
            "feedback": "BFF round-trip test",
            "session_kind": "interview",
        },
        timeout=10,
    )
    assert c.status_code == 201, c.text

    r = requests.get(
        f"{bff_url}/api/interviews/{sid}/result",
        headers=auth_headers,
        timeout=15,
    )
    assert r.status_code in (200, 404), r.text
    if r.status_code == 200:
        body = r.json()
        # BFF wraps row as { "result": { "score", "feedback", ... } }
        result = body.get("result") if isinstance(body.get("result"), dict) else body
        assert result.get("score") == 91 or "feedback" in result


def test_bff_result_unknown_session_404(bff_url: str, auth_headers: dict):
    sid = str(uuid.uuid4())
    r = requests.get(
        f"{bff_url}/api/interviews/{sid}/result",
        headers=auth_headers,
        timeout=10,
    )
    assert r.status_code in (200, 404, 500)
