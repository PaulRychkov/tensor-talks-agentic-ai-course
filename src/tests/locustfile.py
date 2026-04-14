"""
Locust load & chaos test suite for TensorTalks.

Usage:
  # Load test — 5 concurrent users, 60s ramp-up:
  locust -f tests/locustfile.py --host http://localhost:8080 \
         -u 5 -r 1 --run-time 120s --headless

  # Chaos scenario — Kafka restart simulation (external):
  locust -f tests/locustfile.py --host http://localhost:8080 \
         -u 2 -r 1 --run-time 60s --headless \
         -T ChaosUser

Environment variables:
  LOCUST_USER_ID   — override fixed user UUID (optional, random per session)
  LOCUST_BFF_URL   — BFF base URL (default http://localhost:8080)
"""

import os
import time
import uuid
import random
import logging

from locust import HttpUser, task, between, events, tag
from locust.exception import StopUser

logger = logging.getLogger("tensortalks-locust")

# ── Constants ──────────────────────────────────────────────────────────────────

BFF_API = "/api"
AUTH_HEADER = {"Content-Type": "application/json"}

# Realistic user answers for ML questions (rotate randomly)
SAMPLE_ANSWERS = [
    "Градиентный спуск — это оптимизационный алгоритм, который итеративно обновляет параметры модели в направлении, противоположном градиенту функции потерь.",
    "Переобучение происходит когда модель слишком хорошо подстраивается под тренировочные данные и теряет способность к обобщению.",
    "Регуляризация L2 добавляет штраф на квадрат весов к функции потерь, что помогает предотвратить переобучение.",
    "Attention механизм позволяет модели фокусироваться на наиболее релевантных частях входных данных при генерации каждого токена.",
    "Не знаю",
]

TOPICS = ["classic_ml", "nlp", "llm"]
LEVELS = ["junior", "middle"]
MODES = ["interview", "training", "study"]


# ── Helpers ────────────────────────────────────────────────────────────────────

def _headers(token: str) -> dict:
    return {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}


def _random_username() -> str:
    adjectives = ["fast", "bright", "clever", "swift", "bold"]
    nouns = ["panda", "eagle", "tiger", "fox", "wolf"]
    return f"{random.choice(adjectives)}-{random.choice(nouns)}-{random.randint(100, 999)}"


# ── Main load test user ────────────────────────────────────────────────────────

class InterviewUser(HttpUser):
    """Simulates a full interview session: register → login → start chat → answer questions."""

    wait_time = between(1, 3)
    _token: str | None = None
    _user_id: str | None = None
    _session_id: str | None = None

    def on_start(self):
        """Register and login before running tasks."""
        username = _random_username()
        password = "TestPass123!"

        # Register
        reg = self.client.post(
            f"{BFF_API}/auth/register",
            json={"username": username, "password": password},
            name="/api/auth/register",
            catch_response=True,
        )
        with reg:
            if reg.status_code not in (200, 201, 409):
                reg.failure(f"Register failed: {reg.status_code} {reg.text[:100]}")
                raise StopUser()

        # Login
        login = self.client.post(
            f"{BFF_API}/auth/login",
            json={"username": username, "password": password},
            name="/api/auth/login",
            catch_response=True,
        )
        with login:
            if login.status_code != 200:
                login.failure(f"Login failed: {login.status_code}")
                raise StopUser()
            data = login.json()
            self._token = data.get("token") or data.get("access_token")
            self._user_id = data.get("user_id") or data.get("id")
            if not self._token:
                login.failure("No token in login response")
                raise StopUser()

    @task(3)
    @tag("start_session")
    def start_interview_session(self):
        """Start a new interview session and poll for the first question."""
        if not self._token:
            return

        topic = random.choice(TOPICS)
        mode = random.choice(MODES)

        resp = self.client.post(
            f"{BFF_API}/chat/start",
            json={
                "params": {
                    "topics": [topic],
                    "level": random.choice(LEVELS),
                    "mode": mode,
                    "use_previous_results": True,
                }
            },
            headers=_headers(self._token),
            name="/api/chat/start",
            catch_response=True,
        )
        with resp:
            if resp.status_code in (429,):
                # Already has active session — normal during load test
                resp.success()
                return
            if resp.status_code not in (200, 201):
                resp.failure(f"start chat failed: {resp.status_code} {resp.text[:80]}")
                return
            data = resp.json()
            self._session_id = data.get("session_id")

        if self._session_id:
            # Poll for first question (up to 15s)
            self._poll_question(timeout=15)

    @task(5)
    @tag("answer")
    def answer_question(self):
        """Send an answer to the current session's current question."""
        if not self._token or not self._session_id:
            return

        answer = random.choice(SAMPLE_ANSWERS)
        self.client.post(
            f"{BFF_API}/chat/message",
            json={"session_id": self._session_id, "content": answer},
            headers=_headers(self._token),
            name="/api/chat/message",
        )

        # Poll for next question
        self._poll_question(timeout=20)

    @task(1)
    @tag("results")
    def check_results(self):
        """Poll results endpoint for completed sessions."""
        if not self._token or not self._session_id:
            return

        self.client.get(
            f"{BFF_API}/interviews/{self._session_id}/results",
            headers=_headers(self._token),
            name="/api/interviews/:id/results",
        )

    @task(1)
    @tag("dashboard")
    def load_dashboard(self):
        """Load user's interview list."""
        if not self._token:
            return
        self.client.get(
            f"{BFF_API}/interviews",
            headers=_headers(self._token),
            name="/api/interviews",
        )

    def _poll_question(self, timeout: int = 15):
        """Poll /api/chat/:id/question until a question arrives or timeout."""
        if not self._session_id:
            return
        deadline = time.time() + timeout
        while time.time() < deadline:
            resp = self.client.get(
                f"{BFF_API}/chat/{self._session_id}/question",
                headers=_headers(self._token),
                name="/api/chat/:id/question (poll)",
                catch_response=True,
            )
            with resp:
                if resp.status_code == 200:
                    data = resp.json()
                    if data.get("available") is False or data.get("question") is None:
                        resp.success()
                        time.sleep(1)
                        continue
                    # Got a question
                    resp.success()
                    return
                resp.success()  # Treat non-200 as soft failure in poll loop
            time.sleep(1)

    def on_stop(self):
        """Terminate any lingering session on worker stop."""
        if self._token and self._session_id:
            try:
                self.client.post(
                    f"{BFF_API}/chat/{self._session_id}/terminate",
                    headers=_headers(self._token),
                    name="/api/chat/:id/terminate (cleanup)",
                )
            except Exception:
                pass


