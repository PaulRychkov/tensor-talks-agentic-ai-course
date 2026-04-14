"""Prometheus metrics collector"""

from prometheus_client import (
    Counter,
    Histogram,
    Gauge,
    start_http_server,
    REGISTRY,
)
from typing import Optional

from ..config import settings
from ..logger import get_logger

logger = get_logger(__name__)


class MetricsCollector:
    """Collector for Prometheus metrics"""

    def __init__(self):
        self.registry = REGISTRY

        self.sessions_analyzed_total = Counter(
            "analyst_sessions_analyzed_total",
            "Total sessions analyzed",
            ["status", "session_kind"],
            registry=self.registry,
        )

        self.analysis_duration = Histogram(
            "analyst_analysis_duration_seconds",
            "Time spent analyzing a session",
            buckets=[1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 120.0, 180.0],
            registry=self.registry,
        )

        self.llm_calls_total = Counter(
            "analyst_llm_calls_total",
            "Total LLM API calls",
            ["model", "status"],
            registry=self.registry,
        )

        self.llm_call_duration = Histogram(
            "analyst_llm_call_duration_seconds",
            "Time spent on LLM call",
            buckets=[0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0],
            registry=self.registry,
        )

        self.http_requests_total = Counter(
            "analyst_http_requests_total",
            "Total HTTP requests to downstream services",
            ["service", "method", "status"],
            registry=self.registry,
        )

        self.reports_generated_total = Counter(
            "analyst_reports_generated_total",
            "Total reports generated",
            ["session_kind"],
            registry=self.registry,
        )

        self.presets_created_total = Counter(
            "analyst_presets_created_total",
            "Total presets created for follow-up sessions",
            registry=self.registry,
        )

        self.progress_updates_total = Counter(
            "analyst_progress_updates_total",
            "Total user_topic_progress updates",
            registry=self.registry,
        )

        self.active_analyses = Gauge(
            "analyst_active_analyses",
            "Number of analyses currently in progress",
            registry=self.registry,
        )

        self.error_count = Counter(
            "analyst_error_count",
            "Error count by type",
            ["error_type", "service"],
            registry=self.registry,
        )

        # AI quality metrics (§10.8)
        self.report_score_distribution = Histogram(
            "analyst_report_score",
            "Distribution of final report scores (0-100)",
            ["session_kind"],
            buckets=[0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100],
            registry=self.registry,
        )

        self.validation_attempts = Histogram(
            "analyst_validation_attempts",
            "Number of validation retries per report",
            ["session_kind"],
            buckets=[1, 2, 3, 4, 5],
            registry=self.registry,
        )

        self.report_completeness = Gauge(
            "analyst_report_completeness",
            "Fraction of required sections present in final report (0.0-1.0)",
            ["session_kind"],
            registry=self.registry,
        )

        # Product metrics (§10.8)
        self.session_questions_answered = Histogram(
            "analyst_session_questions_answered",
            "Number of questions answered per session",
            buckets=[0, 1, 2, 3, 5, 7, 10, 15, 20],
            registry=self.registry,
        )

        self.session_terminated_early_total = Counter(
            "analyst_session_terminated_early_total",
            "Sessions terminated before completion",
            ["session_kind"],
            registry=self.registry,
        )

    def start_metrics_server(self) -> None:
        """Start HTTP server for Prometheus metrics"""
        if settings.enable_prometheus:
            try:
                start_http_server(settings.metrics_port, registry=self.registry)
                logger.info("Metrics server started", port=settings.metrics_port)
            except OSError as e:
                if e.errno == 98:
                    logger.warning(
                        "Metrics port already in use, skipping metrics server",
                        port=settings.metrics_port,
                        error=str(e),
                    )
                else:
                    logger.error(
                        "Failed to start metrics server",
                        port=settings.metrics_port,
                        error=str(e),
                    )
                    raise


_metrics_collector: Optional[MetricsCollector] = None


def get_metrics_collector() -> MetricsCollector:
    """Get or create metrics collector instance"""
    global _metrics_collector
    if _metrics_collector is None:
        _metrics_collector = MetricsCollector()
    return _metrics_collector
