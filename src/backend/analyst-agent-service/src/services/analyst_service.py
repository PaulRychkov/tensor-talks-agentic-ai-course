"""Main analysis service: generates reports, evaluations, presets, and progress updates."""

import json
import time
import asyncio
from typing import Any, Dict, List, Optional

from openai import AsyncOpenAI

from ..config import settings
from ..logger import get_logger
from ..metrics import get_metrics_collector
from ..models.events import SessionCompletedPayload, SessionKind
from ..models.llm_outputs import AnalystReport
from ..clients.results_client import ResultsCrudClient
from ..clients.session_client import SessionServiceClient
from ..clients.chat_client import ChatCrudClient

logger = get_logger(__name__)

REPORT_SYSTEM_PROMPT = """\
Ты — опытный технический интервьюер и преподаватель. Проанализируй приведённую сессию \
интервью/обучения и сгенерируй структурированный JSON-отчёт.

Контекст сессии:
- Тип сессии: {session_kind}
- Темы: {topics}
- Уровень: {level}

На вход ты получишь полный транскрипт чата (вопросы и ответы) и, если доступна, программу \
сессии с ожидаемыми вопросами.

ВАЖНЫЕ ПРАВИЛА:
1. В транскрипте могут встречаться системные сообщения о защите персональных данных \
(упоминания "персональные данные", "152-ФЗ", "PII"). Это СИСТЕМНЫЕ УВЕДОМЛЕНИЯ, а не часть \
технического интервью. ПОЛНОСТЬЮ ИГНОРИРУЙ их — не упоминай PII в summary, weaknesses, \
preparation_plan или materials. Оценивай ТОЛЬКО технические ответы.
2. Для "materials" указывай ТОЛЬКО общие концептуальные области или названия тем для изучения \
(например, "Логистическая регрессия — decision boundary и сигмоида"). НЕ указывай конкретные \
книги, статьи или URL — мы не можем их верифицировать, их может не существовать.
3. Оценивай ТОЛЬКО те вопросы, на которые студент действительно ответил. Неотвеченные вопросы \
не идут в минус к score.
4. ФАКТИЧЕСКАЯ ТОЧНОСТЬ ОБЯЗАТЕЛЬНА. Отлавливай все фактические ошибки, включая:
   - Переворот направления/причинности: например, "эмбеддинги преобразуют числа в текст" — \
НЕВЕРНО (эмбеддинги преобразуют текст → числа/векторы). Любая путаница A→B vs B→A — критическая ошибка.
   - Перепутанные причина/следствие, вход/выход.
   - Неверное направление процесса или преобразования.
   - Частичная правильность НЕ оправдывает ошибку в направлении — фиксируй её в errors_by_topic \
с точной неверной формулировкой и корректным ответом.
5. Будь строг: расплывчатый или неточный ответ, содержащий переворот фактов, ДОЛЖЕН быть \
помечен, даже если в целом демонстрирует частичное понимание.
6. ДВА РАЗНЫХ ПОЛЯ С РАЗНЫМ НАЗНАЧЕНИЕМ:
   - errors_by_topic — ДЛЯ ОТЧЁТА. Включай КАЖДЫЙ вопрос, где что-то пошло не так: \
фактические ошибки, "не знаю", вопросы, потребовавшие подсказок/уточнений, неточные ответы. \
Это поле видит пользователь, чтобы разобрать все проблемы сессии.
   - unmastered_topics — ДЛЯ ПРЕСЕТОВ-ПОВТОРОВ (что ещё нужно отрабатывать). Вноси сабтопик \
ТОЛЬКО если, проанализировав весь диалог по этому сабтопику (включая то, как студент реагировал \
на подсказки/уточнения), ты считаешь, что студент НЕ достиг достаточного понимания. Если студент \
испытывал затруднения в начале, но после подсказок или доп. вопросов продемонстрировал достаточное \
понимание — НЕ вноси сабтопик сюда (хотя исходные затруднения всё равно должны быть в \
errors_by_topic). Ты судья того, достигнуто ли в итоге понимание.

AVAILABLE_SUBTOPIC_IDS (используй ТОЧНО эти строки для unmastered_topics): {available_subtopics}

Верни JSON-объект ТОЧНО со следующими ключами:
{{
  "summary": "<2-4 предложения общей оценки ТЕХНИЧЕСКОГО выступления>",
  "score": <целое 0-100>,
  "errors_by_topic": {{
    "<topic_name>": [
      {{"question": "<текст вопроса>", "error": "<что было не так>", "correction": "<правильный ответ>"}}
    ]
  }},
  "strengths": ["<сильная сторона 1>", "<сильная сторона 2>"],
  "weaknesses": ["<слабая сторона 1>", "<слабая сторона 2>"],
  "preparation_plan": ["<действие 1: тема — что изучить>", "<действие 2: тема — что практиковать>"],
  "materials": ["<концепция/область для изучения и зачем — БЕЗ названий книг/статей>"],
  "topic_scores": {{
    "<topic_name>": <целое 0-100>
  }},
  "unmastered_topics": ["<subtopic_id из AVAILABLE_SUBTOPIC_IDS — только действительно не освоенные>"]
}}

Верни ТОЛЬКО валидный JSON, без markdown-обёрток и доп. текста.\
"""

