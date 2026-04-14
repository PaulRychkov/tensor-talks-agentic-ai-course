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
    session_mode: str = "interview",
) -> str:
    """Build prompt for evaluating user's answer"""

    if session_mode == "study":
        persona = "преподаватель ML, обучающий студента"
        mode_instructions = """
РЕЖИМ ОБУЧЕНИЯ (study):
- is_complete = true ТОЛЬКО если студент раскрыл ВСЕ ключевые аспекты вопроса.
- Если ответ частичный (упомянул часть, но не всё) → is_complete = false, перечисли missing_points.
- Если ответ "не знаю", "не помню", "забыл" → is_complete = false, overall_score ≤ 0.2, should_move_on = true.
- should_move_on = true ТОЛЬКО если: студент не знает ИЛИ уже было 1+ уточнение.
- should_move_on = false если: ответ частичный и ещё не было уточнений — дай студенту шанс дополнить.
- missing_points — конкретные аспекты из вопроса/теории, которые студент не упомянул.
- evaluation_reasoning — подробно, что хорошо и чего не хватает."""
    elif session_mode == "training":
        persona = "наставник на тренировочной ML-сессии"
        mode_instructions = """
РЕЖИМ ТРЕНИРОВКИ (training):
- Это практическая тренировка, не реальное интервью. Будь поддерживающим.
- should_move_on = true только если: уже было 3+ попыток ИЛИ ответ достаточно полный.
- Даже при неполном ответе — is_complete = false, should_move_on = false (давай ещё попытку).
- missing_points заполни конкретно — что именно нужно добавить в ответ.
- В evaluation_reasoning пиши развёрнуто: что хорошо, чего не хватает."""
    else:  # interview (default)
        persona = "эксперт по машинному обучению, оценивающий ответ кандидата на техническом интервью"
        mode_instructions = ""

    return f"""Ты - {persona}.

Вопрос интервьюера: {question}

Теория (референс для оценки): {theory}

Ответ кандидата: {user_message}

Контекст по текущему вопросу:
- Количество уточнений: {clarification_attempts}
- Количество подсказок: {hint_attempts}
{mode_instructions}
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
        
        session_mode = state.get("session_mode") or "interview"

        if session_mode == "study":
            return f"""Ты - преподаватель ML. Студент дал неполный ответ.

Вопрос: {question_text}

Теория (источник): {current_theory}

Ответ студента: {user_message}

Недостающие аспекты: {', '.join(missing_points) if missing_points else 'Не указаны'}

Количество уточнений: {clarification_attempts}

История последних сообщений:
{recent_context if recent_context else "История недоступна"}

ЗАДАЧА:
- Кратко (1 предложение) отметь что студент ответил правильно
- Назови ОДИН ключевой недостающий аспект (1-2 предложения)
- Задай ОДИН короткий уточняющий вопрос

ВАЖНО:
- НЕ ПОВТОРЯЙ вопросы, которые уже были заданы (смотри историю сообщений!)
- НЕ ПРОСИ переформулировать то, что студент уже сказал верно
- Если студент ответил "не знаю"/"не помню"/"забыл" — НЕ задавай уточняющий вопрос, вместо этого объясни ответ на ВОПРОС и выбери action=move_on
- Если уточнений уже >= 1 — выбирай action=move_on и объясни что студент пропустил

Когда action=move_on:
- В поле "question" объясни то, что студент не упомянул в ответе на ВОПРОС (не пересказывай всю теорию)
- 1-2 предложения на каждый пропущенный аспект

Сгенерируй решение в формате JSON (только JSON, без текста вокруг):
{{
  "action": "clarify" | "move_on",
  "question": "<если clarify: комментарий + уточняющий вопрос; если move_on: объяснение пропущенных аспектов ответа на вопрос>"
}}
"""
        elif session_mode == "training":
            return f"""Ты - наставник на тренировочной ML-сессии. Кандидат дал неполный ответ.

Вопрос: {question_text}

Теория (референс): {current_theory}

