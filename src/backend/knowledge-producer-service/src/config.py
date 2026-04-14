"""Configuration management using Pydantic Settings"""

from pydantic_settings import BaseSettings
from pydantic import Field
from typing import Optional


class Settings(BaseSettings):
    """Application settings loaded from environment variables"""

    # Service
    service_name: str = Field(default="knowledge-producer-service", alias="SERVICE_NAME")
    service_version: str = Field(default="1.0.0", alias="SERVICE_VERSION")
    log_level: str = Field(default="INFO", alias="LOG_LEVEL")
    log_format: str = Field(default="json", alias="LOG_FORMAT")

    # Knowledge Base CRUD Service
    knowledge_base_crud_url: str = Field(
        default="http://localhost:8090", alias="KNOWLEDGE_BASE_CRUD_URL"
    )

    # Questions CRUD Service
    questions_crud_url: str = Field(
        default="http://localhost:8091", alias="QUESTIONS_CRUD_URL"
    )

    # Data paths
    knowledge_data_path: str = Field(
        default="/app/data/knowledge", alias="KNOWLEDGE_DATA_PATH"
    )
    questions_data_path: str = Field(
        default="/app/data/questions", alias="QUESTIONS_DATA_PATH"
    )

    # Server
    server_host: str = Field(default="0.0.0.0", alias="SERVER_HOST")
    server_port: int = Field(default=8092, alias="SERVER_PORT")

    # Metrics
    metrics_port: int = Field(default=9092, alias="METRICS_PORT")
    enable_prometheus: bool = Field(default=True, alias="ENABLE_PROMETHEUS")

    # Auto-load data on startup
    auto_load_on_startup: bool = Field(default=True, alias="AUTO_LOAD_ON_STARTUP")
    startup_load_retry_delay: int = Field(default=5, alias="STARTUP_LOAD_RETRY_DELAY")
    startup_load_max_retries: int = Field(default=10, alias="STARTUP_LOAD_MAX_RETRIES")

    # Dedup settings
    similarity_threshold: float = Field(default=0.8, alias="SIMILARITY_THRESHOLD")
    dedup_max_candidates: int = Field(default=10, alias="DEDUP_MAX_CANDIDATES")

    # LLM pipeline (§9 p.5.6)
    llm_base_url: Optional[str] = Field(default=None, alias="LLM_BASE_URL")
    llm_api_key: Optional[str] = Field(default=None, alias="LLM_API_KEY")
    llm_model: str = Field(default="gpt-5.4", alias="LLM_MODEL")
    llm_model_mini: str = Field(default="gpt-5.4-mini", alias="LLM_MODEL_MINI")
    llm_model_nano: str = Field(default="gpt-5.4-nano", alias="LLM_MODEL_NANO")
    llm_temperature: float = Field(default=0.3, alias="LLM_TEMPERATURE")
    llm_max_tokens: int = Field(default=4000, alias="LLM_MAX_TOKENS")
    llm_timeout: int = Field(default=60, alias="LLM_TIMEOUT")
    max_llm_calls_per_job: int = Field(default=5, alias="MAX_LLM_CALLS_PER_JOB")

    # Ingestion (§9 p.5.8)
    max_ingest_file_bytes: int = Field(default=10_000_000, alias="MAX_INGEST_FILE_BYTES")
    max_fetch_url_bytes: int = Field(default=5_000_000, alias="MAX_FETCH_URL_BYTES")
    fetch_url_timeout: int = Field(default=15, alias="FETCH_URL_TIMEOUT")

    # Web search (§9 p.5.9)
    web_search_provider: str = Field(default="none", alias="WEB_SEARCH_PROVIDER")
    web_search_api_key: Optional[str] = Field(default=None, alias="WEB_SEARCH_API_KEY")
    max_web_search_per_job: int = Field(default=3, alias="MAX_WEB_SEARCH_PER_JOB")
    max_fetch_url_per_job: int = Field(default=5, alias="MAX_FETCH_URL_PER_JOB")
    trusted_domain_patterns: str = Field(
        default="arxiv.org,pytorch.org,scikit-learn.org,tensorflow.org,huggingface.co,docs.python.org",
        alias="TRUSTED_DOMAIN_PATTERNS",
    )
    denied_domain_patterns: str = Field(default="", alias="DENIED_DOMAIN_PATTERNS")
    web_search_rate_limit_per_minute: int = Field(default=10, alias="WEB_SEARCH_RATE_LIMIT_PER_MINUTE")

    # Content types
    allowed_segment_types: str = Field(
        default="definition,intuition,example,proof,algorithm,comparison,application",
        alias="ALLOWED_SEGMENT_TYPES",
    )
    allowed_question_types: str = Field(
        default="open,coding,case,practical,applied,multiple_choice,theoretical",
        alias="ALLOWED_QUESTION_TYPES",
    )

    class Config:
        env_file = ".env"
        case_sensitive = False


# Global settings instance
settings = Settings()

