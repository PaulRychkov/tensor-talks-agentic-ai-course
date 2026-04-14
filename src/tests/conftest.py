# -*- coding: utf-8 -*-
"""Shared fixtures for integration API tests (docker-compose stack).

Environment variables (optional):
  BFF_BASE_URL          default http://localhost:8080
  RESULTS_CRUD_URL      default http://localhost:8088
  QUESTIONS_CRUD_URL    default http://localhost:8091
  KNOWLEDGE_CRUD_URL    default http://localhost:8090
  KNOWLEDGE_PRODUCER_URL default http://localhost:8092
  AGENT_SERVICE_URL     default http://localhost:8093
  DIALOGUE_AGGREGATOR_URL default http://localhost:8084
  FRONTEND_URL          default http://localhost:5173
  SESSION_SERVICE_URL   default http://localhost:8083

Run: pytest tests/ -v -m api
Skip live check: API_SKIP_LIVE_CHECK=1 pytest tests/ -v
"""

from __future__ import annotations

import os

import pytest
import requests


def pytest_configure(config):
    config.addinivalue_line(
        "markers", "api: integration tests against running HTTP services"
    )


@pytest.fixture(scope="session")
def bff_url() -> str:
    return os.environ.get("BFF_BASE_URL", "http://localhost:8080").rstrip("/")


@pytest.fixture(scope="session")
def service_urls() -> dict:
    return {
        "results_crud": os.environ.get("RESULTS_CRUD_URL", "http://localhost:8088").rstrip("/"),
        "questions_crud": os.environ.get("QUESTIONS_CRUD_URL", "http://localhost:8091").rstrip("/"),
        "knowledge_crud": os.environ.get("KNOWLEDGE_CRUD_URL", "http://localhost:8090").rstrip("/"),
        "knowledge_producer": os.environ.get(
            "KNOWLEDGE_PRODUCER_URL", "http://localhost:8092"
        ).rstrip("/"),
        "agent": os.environ.get("AGENT_SERVICE_URL", "http://localhost:8093").rstrip("/"),
        "dialogue_aggregator": os.environ.get("DIALOGUE_AGGREGATOR_URL", "http://localhost:8084").rstrip("/"),
        "frontend": os.environ.get("FRONTEND_URL", "http://localhost:5173").rstrip("/"),
        "session": os.environ.get("SESSION_SERVICE_URL", "http://localhost:8083").rstrip("/"),
    }


@pytest.fixture(scope="session")
def live(bff_url: str) -> None:
    """Fail fast if BFF is not reachable (unless API_SKIP_LIVE_CHECK=1)."""
    if os.environ.get("API_SKIP_LIVE_CHECK", "").lower() in ("1", "true", "yes"):
        return
    try:
        r = requests.post(
            f"{bff_url}/api/auth/login",
            json={},
            timeout=5,
        )
        assert r.status_code in (400, 401), f"unexpected status {r.status_code}"
    except (requests.RequestException, AssertionError) as e:
        pytest.skip(f"BFF not reachable at {bff_url}: {e}")


@pytest.fixture
def auth_user(bff_url: str, live) -> dict:
    """Fresh registered user per test function."""
    from tests.helpers import register_user as _reg

    return _reg(bff_url)


@pytest.fixture
def auth_headers(auth_user: dict) -> dict:
    return {
        "Authorization": f"Bearer {auth_user['access_token']}",
        "Content-Type": "application/json",
    }


@pytest.fixture
def started_session(bff_url: str, auth_headers: dict, live) -> str:
    """Create one interview session; return session_id (may take up to ~30s)."""
    r = requests.post(
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
    assert r.status_code == 201, r.text
    return r.json()["session_id"]
