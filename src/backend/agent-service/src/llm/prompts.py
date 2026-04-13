"""Prompt templates for LLM interactions"""

from typing import List, Dict, Any
from ..logger import get_logger

logger = get_logger(__name__)


def build_off_topic_prompt(user_message: str, recent_messages: List[Dict[str, Any]]) -> str:
    """Build prompt for checking if message is off-topic"""
    if not recent_messages:
        recent_messages = []
    messages_text = "\n".join(
        [
            f"{msg.get('role', 'unknown')}: {msg.get('content', '')}"
            for msg in recent_messages[-5:] if msg and isinstance(msg, dict)
        ]
    )

    return f"""Ты - интервьюер на техническом ML-интервью. Пользователь отправил сообщение.

Сообщение пользователя: {user_message}

История диалога:
{messages_text}

КРИТИЧЕСКИ ВАЖНО: Следующие типы сообщений НЕ являются off-topic и ДОЛЖНЫ быть классифицированы как "answer":
- Запросы помощи по текущему вопросу (например, "можно подсказку", "забыл как расшифровывается", "что это значит", "можете напомнить", "что такое X")
- Запросы уточнения по текущей теме (например, "что такое семантические отношения", "объясни подробнее", "расскажи про X")
- Неполные ответы на текущий вопрос (например, "не помню", "не знаю", "забыл", "не помню что такое X")
- Вопросы пользователя, содержащие термины из текущего вопроса интервью
- Запросы пропустить вопрос (например, "давай пропустим", "не знаю, пропустим")

Off-topic - это ТОЛЬКО сообщения, которые СОВЕРШЕННО не связаны с интервью:
- Вопросы о погоде, личных делах, других темах, не связанных с ML/техническим интервью
- Комментарии типа "привет", "как дела", "спасибо за интервью" (если интервью еще не закончено)

Определи, является ли сообщение:
1. Ответом на вопрос интервьюера (включая ВСЕ запросы помощи, уточнения, неполные ответы) - ответь "answer"
2. Вопросом пользователя о чем-то, СОВЕРШЕННО не связанном с интервью - ответь "off_topic"
3. Комментарием или репликой, не относящейся к интервью - ответь "comment"

Ответь только одним словом: "answer", "off_topic", или "comment"
"""


def build_question_index_prompt(
    program: Dict[str, Any], history: List[Dict[str, Any]]
) -> str:
    """Build prompt for determining current question index"""
    if not program or not isinstance(program, dict):
        program = {}
    questions_text = "\n".join(
        [
            f"{q.get('order', '?')}. {q.get('question', '')}"
            for q in program.get("questions", []) if q and isinstance(q, dict)
        ]
    )

    history_text = "\n".join(
        [
            f"{msg.get('role', 'unknown')}: {msg.get('content', '')}"
            for msg in (history[-10:] if history else [])
            if msg
        ]
    )

    return f"""Ты - интервьюер на техническом ML-интервью. Проанализируй историю диалога и определи, на каком вопросе из программы интервью сейчас находится диалог.

Программа интервью:
{questions_text}

История диалога:
{history_text}

Определи:
1. Номер текущего вопроса из программы (order) - если вопрос еще не задан, верни null
2. Был ли задан уточняющий вопрос после последнего ответа пользователя

Ответь в формате JSON:
{{
    "current_question_order": <int или null>,
    "clarification_asked": <true/false>,
    "reasoning": "<объяснение>"
}}
"""


