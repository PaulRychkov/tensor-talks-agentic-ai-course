#!/usr/bin/env python3
"""
End-to-end system test: from registration → login → start interview →
send messages → get agent response → terminate → check results.

Run: python tests/e2e_full_flow.py
"""

import sys
import time
import uuid
import requests

BFF = "http://localhost:8080"
RESULTS_CRUD = "http://localhost:8088"

PASS = "✅"
FAIL = "❌"
INFO = "ℹ️ "

errors = []


def check(label: str, condition: bool, detail: str = ""):
    if condition:
        print(f"  {PASS} {label}")
    else:
        msg = f"  {FAIL} {label}" + (f": {detail}" if detail else "")
        print(msg)
        errors.append(msg)


def step(name: str):
    print(f"\n{'─'*60}")
    print(f"  {name}")
    print(f"{'─'*60}")


# ── 1. REGISTRATION ────────────────────────────────────────────────────────────
step("1. Registration")

login = f"e2e{uuid.uuid4().hex[:12]}"
password = "E2ePassword123!"

r = requests.post(f"{BFF}/api/auth/register", json={"login": login, "password": password}, timeout=15)
check("POST /api/auth/register → 201", r.status_code == 201, r.text[:200])

reg_body = r.json() if r.status_code == 201 else {}
check("Response has user.id", "user" in reg_body and "id" in reg_body.get("user", {}))
check("Response has tokens.access_token", "tokens" in reg_body and "access_token" in reg_body.get("tokens", {}))

access_token = reg_body.get("tokens", {}).get("access_token", "")
user_id = reg_body.get("user", {}).get("id", "")
print(f"  {INFO} login={login}, user_id={user_id[:8]}...")

# ── 2. LOGIN ───────────────────────────────────────────────────────────────────
step("2. Login")

r = requests.post(f"{BFF}/api/auth/login", json={"login": login, "password": password}, timeout=10)
check("POST /api/auth/login → 200", r.status_code == 200, r.text[:200])

login_body = r.json() if r.status_code == 200 else {}
check("Login tokens present", bool(login_body.get("tokens", {}).get("access_token")))

# Use the login token (fresh)
access_token = login_body.get("tokens", {}).get("access_token", access_token)

# ── 3. ME endpoint ─────────────────────────────────────────────────────────────
step("3. GET /api/auth/me")

headers = {"Authorization": f"Bearer {access_token}", "Content-Type": "application/json"}
r = requests.get(f"{BFF}/api/auth/me", headers=headers, timeout=10)
check("GET /api/auth/me → 200", r.status_code == 200, r.text[:200])
me_body = r.json() if r.status_code == 200 else {}
check("user.login matches registered login", me_body.get("user", {}).get("login", "").lower() == login.lower())

# ── 4. START CHAT (interview) ──────────────────────────────────────────────────
step("4. Start interview session")

chat_params = {
    "params": {
        "type": "ml",
        "level": "middle",
        "topics": ["nlp"],
        "mode": "interview",
    }
}
r = requests.post(f"{BFF}/api/chat/start", headers=headers, json=chat_params, timeout=60)
check("POST /api/chat/start → 201", r.status_code == 201, r.text[:300])

start_body = r.json() if r.status_code == 201 else {}
session_id = start_body.get("session_id", "")
check("session_id returned", bool(session_id))
print(f"  {INFO} session_id={session_id[:8]}...")

# Wait for agent to generate first question
if session_id:
    print(f"  {INFO} Waiting for agent to generate first question...")
    time.sleep(6)

# ── 5. SEND MESSAGE ────────────────────────────────────────────────────────────
step("5. Send candidate answer")

if not session_id:
    print(f"  {FAIL} Cannot send message without session_id")
    errors.append("No session_id")
else:
    msg_payload = {
        "session_id": session_id,
        "content": "Трансформеры используют механизм self-attention для обработки последовательностей.",
    }
    r = requests.post(f"{BFF}/api/chat/message", headers=headers, json=msg_payload, timeout=30)
    check("POST /api/chat/message → 200 or 202", r.status_code in (200, 201, 202), r.text[:300])

    msg_body = r.json() if r.status_code in (200, 201, 202) else {}
    # The BFF returns {'status': 'ok'} – message is processed asynchronously via Kafka
    # The agent's actual response is fetched via GET /api/chat/:id/question
    check("Response is ok (async processing)", msg_body.get("status") == "ok" or bool(msg_body.get("message") or msg_body.get("question")))
    print(f"  {INFO} Message accepted: {str(msg_body)[:150]}...")

# ── 6. GET NEXT QUESTION (optional) ────────────────────────────────────────────
step("6. GET /api/chat/:session_id/question")

if session_id:
    time.sleep(3)
    r = requests.get(f"{BFF}/api/chat/{session_id}/question", headers=headers, timeout=15)
    check("GET question → 200 or 404", r.status_code in (200, 404), r.text[:200])
    if r.status_code == 200:
        print(f"  {INFO} Question: {str(r.json())[:150]}")