# ── Health check user (lightweight baseline) ───────────────────────────────────

class HealthUser(HttpUser):
    """Constantly checks health endpoints to detect degradation under load."""

    wait_time = between(2, 5)

    @task
    @tag("health")
    def check_bff_health(self):
        self.client.get(f"{BFF_API}/health", name="/api/health")

    @task
    @tag("health")
    def check_admin_health(self):
        self.client.get("/admin/health", name="/admin/health")


# ── Chaos scenarios ────────────────────────────────────────────────────────────

class ChaosUser(HttpUser):
    """
    Simulates degraded conditions: rapid session starts, abrupt terminations,
    duplicate messages, and polling bursts.

    Run separately to avoid polluting normal load metrics:
      locust -f tests/locustfile.py -T ChaosUser --host http://localhost:8080
    """

    wait_time = between(0.5, 2)
    _token: str | None = None
    _session_id: str | None = None

    def on_start(self):
        username = f"chaos-{uuid.uuid4().hex[:8]}"
        self.client.post(
            f"{BFF_API}/auth/register",
            json={"username": username, "password": "Chaos123!"},
            name="/api/auth/register",
        )
        login = self.client.post(
            f"{BFF_API}/auth/login",
            json={"username": username, "password": "Chaos123!"},
            name="/api/auth/login",
        )
        if login.status_code == 200:
            data = login.json()
            self._token = data.get("token") or data.get("access_token")

    @task(2)
    @tag("chaos")
    def rapid_start_terminate(self):
        """Start and immediately terminate — tests cleanup path."""
        if not self._token:
            return
        resp = self.client.post(
            f"{BFF_API}/chat/start",
            json={"params": {"topics": ["classic_ml"], "level": "junior", "mode": "interview"}},
            headers=_headers(self._token),
            name="[chaos] start+terminate",
            catch_response=True,
        )
        with resp:
            if resp.status_code in (200, 201):
                sid = resp.json().get("session_id")
                if sid:
                    self.client.post(
                        f"{BFF_API}/chat/{sid}/terminate",
                        headers=_headers(self._token),
                        name="[chaos] terminate",
                    )
            resp.success()

    @task(3)
    @tag("chaos")
    def polling_burst(self):
        """Send 5 rapid poll requests — tests rate handling."""
        if not self._token or not self._session_id:
            return
        for _ in range(5):
            self.client.get(
                f"{BFF_API}/chat/{self._session_id}/question",
                headers=_headers(self._token),
                name="[chaos] poll burst",
            )

    @task(1)
    @tag("chaos")
    def duplicate_message(self):
        """Send the same message twice — tests idempotency."""
        if not self._token or not self._session_id:
            return
        msg = "Дублированный ответ"
        for _ in range(2):
            self.client.post(
                f"{BFF_API}/chat/message",
                json={"session_id": self._session_id, "content": msg},
                headers=_headers(self._token),
                name="[chaos] duplicate msg",
            )

    @task(1)
    @tag("chaos")
    def invalid_session(self):
        """Poll a non-existent session — tests 404 handling."""
        fake_id = str(uuid.uuid4())
        self.client.get(
            f"{BFF_API}/chat/{fake_id}/question",
            headers=_headers(self._token or ""),
            name="[chaos] invalid session",
        )


# ── Event hooks for Locust reporting ──────────────────────────────────────────

@events.test_start.add_listener
def on_test_start(environment, **kwargs):
    logger.info("TensorTalks load test started")
    logger.info(f"Target host: {environment.host}")


@events.test_stop.add_listener
def on_test_stop(environment, **kwargs):
    stats = environment.runner.stats
    logger.info("=== TensorTalks Load Test Results ===")
    for name, stat in stats.entries.items():
        if stat.num_requests > 0:
            logger.info(
                f"{name[1]}: {stat.num_requests} req, "
                f"avg {stat.avg_response_time:.0f}ms, "
                f"p95 {stat.get_response_time_percentile(0.95):.0f}ms, "
                f"failures {stat.num_failures}"
            )
