#!/usr/bin/env python3
"""Test script for Agent Service"""

import json
import requests
import time
from datetime import datetime
from uuid import uuid4

# Configuration
AGENT_API_URL = "http://localhost:8093"
HEALTH_URL = f"{AGENT_API_URL}/health"
PROCESS_URL = f"{AGENT_API_URL}/api/agent/process"

def test_health():
    """Test health endpoint"""
    print("=" * 60)
    print("Testing Health Endpoint")
    print("=" * 60)
    try:
        response = requests.get(HEALTH_URL, timeout=5)
        print(f"Status: {response.status_code}")
        print(f"Response: {response.json()}")
        return response.status_code == 200
    except Exception as e:
        print(f"Error: {e}")
        return False

def create_test_event(chat_id: str, user_message: str, session_id: str = None, user_id: str = None):
    """Create test event in Kafka format"""
    if not session_id:
        session_id = str(uuid4())
    if not user_id:
        user_id = str(uuid4())
    
    return {
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

def test_process_message(user_message: str, test_name: str):
    """Test processing a message"""
    print("=" * 60)
    print(f"Test: {test_name}")
    print("=" * 60)
    print(f"User message: {user_message}")
    
    chat_id = str(uuid4())
    event = create_test_event(chat_id, user_message)
    
    try:
        print(f"\nSending request to {PROCESS_URL}...")
        response = requests.post(
            PROCESS_URL,
            json=event,
            timeout=120  # LLM calls can take time
        )
        
        print(f"Status: {response.status_code}")
        
        if response.status_code == 200:
            data = response.json()
            print(f"Success: {data.get('success')}")
            generated_response = data.get('generated_response')
            if generated_response:
                print(f"Generated response: {generated_response[:200]}...")
            else:
                print("Generated response: None")
            print(f"Agent decision: {data.get('agent_state', {}).get('agent_decision', 'N/A')}")
            print(f"Question index: {data.get('agent_state', {}).get('current_question_index', 'N/A')}")
            if data.get('agent_state', {}).get('answer_evaluation'):
                eval_data = data['agent_state']['answer_evaluation']
                print(f"Evaluation - Complete: {eval_data.get('is_complete', 'N/A')}, Score: {eval_data.get('overall_score', 'N/A')}")
            if data.get('error'):
                print(f"Error in response: {data.get('error')}")
            return True
        else:
            print(f"Error response: {response.text}")
            return False
    except Exception as e:
        print(f"Error: {e}")
        import traceback
        traceback.print_exc()
        return False

def main():
    """Run all tests"""
    print("\n" + "=" * 60)
    print("Agent Service Testing Suite")
    print("=" * 60 + "\n")
    
    # Test 1: Health check
    if not test_health():
        print("\n❌ Health check failed. Is the service running?")
        return
    
    print("\n✅ Health check passed\n")
    time.sleep(1)
    
    # Test 2: Technical answer (should be evaluated)
    test_process_message(
        "L1 регуляризация добавляет сумму абсолютных значений весов к функции потерь, что приводит к обнулению некоторых весов. L2 регуляризация добавляет сумму квадратов весов, что приводит к уменьшению весов, но не обнуляет их.",
        "Test 1: Complete technical answer"
    )
    time.sleep(2)
    
    # Test 3: Incomplete answer (should trigger clarification)
    test_process_message(
        "L1 регуляризация это хорошо",
        "Test 2: Incomplete answer (should ask clarification)"
    )
    time.sleep(2)
    
    # Test 4: Off-topic message (should remind about interview)
    test_process_message(
        "Как дела? Что на ужин?",
        "Test 3: Off-topic message (should remind about interview)"
    )
    time.sleep(2)
    
    # Test 5: Another technical answer
    test_process_message(
        "Кросс-валидация k-fold разделяет данные на k частей, использует k-1 частей для обучения и 1 часть для валидации. Процесс повторяется k раз, каждый раз используя другую часть для валидации. Это позволяет более надежно оценить производительность модели.",
        "Test 4: Another complete technical answer"
    )
    
    print("\n" + "=" * 60)
    print("Testing completed!")
    print("=" * 60)

if __name__ == "__main__":
    main()
