"""Prometheus metrics collector"""

from prometheus_client import (
    Counter,
    Histogram,
    start_http_server,
    REGISTRY,
)
from typing import Optional

from ..config import settings


class MetricsCollector:
    """Collector for Prometheus metrics"""

    def __init__(self):
        self.registry = REGISTRY

        # Business metrics
        self.knowledge_produced_total = Counter(
            "tensortalks_business_knowledge_produced_total",
            "Total number of knowledge items produced",
            ["service", "status", "operation"],
            registry=self.registry,
        )

        self.questions_produced_total = Counter(
            "tensortalks_business_questions_produced_total",
            "Total number of questions produced",
            ["service", "status", "operation"],
            registry=self.registry,
        )

        # Technical metrics
        self.production_duration = Histogram(
            "tensortalks_production_duration_seconds",
            "Knowledge/question production duration in seconds",
            ["service", "type"],
            registry=self.registry,
        )

        self.http_client_duration = Histogram(
            "tensortalks_http_client_duration_seconds",
            "HTTP client request duration in seconds",
            ["service", "target_service"],
            registry=self.registry,
        )

        self.error_count = Counter(
            "tensortalks_error_count",
            "Error count by type",
            ["error_type", "service"],
            registry=self.registry,
        )

    def start_metrics_server(self) -> None:
        """Start HTTP server for Prometheus metrics"""
        if settings.enable_prometheus:
            start_http_server(settings.metrics_port, registry=self.registry)


# Global metrics collector instance
_metrics_collector: Optional[MetricsCollector] = None


def get_metrics_collector() -> MetricsCollector:
    """Get or create metrics collector instance"""
    global _metrics_collector
    if _metrics_collector is None:
        _metrics_collector = MetricsCollector()
    return _metrics_collector