def build_evaluation_prompt(
    question: str,
    theory: str,
    user_message: str,
    clarification_attempts: int = 0,
    hint_attempts: int = 0,
) -> str:
    """Build prompt for evaluating user's answer"""
    return f"""Ты - эксперт по машинному обучению, оценивающий ответ кандидата на техническом интервью.

Вопрос интервьюера: {question}

Теория (референс для оценки): {theory}

Ответ кандидата: {user_message}

Контекст по текущему вопросу:
- Количество уточнений: {clarification_attempts}
- Количество подсказок: {hint_attempts}

ВАЖНО: Если кандидат ответил "не помню", "не знаю", "забыл", "не уверен", "подзабыл", "что такое X", "как расшифровывается X", "можно подсказку", "можете напомнить" или дал очень короткий/неполный ответ, это НЕ ошибка. Это просто неполный ответ, который требует уточнения. Такие ответы должны быть оценены как НЕПОЛНЫЕ (is_complete: false).

КРИТИЧЕСКИ ВАЖНО: Если в ответе кандидата есть запрос помощи/уточнения (например, "что такое исчезающие градиенты подзабыл", "забыл а как расшифровывается Dropout"), это означает, что кандидат НЕ дал полный ответ на текущий вопрос и требуется уточнение. Оцени такой ответ как НЕПОЛНЫЙ (is_complete: false).

КРИТИЧЕСКИ ВАЖНО:
- Оценивай ТОЛЬКО по вопросу и теории, не добавляй требований от себя.
- Если теория пустая, оценивай строго по формулировке вопроса.
- Если ответ содержит фактические ошибки или утверждения, ПРОТИВОРЕЧАЩИЕ вопросу/теории, это должно снижать оценку:
  - accuracy_score ≤ 0.3
  - overall_score ≤ 0.4
  - is_complete = false
  - обязательно перечисли ошибки в поле errors и укажи их в evaluation_reasoning.

Оцени ответ кандидата по следующим критериям:
1. Полнота (completeness) - насколько полно раскрыт вопрос (0.0 - 1.0)
   - Если ответ "не помню"/"не знаю" или очень короткий (< 10 слов) → completeness_score = 0.0-0.2
   - Если ответ частично раскрывает вопрос → completeness_score = 0.3-0.6
   - Если ответ полный → completeness_score = 0.7-1.0
2. Точность (accuracy) - насколько правильно ответ (0.0 - 1.0)
   - Если ответ "не помню" → accuracy_score = 0.0 (нельзя оценить точность)
   - Если есть технические ошибки → accuracy_score = 0.0-0.5
   - Если ответ правильный → accuracy_score = 0.7-1.0
3. Общая оценка (overall) - среднее между полнотой и точностью

Также определи:
- Является ли ответ полным (is_complete: true/false)
  - "не помню", "не знаю", очень короткие ответы (< 10 слов) → is_complete: false
  - Ответ закрывает все аспекты, прямо указанные в вопросе → is_complete: true
- Какие аспекты не раскрыты (missing_points: список строк)
- Какие ошибки допущены (errors: список строк, каждая ошибка — кратко и по существу)
- Нужна ли подсказка по запросу кандидата (wants_hint: true/false)
- Кандидат просит пропустить вопрос (skip_question_request: true/false)
- Следует ли переходить к следующему вопросу (should_move_on: true/false)
  - ВАЖНО: прежде чем ставить should_move_on=true, сначала попробуй помочь кандидату:
    - если ответ неверный или неполный — СНАЧАЛА задай 1–2 точных уточняющих вопроса ИЛИ дай подсказку;
    - только если уже были попытки уточнений/подсказок и ответ всё равно в корне неверный/противоречит теории ИЛИ кандидат явно не знает и не просит подсказку, ставь should_move_on=true;
  - если ещё не было ни одного уточнения/подсказки по этому вопросу, обычно should_move_on=false, даже при неправильном ответе.
- Краткое объяснение оценки (evaluation_reasoning: строка)

ВАЖНО ДЛЯ missing_points:
- missing_points должны быть конкретными аспектами, прямо вытекающими из вопроса и теории.
- Если ты НЕ можешь назвать конкретные недостающие аспекты, установи is_complete: true и оставь missing_points пустым.

Ответь в формате JSON:
{{
    "completeness_score": <float 0.0-1.0>,
    "accuracy_score": <float 0.0-1.0>,
    "overall_score": <float 0.0-1.0>,
    "is_complete": <true/false>,
    "missing_points": ["<пункт 1>", "<пункт 2>", ...],
    "errors": ["<ошибка 1>", "<ошибка 2>", ...],
    "wants_hint": <true/false>,
    "skip_question_request": <true/false>,
    "should_move_on": <true/false>,
    "evaluation_reasoning": "<объяснение>"
}}
"""


