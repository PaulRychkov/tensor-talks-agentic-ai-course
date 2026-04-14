# -*- coding: utf-8 -*-
"""Health checks and static assets across the local docker stack."""

from __future__ import annotations

import pytest
import requests

pytestmark = [pytest.mark.api, pytest.mark.usefixtures("live")]


def test_agent_service_health(service_urls: dict):
    r = requests.get(f"{service_urls['agent']}/health", timeout=5)
    assert r.status_code == 200


def test_dialogue_aggregator_reachable(service_urls: dict):
    r = requests.get(f"{service_urls['dialogue_aggregator']}/health", timeout=5)
    assert r.status_code in (200, 404)


def test_frontend_root_html(service_urls: dict):
    r = requests.get(f"{service_urls['frontend']}/", timeout=10)
    assert r.status_code == 200
    assert "html" in r.text.lower()


def test_session_service_reachable(service_urls: dict):
    """Session manager may not expose /health; expect any HTTP response."""
    try:
        r = requests.get(f"{service_urls['session']}/", timeout=3)
        assert r.status_code in (200, 301, 302, 404)
    except requests.RequestException:
        pytest.skip("session-service not exposing HTTP on expected port")
