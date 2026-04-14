"""Configuration management using Pydantic Settings"""

from pydantic_settings import BaseSettings
from pydantic import Field
from typing import Optional


class AgentConfig(BaseSettings):
    """Application settings loaded from environment variables"""

    # Service
    service_name: str = Field(default="interviewer-agent-service", alias="AGENT_SERVICE_NAME")
    service_version: str = Field(default="1.0.0", alias="AGENT_SERVICE_VERSION")
    log_level: str = Field(default="INFO", alias="LOG_LEVEL")
    log_format: str = Field(default="json", alias="LOG_FORMAT")

    # Kafka
    kafka_bootstrap_servers: str = Field(
        default="localhost:9092", alias="KAFKA_BOOTSTRAP_SERVERS"
    )
    kafka_topic_messages_full: str = Field(
        default="tensor-talks-messages.full.data", alias="KAFKA_TOPIC_MESSAGES_FULL"
    )
    kafka_topic_generated: str = Field(
        default="tensor-talks-generated.phrases", alias="KAFKA_TOPIC_GENERATED"
    )
    kafka_topic_session_completed: str = Field(
        default="tensor-talks-session.completed", alias="KAFKA_TOPIC_SESSION_COMPLETED"
    )
    kafka_consumer_group: str = Field(
        default="tensor-talks-agent-service-group", alias="KAFKA_CONSUMER_GROUP"
    )
    kafka_consumer_auto_offset_reset: str = Field(
        default="earliest", alias="KAFKA_CONSUMER_AUTO_OFFSET_RESET"
    )
    kafka_consumer_enable_auto_commit: bool = Field(
        default=False, alias="KAFKA_CONSUMER_ENABLE_AUTO_COMMIT"
    )
    kafka_consumer_max_poll_records: int = Field(
        default=100, alias="KAFKA_CONSUMER_MAX_POLL_RECORDS"
    )
    kafka_consumer_session_timeout_ms: int = Field(
        default=30000, alias="KAFKA_CONSUMER_SESSION_TIMEOUT_MS"
    )
    kafka_consumer_heartbeat_interval_ms: int = Field(
        default=10000, alias="KAFKA_CONSUMER_HEARTBEAT_INTERVAL_MS"
    )
    kafka_producer_acks: str = Field(default="all", alias="KAFKA_PRODUCER_ACKS")
    kafka_producer_retries: int = Field(default=3, alias="KAFKA_PRODUCER_RETRIES")
    kafka_producer_compression_type: str = Field(
        default="snappy", alias="KAFKA_PRODUCER_COMPRESSION_TYPE"
    )

    # LLM
    llm_provider: str = Field(
        default="openai", alias="LLM_PROVIDER"  # openai, anthropic, local
    )
    llm_base_url: Optional[str] = Field(
        default=None, alias="LLM_BASE_URL"  # For local models or proxy
    )
    llm_api_key: Optional[str] = Field(default=None, alias="LLM_API_KEY")
    llm_model: str = Field(default="gpt-5.4-mini", alias="LLM_MODEL")
    llm_model_nano: str = Field(default="gpt-5.4-nano", alias="LLM_MODEL_NANO")
    llm_temperature: float = Field(default=0.7, alias="LLM_TEMPERATURE")
    llm_max_tokens: int = Field(default=2000, alias="LLM_MAX_TOKENS")
    llm_timeout: int = Field(default=60, alias="LLM_TIMEOUT")

    # External Services
    session_service_url: str = Field(
        default="http://session-service:8083", alias="SESSION_SERVICE_URL"
    )
    chat_crud_service_url: str = Field(
        default="http://chat-crud-service:8087", alias="CHAT_CRUD_SERVICE_URL"
    )
    knowledge_base_crud_service_url: str = Field(
        default="http://knowledge-base-crud-service:8090",
        alias="KNOWLEDGE_BASE_CRUD_SERVICE_URL",
    )
    redis_host: str = Field(default="localhost", alias="REDIS_HOST")
    redis_port: int = Field(default=6379, alias="REDIS_PORT")
    redis_db: int = Field(default=0, alias="REDIS_DB")
    redis_password: Optional[str] = Field(default=None, alias="REDIS_PASSWORD")
    redis_max_connections: int = Field(
        default=50, alias="REDIS_MAX_CONNECTIONS"
    )
    redis_connection_timeout: int = Field(
        default=5, alias="REDIS_CONNECTION_TIMEOUT"
    )
    redis_socket_timeout: int = Field(default=5, alias="REDIS_SOCKET_TIMEOUT")
    redis_retry_on_timeout: bool = Field(
        default=True, alias="REDIS_RETRY_ON_TIMEOUT"
    )

    # Processing
    max_retries: int = Field(default=3, alias="MAX_RETRIES")
    processing_timeout: int = Field(default=120, alias="PROCESSING_TIMEOUT")
    enable_redis_cache: bool = Field(default=True, alias="ENABLE_REDIS_CACHE")
    max_clarifications: int = Field(default=3, alias="MAX_CLARIFICATIONS")

    # Interview flow limits (§10.13 — moved from state.py to allow env-override)
    max_hints_per_question: int = Field(default=2, alias="MAX_HINTS_PER_QUESTION")
    max_attempts_per_question: int = Field(default=3, alias="MAX_ATTEMPTS_PER_QUESTION")
    max_tool_calls_per_step: int = Field(default=5, alias="MAX_TOOL_CALLS_PER_STEP")

    # Confidence-aware decision thresholds (§10.7)
    # Scores above STRONG_NEXT unconditionally move to the next question.
    strong_next_threshold: float = Field(default=0.85, alias="STRONG_NEXT_THRESHOLD")
    # Scores in [WEAK_NEXT, STRONG_NEXT) move to next only when confidence >= this.
    weak_next_threshold: float = Field(default=0.75, alias="WEAK_NEXT_THRESHOLD")
    # Minimum confidence for a weak-zone next-question decision.
    next_confidence_threshold: float = Field(default=0.7, alias="NEXT_CONFIDENCE_THRESHOLD")
    # Scores below STRONG_HINT → hint/clarify.
    strong_hint_threshold: float = Field(default=0.4, alias="STRONG_HINT_THRESHOLD")
    # Confidence below this triggers a single re-evaluation (self-reflection).
    confidence_re_evaluate_threshold: float = Field(
        default=0.5, alias="CONFIDENCE_RE_EVALUATE_THRESHOLD"
    )

    # REST API
    rest_api_host: str = Field(default="0.0.0.0", alias="REST_API_HOST")
    rest_api_port: int = Field(default=8093, alias="REST_API_PORT")
    enable_rest_api: bool = Field(default=True, alias="ENABLE_REST_API")

    # Metrics
    metrics_port: int = Field(default=9092, alias="METRICS_PORT")
    enable_prometheus: bool = Field(
        default=True, alias="ENABLE_PROMETHEUS"
    )

    class Config:
        env_file = ".env"
        case_sensitive = False


# Global settings instance
settings = AgentConfig()
