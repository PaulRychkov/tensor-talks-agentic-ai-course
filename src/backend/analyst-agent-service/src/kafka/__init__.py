"""Kafka integration"""

from .consumer import KafkaConsumer, create_kafka_consumer

__all__ = [
    "KafkaConsumer",
    "create_kafka_consumer",
]
