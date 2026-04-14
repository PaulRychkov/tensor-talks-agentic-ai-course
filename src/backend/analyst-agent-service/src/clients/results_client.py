"""HTTP client for Results CRUD Service"""

import httpx
from typing import Optional, Dict, Any, List

from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector

logger = get_logger(__name__)


class ResultsCrudClient:
    """Client for interacting with Results CRUD Service"""

    def __init__(self, base_url: Optional[str] = None):
        self.base_url = base_url or settings.results_crud_service_url
        self.client = httpx.AsyncClient(timeout=60.0)
        self.metrics = get_metrics_collector()
        logger.info("ResultsCrudClient initialized", base_url=self.base_url)

    async def save_report(
        self,
        session_id: str,
        user_id: str,
        session_kind: str,
        report_json: Dict[str, Any],
        preset_training: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """Save analysis report via POST /results (includes preset_training if provided)."""
        try:
            # results-crud expects top-level score/feedback + report_json blob
            # LLM may return a float (e.g. 72.5); Go's int field rejects non-integers → 400 → save fails
            raw_score = report_json.get("score", 0)
            try:
                score = max(0, min(100, int(round(float(raw_score)))))
            except (TypeError, ValueError):
                score = 0
            feedback = report_json.get("summary", report_json.get("feedback", ""))
            payload: Dict[str, Any] = {
                "session_id": session_id,
                "user_id": user_id,
                "session_kind": session_kind,
                "score": score,
                "feedback": feedback,
                "report_json": report_json,
                "result_format_version": 1,
            }
            if preset_training:
                payload["preset_training"] = preset_training
            response = await self.client.post(
                f"{self.base_url}/results",
                json=payload,
            )
            response.raise_for_status()
            self.metrics.http_requests_total.labels(
                service="results-crud", method="POST", status="success"
            ).inc()
            logger.info(
                "Report saved",
                session_id=session_id,
                status_code=response.status_code,
            )
            return response.json()
        except httpx.HTTPStatusError as e:
            self.metrics.http_requests_total.labels(
                service="results-crud", method="POST", status="error"
            ).inc()
            logger.error(
                "Failed to save report",
                session_id=session_id,
                status_code=e.response.status_code,
                error=str(e),
            )
            raise
        except Exception as e:
            self.metrics.http_requests_total.labels(
                service="results-crud", method="POST", status="error"
            ).inc()
            logger.error("Failed to save report", session_id=session_id, error=str(e))
            raise

    async def create_presets(
        self,
        session_id: str,
        user_id: str,
        presets: List[Dict[str, Any]],
    ) -> Dict[str, Any]:
        """Create follow-up session presets via POST /presets (one per request)."""
        results = []
        try:
            for preset in presets:
                # Map analyst preset fields to results-crud schema
                payload = {
                    "user_id": user_id,
                    "target_mode": preset.get("mode", preset.get("target_mode", "training")),
                    "topics": preset.get("topics", []) + preset.get("weak_topics", []),
                    "source_session_id": session_id,
                }
                response = await self.client.post(
                    f"{self.base_url}/presets",
                    json=payload,
                )
                response.raise_for_status()
                results.append(response.json())
                self.metrics.presets_created_total.inc()

            self.metrics.http_requests_total.labels(
                service="results-crud", method="POST", status="success"
            ).inc()
            logger.info(
                "Presets created",
                session_id=session_id,
                presets_count=len(presets),
            )
            return {"presets": results}
        except httpx.HTTPStatusError as e:
            self.metrics.http_requests_total.labels(
                service="results-crud", method="POST", status="error"
            ).inc()
            logger.error(
                "Failed to create presets",
                session_id=session_id,
                status_code=e.response.status_code,
                error=str(e),
            )
            raise
        except Exception as e:
            self.metrics.http_requests_total.labels(
                service="results-crud", method="POST", status="error"
            ).inc()
            logger.error("Failed to create presets", session_id=session_id, error=str(e))
            raise

    async def update_user_progress(
        self,
        user_id: str,
        session_id: str,
        topic_progress: List[Dict[str, Any]],
    ) -> Dict[str, Any]:
        """Update user topic progress via POST /user-progress"""
        try:
            payload = {
                "user_id": user_id,
                "session_id": session_id,
                "topic_progress": topic_progress,
            }
            response = await self.client.post(
                f"{self.base_url}/user-progress",
                json=payload,
            )
            response.raise_for_status()
            self.metrics.http_requests_total.labels(
                service="results-crud", method="POST", status="success"
            ).inc()
            self.metrics.progress_updates_total.inc()
            logger.info(
                "User progress updated",
                user_id=user_id,
                session_id=session_id,
                topics_count=len(topic_progress),
                status_code=response.status_code,
            )
            return response.json()
        except httpx.HTTPStatusError as e:
            self.metrics.http_requests_total.labels(
                service="results-crud", method="POST", status="error"
            ).inc()
            logger.error(
                "Failed to update user progress",
                user_id=user_id,
                session_id=session_id,
                status_code=e.response.status_code,
                error=str(e),
            )
            raise
        except Exception as e:
            self.metrics.http_requests_total.labels(
                service="results-crud", method="POST", status="error"
            ).inc()
            logger.error(
                "Failed to update user progress",
                user_id=user_id,
                session_id=session_id,
                error=str(e),
            )
            raise

    async def close(self):
        """Close HTTP client"""
        await self.client.aclose()
