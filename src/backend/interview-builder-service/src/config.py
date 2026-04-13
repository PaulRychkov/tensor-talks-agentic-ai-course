"""Configuration management using Pydantic Settings"""

from pydantic_settings import BaseSettings
from pydantic import Field
from typing import Optional


class Settings(BaseSettings):
    """Application settings loaded from environment variables"""

    # Service
    service_name: str = Field(default="interview-builder-service", alias="SERVICE_NAME")
    service_version: str = Field(default="1.0.0", alias="SERVICE_VERSION")
    log_level: str = Field(default="INFO", alias="LOG_LEVEL")
    log_format: str = Field(default="json", alias="LOG_FORMAT")

    # Kafka
    kafka_bootstrap_servers: str = Field(
        default="localhost:9092", alias="KAFKA_BOOTSTRAP_SERVERS"
    )
    kafka_topic_request: str = Field(
        default="team10-interview.build.request", alias="KAFKA_TOPIC_REQUEST"  # Prefixed for Kubernetes shared cluster
    )
    kafka_topic_response: str = Field(
        default="team10-interview.build.response", alias="KAFKA_TOPIC_RESPONSE"  # Prefixed for Kubernetes shared cluster
    )
    kafka_consumer_group: str = Field(
        default="team10-interview-builder-service-group", alias="KAFKA_CONSUMER_GROUP"  # Prefixed for Kubernetes shared cluster
    )

    # Questions CRUD Service
    questions_crud_url: str = Field(
        default="http://localhost:8091", alias="QUESTIONS_CRUD_URL"
    )

    # Knowledge Base CRUD Service
    knowledge_base_crud_url: str = Field(
        default="http://localhost:8090", alias="KNOWLEDGE_BASE_CRUD_URL"
    )

    # Server
    server_host: str = Field(default="0.0.0.0", alias="SERVER_HOST")
    server_port: int = Field(default=8089, alias="SERVER_PORT")

    # Metrics
    metrics_port: int = Field(default=9093, alias="METRICS_PORT")
    enable_prometheus: bool = Field(default=True, alias="ENABLE_PROMETHEUS")

    # Interview building
    questions_per_interview: int = Field(default=5, alias="QUESTIONS_PER_INTERVIEW")

    class Config:
        env_file = ".env"
        case_sensitive = False


# Global settings instance
settings = Settings()