Ответ кандидата: {user_message}

Недостающие аспекты: {', '.join(missing_points) if missing_points else 'Не указаны'}

История последних сообщений:
{recent_context if recent_context else "История недоступна"}

Количество уточнений: {clarification_attempts}

ЗАДАЧА: Помоги кандидату раскрыть недостающий аспект:
- Можно кратко намекнуть на направление (1 предложение)
- Задай конкретный уточняющий вопрос об одном конкретном аспекте

ПРАВИЛО ПРОТИВ ЗАЦИКЛИВАНИЯ: если уже 3+ уточнений — выбирай action=move_on.

Сгенерируй решение в формате JSON (только JSON, без текста вокруг):
{{
  "action": "clarify" | "move_on",
  "question": "<уточняющий вопрос по конкретному аспекту; пусто если action=move_on>"
}}
"""
        else:  # interview
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
    """Build prompt for generating a hint (training/interview only; study mode never calls give_hint)."""
    current_question = (state or {}).get("current_question") or {}
    current_theory = (state or {}).get("current_theory") or ""
    user_message = (state or {}).get("user_message", "")
    question_text = current_question.get("question", "")
    session_mode = (state or {}).get("session_mode") or "interview"

    if session_mode == "training":
        return f"""Ты - наставник на тренировочной ML-сессии. Кандидат не смог полностью ответить — помоги разобраться.

Вопрос: {question_text}
Теория (референс): {current_theory}
Ответ кандидата: {user_message}

ЗАДАЧА: Дай развёрнутую подсказку:
1. Укажи ключевую концепцию, которой не хватило в ответе (1-2 предложения)
2. Дай конкретную наводящую подсказку из теории
3. Задай ОДИН упрощённый вопрос по теме

НЕ давай полный ответ — помоги кандидату додуматься самому.

Формат ответа СТРОГО:
Подсказка: <конкретная подсказка из теории 1-2 предложения>.
Вопрос: <упрощённый вопрос по теме>.
"""
    else:  # interview
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

    session_mode = state.get("session_mode") or "interview"
    current_question = state.get("current_question") or {}
    user_message = state.get("user_message", "") or ""
    evaluation = state.get("answer_evaluation")
    if not evaluation or not isinstance(evaluation, dict):
        evaluation = {}
    program = state.get("interview_program") or {}
    current_index = state.get("current_question_index")

    questions = program.get("questions", []) if program else []
    next_question: Dict[str, Any] = {}

    # Mode-dependent persona strings
    if session_mode == "study":
        persona = "преподаватель ML"
        session_label = "учебная сессия"
    elif session_mode == "training":
        persona = "наставник на тренировочной ML-сессии"
        session_label = "тренировочная сессия"
    else:
        persona = "интервьюер на техническом ML-интервью"
        session_label = "интервью"

    # Chat start: нет ещё заданных вопросов и нет ответа пользователя.
    is_chat_start = not user_message.strip()
    if is_chat_start:
        if questions:
            next_question = questions[0]
        else:
            # Нет программы – просим модель задать общий технический вопрос.
            return f"""Ты - {persona}.

{session_label.capitalize()} только начинается. Сформулируй короткое приветствие (1-2 предложения),
затем задай первый общий технический вопрос по машинному обучению.

Формат ответа: только текст приветствия и вопроса.
"""

        if session_mode == "study":
            return f"""Ты - {persona}.

Учебная сессия начинается. Коротко поприветствуй студента (1-2 предложения) и скажи что начинаем изучение.

КРИТИЧЕСКИ ВАЖНО:
- Только приветствие — вопрос и теория будут показаны студенту отдельно
- НЕ задавай вопрос в этом сообщении
- НЕ объясняй теорию
- Тон: дружелюбный преподаватель

Формат ответа: только приветствие (1-2 предложения).
"""
        elif session_mode == "training":
            return f"""Ты - {persona}.

