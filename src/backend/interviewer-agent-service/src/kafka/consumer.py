"""Kafka consumer for consuming events"""

import json
import asyncio
import inspect
import uuid
from typing import List, Callable, Optional
from confluent_kafka import Consumer, KafkaError, KafkaException

import structlog

from ..config import settings
from ..logger import get_logger
from ..models.events import KafkaEvent, EventType
from ..metrics import get_metrics_collector

logger = get_logger(__name__)


class KafkaConsumer:
    """Kafka consumer wrapper"""

    def __init__(self, topic: str, group_id: str):
        self.topic = topic
        self.group_id = group_id
        self.handler: Optional[Callable] = None

        consumer_config = {
            "bootstrap.servers": settings.kafka_bootstrap_servers,
            "group.id": group_id,
            "auto.offset.reset": settings.kafka_consumer_auto_offset_reset,
            "enable.auto.commit": str(settings.kafka_consumer_enable_auto_commit).lower(),
            "session.timeout.ms": str(settings.kafka_consumer_session_timeout_ms),
            "heartbeat.interval.ms": str(settings.kafka_consumer_heartbeat_interval_ms),
        }

        self.consumer = Consumer(consumer_config)
        self.consumer.subscribe([topic])
        self.metrics = get_metrics_collector()
        self.running = False
        self._accepted_events = {
            EventType.MESSAGE_FULL,
            EventType.CHAT_STARTED,
            EventType.CHAT_RESUMED,
            EventType.CHAT_USER_MESSAGE,
        }
        logger.info(
            "Kafka consumer initialized",
            topic=topic,
            group_id=group_id,
            bootstrap_servers=settings.kafka_bootstrap_servers,
        )

    def set_handler(self, handler: Callable):
        """Set event handler function"""
        self.handler = handler

    def poll(self, timeout: float = 1.0) -> List[KafkaEvent]:
        """Poll for messages from Kafka"""
        events = []
        try:
            msg = self.consumer.poll(timeout=timeout)
            if msg is None:
                return events

            if msg.error():
                if msg.error().code() == KafkaError._PARTITION_EOF:
                    logger.debug(
                        "Reached end of partition",
                        topic=msg.topic(),
                        partition=msg.partition(),
                    )
                else:
                    logger.error(
                        "Consumer error",
                        error=str(msg.error()),
                        topic=msg.topic(),
                    )
                    self.metrics.error_count.labels(
                        error_type="kafka_consumer_error",
                        service=settings.service_name,
                    ).inc()
                return events

            try:
                event_dict = json.loads(msg.value().decode("utf-8"))
                event = KafkaEvent(**event_dict)
                events.append(event)

                logger.debug(
                    "Event consumed",
                    event_id=event.event_id,
                    event_type=event.event_type,
                    topic=msg.topic(),
                    partition=msg.partition(),
                    offset=msg.offset(),
                )

            except Exception as e:
                logger.error(
                    "Failed to parse event",
                    error=str(e),
                    topic=msg.topic(),
                    partition=msg.partition(),
                    offset=msg.offset(),
                )
                self.metrics.error_count.labels(
                    error_type="kafka_consumer_parse_error",
                    service=settings.service_name,
                ).inc()

        except KafkaException as e:
            logger.error("Kafka consumer exception", error=str(e))
            self.metrics.error_count.labels(
                error_type="kafka_consumer_exception", service=settings.service_name
            ).inc()

        return events

    def commit(self) -> None:
        """Commit current offsets"""
        try:
            self.consumer.commit(asynchronous=False)
        except Exception as e:
            logger.error("Failed to commit offsets", error=str(e))
            self.metrics.error_count.labels(
                error_type="kafka_consumer_commit_error",
                service=settings.service_name,
            ).inc()
            raise

    def start(self):
        """Start consuming messages in a loop"""
        import threading

        self.running = True
        self._is_handler_async = None  # Cache for handler type check

        def consume_loop():
            logger.info(f"Starting Kafka consumer loop for topic {self.topic}")
            # Create event loop for async handlers
            loop = None
            if self.handler and inspect.iscoroutinefunction(self.handler):
                loop = asyncio.new_event_loop()
                asyncio.set_event_loop(loop)
                self._is_handler_async = True
            else:
                self._is_handler_async = False

            try:
                while self.running:
                    try:
                        events = self.poll(timeout=1.0)
                        for event in events:
                            if not self.handler:
                                self.commit()
                                continue

                            if event.event_type not in self._accepted_events:
                                logger.debug(
                                    "Skipping unhandled event_type",
                                    event_type=event.event_type,
                                    event_id=event.event_id,
                                )
                                self.commit()
                                continue

                            payload = event.payload or {}
                            session_id = payload.get("session_id") or payload.get("chat_id", "")
                            if not session_id:
                                logger.warning(
                                    "Event missing session_id/chat_id, skipping",
                                    event_id=event.event_id,
                                    event_type=event.event_type,
                                )
                                self.metrics.error_count.labels(
                                    error_type="invalid_payload",
                                    service=settings.service_name,
                                ).inc()
                                self.commit()
                                continue

                            trace_id = getattr(event, "trace_id", None) or str(uuid.uuid4())
                            structlog.contextvars.bind_contextvars(trace_id=trace_id)
                            try:
                                if self._is_handler_async:
                                    loop.run_until_complete(self.handler(event))
                                else:
                                    self.handler(event)
                                self.commit()
                            except Exception as e:
                                logger.error(
                                    "Error handling event",
                                    event_id=event.event_id,
                                    event_type=event.event_type,
                                    session_id=session_id,
                                    error=str(e),
                                    exc_info=True,
                                )
                                self.commit()
                            finally:
                                structlog.contextvars.unbind_contextvars("trace_id")
                    except Exception as e:
                        if self.running:
                            logger.error(
                                "Error in consumer loop",
                                error=str(e),
                                exc_info=True,
                            )
                        import time
                        time.sleep(1)
            finally:
                if loop:
                    loop.close()

            logger.info(f"Kafka consumer loop stopped for topic {self.topic}")

        thread = threading.Thread(target=consume_loop, daemon=True)
        thread.start()
        logger.info("Kafka consumer thread started")

    def stop(self):
        """Stop consuming messages"""
        self.running = False
        logger.info("Stopping Kafka consumer")

    def close(self) -> None:
        """Close consumer"""
        self.stop()
        self.consumer.close()
        logger.info("Kafka consumer closed", topic=self.topic, group_id=self.group_id)


def create_kafka_consumer(topic: str, group_id: str) -> KafkaConsumer:
    """Create and return Kafka consumer instance"""
    return KafkaConsumer(topic, group_id)
