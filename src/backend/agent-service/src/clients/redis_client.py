"""Redis client for dialogue state and messages"""

import json
import redis.asyncio as redis
from typing import Optional, List, Dict, Any

from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector

logger = get_logger(__name__)


class RedisClient:
    """Async Redis client wrapper with dialogue-specific operations"""

    def __init__(self):
        self.client = None
        self.metrics = get_metrics_collector()
        logger.info(
            "RedisClient initialized",
            host=settings.redis_host,
            port=settings.redis_port,
            db=settings.redis_db,
        )

    async def connect(self):
        """Connect to Redis"""
        if self.client is None:
            self.client = redis.Redis(
                host=settings.redis_host,
                port=settings.redis_port,
                db=settings.redis_db,
                password=settings.redis_password,
                max_connections=settings.redis_max_connections,
                socket_connect_timeout=settings.redis_connection_timeout,
                socket_timeout=settings.redis_socket_timeout,
                retry_on_timeout=settings.redis_retry_on_timeout,
                decode_responses=True,
            )
            logger.info("Redis client connected")

    def _get_state_key(self, chat_id: str) -> str:
        """Get Redis key for dialogue state"""
        return f"dialogue:{chat_id}:state"

    def _get_messages_key(self, chat_id: str) -> str:
        """Get Redis key for messages list"""
        return f"dialogue:{chat_id}:messages"

    async def get_messages(self, chat_id: str, limit: int = 50) -> List[Dict[str, Any]]:
        """Get last N messages from dialogue"""
        if not settings.enable_redis_cache:
            return []

        with self.metrics.redis_operation_duration.labels(operation="get_messages").time():
            try:
                if self.client is None:
                    await self.connect()

                key = self._get_messages_key(chat_id)
                messages_data = await self.client.lrange(key, -limit, -1)
                messages = []
                for msg_data in messages_data:
                    try:
                        msg_dict = json.loads(msg_data)
                        messages.append(msg_dict)
                    except Exception as e:
                        logger.warning(
                            "Failed to parse message from Redis",
                            chat_id=chat_id,
                            error=str(e),
                        )

                return messages
            except Exception as e:
                logger.error(
                    "Failed to get messages from Redis",
                    chat_id=chat_id,
                    error=str(e),
                )
                self.metrics.error_count.labels(
                    error_type="redis_get_messages", service=settings.service_name
                ).inc()
                return []  # Return empty list on error, don't fail

    async def get_dialogue_state(self, chat_id: str) -> Optional[Dict[str, Any]]:
        """Get dialogue state from Redis"""
        if not settings.enable_redis_cache:
            return None

        with self.metrics.redis_operation_duration.labels(operation="get_state").time():
            try:
                if self.client is None:
                    await self.connect()

                key = self._get_state_key(chat_id)
                data = await self.client.get(key)
                if data is None:
                    return None

                state_dict = json.loads(data)
                return state_dict
            except Exception as e:
                logger.error(
                    "Failed to get dialogue state from Redis",
                    chat_id=chat_id,
                    error=str(e),
                )
                self.metrics.error_count.labels(
                    error_type="redis_get_state", service=settings.service_name
                ).inc()
                return None  # Return None on error, don't fail

    async def close(self) -> None:
        """Close Redis connection"""
        if self.client:
            await self.client.close()
            self.client = None
            logger.info("Redis client closed")