STUDY_REPORT_SYSTEM_PROMPT = """\
Ты — опытный преподаватель ML. Проанализируй учебную сессию и сгенерируй структурированный JSON-отчёт.

Контекст сессии:
- Тип сессии: study
- Темы: {topics}
- Уровень: {level}

На вход ты получишь полный транскрипт чата (вопросы и ответы) и, если доступна, программу \
сессии с теорией и вопросами.

ВАЖНЫЕ ПРАВИЛА:
1. В транскрипте могут встречаться системные сообщения о защите персональных данных — \
ПОЛНОСТЬЮ ИГНОРИРУЙ их.
2. Для "materials" указывай ТОЛЬКО общие концептуальные области или названия тем для практики \
(например, "Векторные базы данных — индексирование и поиск по сходству"). НЕ указывай конкретные \
книги или URL.
3. Оценивай ТОЛЬКО те вопросы, на которые студент действительно ответил. Неотвеченные вопросы \
не идут в минус к score по п.6.
4. ФАКТИЧЕСКАЯ ТОЧНОСТЬ ОБЯЗАТЕЛЬНА. Отлавливай все фактические ошибки, включая переворот \
направления/причинности.
5. Используй учебный стиль: "студент" (не "кандидат"), "учебная сессия" (не "интервью"). \
Фокус — что изучено и что требует больше практики.
6. ПРАВИЛО SCORING — study это НЕ экзамен. По умолчанию score = 100. Снижай за:
   а) Конкретные фактические ошибки (каждая → -10…-20 баллов в зависимости от серьёзности).
   б) Неотвеченные вопросы: если студент совсем не смог ответить ("не знаю", пусто или мимо темы) — \
снижай на 5-10 баллов за каждый. Одно "не знаю" в одном вопросе допустимо (небольшое снижение), \
но если студент НЕ ответил на большинство вопросов, score должен отражать, что материал не освоен \
(например, 20-40).
   Для неотвеченных вопросов добавляй в errors_by_topic запись с "error": "Студент не смог ответить" \
и "correction" с кратким ожидаемым ответом.
7. ДВА РАЗНЫХ ПОЛЯ С РАЗНЫМ НАЗНАЧЕНИЕМ:
   - errors_by_topic — ДЛЯ ОТЧЁТА. Включай КАЖДЫЙ вопрос, где что-то пошло не так: \
фактические ошибки, "не знаю", вопросы, потребовавшие подсказок/уточнений, неточные ответы. \
Студент смотрит это поле, чтобы разобрать все проблемы сессии.
   - unmastered_points — ДЛЯ СЛЕДУЮЩЕЙ УЧЕБНОЙ СЕССИИ ПО ТОЙ ЖЕ ТЕМЕ. \
Это СПИСОК НАЗВАНИЙ ПУНКТОВ ПРОГРАММЫ (point_title), которые студент НЕ освоил. \
Берёшь названия пунктов СТРОГО из программы сессии (поля point_title). НИКАКИХ subtopic_id, \
никаких theory_*, practice_* — только человекочитаемые названия пунктов как они написаны в программе. \
Включай пункт ТОЛЬКО если, проанализировав весь диалог по нему (включая реакцию на подсказки, \
теорию, уточнения), ты считаешь, что студент НЕ достиг достаточного понимания. Если затруднения \
были в начале, но к концу студент продемонстрировал владение пунктом — НЕ вноси его сюда (но \
исходные затруднения оставь в errors_by_topic). Никогда не вноси пункты, которых студент не касался.
8. ЧЕЛОВЕКОЧИТАЕМЫЕ НАЗВАНИЯ. В полях preparation_plan, materials, weaknesses, strengths, \
errors_by_topic (ключи), topic_scores (ключи) ВСЕГДА используй понятные русскоязычные названия тем \
(например, "RAG", "Векторные базы данных", "Эмбеддинги"). НИКОГДА не вставляй технические \
идентификаторы вроде theory_rag, theory_vector_databases, practice_llm — это пользовательский текст.

Верни JSON-объект ТОЧНО со следующими ключами:
{{
  "summary": "<2-4 предложения: что студент изучил, общий уровень понимания>",
  "score": <целое 0-100 — по умолчанию 100; снижай только за конкретные фактические ошибки и неответы>,
  "errors_by_topic": {{
    "<человекочитаемое название темы, БЕЗ префиксов theory_/practice_>": [
      {{"question": "<текст вопроса>", "error": "<точная неверная формулировка>", "correction": "<правильный ответ>"}}
    ]
  }},
  "strengths": ["<тема или концепция, которую студент понял хорошо>"],
  "weaknesses": ["<тема или концепция, требующая больше практики>"],
  "preparation_plan": ["<действие: тема — что изучить или отработать (человекочитаемо!)>"],
  "materials": ["<концепция/область для повторения и зачем — БЕЗ названий книг/статей>"],
  "topic_scores": {{
    "<человекочитаемое название темы>": <целое 0-100>
  }},
  "unmastered_points": ["<точное название пункта программы (point_title), который НЕ освоен>"]
}}

Верни ТОЛЬКО валидный JSON, без markdown-обёрток и доп. текста.\
"""

