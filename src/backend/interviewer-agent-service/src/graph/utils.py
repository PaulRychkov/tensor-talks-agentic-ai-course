"""Utility functions for graph nodes"""

from typing import List, Dict, Any, Optional


def check_clarification_in_history(
    history: List[Dict[str, Any]], question_index: Optional[int]
) -> bool:
    """Check if clarification question was asked after last user answer"""
    # Analyze last assistant messages
    for msg in reversed(history[-10:]):  # Last 10 messages
        if msg.get("role") == "assistant":
            # Simple heuristic: check for clarification keywords
            content = msg.get("content", "").lower()
            clarification_keywords = [
                "уточни",
                "расскажи подробнее",
                "можешь пояснить",
                "дополни",
                "расширь",
                "что именно",
            ]
            if any(keyword in content for keyword in clarification_keywords):
                return True
    return False


def count_clarification_attempts(
    history: List[Dict[str, Any]], question_index: Optional[int]
) -> int:
    """Count how many times clarification was asked for current question"""
    count = 0
    # Look for pattern: user says "не помню" -> assistant asks clarification question
    # Count such patterns in reverse order (most recent first)
    i = len(history) - 1
    while i >= 0 and i >= len(history) - 20:  # Check last 20 messages
        msg = history[i]
        if msg.get("role") == "user":
            content = msg.get("content", "").lower()
            # Check if user said "не помню", "забыл", etc.
            unknown_phrases = ["не помню", "забыл", "забыла", "не знаю", "не знаю что", "не помню что"]
            if any(phrase in content for phrase in unknown_phrases):
                # Look for assistant clarification question after this user message
                if i + 1 < len(history):
                    next_msg = history[i + 1]
                    if next_msg.get("role") == "assistant":
                        assistant_content = next_msg.get("content", "").lower()
                        # Check if it's a clarification question
                        clarification_keywords = [
                            "уточни",
                            "расскажи подробнее",
                            "можешь пояснить",
                            "дополни",
                            "расширь",
                            "что именно",
                            "можете рассказать",
                            "можете, пожалуйста",
                            "что вы знаете",
                            "можете, пожалуйста, рассказать",
                        ]
                        is_question = "?" in assistant_content
                        if any(keyword in assistant_content for keyword in clarification_keywords) or is_question:
                            count += 1
        i -= 1
    return count


def extract_session_id_from_state(state: Dict[str, Any]) -> Optional[str]:
    """Extract session_id from dialogue_state"""
    dialogue_state = state.get("dialogue_state")
    if dialogue_state and isinstance(dialogue_state, dict):
        return dialogue_state.get("session_id")
    return None