# ── 7. SEND ANOTHER ANSWER ────────────────────────────────────────────────────
step("7. Send second answer")

if session_id:
    msg_payload2 = {
        "session_id": session_id,
        "content": "BERT использует двунаправленный encoder для классификации и NER задач.",
    }
    r = requests.post(f"{BFF}/api/chat/message", headers=headers, json=msg_payload2, timeout=30)
    check("POST /api/chat/message (2nd) → 200-202", r.status_code in (200, 201, 202), r.text[:300])
    time.sleep(3)

# ── 8. TERMINATE SESSION ───────────────────────────────────────────────────────
step("8. Terminate session")

if session_id:
    r = requests.post(f"{BFF}/api/chat/{session_id}/terminate", headers=headers, timeout=15)
    check("POST /api/chat/:id/terminate → 200", r.status_code == 200, r.text[:200])
    print(f"  {INFO} Waiting for analyst to process session...")
    time.sleep(10)  # let analyst-agent-service process the session.completed event

# ── 9. GET RESULTS VIA BFF ────────────────────────────────────────────────────
step("9. GET /api/chat/:session_id/results")

if session_id:
    r = requests.get(f"{BFF}/api/chat/{session_id}/results", headers=headers, timeout=15)
    check("GET results → 200 or 404", r.status_code in (200, 404), r.text[:300])
    if r.status_code == 200:
        result_body = r.json()
        # Results may show available=False if analyst hasn't completed yet (async processing)
        # available=True means the report is ready; False means placeholder score=0 is stored
        is_available = result_body.get("available", False)
        has_result = "result" in result_body or "results" in result_body
        check("Result response has expected structure", has_result or "available" in result_body)
        print(f"  {INFO} Results (available={is_available}): {str(result_body)[:200]}")

# ── 10. GET INTERVIEWS LIST ───────────────────────────────────────────────────
step("10. GET /api/interviews (list of sessions)")

r = requests.get(f"{BFF}/api/interviews", headers=headers, timeout=10)
check("GET /api/interviews → 200", r.status_code == 200, r.text[:200])
if r.status_code == 200:
    interviews_body = r.json()
    check("interviews key present", "interviews" in interviews_body)
    interviews_list = interviews_body.get("interviews", [])
    check("At least 1 interview in list", len(interviews_list) >= 1, f"got {len(interviews_list)}")
    if interviews_list:
        print(f"  {INFO} First interview: {str(interviews_list[0])[:150]}")

# ── 11. GET DIRECT RESULT FROM results-crud ──────────────────────────────────
step("11. GET /results/:session_id (direct results-crud check)")

if session_id:
    r = requests.get(f"{RESULTS_CRUD}/results/{session_id}", timeout=10)
    check("GET results from results-crud → 200 or 404", r.status_code in (200, 404), r.text[:200])
    if r.status_code == 200:
        rc_body = r.json()
        result = rc_body.get("result", {})
        check("Result.session_id matches", result.get("session_id", "") == session_id)
        print(f"  {INFO} Score: {result.get('score')}, Kind: {result.get('session_kind')}")

# ── 12. TOKEN REFRESH ─────────────────────────────────────────────────────────
step("12. Token refresh")

refresh_token = login_body.get("tokens", {}).get("refresh_token", "")
if refresh_token:
    r = requests.post(f"{BFF}/api/auth/refresh", json={"refresh_token": refresh_token}, timeout=10)
    check("POST /api/auth/refresh → 200", r.status_code == 200, r.text[:200])
    new_tokens = r.json().get("tokens", {})
    check("New access_token returned", bool(new_tokens.get("access_token")))

# ── 13. DUPLICATE REGISTRATION ────────────────────────────────────────────────
step("13. Duplicate registration → 409")

r = requests.post(f"{BFF}/api/auth/register", json={"login": login, "password": "AnotherPass456!"}, timeout=10)
check("Duplicate register → 409", r.status_code == 409, r.text[:200])

# ── 14. WRONG PASSWORD LOGIN ─────────────────────────────────────────────────
step("14. Wrong password → 401")

r = requests.post(f"{BFF}/api/auth/login", json={"login": login, "password": "WrongPass!!!"}, timeout=10)
check("Wrong password → 401", r.status_code == 401, r.text[:200])

# ── SUMMARY ──────────────────────────────────────────────────────────────────
print(f"\n{'═'*60}")
if not errors:
    print(f"  {PASS} ALL E2E CHECKS PASSED")
    print(f"{'═'*60}")
    sys.exit(0)
else:
    print(f"  {FAIL} {len(errors)} CHECK(S) FAILED:")
    for e in errors:
        print(f"    {e}")
    print(f"{'═'*60}")
    sys.exit(1)