PRESET_SYSTEM_PROMPT = """\
На основании приведённого ниже анализа сессии сгенерируй пресеты для follow-up сессий. \
Каждый пресет должен целиться в слабые места, выявленные в отчёте.

Summary отчёта: {summary}
Weaknesses: {weaknesses}
Errors by topic: {errors_by_topic}
Реально не освоенные темы (unmastered_topics) — именно их и нужно закрыть: {unmastered_topics}
Исходные темы: {topics}
Исходный уровень: {level}

AVAILABLE SUBTOPIC IDs — weak_topics ОБЯЗАНЫ выбираться ТОЛЬКО из этого списка:
{available_subtopics}

ПРАВИЛА:
1. Создавай пресеты ТОЛЬКО по сабтопикам из unmastered_topics. Если unmastered_topics пустой — \
верни пустой массив [].
2. errors_by_topic используй, чтобы сформулировать description пресета — перечисли конкретные \
пробелы, которые нужно закрыть.
3. Максимум пресетов = число элементов в unmastered_topics (по одному пресету на сабтопик).

Сгенерируй JSON-массив объектов-пресетов. Каждый пресет:
{{
  "name": "<описательное название пресета>",
  "description": "<на чём фокусируется пресет — конкретные пробелы из errors_by_topic>",
  "topics": ["<classic_ml|nlp|llm>"],
  "level": "<junior|middle|senior>",
  "mode": "training",
  "weak_topics": ["<ID из AVAILABLE SUBTOPIC IDs выше>"],
  "question_count": <5-15>
}}

Сгенерируй пресеты строго по unmastered_topics (по одному на сабтопик). Верни ТОЛЬКО валидный JSON-массив.\
"""

# Complete catalogue of valid subtopic IDs, grouped by topic area
SUBTOPICS_BY_TOPIC: Dict[str, List[str]] = {
    "classic_ml": [
        "theory_linear_regression",
        "theory_logistic_regression",
        "theory_gradient_descent",
        "theory_kmeans",
        "theory_overfitting",
        "theory_cross_validation",
        "theory_naive_bayes",
    ],
    "nlp": [
        "theory_tokenization",
        "theory_word_embeddings",
        "theory_attention",
        "theory_transformer",
        "theory_bert",
        "theory_rnn",
        "theory_lstm",
        "theory_gru",
        "theory_elmo",
        "theory_roberta",
        "theory_t5",
        "theory_positional_encoding",
        "theory_beam_search",
    ],
    "llm": [
        "theory_gpt",
        "theory_fine_tuning",
        "theory_rag",
        "theory_rlhf",
        "theory_prompt_engineering",
        "theory_chain_of_thought",
        "theory_vector_databases",
        "theory_llama",
    ],
}

ALL_VALID_SUBTOPICS: List[str] = [s for ids in SUBTOPICS_BY_TOPIC.values() for s in ids]


def get_subtopics_for_topics(topics: List[str]) -> List[str]:
    """Return valid subtopic IDs for the given topic areas.

    If topics list is empty or unrecognised, returns the full catalogue.
    """
    result: List[str] = []
    for t in topics:
        result.extend(SUBTOPICS_BY_TOPIC.get(t, []))
    return result if result else ALL_VALID_SUBTOPICS