def build_clarification_prompt(state: Dict[str, Any]) -> str:
    """Build prompt for generating clarification question"""
    # Wrap entire function in try-except for maximum safety
    try:
        # Ensure state is a dict - defensive programming
        if state is None or not isinstance(state, dict):
            state = {}
        
        current_question = state.get("current_question") or {}
        current_theory = state.get("current_theory") or ""
        user_message = state.get("user_message", "")
        evaluation = state.get("answer_evaluation")
        if not evaluation or not isinstance(evaluation, dict):
            evaluation = {}
        missing_points = evaluation.get("missing_points", [])
        if not isinstance(missing_points, list):
            missing_points = []
        
        question_text = current_question.get('question', '') if current_question else ''
        clarification_attempt = state.get("clarification_attempt", 1)
        clarification_attempts = state.get("clarification_attempts", 0)
        history = state.get("dialogue_history", [])
        
        # Build recent conversation context for LLM to understand the situation
        recent_context = ""
        if history:
            # Get last 6 messages (3 user + 3 assistant) for context
            recent_messages = history[-6:]
            recent_context = "\n".join([
                f"{msg.get('role', 'unknown')}: {msg.get('content', '')[:200]}"
                for msg in recent_messages if msg
            ])
        
        return f"""Ты - интервьюер на техническом ML-интервью. Кандидат дал неполный ответ.

Текущий вопрос из программы: {question_text}

Теория (референс): {current_theory}

Ответ кандидата: {user_message}

Недостающие аспекты: {', '.join(missing_points) if missing_points else 'Не указаны'}

История последних сообщений:
{recent_context if recent_context else "История недоступна"}

Количество уточнений по этому вопросу: {clarification_attempts}

КРИТИЧЕСКИ ВАЖНО - ЗАПРЕЩЕНО:
- НЕ объясняй тему и НЕ давай подсказки
- НЕ давай фидбек о качестве ответа
- НЕ повторяй исходный вопрос дословно
- НЕДОПУСТИМО использовать общие формулировки типа "Можете уточнить ваш ответ?"

РАЗРЕШЕНО ТОЛЬКО:
- Задать уточняющий вопрос с конкретным аспектом (например: "Можете уточнить [конкретный аспект]?")
- Предложить перейти к следующему вопросу, если кандидат явно не может продолжать

ОСНОВЫВАЙСЯ СТРОГО на вопросе и теории:
- НЕ добавляй новые требования, которых нет в вопросе/теории.
- Если theory пустая, опирайся только на формулировку вопроса.

ПРАВИЛО ПРОТИВ ЗАЦИКЛИВАНИЯ:
- Если уже были уточнения по этому вопросу и кандидат ответил по сути, не задавай новое уточнение — выбирай action=move_on.
- Уточняй только то, что напрямую требуется формулировкой вопроса.

Сгенерируй решение в формате JSON (только JSON, без текста вокруг):
{{
  "action": "clarify" | "move_on",
  "question": "<уточняющий вопрос по конкретному аспекту; пусто, если action=move_on>"
}}

Правила:
- Если action=move_on, question пустое.
- Если action=clarify, question ОБЯЗАТЕЛЕН и должен ссылаться на конкретный аспект.
"""
    except Exception as e:
        # If anything goes wrong, return a minimal prompt that will work
        logger.error("Error building clarification prompt", error=str(e), exc_info=True)
        return """Ты - интервьюер на техническом ML-интервью. Кандидат дал неполный ответ.

Сгенерируй краткую подсказку (1 предложение) и упрощенный вопрос по той же теме. Будь вежливым.

Формат ответа: только текст (подсказка + вопрос).
"""


def build_hint_prompt(state: Dict[str, Any]) -> str:
    """Build prompt for generating a short hint and a simplified question."""
    current_question = (state or {}).get("current_question") or {}
    current_theory = (state or {}).get("current_theory") or ""
    user_message = (state or {}).get("user_message", "")
    question_text = current_question.get("question", "")
    return f"""Ты - интервьюер. Кандидат явно не знает ответ.

Текущий вопрос: {question_text}
Теория (референс): {current_theory}
Ответ кандидата: {user_message}

КРИТИЧЕСКИ ВАЖНО:
- НЕ проси "уточнить" и НЕ задавай уточняющий вопрос.
- Дай подсказку и ОДИН упрощенный вопрос.
- Подсказка должна быть СТРОГО по теории.

Формат ответа СТРОГО:
Подсказка: <1 короткое предложение>.
Вопрос: <упрощенный вопрос по теме>.
"""

