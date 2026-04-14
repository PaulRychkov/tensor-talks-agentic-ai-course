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

        # Confidence-aware metrics (§10.7, §10.8)
        self.agent_decision_confidence = Histogram(
            "agent_decision_confidence",
            "Distribution of agent decision confidence scores (0.0-1.0)",
            ["decision"],
            buckets=[0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0],
            registry=self.registry,
        )

        self.agent_low_confidence_decisions_total = Counter(
            "agent_low_confidence_decisions_total",
            "Number of decisions made with confidence < 0.5",
            ["decision"],
            registry=self.registry,
        )

        self.agent_evaluation_score = Histogram(
            "agent_evaluation_score",
            "Distribution of answer evaluation scores (0.0-1.0)",
            ["session_mode"],
            buckets=[0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0],
            registry=self.registry,
        )

        # Product metrics (§10.product)
        self.session_completed_total = Counter(
            "session_completed_total",
            "Sessions finished (naturally completed vs terminated early)",
            ["mode", "completed_naturally"],
            registry=self.registry,
        )

        self.session_hints_used = Histogram(
            "session_hints_used",
            "Total hints given per session",
            ["mode"],
            buckets=[0, 1, 2, 3, 5, 8, 13],
            registry=self.registry,
        )

        self.session_questions_answered = Histogram(
            "session_questions_answered",
            "Number of questions answered per session",
            ["mode"],
            buckets=[1, 2, 3, 5, 7, 10, 15],
            registry=self.registry,
        )

        self.session_user_rating = Histogram(
            "session_user_rating",
            "User satisfaction rating (1-5) collected after session",
            ["mode"],
            buckets=[1, 2, 3, 4, 5],
            registry=self.registry,
        )

        # PII filter metrics (§10.11)
        self.pii_filter_triggered_total = Counter(
            "pii_filter_triggered_total",
            "Number of times PII filter triggered",
            ["level", "category"],
            registry=self.registry,
        )

        self.pii_filter_latency_seconds = Histogram(
            "pii_filter_latency_seconds",
            "Time spent on PII filter check",
            ["level"],
            buckets=[0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 2.0],
            registry=self.registry,
        )

    def record_decision_confidence(self, decision: str, confidence: float) -> None:
        """Record decision confidence metric and flag low-confidence decisions."""
        self.agent_decision_confidence.labels(decision=decision).observe(confidence)
        if confidence < 0.5:
            self.agent_low_confidence_decisions_total.labels(decision=decision).inc()

    def record_session_end(
        self,
        mode: str,
        completed_naturally: bool,
        hints_used: int,
        questions_answered: int,
    ) -> None:
        """Record product metrics when a session ends (naturally or via terminate)."""
        label = "true" if completed_naturally else "false"
        self.session_completed_total.labels(mode=mode, completed_naturally=label).inc()
        self.session_hints_used.labels(mode=mode).observe(hints_used)
        self.session_questions_answered.labels(mode=mode).observe(questions_answered)

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
