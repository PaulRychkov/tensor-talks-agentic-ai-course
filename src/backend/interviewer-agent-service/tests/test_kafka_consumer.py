"""Tests for src.kafka.consumer – KafkaConsumer."""

from __future__ import annotations

import json
import time
import threading
from datetime import datetime, timezone
from types import SimpleNamespace
from unittest.mock import MagicMock, patch, AsyncMock

import pytest

from src.models.events import EventType


# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------

def _make_event_dict(
    event_type: str = "message.full",
    session_id: str | None = "sess-1",
    chat_id: str | None = None,
    event_id: str = "evt-1",
) -> dict:
    payload = {}
    if session_id is not None:
        payload["session_id"] = session_id
    if chat_id is not None:
        payload["chat_id"] = chat_id
    return {
        "event_id": event_id,
        "event_type": event_type,
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "service": "test-service",
        "version": "1.0.0",
        "payload": payload,
    }


def _make_kafka_msg(event_dict: dict, error=None):
    """Create a mock confluent_kafka Message."""
    msg = MagicMock()
    msg.error.return_value = error
    msg.value.return_value = json.dumps(event_dict).encode("utf-8")
    msg.topic.return_value = "test-topic"
    msg.partition.return_value = 0
    msg.offset.return_value = 1
    return msg


def _build_consumer(mock_settings, mock_metrics):
    """Build a KafkaConsumer with mocked dependencies."""
    with (
        patch("src.kafka.consumer.Consumer") as MockConsumer,
        patch("src.kafka.consumer.settings", mock_settings),
        patch("src.kafka.consumer.get_metrics_collector", return_value=mock_metrics),
    ):
        mock_consumer_instance = MagicMock()
        MockConsumer.return_value = mock_consumer_instance

        from src.kafka.consumer import KafkaConsumer
        consumer = KafkaConsumer(topic="test-topic", group_id="test-group")

    consumer.metrics = mock_metrics
    return consumer, mock_consumer_instance


# ===================================================================
# _accepted_events filtering
# ===================================================================

class TestAcceptedEvents:

    def test_accepted_event_types(self, mock_settings, mock_metrics):
        consumer, _ = _build_consumer(mock_settings, mock_metrics)
        accepted = consumer._accepted_events

        assert EventType.MESSAGE_FULL in accepted
        assert "chat.started" in accepted
        assert "chat.resumed" in accepted
        assert "chat.user_message" in accepted

    def test_unaccepted_type_not_in_set(self, mock_settings, mock_metrics):
        consumer, _ = _build_consumer(mock_settings, mock_metrics)
        assert "some.random.event" not in consumer._accepted_events
        assert "chat.deleted" not in consumer._accepted_events


# ===================================================================
# poll() – message parsing
# ===================================================================

class TestPoll:

    def test_poll_returns_event_on_valid_message(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        event_dict = _make_event_dict()
        raw.poll.return_value = _make_kafka_msg(event_dict)

        events = consumer.poll(timeout=0.1)
        assert len(events) == 1
        assert events[0].event_id == "evt-1"

    def test_poll_returns_empty_on_none(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        raw.poll.return_value = None

        assert consumer.poll() == []

    def test_poll_handles_parse_error(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        msg = MagicMock()
        msg.error.return_value = None
        msg.value.return_value = b"not-json"
        msg.topic.return_value = "t"
        msg.partition.return_value = 0
        msg.offset.return_value = 0
        raw.poll.return_value = msg

        events = consumer.poll()
        assert events == []
        mock_metrics.error_count.labels.assert_called_with(
            error_type="kafka_consumer_parse_error",
            service=mock_settings.service_name,
        )


# ===================================================================
# start() consume loop – event filtering & session_id validation
# ===================================================================

class TestConsumeLoop:

    def _run_loop_once(self, consumer, raw, events_or_msgs, handler=None, iterations=1):
        """Helper: set up poll side-effects, start, and let it run briefly."""
        if handler:
            consumer.set_handler(handler)

        side_effects = list(events_or_msgs) + [None] * 5
        raw.poll.side_effect = side_effects

        call_count_before = raw.poll.call_count
        consumer.start()
        deadline = time.time() + 3
        while raw.poll.call_count < call_count_before + len(events_or_msgs) + 1:
            if time.time() > deadline:
                break
            time.sleep(0.05)
        consumer.stop()
        time.sleep(0.15)

    def test_accepted_event_calls_handler(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        handler = MagicMock()
        event_dict = _make_event_dict(event_type="message.full", session_id="s1")

        self._run_loop_once(consumer, raw, [_make_kafka_msg(event_dict)], handler)
        handler.assert_called_once()

    def test_skipped_event_does_not_call_handler(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        handler = MagicMock()
        event_dict = _make_event_dict(event_type="session.completed", session_id="s1")

        self._run_loop_once(consumer, raw, [_make_kafka_msg(event_dict)], handler)
        handler.assert_not_called()

    def test_missing_session_id_skipped_with_metric(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        handler = MagicMock()
        event_dict = _make_event_dict(event_type="message.full", session_id=None)

        self._run_loop_once(consumer, raw, [_make_kafka_msg(event_dict)], handler)
        handler.assert_not_called()
        mock_metrics.error_count.labels.assert_any_call(
            error_type="invalid_payload",
            service=mock_settings.service_name,
        )

    def test_chat_id_used_when_session_id_missing(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        handler = MagicMock()
        event_dict = _make_event_dict(event_type="message.full", session_id=None, chat_id="chat-1")

        self._run_loop_once(consumer, raw, [_make_kafka_msg(event_dict)], handler)
        handler.assert_called_once()

    def test_handler_exception_does_not_crash_loop(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        handler = MagicMock(side_effect=RuntimeError("boom"))
        ev1 = _make_event_dict(event_type="message.full", session_id="s1", event_id="evt-1")
        ev2 = _make_event_dict(event_type="message.full", session_id="s2", event_id="evt-2")

        self._run_loop_once(
            consumer, raw,
            [_make_kafka_msg(ev1), _make_kafka_msg(ev2)],
            handler,
        )
        assert handler.call_count == 2

    def test_async_handler_support(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        handler = AsyncMock()
        event_dict = _make_event_dict(event_type="message.full", session_id="s1")

        self._run_loop_once(consumer, raw, [_make_kafka_msg(event_dict)], handler)
        handler.assert_called_once()

    def test_no_handler_still_commits(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        event_dict = _make_event_dict(event_type="message.full", session_id="s1")

        consumer.handler = None
        side_effects = [_make_kafka_msg(event_dict), None, None, None]
        raw.poll.side_effect = side_effects
        consumer.start()
        time.sleep(0.5)
        consumer.stop()
        time.sleep(0.15)
        raw.commit.assert_called()


# ===================================================================
# stop / close
# ===================================================================

class TestLifecycle:

    def test_stop_sets_running_false(self, mock_settings, mock_metrics):
        consumer, _ = _build_consumer(mock_settings, mock_metrics)
        consumer.running = True
        consumer.stop()
        assert consumer.running is False

    def test_close_calls_consumer_close(self, mock_settings, mock_metrics):
        consumer, raw = _build_consumer(mock_settings, mock_metrics)
        consumer.close()
        raw.close.assert_called_once()