Тренировочная сессия начинается. Поприветствуй кандидата и объясни, что это тренировка — можно делать ошибки, получать подсказки и учиться на них.
Первый вопрос программы: {next_question.get('question', '')}

КРИТИЧЕСКИ ВАЖНО:
- Короткое приветствие (1-2 предложения), упомяни что это тренировка
- Задай ТОЛЬКО ЭТОТ ОДИН вопрос
- НЕ задавай несколько вопросов
- Тон: наставник, поддерживающий, не строгий оценщик

Формат ответа: только текст приветствия и ОДНОГО вопроса. Ничего больше.
"""
        else:  # interview
            return f"""Ты - {persona}.

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
        if session_mode == "study":
            return f"""Ты - {persona}.

Все темы пройдены. Поздравь студента с завершением учебной сессии. Кратко отметь прогресс.

Формат ответа: только короткий текст завершения (1–2 предложения), без JSON."""
        elif session_mode == "training":
            return f"""Ты - {persona}.

Все вопросы тренировки разобраны. Поздравь с завершением и кратко обозначь, что стоит повторить.

Формат ответа: только короткий текст завершения (1–2 предложения), без JSON."""
        else:
            return """Ты - интервьюер на техническом ML-интервью.

Интервью подходит к завершению. Кратко поблагодари кандидата за участие и скажи,
что подробные результаты и рекомендации будут доступны в разделе результатов.

Формат ответа: только короткий текст благодарности (1–2 предложения), без JSON."""

    evaluation_reasoning = evaluation.get("evaluation_reasoning", "") if evaluation else ""
    question_text = current_question.get("question", "") if current_question else ""

    current_theory = state.get("current_theory") or ""
    overall_score = evaluation.get("overall_score", 1.0) if evaluation else 1.0
    try:
        overall_score = float(overall_score)
    except (TypeError, ValueError):
        overall_score = 1.0
    missing_points = evaluation.get("missing_points", []) if evaluation else []
    if not isinstance(missing_points, list):
        missing_points = []
    missing_text = "\n".join(f"- {p}" for p in missing_points) if missing_points else "Нет"

    if session_mode == "study" and overall_score < 0.3:
        # Student doesn't know — explain the answer to the QUESTION (not all theory)
        return f"""Ты - {persona}. Студент не смог ответить на вопрос. Объясни ему ответ.

Вопрос: {question_text}

Теория (референс для тебя): {current_theory}

Ответ студента: {user_message}

ЗАДАЧА: Объясни студенту ответ именно на ЭТОТ ВОПРОС:
- Используй теорию как источник, но отвечай строго в рамках вопроса
- Объясни простым языком, чтобы студент понял суть
- Тон: дружелюбный преподаватель, без осуждения

КРИТИЧЕСКИ ВАЖНО:
- Отвечай ТОЛЬКО на заданный вопрос — не пересказывай всю теорию
- НЕ задавай вопросов
- НЕ проси переформулировать
- Следующий вопрос студент увидит отдельно

Формат ответа: объяснение ответа на вопрос (2-4 предложения).
"""
    elif session_mode == "study":
        return f"""Ты - {persona}. Студент ответил на вопрос, переходим дальше.

Вопрос: {question_text}
Теория (референс): {current_theory}
Ответ студента: {user_message}
Пропущенные аспекты: {missing_text}

Дай обратную связь как живой преподаватель:
- Отметь что студент понял правильно, подчеркни сильные стороны ответа (1 предложение)
- Если есть пропущенные аспекты — объясни каждый в контексте вопроса (1-2 предложения на аспект)
- Не пересказывай всю теорию, не задавай вопросов
"""
    elif session_mode == "training":
        return f"""Ты - {persona}. Кандидат ответил на вопрос тренировки.

Текущий вопрос: {question_text}

Ответ кандидата: {user_message}

Оценка: {evaluation_reasoning}

Следующий вопрос программы: {next_question.get('question', '')}

Сформулируй переход к следующему вопросу:
1. Дай конструктивную обратную связь (что хорошо, что можно улучшить — 1-2 предложения)
2. Задай следующий вопрос из программы

Тон: поддерживающий наставник. НЕ задавай несколько вопросов.

Формат ответа: обратная связь и следующий вопрос.
"""
    else:  # interview
        return f"""Ты - {persona}. Кандидат дал полный ответ на вопрос.

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

    return f"""Ты - интервьюер на техническом ML-интервью. Кандидат написал сообщение, не относящееся к интервью.

