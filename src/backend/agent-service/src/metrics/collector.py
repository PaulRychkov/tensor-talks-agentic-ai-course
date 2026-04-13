"""Prometheus metrics collector"""

from prometheus_client import (
    Counter,
    Histogram,
    Gauge,
    start_http_server,
    CollectorRegistry,
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

        # Business metrics
        self.messages_processed_total = Counter(
            "agent_messages_processed_total",
            "Total messages processed",
            ["status", "decision"],
            registry=self.registry,
        )

        self.processing_duration = Histogram(
            "agent_processing_duration_seconds",
            "Time spent processing message",
            buckets=[0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0, 120.0],
            registry=self.registry,
        )

        self.llm_calls_total = Counter(
            "agent_llm_calls_total",
            "Total LLM API calls",
            ["provider", "model", "status"],
            registry=self.registry,
        )

        self.llm_call_duration = Histogram(
            "agent_llm_call_duration_seconds",
            "Time spent on LLM call",
            buckets=[0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0],
            registry=self.registry,
        )

        # State metrics
        self.active_dialogues = Gauge(
            "agent_active_dialogues",
            "Number of active dialogues being processed",
            registry=self.registry,
        )

        self.current_question_index = Histogram(
            "agent_current_question_index",
            "Distribution of current question indices",
            buckets=list(range(0, 11)),
            registry=self.registry,
        )

        # Technical metrics
        self.kafka_producer_duration = Histogram(
            "agent_kafka_producer_duration_seconds",
            "Kafka producer duration in seconds",
            registry=self.registry,
        )

        self.kafka_consumer_lag = Gauge(
            "agent_kafka_consumer_lag",
            "Kafka consumer lag",
            ["topic", "partition"],
            registry=self.registry,
        )

        self.error_count = Counter(
            "agent_error_count",
            "Error count by type",
            ["error_type", "service"],
            registry=self.registry,
        )

        self.redis_operation_duration = Histogram(
            "agent_redis_operation_duration_seconds",
            "Redis operation duration in seconds",
            ["operation"],
            registry=self.registry,
        )

    def start_metrics_server(self) -> None:
        """Start HTTP server for Prometheus metrics"""
        if settings.enable_prometheus:
            try:
                start_http_server(settings.metrics_port, registry=self.registry)
                logger.info(
                    "Metrics server started",
                    port=settings.metrics_port,
                )
            except OSError as e:
                if e.errno == 98:  # Address already in use
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


# Global metrics collector instance
_metrics_collector: Optional[MetricsCollector] = None


def get_metrics_collector() -> MetricsCollector:
    """Get or create metrics collector instance"""
    global _metrics_collector
    if _metrics_collector is None:
        _metrics_collector = MetricsCollector()
    return _metrics_collector
