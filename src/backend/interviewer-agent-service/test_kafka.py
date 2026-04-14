#!/usr/bin/env python3
"""Test script for sending Kafka events to agent service"""

import json
import time
from datetime import datetime
from uuid import uuid4
from confluent_kafka import Producer

# Configuration
KAFKA_BOOTSTRAP_SERVERS = "localhost:29092"
TOPIC = "tensor-talks-messages.full.data"

def create_message_full_event(chat_id: str, user_message: str, session_id: str = None, user_id: str = None):
    """Create message.full event"""
    if not session_id:
        session_id = str(uuid4())
    if not user_id:
        user_id = str(uuid4())
    
    event = {
        "event_id": str(uuid4()),
        "event_type": "message.full",
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "service": "test-service",
        "version": "1.0.0",
        "payload": {
            "chat_id": chat_id,
            "message_id": str(uuid4()),
            "role": "user",
            "content": user_message,
            "metadata": {
                "user_id": user_id,
                "message_index": 1,
                "dialogue_context": {
                    "session_id": session_id,
                    "total_messages": 2,
                    "dialogue_type": "ml_interview",
                    "status": "active",
                    "topic": "ml",
                    "difficulty": "middle",
                    "current_question_index": 1
                }
            },
            "source": "user_input",
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "processed_at": datetime.utcnow().isoformat() + "Z"
        },
        "metadata": {
            "correlation_id": str(uuid4())
        }
    }
    return event

def send_kafka_event(producer: Producer, topic: str, event: dict, chat_id: str):
    """Send event to Kafka"""
    try:
        event_json = json.dumps(event)
        producer.produce(
            topic,
            value=event_json.encode('utf-8'),
            key=chat_id.encode('utf-8'),
            callback=lambda err, msg: delivery_callback(err, msg, event.get('event_id'))
        )
        producer.poll(0)
        print(f"✅ Event sent: {event.get('event_id')} - {event['payload']['content'][:50]}...")
        return True
    except Exception as e:
        print(f"❌ Error sending event: {e}")
        return False

def delivery_callback(err, msg, event_id):
    """Delivery callback"""
    if err:
        print(f"❌ Message delivery failed for {event_id}: {err}")
    else:
        print(f"📨 Message delivered: topic={msg.topic()}, partition={msg.partition()}, offset={msg.offset()}")

def main():
    """Send test events to Kafka"""
    print("=" * 60)
    print("Kafka Event Testing")
    print("=" * 60)
    
    # Create producer
    producer = Producer({
        'bootstrap.servers': KAFKA_BOOTSTRAP_SERVERS,
        'acks': 'all',
        'retries': 3,
    })
    
    chat_id = str(uuid4())
    session_id = str(uuid4())
    user_id = str(uuid4())
    
    test_messages = [
        ("L1 регуляризация добавляет сумму абсолютных значений весов к функции потерь.", "Complete answer"),
        ("L1 это хорошо", "Incomplete answer"),
        ("Как дела?", "Off-topic message"),
        ("Кросс-валидация k-fold разделяет данные на k частей для более надежной оценки модели.", "Another complete answer"),
    ]
    
    for i, (message, description) in enumerate(test_messages, 1):
        print(f"\n--- Test {i}: {description} ---")
        event = create_message_full_event(chat_id, message, session_id, user_id)
        send_kafka_event(producer, TOPIC, event, chat_id)
        time.sleep(3)  # Wait between messages
    
    # Flush and wait for delivery
    print("\n⏳ Flushing producer...")
    producer.flush(10)
    
    print("\n✅ All events sent!")
    print(f"📊 Check agent-service logs to see processing results")
    print(f"📊 Check generated.phrases topic for agent responses")

if __name__ == "__main__":
    main()
