"""Agent state schema for LangGraph"""

from typing import TypedDict, List, Optional, Literal
from datetime import datetime


class AgentState(TypedDict):
    """Состояние агента в LangGraph"""

    # Входные данные
    chat_id: str
    session_id: str
    user_id: str
    message_id: str
    user_message: str
    message_timestamp: datetime
    question_id: str

    # История диалога
    dialogue_history: List[dict]  # [{role, content, timestamp}, ...]
    dialogue_state: Optional[dict]  # DialogueState из Redis

    # Программа интервью
    interview_program: Optional[dict]  # InterviewProgram
    total_questions: int
    current_question_index: Optional[int]  # Определяется агентом
    current_question_id: Optional[str]
    current_question: Optional[dict]  # QuestionItem
    current_theory: Optional[str]

    # Оценка ответа
    answer_evaluation: Optional[dict]
    # Структура:
    #   completeness_score: float  # 0.0 - 1.0
    #   accuracy_score: float  # 0.0 - 1.0
    #   overall_score: float  # 0.0 - 1.0
    #   is_complete: bool
    #   missing_points: List[str]
    #   evaluation_reasoning: str

    # Решение агента
    agent_decision: Optional[
        Literal[
            "ask_clarification",  # Задать уточняющий вопрос
            "give_hint",  # Дать подсказку и упростить вопрос
            "next_question",  # Перейти к следующему вопросу
            "off_topic_reminder",  # Напомнить об интервью
            "thank_you",  # Благодарность за интервью
            "error",  # Ошибка обработки
        ]
    ]

    # Специальные флаги обработки
    hint_request: Optional[bool]
    skip_question_request: Optional[bool]

    # Сгенерированный ответ
    generated_response: Optional[str]
    generated_question: Optional[str]
    response_metadata: Optional[dict]

    # Метаданные обработки
    processing_steps: List[str]  # Для отладки
    error: Optional[str]
    retry_count: int
