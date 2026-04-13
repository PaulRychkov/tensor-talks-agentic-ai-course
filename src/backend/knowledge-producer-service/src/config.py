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

    class Config:
        env_file = ".env"
        case_sensitive = False


# Global settings instance
settings = Settings()