class AnalystService:
    """Orchestrates session analysis via a LangGraph agent (§8 stage 4, §9 p.2.7).

    Graph: fetch_data → evaluate_answers → generate_report → validate_report
         → [pass/retry loop] → save_results → END

    Falls back to the linear pipeline if LangGraph init fails.
    """

    def __init__(
        self,
        results_client: ResultsCrudClient,
        session_client: SessionServiceClient,
        chat_client: ChatCrudClient,
    ):
        self.results_client = results_client
        self.session_client = session_client
        self.chat_client = chat_client
        self.metrics = get_metrics_collector()
        self._init_llm_client()
        self._analyst_graph = None
        self._init_graph()

    def _init_graph(self) -> None:
        """Build the LangGraph analyst state machine."""
        try:
            from ..graph.builder import create_analyst_graph
            self._analyst_graph = create_analyst_graph(self)
            logger.info("Analyst agent graph ready")
        except Exception as exc:
            logger.warning("Analyst graph init failed, will use linear pipeline", error=str(exc))
            self._analyst_graph = None

    def _init_llm_client(self):
        if settings.llm_provider in ("openai", "local"):
            api_key = settings.llm_api_key or "not-needed"
            base_url = settings.llm_base_url
            if settings.llm_provider == "local" and not base_url:
                base_url = "http://localhost:11434/v1"
            self.llm = AsyncOpenAI(
                api_key=api_key,
                base_url=base_url,
                timeout=settings.llm_timeout,
            )
        else:
            raise ValueError(f"Unsupported LLM provider: {settings.llm_provider}")
        logger.info(
            "LLM client initialized",
            provider=settings.llm_provider,
            model=settings.llm_model,
        )

    async def chat_with_tools(
        self,
        messages: list,
        tools: list,
        tool_choice: str = "auto",
    ) -> Dict[str, Any]:
        """Call the LLM with OpenAI function-calling tool definitions (§8 stage 4).

        Used by the analyst ReAct agent loop: LLM chooses which tool to invoke
        next (get_evaluations, group_errors_by_topic, generate_report_section,
        validate_report, search_knowledge_base, web_search, fetch_url) until
        it emits the final report via emit_report.

        Returns a plain-dict assistant message with role/content/tool_calls.
        """
        is_gpt5 = "gpt-5" in settings.llm_model.lower()
        params: Dict[str, Any] = {
            "model": settings.llm_model,
            "messages": messages,
            "tools": tools,
            "tool_choice": tool_choice,
        }
        if not is_gpt5:
            params["temperature"] = settings.llm_temperature
            params["max_tokens"] = settings.llm_max_tokens
        else:
            params["max_completion_tokens"] = settings.llm_max_tokens

        start = time.perf_counter()
        try:
            resp = await self.llm.chat.completions.create(**params)
            duration_ms = round((time.perf_counter() - start) * 1000, 2)
            self.metrics.llm_calls_total.labels(
                model=settings.llm_model, status="success"
            ).inc()
            logger.info(
                "Analyst tool-call completed",
                model=settings.llm_model,
                duration_ms=duration_ms,
                tokens=resp.usage.total_tokens if resp.usage else None,
            )
        except Exception as exc:
            self.metrics.llm_calls_total.labels(
                model=settings.llm_model, status="error"
            ).inc()
            logger.error("analyst chat_with_tools failed", error=str(exc))
            raise

        msg = resp.choices[0].message
        return {
            "role": "assistant",
            "content": msg.content or "",
            "tool_calls": [
                {
                    "id": tc.id,
                    "type": "function",
                    "function": {
                        "name": tc.function.name,
                        "arguments": tc.function.arguments,
                    },
                }
                for tc in (msg.tool_calls or [])
            ],
        }

    async def _call_llm(self, prompt: str, max_retries: int = 3) -> str:
        """Call LLM with retry and exponential backoff."""
        last_error = None
        for attempt in range(max_retries):
            start = time.perf_counter()
            try:
                base_params: Dict[str, Any] = {
                    "model": settings.llm_model,
                    "messages": [{"role": "user", "content": prompt}],
                    "temperature": settings.llm_temperature,
                }
                is_gpt5 = "gpt-5" in settings.llm_model.lower()
                if is_gpt5:
                    base_params.pop("temperature", None)

                token_params = (
                    ["max_completion_tokens", "max_tokens"]
                    if is_gpt5
                    else ["max_tokens", "max_completion_tokens"]
                )

                response = None
                for token_param in token_params:
                    request_params = dict(base_params)
                    request_params[token_param] = settings.llm_max_tokens
                    try:
                        with self.metrics.llm_call_duration.time():
                            response = await self.llm.chat.completions.create(**request_params)
                        break
                    except Exception as e:
                        msg = str(e)
                        if (
                            (token_param == "max_completion_tokens" and "max_completion_tokens" in msg)
                            or (token_param == "max_tokens" and "max_tokens" in msg and "not supported" in msg)
                        ):
                            continue
                        raise

                if response is None:
                    raise RuntimeError("All token parameter variants failed")

                content = response.choices[0].message.content
                if not content or not content.strip():
                    raise ValueError("Empty response from LLM")

                duration_ms = round((time.perf_counter() - start) * 1000, 2)
                self.metrics.llm_calls_total.labels(
                    model=settings.llm_model, status="success"
                ).inc()
                logger.debug(
                    "LLM call completed",
                    model=settings.llm_model,
                    attempt=attempt + 1,
                    duration_ms=duration_ms,
                    tokens=response.usage.total_tokens if response.usage else None,
                )
                return content

            except Exception as e:
                last_error = e
                self.metrics.llm_calls_total.labels(
                    model=settings.llm_model, status="error"
                ).inc()
                logger.warning(
                    "LLM call failed",
                    error=str(e),
                    attempt=attempt + 1,
                    max_retries=max_retries,
                )
                if attempt < max_retries - 1:
                    await asyncio.sleep(2 ** attempt)

        self.metrics.error_count.labels(
            error_type="llm_call_error", service=settings.service_name
        ).inc()
        raise last_error  # type: ignore[misc]

    @staticmethod
    def _parse_json_response(text: str) -> Any:
        """Extract JSON from LLM response, stripping markdown fences if present."""
        cleaned = text.strip()
        if cleaned.startswith("```"):
            lines = cleaned.split("\n")
            lines = lines[1:]  # drop opening fence
            if lines and lines[-1].strip() == "```":
                lines = lines[:-1]
            cleaned = "\n".join(lines).strip()
        return json.loads(cleaned)

    # Markers that identify PII system notifications — skip these from the transcript
    _PII_SKIP_MARKERS = (
        "персональные данные",   # PII rejection / clarification messages
        "152-ФЗ",
        "конфиденциальную информацию и было отклонено",  # PII_REDACTED_PLACEHOLDER
    )

    @staticmethod
    def _is_pii_system_message(content: str) -> bool:
        """Return True if the message is a PII-related system notification to skip."""
        lower = content.lower()
        return any(m.lower() in lower for m in AnalystService._PII_SKIP_MARKERS)

    @staticmethod
    def _format_transcript(messages: List[Dict[str, Any]]) -> str:
        """Format chat messages into a readable transcript for the LLM.

        PII system notifications (rejections, clarifications, redacted placeholders)
        are filtered out so the analyst evaluates only the technical content.
        """
        lines = []
        for msg in messages:
            # Support both lowercase (Redis) and capitalized (chat-crud DB) field names
            role = (msg.get("role") or msg.get("Type") or msg.get("type") or "unknown").upper()
            content = msg.get("content") or msg.get("Content") or ""
            if isinstance(msg.get("data"), dict):
                content = msg["data"].get("content", content)
            if not content:
                continue
            # Skip PII-related system messages — they are not technical interview content
            if AnalystService._is_pii_system_message(content):
                continue
            # Map chat-crud type names to readable roles
            if role == "SYSTEM":
                role = "INTERVIEWER"
            elif role == "USER":
                role = "CANDIDATE"
            lines.append(f"[{role}]: {content}")
        return "\n".join(lines)

    async def _generate_report(
        self,
        payload: SessionCompletedPayload,
        messages: List[Dict[str, Any]],
        program: Optional[Dict[str, Any]],
    ) -> Dict[str, Any]:
        """Generate the analysis report via LLM."""
        transcript = self._format_transcript(messages)

        topics_str = ", ".join(payload.topics) if payload.topics else "general"
        level_str = payload.level or "middle"
        if payload.session_kind == SessionKind.STUDY:
            system_prompt = STUDY_REPORT_SYSTEM_PROMPT.format(
                topics=topics_str,
                level=level_str,
            )
        else:
            system_prompt = REPORT_SYSTEM_PROMPT.format(
                session_kind=payload.session_kind,
                topics=topics_str,
                level=level_str,
                available_subtopics=", ".join(get_subtopics_for_topics(payload.topics or [])),
            )

        user_content_parts = [f"Chat transcript:\n{transcript}"]
        if program:
            user_content_parts.append(
                f"\nSession program:\n{json.dumps(program, ensure_ascii=False, indent=2)}"
            )
        if payload.total_questions is not None:
            user_content_parts.append(
                f"\nВсего вопросов в программе: {payload.total_questions}. "
                "Используй это значение для поля total_questions в отчёте."
            )
        if payload.terminated_early:
            early_info = f"\nВАЖНО: Сессия была завершена пользователем досрочно."
            if payload.answered_questions is not None and payload.total_questions is not None:
                early_info += f" Пользователь ответил на {payload.answered_questions} из {payload.total_questions} вопросов."
            early_info += " Оценивай ТОЛЬКО те вопросы, на которые действительно был дан ответ. Неотвеченные вопросы НЕ должны снижать score."
            user_content_parts.append(early_info)

        prompt = f"{system_prompt}\n\n{''.join(user_content_parts)}"
        raw = await self._call_llm(prompt)
        report = self._parse_json_response(raw)

        if not isinstance(report, dict):
            raise ValueError(f"Expected JSON object from LLM, got {type(report).__name__}")

        return report

    async def _generate_presets(
        self,
        payload: SessionCompletedPayload,
        report: Dict[str, Any],
    ) -> List[Dict[str, Any]]:
        """Generate follow-up session presets based on the report."""
        topics_list = payload.topics or []
        available = get_subtopics_for_topics(topics_list)

        unmastered_raw = report.get("unmastered_topics") or []
        valid_set = set(ALL_VALID_SUBTOPICS)
        unmastered = [t for t in unmastered_raw if t in valid_set]
        if not unmastered:
            return []

        prompt = PRESET_SYSTEM_PROMPT.format(
            summary=report.get("summary", ""),
            weaknesses=json.dumps(report.get("weaknesses", []), ensure_ascii=False),
            errors_by_topic=json.dumps(report.get("errors_by_topic", {}), ensure_ascii=False),
            unmastered_topics=json.dumps(unmastered, ensure_ascii=False),
            topics=", ".join(topics_list) if topics_list else "general",
            level=payload.level or "middle",
            available_subtopics=", ".join(available),
        )

        raw = await self._call_llm(prompt)
        presets = self._parse_json_response(raw)

        if not isinstance(presets, list):
            raise ValueError(f"Expected JSON array from LLM, got {type(presets).__name__}")

        # Filter weak_topics to only valid IDs — strip any hallucinated free-form strings
        valid_set = set(ALL_VALID_SUBTOPICS)
        for preset in presets:
            if isinstance(preset.get("weak_topics"), list):
                preset["weak_topics"] = [t for t in preset["weak_topics"] if t in valid_set]

        return presets

    @staticmethod
    def _pretty_topic(tag: str) -> str:
        if not tag:
            return ""
        t = tag
        for prefix in ("theory_", "practice_"):
            if t.startswith(prefix):
                t = t[len(prefix):]
                break
        known = {"rag": "RAG", "llm": "LLM", "ml": "ML", "nlp": "NLP",
                 "cv": "CV", "db": "БД", "sql": "SQL", "api": "API",
                 "gpt": "GPT", "bert": "BERT", "lstm": "LSTM", "gru": "GRU",
                 "rnn": "RNN", "rlhf": "RLHF", "peft": "PEFT"}
        parts = [known.get(p, p.capitalize()) for p in t.split("_") if p]
        return " ".join(parts)

    @staticmethod
    def _attach_study_details(report: Dict[str, Any], program: Dict[str, Any]) -> None:
        """Enrich a study report with the original plan and covered theory so the
        final page shows what was studied, not just how answers were graded."""
        questions = (program or {}).get("questions") or []
        if not questions:
            return

        # Plan: group by topic with counts (falls back to per-question lines)
        order: List[str] = []
        counts: Dict[str, int] = {}
        for q in questions:
            t = (q.get("topic") or "").strip()
            if not t:
                continue
            if t not in counts:
                order.append(t)
                counts[t] = 0
            counts[t] += 1

        plan_lines: List[str] = []
        if order:
            for t in order:
                label = AnalystService._pretty_topic(t)
                n = counts[t]
                plural = "пункт" if n % 10 == 1 and n % 100 != 11 else (
                    "пункта" if 2 <= n % 10 <= 4 and not (12 <= n % 100 <= 14) else "пунктов"
                )
                plan_lines.append(f"{label} — {n} {plural}")
        else:
            for i, q in enumerate(questions, 1):
                text = (q.get("question") or "").strip()
                if len(text) > 140:
                    text = text[:137] + "..."
                plan_lines.append(text or f"Тема {i}")

        report["study_plan"] = plan_lines

        theory_reviewed: List[Dict[str, Any]] = []
        for q in questions:
            theory = (q.get("theory") or "").strip()
            if not theory:
                continue
            theory_reviewed.append({
                "topic": AnalystService._pretty_topic(q.get("topic") or ""),
                "question": q.get("question") or "",
                "theory": theory,
                "order": q.get("order") or 0,
            })
        if theory_reviewed:
            report["theory_reviewed"] = theory_reviewed

    @staticmethod
    def _maybe_build_study_followup(
        payload: SessionCompletedPayload,
        report: Dict[str, Any],
        program: Optional[Dict[str, Any]],
    ) -> Optional[Dict[str, Any]]:
        """Build a SINGLE follow-up study preset focused on weak points.

        unmastered_points — free-text point titles (point_title from the program)
        that the student did NOT adequately master.  We create ONE follow-up study
        preset for the same topic(s) and pass these point titles as `focus_points`
        so the interview-builder regenerates a study program targeting exactly
        the gaps the student showed.
        """
        unmastered_points: List[str] = []
        for raw in (report.get("unmastered_points") or []):
            if isinstance(raw, str) and raw.strip():
                unmastered_points.append(raw.strip())
        if not unmastered_points:
            return None

        # Prefer the original session's subtopic IDs (e.g. "theory_rag") so the
        # follow-up study session targets the same subtopic specifically.
        # Fall back to top-level topics only if subtopics are missing.
        preset_topics = list(payload.subtopics or []) or list(payload.topics or [])
        if not preset_topics:
            return None

        description = "Слабые пункты: " + "; ".join(unmastered_points[:5])
        question_count = max(3, min(10, len(unmastered_points) * 2))

        preset = {
            "name": "Доизучение слабых пунктов",
            "description": description,
            "topics": preset_topics,
            "level": payload.level or "middle",
            "mode": "study",
            "focus_points": unmastered_points,
            "question_count": question_count,
            "source": "study_followup",
        }

        return {
            "presets": [preset],
            "unmastered_points": unmastered_points,
            "recommended_materials": report.get("materials", []),
            "follow_up_kind": "study",
        }

    @staticmethod
    def _maybe_build_training_followup(
        payload: SessionCompletedPayload,
        report: Dict[str, Any],
        program: Optional[Dict[str, Any]],
    ) -> Optional[Dict[str, Any]]:
        """Build follow-up training presets — one per subtopic in unmastered_topics.

        unmastered_topics — LLM's judgement of which subtopics are still not
        adequately mastered. Description for each preset lists the concrete gaps
        from errors_by_topic[subtopic] so the next planner sees what to address.
        Max presets = number of unmastered subtopics (<= number of subtopics in session).
        """
        unmastered_raw = report.get("unmastered_topics") or []
        valid_set = set(ALL_VALID_SUBTOPICS)
        unmastered = [t for t in unmastered_raw if t in valid_set]
        if not unmastered:
            return None

        errors_by_topic = report.get("errors_by_topic") or {}
        topic_area = (payload.topics or ["llm"])[0]

        presets = []
        for subtopic in unmastered:
            pretty = AnalystService._pretty_topic(subtopic)
            errs = errors_by_topic.get(subtopic) or []
            gap_lines: List[str] = []
            for e in errs:
                if isinstance(e, dict) and e.get("error"):
                    gap_lines.append(str(e["error"]))
                elif isinstance(e, str) and e.strip():
                    gap_lines.append(e)
            description = (
                "; ".join(gap_lines[:3])
                if gap_lines
                else f"Отработка темы: {pretty or subtopic}"
            )
            presets.append({
                "name": f"Тренировка: {pretty}" if pretty else "Тренировка по слабым местам",
                "description": description,
                # Save subtopic ID in topics so dashboard → builder routes the
                # follow-up training to the exact subtopic, not the whole area.
                "topics": [subtopic],
                "level": payload.level or "middle",
                "mode": "training",
                "weak_topics": [subtopic],
                "question_count": max(3, min(6, max(len(gap_lines), 1) * 2)),
                "source": "training_followup",
            })

        if not presets:
            return None

        return {
            "presets": presets,
            "weak_topics": unmastered,
            "recommended_materials": report.get("materials", []),
            "follow_up_kind": "training",
        }

    @staticmethod
    def _extract_topic_progress(
        report: Dict[str, Any],
        payload: SessionCompletedPayload,
    ) -> List[Dict[str, Any]]:
        """Extract per-topic progress entries from the report for study sessions."""
        topic_scores = report.get("topic_scores", {})
        errors_by_topic = report.get("errors_by_topic", {})
        progress = []

        all_topics = set(topic_scores.keys()) | set(errors_by_topic.keys())
        if not all_topics and payload.topics:
            all_topics = set(payload.topics)

        for topic in sorted(all_topics):
            score = topic_scores.get(topic, 0)
            error_count = len(errors_by_topic.get(topic, []))
            progress.append({
                "topic": topic,
                "score": score,
                "errors_count": error_count,
                "session_kind": payload.session_kind,
            })

        return progress

    async def analyze_session(self, payload: SessionCompletedPayload) -> None:
        """Full analysis pipeline for a completed session.

        Uses the LangGraph agent graph (§8 stage 4) when available; falls back
        to the linear pipeline for backward compatibility.
        """
        self.metrics.active_analyses.inc()
        start = time.perf_counter()

        # ── LangGraph agent path ─────────────────────────────────────────────
        if self._analyst_graph:
            try:
                await self._analyze_with_graph(payload)
                return
            except Exception as exc:
                logger.error(
                    "Graph analysis failed, falling back to linear pipeline",
                    session_id=payload.session_id,
                    error=str(exc),
                )

        # ── Linear pipeline fallback ─────────────────────────────────────────
        try:
            logger.info(
                "Starting session analysis (linear)",
                session_id=payload.session_id,
                session_kind=payload.session_kind,
                user_id=payload.user_id,
                chat_id=payload.chat_id,
            )

            # 1. Fetch chat messages
            messages = await self.chat_client.get_messages(payload.chat_id)
            if not messages:
                logger.warning(
                    "No messages found for session, skipping analysis",
                    session_id=payload.session_id,
                    chat_id=payload.chat_id,
                )
                self.metrics.sessions_analyzed_total.labels(
                    status="skipped", session_kind=payload.session_kind
                ).inc()
                return

            # 2. Fetch session program (best-effort)
            program = None
            try:
                program = await self.session_client.get_program(payload.session_id)
            except Exception as e:
                logger.warning(
                    "Failed to fetch program, continuing without it",
                    session_id=payload.session_id,
                    error=str(e),
                )

            # 3. Generate report via LLM
            report = await self._generate_report(payload, messages, program)
            logger.info(
                "Report generated",
                session_id=payload.session_id,
                score=report.get("score"),
                topics_analyzed=list(report.get("topic_scores", {}).keys()),
            )

            # 4. Add total_questions + early termination metadata to report
            if payload.total_questions is not None:
                report["total_questions"] = payload.total_questions
            if payload.terminated_early:
                report["terminated_early"] = True
                if payload.answered_questions is not None:
                    report["answered_questions"] = payload.answered_questions

            # 4b. For study: attach plan + theory reviewed directly from program
            # so the report shows the full learning path, not just an answer analysis.
            if payload.session_kind == SessionKind.STUDY and program:
                try:
                    self._attach_study_details(report, program)
                except Exception as e:
                    logger.warning(
                        "Failed to attach study details (non-fatal)",
                        session_id=payload.session_id,
                        error=str(e),
                    )

            # 5. For interview sessions: generate presets BEFORE saving (stored as preset_training blob)
            preset_training: Optional[Dict[str, Any]] = None
            if payload.session_kind == SessionKind.STUDY:
                preset_training = self._maybe_build_study_followup(payload, report, program)
                if preset_training:
                    logger.info(
                        "Study follow-up preset generated",
                        session_id=payload.session_id,
                        unmastered_points=preset_training.get("unmastered_points"),
                    )
            if payload.session_kind == SessionKind.TRAINING:
                preset_training = self._maybe_build_training_followup(
                    payload, report, program,
                )
                if preset_training:
                    logger.info(
                        "Training follow-up presets generated",
                        session_id=payload.session_id,
                        weak_topics=preset_training.get("weak_topics"),
                        presets_count=len(preset_training.get("presets", [])),
                    )
            if payload.session_kind == SessionKind.INTERVIEW:
                try:
                    presets = await self._generate_presets(payload, report)
                    if presets:
                        weak_topics: List[str] = []
                        for p in presets:
                            for t in p.get("weak_topics", []) + p.get("topics", []):
                                if t not in weak_topics:
                                    weak_topics.append(t)
                        preset_training = {
                            "presets": presets,
                            "weak_topics": weak_topics,
                            "recommended_materials": report.get("materials", []),
                        }
                        logger.info(
                            "Presets generated",
                            session_id=payload.session_id,
                            presets_count=len(presets),
                        )
                except Exception as e:
                    logger.error(
                        "Failed to generate presets (non-fatal)",
                        session_id=payload.session_id,
                        error=str(e),
                        exc_info=True,
                    )
                    self.metrics.error_count.labels(
                        error_type="preset_generation_error",
                        service=settings.service_name,
                    ).inc()

            # Save report (with preset_training if available) to results-crud-service
            await self.results_client.save_report(
                session_id=payload.session_id,
                user_id=payload.user_id,
                session_kind=payload.session_kind,
                report_json=report,
                preset_training=preset_training,
            )
            self.metrics.reports_generated_total.labels(
                session_kind=payload.session_kind
            ).inc()

            # 5b. Persist follow-up presets to /presets so they appear on the dashboard.
            if preset_training and preset_training.get("presets"):
                try:
                    await self.results_client.create_presets(
                        session_id=payload.session_id,
                        user_id=payload.user_id,
                        presets=preset_training["presets"],
                    )
                except Exception as e:
                    logger.error(
                        "Failed to create dashboard presets (non-fatal)",
                        session_id=payload.session_id,
                        error=str(e),
                    )

            # 6. For study sessions: update user_topic_progress
            if payload.session_kind == SessionKind.STUDY:
                try:
                    topic_progress = self._extract_topic_progress(report, payload)
                    if topic_progress:
                        await self.results_client.update_user_progress(
                            user_id=payload.user_id,
                            session_id=payload.session_id,
                            topic_progress=topic_progress,
                        )
                        logger.info(
                            "User progress updated",
                            session_id=payload.session_id,
                            topics_count=len(topic_progress),
                        )
                except Exception as e:
                    logger.error(
                        "Failed to update user progress (non-fatal)",
                        session_id=payload.session_id,
                        error=str(e),
                        exc_info=True,
                    )
                    self.metrics.error_count.labels(
                        error_type="progress_update_error",
                        service=settings.service_name,
                    ).inc()

            duration = time.perf_counter() - start
            self.metrics.analysis_duration.observe(duration)
            self.metrics.sessions_analyzed_total.labels(
                status="success", session_kind=payload.session_kind
            ).inc()

            logger.info(
                "Session analysis completed",
                session_id=payload.session_id,
                session_kind=payload.session_kind,
                duration_s=round(duration, 3),
                score=report.get("score"),
            )

        except Exception as e:
            duration = time.perf_counter() - start
            self.metrics.analysis_duration.observe(duration)
            self.metrics.sessions_analyzed_total.labels(
                status="error", session_kind=payload.session_kind
            ).inc()
            self.metrics.error_count.labels(
                error_type="analysis_error", service=settings.service_name
            ).inc()
            logger.error(
                "Session analysis failed",
                session_id=payload.session_id,
                session_kind=payload.session_kind,
                error=str(e),
                duration_s=round(duration, 3),
                exc_info=True,
            )
            raise

        finally:
            self.metrics.active_analyses.dec()

    async def _analyze_with_graph(self, payload: SessionCompletedPayload) -> None:
        """Run the LangGraph analyst agent for a completed session (§8 stage 4)."""
        from ..graph.state import AnalystState

        initial: AnalystState = {
            "session_id": payload.session_id,
            "session_kind": payload.session_kind,
            "user_id": payload.user_id,
            "chat_id": payload.chat_id,
            "topics": payload.topics or [],
            "level": payload.level or "middle",
            "terminated_early": payload.terminated_early or False,
            "answered_questions": payload.answered_questions,
            "total_questions": payload.total_questions,
            "chat_messages": [],
            "program": None,
            "evaluations": [],
            "errors_by_topic": {},
            "report": None,
            "validation_result": None,
            "validation_attempts": 0,
            "max_validation_attempts": 2,
            "presets": None,
            "topic_progress": None,
            "error": None,
        }

        self.metrics.active_analyses.inc()
        start = time.perf_counter()
        try:
            final = await self._analyst_graph.ainvoke(initial)
            duration = time.perf_counter() - start

            graph_error = final.get("error")
            if graph_error and graph_error not in ("no_messages",):
                raise RuntimeError(f"Analyst graph error: {graph_error}")

            # If agent graph completed but produced no report, fall back
            if not final.get("report"):
                raise RuntimeError("Agent graph produced empty report — falling back to linear")

            self.metrics.analysis_duration.observe(duration)
            self.metrics.sessions_analyzed_total.labels(
                status="success", session_kind=payload.session_kind
            ).inc()
            logger.info(
                "Session analysis completed (agent graph)",
                session_id=payload.session_id,
                session_kind=payload.session_kind,
                duration_s=round(duration, 3),
                score=(final.get("report") or {}).get("score"),
                validation_attempts=final.get("validation_attempts", 0),
            )
        except Exception:
            duration = time.perf_counter() - start
            self.metrics.analysis_duration.observe(duration)
            self.metrics.sessions_analyzed_total.labels(
                status="error", session_kind=payload.session_kind
            ).inc()
            raise
        finally:
            self.metrics.active_analyses.dec()

    async def close(self):
        """Close all clients."""
        await self.results_client.close()
        await self.session_client.close()
        await self.chat_client.close()
