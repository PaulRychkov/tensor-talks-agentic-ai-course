#!/usr/bin/env python3
"""Simple component tests"""

import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent / "src"))

from dotenv import load_dotenv
load_dotenv()

def test_config():
    """Test configuration loading"""
    print("Testing configuration...")
    from src.config import settings
    print(f"✅ Service: {settings.service_name}")
    print(f"✅ Kafka: {settings.kafka_bootstrap_servers}")
    print(f"✅ LLM: {settings.llm_provider} - {settings.llm_model}")
    return True

def test_imports():
    """Test all imports"""
    print("\nTesting imports...")
    try:
        from src.graph.state import AgentState
        from src.graph.builder import create_agent_graph
        from src.llm import LLMClient
        from src.clients import SessionServiceClient, RedisClient
        from src.kafka import KafkaProducer
        print("✅ All imports OK")
        return True
    except Exception as e:
        print(f"❌ Import error: {e}")
        import traceback
        traceback.print_exc()
        return False

def test_llm_client():
    """Test LLM client initialization"""
    print("\nTesting LLM client...")
    try:
        from src.llm import LLMClient
        client = LLMClient()
        print(f"✅ LLM client initialized: {client.config.llm_provider}")
        return True
    except Exception as e:
        print(f"❌ LLM client error: {e}")
        import traceback
        traceback.print_exc()
        return False

def test_kafka_producer():
    """Test Kafka producer"""
    print("\nTesting Kafka producer...")
    try:
        from src.kafka import KafkaProducer
        producer = KafkaProducer()
        print("✅ Kafka producer initialized")
        producer.close()
        return True
    except Exception as e:
        print(f"❌ Kafka producer error: {e}")
        import traceback
        traceback.print_exc()
        return False

if __name__ == "__main__":
    print("=" * 60)
    print("Simple Component Tests")
    print("=" * 60)
    
    results = []
    results.append(test_config())
    results.append(test_imports())
    results.append(test_llm_client())
    results.append(test_kafka_producer())
    
    print("\n" + "=" * 60)
    if all(results):
        print("✅ All tests passed!")
    else:
        print("❌ Some tests failed")
    print("=" * 60)
