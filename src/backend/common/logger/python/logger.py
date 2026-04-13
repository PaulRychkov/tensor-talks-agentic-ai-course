"""
Единый модуль логирования для всех Python-микросервисов TensorTalks.

Все логи выводятся в stdout в JSON-формате для централизованного сбора через Grafana Loki.
Формат логов соответствует единому стандарту TensorTalks для удобного парсинга и анализа.

Использование:
    from logger import get_logger
    
    logger = get_logger("auth-service", "1.0.0")
    logger.info("service started", extra={"port": 8081})
    logger.error("failed to connect", exc_info=True)
"""

import json
import logging
import sys
from datetime import datetime
from typing import Any, Dict, Optional


class JSONFormatter(logging.Formatter):
    """Форматтер для вывода логов в JSON-формате для Loki."""

    def format(self, record: logging.LogRecord) -> str:
        """Форматирует запись лога в JSON."""
        log_data: Dict[str, Any] = {
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "level": record.levelname.lower(),
            "message": record.getMessage(),
            "service": getattr(record, "service", "unknown"),
            "version": getattr(record, "version", "unknown"),
            "environment": getattr(record, "environment", "production"),
        }

        # Добавляем caller информацию
        if record.pathname:
            log_data["caller"] = f"{record.pathname}:{record.lineno}"

        # Добавляем request_id если есть
        if hasattr(record, "request_id"):
            log_data["request_id"] = record.request_id

        # Добавляем user_id если есть
        if hasattr(record, "user_id"):
            log_data["user_id"] = record.user_id

        # Добавляем дополнительные поля из extra
        if hasattr(record, "extra_fields"):
            log_data.update(record.extra_fields)

        # Добавляем exception информацию если есть
        if record.exc_info:
            log_data["stacktrace"] = self.formatException(record.exc_info)

        return json.dumps(log_data, ensure_ascii=False)


def get_logger(
    service_name: str,
    version: str,
    level: str = "INFO",
    environment: str = "production",
) -> logging.Logger:
    """
    Создаёт и настраивает логгер для микросервиса.

    Args:
        service_name: Имя сервиса (например, "auth-service")
        version: Версия сервиса (например, "1.0.0")
        level: Уровень логирования (DEBUG, INFO, WARNING, ERROR, CRITICAL)
        environment: Окружение (development, production)

    Returns:
        Настроенный logger
    """
    logger = logging.getLogger(service_name)
    logger.setLevel(getattr(logging, level.upper(), logging.INFO))

    # Удаляем существующие handlers чтобы избежать дублирования
    logger.handlers.clear()

    # Создаём handler для stdout
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JSONFormatter())

    logger.addHandler(handler)
    logger.propagate = False

    # Добавляем стандартные атрибуты к записям
    old_factory = logging.getLogRecordFactory()

    def record_factory(*args, **kwargs):
        record = old_factory(*args, **kwargs)
        record.service = service_name
        record.version = version
        record.environment = environment
        return record

    logging.setLogRecordFactory(record_factory)

    return logger


def with_request_id(logger: logging.Logger, request_id: str) -> logging.Logger:
    """
    Создаёт адаптер логгера с request_id для трейсинга запросов.

    Args:
        logger: Базовый логгер
        request_id: ID запроса

    Returns:
        Адаптер логгера
    """
    return logging.LoggerAdapter(logger, {"request_id": request_id})


def with_user_id(logger: logging.Logger, user_id: str) -> logging.Logger:
    """
    Создаёт адаптер логгера с user_id.

    Args:
        logger: Базовый логгер
        user_id: ID пользователя

    Returns:
        Адаптер логгера
    """
    return logging.LoggerAdapter(logger, {"user_id": user_id})


def with_fields(logger: logging.Logger, **fields: Any) -> logging.Logger:
    """
    Создаёт адаптер логгера с дополнительными полями.

    Args:
        logger: Базовый логгер
        **fields: Дополнительные поля для логирования

    Returns:
        Адаптер логгера
    """
    return logging.LoggerAdapter(logger, fields)
