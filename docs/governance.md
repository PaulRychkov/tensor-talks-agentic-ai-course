# Риски и правила: TensorTalks

## register рисков

| Риск | Вероятность | Влияние | Обнаружение | Защита | Остаточный риск |
|------|-------------|---------|-------------|--------|-----------------|
| Prompt injection (сломать логику, вытащить промпт) | Высокая | Высокое | Eval-кейсы, ручная разметка | Жёсткие системные инструкции, отделение данных от инструкций, пост-проверки | Средний |
| Утечка PII в логи или в LLM | Средняя | Высокое | Аудит логов, поиск PII-паттернов | Маскирование PII, минимальные логи в проде, сроки хранения | Низкий-средний |
| Dev-логи с текстом сообщений попадают в прод | Средняя | Высокое | Ревью конфигов, smoke-тесты | Флаг окружения, запрет content в прод-логах | Низкий |
| Галлюцинации в рекомендациях | Высокая | Среднее | Тест-кейсы, ручная проверка | Привязка к источникам, шаблонные фоллбеки | Средний |
| Нестабильная оценка (score) | Средняя | Высокое | Сравнение с ручной оценкой, контроль версий промптов | Единый источник score, калибровка | Средний |
| Зацикливание (много уточнений) | Средняя | Среднее | Метрики "ходов на вопрос" | Лимиты, стратегия "двигаемся дальше" | Низкий |
| Перерасход токенов | Средняя | Высокое | Метрики стоимости на интервью | Бюджет на сессию, лимит вызовов | Низкий-средний |
| Недоступность LLM | Высокая | Среднее | Доля таймаутов, алерты | Ретраи, fallback-ответ пользователю | Средний |
| Утечка секретов (ключи) | Низкая | Высокое | Secret-скан, аудит CI/CD | Vault, запрет секретов в git | Низкий |

## Персональные данные (152-ФЗ)

Платформа не собирает персональные данные: регистрация через автогенерированный логин (`{Прилагательное}{Существительное}{Число}`), без email. Восстановление — через recovery key.

PII-фильтрация реализована в interviewer-agent-service (3 уровня):
- **Level 1 (regex, hard block)**: email, телефон, банковская карта (4x4), ИНН, СНИЛС, паспорт, самоидентификация ("меня зовут..."), компании из blacklist (company_blacklist.json). Блокировка сообщения, маскирование в chat-crud-service
- **Level 2 (LLM, soft block)**: классификация неявных данных (непрямые упоминания, даты). Temp=0, structured output PIICheckResult (Pydantic)
- **Level 3 (sanitize)**: truncation, strip prompt injection markers, residual PII masking перед LLM

Правила: в прод-логах нет текста сообщений (structlog JSON, content подавлен по флагу окружения). PII не сохраняется в БД, не логируется, не передаётся в LLM.

## Защита от injection

LLM не управляет системой — выдаёт оценки/текст в заданном формате (Pydantic-модели), а правила и лимиты применяются кодом. Structured output: все ответы LLM парсятся через `model_validate_json` в типизированные модели (AnswerEvaluation, AnalystReport, OffTopicClassification и др.).

Pre-call: PII фильтрация (3 уровня), strip prompt injection markers (`<|...|>`, `[INST]`, `<<SYS>>`), truncation (max 4000 chars).
Post-call: Pydantic-валидация, range checks (score 0-1, confidence 0-1), leakage detection (системный промпт в ответе), tone sanitization, fallback при невалидном JSON.

## Мониторинг и evals

Prometheus-метрики: `agent_llm_call_duration_seconds` (Histogram), `agent_decision_confidence` (Histogram), `agent_low_confidence_decisions_total` (Counter, confidence < 0.5), `guardrail_triggered_total` (Counter с labels guardrail_name, stage), `agent_error_count` (Counter), `analyst_report_score` (Histogram), `analyst_validation_attempts` (Histogram).

Продуктовые: completion rate, hint usage rate, training unlock rate, session duration.

Тест-набор eval-кейсов с периодическим прогоном. Контроль версий промптов и моделей.