def build_next_question_prompt(state: Dict[str, Any]) -> str:
    """Build prompt for generating greeting / transition to the next question."""
    if not state or not isinstance(state, dict):
        state = {}

    current_question = state.get("current_question") or {}
    user_message = state.get("user_message", "") or ""
    evaluation = state.get("answer_evaluation")
    if not evaluation or not isinstance(evaluation, dict):
        evaluation = {}
    program = state.get("interview_program") or {}
    current_index = state.get("current_question_index")

    questions = program.get("questions", []) if program else []
    next_question: Dict[str, Any] = {}

    # Chat start: нет ещё заданных вопросов и нет ответа пользователя.
    is_chat_start = not user_message.strip()
    if is_chat_start:
        if questions:
            next_question = questions[0]
        else:
            # Нет программы – просим модель задать общий технический вопрос.
            return """Ты - интервьюер на техническом ML-интервью.

Интервью только начинается. Сформулируй короткое приветствие (1-2 предложения),
затем задай первый общий технический вопрос по машинному обучению.

Формат ответа: только текст приветствия и вопроса.
"""
        return f"""Ты - интервьюер на техническом ML-интервью.

Интервью только начинается. У тебя есть заранее подготовленная программа вопросов.
Первый вопрос программы:
{next_question.get('question', '')}

КРИТИЧЕСКИ ВАЖНО:
- Сформулируй ТОЛЬКО короткое приветствие (1-2 предложения)
- Затем задай ТОЛЬКО ЭТОТ ОДИН первый вопрос из программы
- НЕ задавай несколько вопросов
- НЕ добавляй дополнительные вопросы или комментарии
- Будь вежливым, профессиональным и дружелюбным
- НЕ используй агрессивный или грубый тон

Формат ответа: только текст приветствия и ОДНОГО вопроса. Ничего больше.
"""

    # Обычный случай: переходим к следующему вопросу после ответа кандидата.
    if current_index is not None:
        for q in questions:
            if q and isinstance(q, dict) and q.get("order", 0) == current_index + 1:
                next_question = q
                break

    # Если следующего вопроса нет – это крайний случай, обычно сюда не попадаем,
    # потому что завершение интервью обрабатывается узлом принятия решения (thank_you).
    # На всякий случай сформулируем вежливое текстовое завершение без JSON.
    if not next_question:
        return """Ты - интервьюер на техническом ML-интервью.

Интервью подходит к завершению. Кратко поблагодари кандидата за участие и скажи,
что подробные результаты и рекомендации будут доступны в разделе результатов.

Формат ответа: только короткий текст благодарности (1–2 предложения), без JSON."""

    evaluation_reasoning = evaluation.get("evaluation_reasoning", "") if evaluation else ""
    question_text = current_question.get("question", "") if current_question else ""

    return f"""Ты - интервьюер на техническом ML-интервью. Кандидат дал полный ответ на вопрос.

Текущий вопрос: {question_text}

Ответ кандидата: {user_message}

Оценка: {evaluation_reasoning}

Следующий вопрос из программы: {next_question.get('question', '')}

Сформулируй плавный переход от текущего вопроса к следующему.
Сначала кратко подтверди, что ответ хороший (1-2 предложения),
затем задай следующий вопрос из программы.

Формат ответа: текст перехода и нового вопроса.
"""


def build_off_topic_reminder_prompt(state: Dict[str, Any]) -> str:
    """Build prompt for reminding user about interview"""
    if not state or not isinstance(state, dict):
        state = {}
    user_message = state.get("user_message", "")
    history = state.get("dialogue_history") or []
    
    # Сначала пытаемся получить текущий вопрос из программы интервью
    current_question_obj = state.get("current_question")
    current_question_text = None
    if current_question_obj and isinstance(current_question_obj, dict):
        current_question_text = current_question_obj.get("question", "")
    
    # Если текущего вопроса нет, ищем в истории
    last_question = current_question_text
    if not last_question and history:
        for msg in reversed(history[-10:]):
            if msg and msg.get("role") == "assistant":
                content = msg.get("content", "")
                # Извлекаем вопрос, убирая префиксы типа "Вопрос: "
                if content.startswith("Вопрос: "):
                    content = content[8:]
                last_question = content
                break
    
    # Если и в истории нет, пытаемся взять первый вопрос из программы
    if not last_question:
        program = state.get("interview_program")
        if program and isinstance(program, dict):
            questions = program.get("questions", [])
            if questions and len(questions) > 0:
                first_q = questions[0]
                if isinstance(first_q, dict):
                    last_question = first_q.get("question", "")

    question_context = f"Текущий вопрос интервью: {last_question}" if last_question else ""

    return f"""Ты - интервьюер на техническом ML-интервью. Кандидат задал вопрос или написал сообщение, не относящееся к интервью.

Сообщение кандидата: {user_message}

{question_context}

ВАЖНО: 
- НЕ начинай с "Здравствуйте" или "Спасибо за ваше сообщение"
- НЕ используй фразы типа "Напоминаю, что сейчас у нас идет техническое интервью"
- Просто вежливо и кратко напомни, что сейчас идет интервью
- {'Обязательно повтори текущий вопрос интервью' if last_question else 'Попроси ответить на вопрос интервьюера'}

Формат ответа: короткое вежливое напоминание (1-2 предложения) и {'обязательно повтори текущий вопрос интервью' if last_question else 'призыв к ответу'}.

Пример хорошего ответа:
{'Пожалуйста, сосредоточьтесь на интервью. ' + last_question if last_question else 'Пожалуйста, отвечайте на вопросы интервью.'}
"""


