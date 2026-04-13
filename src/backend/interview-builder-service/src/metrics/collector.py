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


class MetricsCollector:
    """Collector for Prometheus metrics"""

    def __init__(self):
        self.registry = REGISTRY

        # Business metrics
        self.interview_programs_built_total = Counter(
            "tensortalks_business_interview_programs_built_total",
            "Total number of interview programs built",
            ["service", "status"],
            registry=self.registry,
        )

        self.questions_fetched_total = Counter(
            "tensortalks_business_questions_fetched_total",
            "Total number of questions fetched",
            ["service", "status"],
            registry=self.registry,
        )

        self.knowledge_fetched_total = Counter(
            "tensortalks_business_knowledge_fetched_total",
            "Total number of knowledge items fetched",
            ["service", "status"],
            registry=self.registry,
        )

        # Technical metrics
        self.program_building_duration = Histogram(
            "tensortalks_program_building_duration_seconds",
            "Interview program building duration in seconds",
            ["service"],
            registry=self.registry,
        )

        self.http_client_duration = Histogram(
            "tensortalks_http_client_duration_seconds",
            "HTTP client request duration in seconds",
            ["service", "target_service"],
            registry=self.registry,
        )

        self.kafka_producer_duration = Histogram(
            "tensortalks_kafka_producer_duration_seconds",
            "Kafka producer duration in seconds",
            ["service", "topic"],
            registry=self.registry,
        )

        self.kafka_consumer_messages_total = Counter(
            "tensortalks_kafka_messages_consumed_total",
            "Total number of Kafka messages consumed",
            ["service", "topic", "status"],
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