Сообщение кандидата: {user_message}

{question_context}

ВАЖНО:
- НЕ начинай с "Здравствуйте" или "Спасибо за ваше сообщение"
- НЕ повторяй вопрос целиком — кандидат его помнит
- Просто вежливо и кратко напомни сосредоточиться на интервью (1 предложение)
- Если нужно, упомяни тему вопроса одним словом, но не цитируй вопрос

Формат ответа: одно короткое вежливое предложение-напоминание. Никаких повторений вопроса.

Пример хорошего ответа:
Пожалуйста, сосредоточьтесь на вопросах интервью.
"""


def build_final_evaluation_prompt(state: Dict[str, Any]) -> str:
    """Build prompt for final session score and recommendations (mode-aware)"""
    if not state or not isinstance(state, dict):
        state = {}
    session_mode = state.get("session_mode") or "interview"
    
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
    
    if session_mode == "study":
        persona = "преподаватель ML"
        session_label = "Учебная сессия"
        score_guidance = "Отражай учебный прогресс: если студент активно участвовал и понял хотя бы половину тем — балл выше 60. Штрафуй только за полное непонимание ключевых концепций."
        recommendations_guidance = "В рекомендациях укажи: какие темы стоит повторить, что понято хорошо, что требует дополнительной практики. Тон поддерживающий, не оценочный."
    elif session_mode == "training":
        persona = "наставник на тренировочной ML-сессии"
        session_label = "Тренировочная сессия"
        score_guidance = "Отражай уровень подготовки к реальному интервью. Учитывай прогресс: если кандидат улучшал ответы с подсказками — это плюс. Не штрафуй за первоначальные ошибки, которые были исправлены."
        recommendations_guidance = "В рекомендациях укажи: что отработано хорошо, что требует дополнительной тренировки, конкретные темы для изучения. Тон наставника, ориентированного на рост."
    else:
        persona = "интервьюер на техническом ML-интервью"
        session_label = "Интервью"
        score_guidance = "сильные, корректные ответы БЕЗ существенных ошибок → более высокий балл; наличие фактических ошибок, противоречий теории, пропущенных ключевых тем ДОЛЖНО СИЛЬНО снижать итоговый балл; если по нескольким вопросам были серьёзные ошибки или непонимание базовой теории, итоговый скор не может быть высоким."
        recommendations_guidance = "В рекомендациях ОБЯЗАТЕЛЬНО отрази главные ошибки кандидата. ПЕРВЫЙ пункт ОБЯЗАТЕЛЬНО: \"Ключевые ошибки: ...\" с 1-3 главными ошибками по фактам."

    return f"""Ты - {persona}. {session_label} завершено, все {total_questions} вопросов программы заданы.

История диалога:
{history_text if history_text else "История недоступна"}

Сводка проблемных оценок (если были):
{summary_text}

На основе всей истории сессии и сводки оценок:
1. Определи ИТОГОВЫЙ СКОР (final_score) в диапазоне 0-100:
   - {score_guidance}
2. Сформируй дружелюбное сообщение завершения сессии (1-2 предложения)
3. Сформируй список конкретных рекомендаций (3-5 пунктов):
   - {recommendations_guidance}
   - Не добавляй темы, которых нет в вопросах/истории диалога
   - Если ошибок не было, дай рекомендации по углублению знаний

Ответь в формате JSON:
{{
    "final_score": <целое число 0-100>,
    "thank_you_message": "<текст завершения сессии>",
    "recommendations": [
        "<рекомендация 1>",
        "<рекомендация 2>",
        ...
    ]
}}
"""
