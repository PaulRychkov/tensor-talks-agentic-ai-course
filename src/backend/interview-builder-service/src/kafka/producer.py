"""Kafka producer for interview.build.response events"""

import json
import uuid
from datetime import datetime
from typing import Dict, Any
from confluent_kafka import Producer
from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector
import time

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

    def send_interview_program(self, session_id: str, program: Dict[str, Any]) -> None:
        """Send interview program to Kafka"""
        start_time = time.time()
        event = {
            "event_id": str(uuid.uuid4()),
            "event_type": "interview.build.response",
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "service": settings.service_name,
            "version": settings.service_version,
            "payload": {
                "session_id": session_id,
                "program": program,
            },
        }

        try:
            self.producer.produce(
                self.topic,
                key=session_id,
                value=json.dumps(event).encode("utf-8"),
                callback=self._delivery_callback,
            )
            self.producer.poll(0)
            duration = time.time() - start_time
            self.metrics.kafka_producer_duration.labels(
                service=settings.service_name,
                topic=self.topic
            ).observe(duration)
            logger.info("Interview program sent", session_id=session_id, topic=self.topic)
        except Exception as e:
            logger.error("Failed to send interview program", session_id=session_id, error=str(e))
            raise

    def _delivery_callback(self, err, msg):
        """Callback for message delivery"""
        if err:
            logger.error("Message delivery failed", error=str(err))
        else:
            logger.debug("Message delivered", topic=msg.topic(), partition=msg.partition())

    def flush(self, timeout: float = 10.0):
        """Flush pending messages"""
        self.producer.flush(timeout)

    def close(self):
        """Close producer"""
        self.flush()
        logger.info("Kafka producer closed")

