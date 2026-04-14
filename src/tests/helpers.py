# -*- coding: utf-8 -*-
"""Shared helpers for API tests."""

from __future__ import annotations

import uuid

import requests


def register_user(bff_url: str, suffix: str | None = None) -> dict:
    """Register + login; return access_token, refresh_token, user_id, login, password.

    Login must be 3–30 chars (auth-service validateCredentials).
    """
    uniq = (suffix or uuid.uuid4().hex)[:20]
    login = f"u{uniq}"[:30]
    password = "TestPass123!"
    reg = requests.post(
        f"{bff_url.rstrip('/')}/api/auth/register",
        json={"login": login, "password": password},
        timeout=15,
    )
    assert reg.status_code in (201, 409), reg.text
    if reg.status_code == 409:
        return register_user(bff_url, suffix=uuid.uuid4().hex)
    log = requests.post(
        f"{bff_url.rstrip('/')}/api/auth/login",
        json={"login": login, "password": password},
        timeout=15,
    )
    log.raise_for_status()
    data = log.json()
    return {
        "login": login,
        "password": password,
        "access_token": data["tokens"]["access_token"],
        "refresh_token": data["tokens"]["refresh_token"],
        "user_id": data["user"]["id"],
    }
