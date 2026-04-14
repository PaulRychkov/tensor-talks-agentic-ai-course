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
        default="tensor-talks-interview.build.request", alias="KAFKA_TOPIC_REQUEST"
    )
    kafka_topic_response: str = Field(
        default="tensor-talks-interview.build.response", alias="KAFKA_TOPIC_RESPONSE"
    )
    kafka_consumer_group: str = Field(
        default="tensor-talks-interview-builder-service-group", alias="KAFKA_CONSUMER_GROUP"
    )

    # Questions CRUD Service
    questions_crud_url: str = Field(
        default="http://localhost:8091", alias="QUESTIONS_CRUD_URL"
    )

    # Knowledge Base CRUD Service
    knowledge_base_crud_url: str = Field(
        default="http://localhost:8090", alias="KNOWLEDGE_BASE_CRUD_URL"
    )

    # Results CRUD Service (episodic memory §10.6)
    results_crud_url: str = Field(
        default="http://results-crud-service:8088", alias="RESULTS_CRUD_URL"
    )

    # Server
    server_host: str = Field(default="0.0.0.0", alias="SERVER_HOST")
    server_port: int = Field(default=8089, alias="SERVER_PORT")

    # Metrics
    metrics_port: int = Field(default=9093, alias="METRICS_PORT")
    enable_prometheus: bool = Field(default=True, alias="ENABLE_PROMETHEUS")

    # LLM (§9 interview-builder-service.md: GPT-5.4-mini main, GPT-5.4-nano light)
    llm_base_url: Optional[str] = Field(default=None, alias="LLM_BASE_URL")
    llm_api_key: Optional[str] = Field(default=None, alias="LLM_API_KEY")
    llm_model: str = Field(default="gpt-5.4-mini", alias="LLM_MODEL")
    llm_model_nano: str = Field(default="gpt-5.4-nano", alias="LLM_MODEL_NANO")
    llm_temperature: float = Field(default=0.3, alias="LLM_TEMPERATURE")
    llm_max_tokens: int = Field(default=2000, alias="LLM_MAX_TOKENS")
    llm_timeout: int = Field(default=60, alias="LLM_TIMEOUT")

    # Interview building
    questions_per_interview: int = Field(default=5, alias="QUESTIONS_PER_INTERVIEW")

    # Mode-specific ratios (plan §9 p.1.2.2)
    max_same_topic_ratio: float = Field(default=0.6, alias="MAX_SAME_TOPIC_RATIO")
    training_practice_ratio: float = Field(default=0.7, alias="TRAINING_PRACTICE_RATIO")
    study_theory_ratio: float = Field(default=0.7, alias="STUDY_THEORY_RATIO")

    # Study mode dynamic counts: questions per chosen subtopic.
    # Total questions = sum across selected subtopics, capped by available material.
    study_per_subtopic_max: int = Field(default=8, alias="STUDY_PER_SUBTOPIC_MAX")
    study_per_subtopic_min: int = Field(default=3, alias="STUDY_PER_SUBTOPIC_MIN")

    class Config:
        env_file = ".env"
        case_sensitive = False


settings = Settings()
