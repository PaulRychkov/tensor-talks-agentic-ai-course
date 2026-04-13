"""Kafka producer for publishing events"""

import json
from typing import Optional, Dict, Any
from confluent_kafka import Producer, KafkaError, KafkaException

from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector

logger = get_logger(__name__)


class KafkaProducer:
    """Kafka producer wrapper"""

    def __init__(self):
        producer_config = {
            "bootstrap.servers": settings.kafka_bootstrap_servers,
            "acks": settings.kafka_producer_acks,
            "retries": settings.kafka_producer_retries,
            "compression.type": settings.kafka_producer_compression_type,
            "max.in.flight.requests.per.connection": 5,
            "enable.idempotence": True,
        }

        self.producer = Producer(producer_config)
        self.metrics = get_metrics_collector()
        logger.info(
            "Kafka producer initialized",
            bootstrap_servers=settings.kafka_bootstrap_servers,
        )

    def _delivery_callback(self, err: Optional[KafkaError], msg, context):
        """Callback for message delivery"""
        if err is not None:
            logger.error(
                "Message delivery failed",
                error=str(err),
                topic=context.get("topic"),
                key=context.get("key"),
            )
            self.metrics.error_count.labels(
                error_type="kafka_producer_delivery",
                service=settings.service_name,
            ).inc()
        else:
            logger.debug(
                "Message delivered",
                topic=msg.topic(),
                partition=msg.partition(),
                offset=msg.offset(),
            )

    def publish(self, topic: str, event: Dict[str, Any], key: Optional[str] = None) -> None:
        """Publish single event to Kafka topic"""
        with self.metrics.kafka_producer_duration.time():
            try:
                # Use chat_id as key if available in payload
                if key is None and "payload" in event and "chat_id" in event["payload"]:
                    key = str(event["payload"]["chat_id"])

                event_json = json.dumps(event)

                context = {"topic": topic, "key": key}

                self.producer.produce(
                    topic,
                    value=event_json.encode("utf-8"),
                    key=key.encode("utf-8") if key else None,
                    callback=lambda err, msg: self._delivery_callback(
                        err, msg, context
                    ),
                )

                # Trigger delivery callbacks (non-blocking)
                self.producer.poll(0)

                logger.debug(
                    "Event published",
                    event_id=event.get("event_id"),
                    event_type=event.get("event_type"),
                    topic=topic,
                    key=key,
                )
            except Exception as e:
                logger.error(
                    "Failed to publish event",
                    event_id=event.get("event_id"),
                    topic=topic,
                    error=str(e),
                )
                self.metrics.error_count.labels(
                    error_type="kafka_producer_error", service=settings.service_name
                ).inc()
                raise

    def flush(self, timeout: float = 10.0) -> None:
        """Flush producer queue"""
        try:
            self.producer.flush(timeout=timeout)
        except KafkaException as e:
            logger.error("Failed to flush producer", error=str(e))
            raise

    def close(self) -> None:
        """Close producer"""
        self.flush()
        logger.info("Kafka producer closed")


def create_kafka_producer() -> KafkaProducer:
    """Create and return Kafka producer instance"""
    return KafkaProducer()