def build_final_evaluation_prompt(state: Dict[str, Any]) -> str:
    """Build prompt for final interview score and recommendations"""
    if not state or not isinstance(state, dict):
        state = {}
    
    history = state.get("dialogue_history", [])
    program = state.get("interview_program", {})
    total_questions = state.get("total_questions", 0)
    
    # Формируем историю вопросов и ответов
    history_text = ""
    if history:
        for msg in history[-20:]:  # Последние 20 сообщений
            if msg and isinstance(msg, dict):
                role = msg.get("role", "unknown")
                content = msg.get("content", "")
                if content:
                    history_text += f"{role}: {content}\n"
    
    # Собираем все оценки из истории (если они есть в метаданных)
    evaluations_summary = []
    for msg in history:
        if msg and isinstance(msg, dict) and msg.get("role") == "assistant":
            metadata = msg.get("metadata", {})
            if metadata and isinstance(metadata, dict):
                eval_data = metadata.get("evaluation", {})
                if eval_data and isinstance(eval_data, dict):
                    overall = eval_data.get("overall_score", 0.0)
                    missing = eval_data.get("missing_points", [])
                    errors = eval_data.get("errors", [])
                    if overall < 0.7 or missing or errors:
                        evaluations_summary.append({
                            "question_id": metadata.get("question_id"),
                            "score": overall,
                            "accuracy": eval_data.get("accuracy_score"),
                            "completeness": eval_data.get("completeness_score"),
                            "missing": missing,
                            "errors": errors,
                            "reasoning": eval_data.get("evaluation_reasoning", ""),
                        })
    
    summary_text = "Нет проблемных оценок."
    if evaluations_summary:
        summary_lines = []
        for item in evaluations_summary:
            summary_lines.append(
                f"- question_id={item.get('question_id')}, "
                f"score={item.get('score')}, accuracy={item.get('accuracy')}, "
                f"completeness={item.get('completeness')}, "
                f"missing={item.get('missing')}, errors={item.get('errors')}, "
                f"reasoning={item.get('reasoning')}"
            )
        summary_text = "\n".join(summary_lines)
    
    return f"""Ты - интервьюер на техническом ML-интервью. Интервью завершено, все {total_questions} вопросов программы заданы.

История диалога:
{history_text if history_text else "История недоступна"}

Сводка проблемных оценок (если были):
{summary_text}

На основе всей истории интервью и сводки оценок:
1. Определи ИТОГОВЫЙ СКОР (final_score) в диапазоне 0-100:
   - сильные, корректные ответы БЕЗ существенных ошибок → более высокий балл;
   - наличие фактических ошибок, противоречий теории, пропущенных ключевых тем ДОЛЖНО СИЛЬНО снижать итоговый балл;
   - если по нескольким вопросам были серьёзные ошибки или непонимание базовой теории, итоговый скор не может быть высоким.
2. Сформируй дружелюбное и профессиональное сообщение благодарности кандидату за участие
3. Сформируй список конкретных рекомендаций для дальнейшего развития (3-5 пунктов) на основе:
   - Общей оценки ответов и final_score
   - Обнаруженных пробелов в знаниях (missing_points)
   - Областей, где были ошибки (errors)
4. В рекомендациях ОБЯЗАТЕЛЬНО отрази главные ошибки кандидата:
   - Ссылайся на конкретные неверные утверждения или пропуски из ответов
   - Не добавляй темы, которых нет в вопросах/истории диалога
   - Если ошибок не было, укажи, что основные ответы корректны, и дай рекомендации по углублению
5. ПОРЯДОК РЕКОМЕНДАЦИЙ:
   - ПЕРВЫЙ пункт ОБЯЗАТЕЛЬНО: "Ключевые ошибки: ..." с 1-3 главными ошибками по фактам.
   - Только затем остальные рекомендации.

Ответь в формате JSON:
{{
    "final_score": <целое число 0-100>,
    "thank_you_message": "<текст благодарности>",
    "recommendations": [
        "<рекомендация 1>",
        "<рекомендация 2>",
        ...
    ]
}}
"""
