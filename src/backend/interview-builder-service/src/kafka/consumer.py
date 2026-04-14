"""Kafka consumer for interview.build.request events"""

import json
import threading
from typing import Callable, Optional
from confluent_kafka import Consumer, KafkaError
from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector
from ..schemas import BuildRequest

logger = get_logger(__name__)


class KafkaConsumer:
    """Kafka consumer for receiving interview build requests"""

    def __init__(self, bootstrap_servers: str = None):
        self.bootstrap_servers = bootstrap_servers or settings.kafka_bootstrap_servers
        self.topic = settings.kafka_topic_request
        self.group_id = settings.kafka_consumer_group
        self.handler: Optional[Callable] = None
        self.running = False
        self.thread: Optional[threading.Thread] = None
        self.metrics = get_metrics_collector()

        config = {
            "bootstrap.servers": self.bootstrap_servers,
            "group.id": self.group_id,
            "auto.offset.reset": "earliest",
            "enable.auto.commit": False,
        }

        self.consumer = Consumer(config)
        logger.info("Kafka consumer initialized",
                     bootstrap_servers=self.bootstrap_servers,
                     topic=self.topic,
                     group_id=self.group_id)

    def set_handler(self, handler: Callable):
        """Set event handler"""
        self.handler = handler

    def start(self):
        """Start consuming messages"""
        if self.running:
            logger.warning("Consumer already running")
            return

        self.consumer.subscribe([self.topic])
        self.running = True
        self.thread = threading.Thread(target=self._consume_loop, daemon=True)
        self.thread.start()
        logger.info("Kafka consumer started", topic=self.topic)

    def _consume_loop(self):
        """Main consumption loop"""
        while self.running:
            try:
                msg = self.consumer.poll(timeout=1.0)

                if msg is None:
                    continue

                if msg.error():
                    if msg.error().code() == KafkaError._PARTITION_EOF:
                        continue
                    logger.error("Consumer error", error=str(msg.error()))
                    continue

                try:
                    event = json.loads(msg.value().decode("utf-8"))
                except json.JSONDecodeError as e:
                    logger.error("Failed to decode message JSON", error=str(e))
                    self.consumer.commit(msg)
                    continue

                event_type = event.get("event_type")
                if event_type != "interview.build.request":
                    logger.warning("Unknown event type, skipping",
                                   event_type=event_type)
                    self.consumer.commit(msg)
                    continue

                payload = event.get("payload", {})
                try:
                    request = BuildRequest.from_raw_payload(payload)
                except Exception as e:
                    logger.error("Invalid build request payload",
                                 error=str(e),
                                 event_type=event_type,
                                 session_id=payload.get("session_id"),
                                 parse_error_code="invalid_payload")
                    self.consumer.commit(msg)
                    continue

                logger.info("Received interview build request",
                            session_id=request.session_id,
                            mode=request.params.mode.value,
                            source=request.params.source.value,
                            topics=request.params.topics,
                            subtopics=request.params.subtopics,
                            level=request.params.level)

                if self.handler:
                    try:
                        self.handler(request.session_id, request.params.model_dump())
                        self.metrics.kafka_consumer_messages_total.labels(
                            service=settings.service_name,
                            topic=self.topic,
                            status="success"
                        ).inc()
                    except Exception as e:
                        self.metrics.kafka_consumer_messages_total.labels(
                            service=settings.service_name,
                            topic=self.topic,
                            status="error"
                        ).inc()
                        logger.error("Handler error",
                                     session_id=request.session_id,
                                     error=str(e),
                                     exc_info=True)

                self.consumer.commit(msg)

            except Exception as e:
                logger.error("Consumer loop error", error=str(e), exc_info=True)
                if not self.running:
                    break

    def stop(self):
        """Stop consuming messages"""
        self.running = False
        if self.thread:
            self.thread.join(timeout=5.0)
        self.consumer.close()
        logger.info("Kafka consumer stopped")
