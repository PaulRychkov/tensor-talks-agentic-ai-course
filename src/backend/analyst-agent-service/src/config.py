"""Configuration management using Pydantic Settings"""

from pydantic_settings import BaseSettings
from pydantic import Field
from typing import Optional


class AgentConfig(BaseSettings):
    """Application settings loaded from environment variables"""

    # Service
    service_name: str = Field(default="analyst-agent-service", alias="AGENT_SERVICE_NAME")
    service_version: str = Field(default="1.0.0", alias="AGENT_SERVICE_VERSION")
    log_level: str = Field(default="INFO", alias="LOG_LEVEL")
    log_format: str = Field(default="json", alias="LOG_FORMAT")

    # Kafka
    kafka_bootstrap_servers: str = Field(
        default="localhost:9092", alias="KAFKA_BOOTSTRAP_SERVERS"
    )
    kafka_topic_session_completed: str = Field(
        default="tensor-talks-session.completed", alias="KAFKA_TOPIC_SESSION_COMPLETED"
    )
    kafka_consumer_group: str = Field(
        default="tensor-talks-analyst-agent-service-group", alias="KAFKA_CONSUMER_GROUP"
    )
    kafka_consumer_auto_offset_reset: str = Field(
        default="earliest", alias="KAFKA_CONSUMER_AUTO_OFFSET_RESET"
    )
    kafka_consumer_enable_auto_commit: bool = Field(
        default=False, alias="KAFKA_CONSUMER_ENABLE_AUTO_COMMIT"
    )
    kafka_consumer_session_timeout_ms: int = Field(
        default=30000, alias="KAFKA_CONSUMER_SESSION_TIMEOUT_MS"
    )
    kafka_consumer_heartbeat_interval_ms: int = Field(
        default=10000, alias="KAFKA_CONSUMER_HEARTBEAT_INTERVAL_MS"
    )

    # LLM
    llm_provider: str = Field(default="openai", alias="LLM_PROVIDER")
    llm_base_url: Optional[str] = Field(default=None, alias="LLM_BASE_URL")
    llm_api_key: Optional[str] = Field(default=None, alias="LLM_API_KEY")
    llm_model: str = Field(default="gpt-5.4", alias="LLM_MODEL")
    llm_model_mini: str = Field(default="gpt-5.4-mini", alias="LLM_MODEL_MINI")
    llm_model_nano: str = Field(default="gpt-5.4-nano", alias="LLM_MODEL_NANO")
    llm_temperature: float = Field(default=0.4, alias="LLM_TEMPERATURE")
    llm_max_tokens: int = Field(default=4000, alias="LLM_MAX_TOKENS")
    llm_timeout: int = Field(default=120, alias="LLM_TIMEOUT")

    # External Services
    results_crud_service_url: str = Field(
        default="http://tensor-talks-results-crud-service:8088", alias="RESULTS_CRUD_URL"
    )
    session_service_url: str = Field(
        default="http://tensor-talks-session-service:8083", alias="SESSION_SERVICE_URL"
    )
    chat_crud_service_url: str = Field(
        default="http://tensor-talks-chat-crud-service:8087", alias="CHAT_CRUD_URL"
    )
    knowledge_producer_service_url: str = Field(
        default="http://tensor-talks-knowledge-producer-service:8095",
        alias="KNOWLEDGE_PRODUCER_URL",
    )

    # Processing
    max_retries: int = Field(default=3, alias="MAX_RETRIES")
    processing_timeout: int = Field(default=180, alias="PROCESSING_TIMEOUT")

    # REST API
    rest_api_host: str = Field(default="0.0.0.0", alias="REST_API_HOST")
    rest_api_port: int = Field(default=8094, alias="REST_API_PORT")

    # Metrics
    metrics_port: int = Field(default=9094, alias="METRICS_PORT")
    enable_prometheus: bool = Field(default=True, alias="ENABLE_PROMETHEUS")

    class Config:
        env_file = ".env"
        case_sensitive = False


settings = AgentConfig()
