"""Kafka producer for interview.build.response events"""

import json
import uuid
import time
from datetime import datetime, timezone
from typing import Any, Dict, Optional
from confluent_kafka import Producer
from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector
from ..schemas import BuildResponse, ProgramMeta

logger = get_logger(__name__)


class KafkaProducer:
    """Kafka producer for sending interview build responses"""

    def __init__(self, bootstrap_servers: str = None):
        self.bootstrap_servers = bootstrap_servers or settings.kafka_bootstrap_servers
        self.topic = settings.kafka_topic_response
        self.metrics = get_metrics_collector()

        config = {
            "bootstrap.servers": self.bootstrap_servers,
            "acks": "all",
            "retries": 3,
        }

        self.producer = Producer(config)
        logger.info("Kafka producer initialized", bootstrap_servers=self.bootstrap_servers)

    def _serialize_response(self, response: BuildResponse) -> Dict[str, Any]:
        """Build the full Kafka event envelope for a BuildResponse."""
        return {
            "event_id": str(uuid.uuid4()),
            "event_type": "interview.build.response",
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "service": settings.service_name,
            "version": settings.service_version,
            "payload": response.to_kafka_payload(),
        }

    def send_interview_program(
        self,
        session_id: str,
        program: Dict[str, Any],
        program_meta: Optional[ProgramMeta] = None,
    ) -> None:
        """Send a successful interview program to Kafka."""
        if program_meta is None:
            program_meta = ProgramMeta()

        response = BuildResponse(
            session_id=session_id,
            interview_program=program,
            program_meta=program_meta,
        )
        self._publish(response)

    def send_failure(
        self,
        session_id: str,
        fallback_reason: str,
    ) -> None:
        """Send a controlled failure response so downstream services
        can react instead of waiting indefinitely."""
        meta = ProgramMeta(
            validation_passed=False,
            fallback_reason=fallback_reason,
        )
        response = BuildResponse(
            session_id=session_id,
            interview_program={"questions": []},
            program_meta=meta,
        )
        self._publish(response)
        logger.warning("Sent failure response",
                       session_id=session_id,
                       fallback_reason=fallback_reason)

    def _publish(self, response: BuildResponse) -> None:
        """Serialize and produce a BuildResponse to Kafka."""
        start_time = time.time()
        event = self._serialize_response(response)

        try:
            self.producer.produce(
                self.topic,
                key=response.session_id,
                value=json.dumps(event).encode("utf-8"),
                callback=self._delivery_callback,
            )
            self.producer.poll(0)
            duration = time.time() - start_time
            self.metrics.kafka_producer_duration.labels(
                service=settings.service_name,
                topic=self.topic,
            ).observe(duration)
            logger.info("Interview build response sent",
                        session_id=response.session_id,
                        validation_passed=response.program_meta.validation_passed,
                        topic=self.topic)
        except Exception as e:
            logger.error("Failed to send interview build response",
                         session_id=response.session_id,
                         error=str(e))
            raise

    def _delivery_callback(self, err, msg):
        if err:
            logger.error("Message delivery failed", error=str(err))
        else:
            logger.debug("Message delivered",
                         topic=msg.topic(),
                         partition=msg.partition())

    def flush(self, timeout: float = 10.0):
        self.producer.flush(timeout)

    def close(self):
        self.flush()
        logger.info("Kafka producer closed")
