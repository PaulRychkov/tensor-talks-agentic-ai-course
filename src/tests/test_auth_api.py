# -*- coding: utf-8 -*-
"""BFF /api/auth/* integration tests."""

from __future__ import annotations

import pytest
import requests

pytestmark = [pytest.mark.api, pytest.mark.usefixtures("live")]


def test_register_success(bff_url: str):
    from tests.helpers import register_user

    u = register_user(bff_url)
    assert u["access_token"]
    assert u["user_id"]


def test_register_duplicate_returns_409(bff_url: str):
    from tests.helpers import register_user

    u = register_user(bff_url)
    dup = requests.post(
        f"{bff_url}/api/auth/register",
        json={"login": u["login"], "password": "OtherPass456!"},
        timeout=10,
    )
    assert dup.status_code == 409
    assert "exists" in dup.text.lower() or "error" in dup.text.lower()


def test_register_invalid_payload(bff_url: str):
    r = requests.post(f"{bff_url}/api/auth/register", json={}, timeout=5)
    assert r.status_code == 400


def test_login_success(bff_url: str):
    from tests.helpers import register_user

    u = register_user(bff_url)
    r = requests.post(
        f"{bff_url}/api/auth/login",
        json={"login": u["login"], "password": u["password"]},
        timeout=10,
    )
    assert r.status_code == 200
    body = r.json()
    assert "tokens" in body
    assert body["tokens"]["access_token"]


def test_login_wrong_password_401(bff_url: str):
    from tests.helpers import register_user

    u = register_user(bff_url)
    r = requests.post(
        f"{bff_url}/api/auth/login",
        json={"login": u["login"], "password": "WrongPassword!!!"},
        timeout=10,
    )
    assert r.status_code == 401


def test_login_invalid_payload_400(bff_url: str):
    r = requests.post(f"{bff_url}/api/auth/login", json={"login": "x"}, timeout=5)
    assert r.status_code == 400


def test_refresh_success(bff_url: str):
    from tests.helpers import register_user

    u = register_user(bff_url)
    r = requests.post(
        f"{bff_url}/api/auth/refresh",
        json={"refresh_token": u["refresh_token"]},
        timeout=10,
    )
    assert r.status_code == 200
    assert "tokens" in r.json()


def test_refresh_invalid_token_401(bff_url: str):
    r = requests.post(
        f"{bff_url}/api/auth/refresh",
        json={"refresh_token": "invalid.token.here"},
        timeout=10,
    )
    assert r.status_code == 401


def test_refresh_invalid_payload_400(bff_url: str):
    r = requests.post(f"{bff_url}/api/auth/refresh", json={}, timeout=5)
    assert r.status_code == 400


def test_me_with_valid_token(bff_url: str, auth_user: dict):
    r = requests.get(
        f"{bff_url}/api/auth/me",
        headers={"Authorization": f"Bearer {auth_user['access_token']}"},
        timeout=10,
    )
    assert r.status_code == 200
    assert "user" in r.json()


def test_me_missing_token_401(bff_url: str):
    r = requests.get(f"{bff_url}/api/auth/me", timeout=5)
    assert r.status_code == 401


def test_me_garbage_token_401(bff_url: str):
    r = requests.get(
        f"{bff_url}/api/auth/me",
        headers={"Authorization": "Bearer not-a-jwt"},
        timeout=10,
    )
    assert r.status_code == 401
