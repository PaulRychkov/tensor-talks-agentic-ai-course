# Архитектура Agent Service на LangGraph

## Обзор

Agent Service - это микросервис на Python FastAPI, реализованный с использованием LangGraph для управления состоянием и логикой интервью. Агент получает историю сообщений, оценивает ответы пользователя на основе референса, генерирует уточняющие вопросы при необходимости и переходит к следующим вопросам программы интервью.

---

## 1. Общая архитектура

### 1.1 Компоненты системы

```
Agent Service
├── LangGraph State Machine
│   ├── Nodes (состояния)
│   ├── Edges (переходы)
│   └── State Schema
├── Kafka Integration
│   ├── Consumer (messages.full.data)
│   └── Producer (generated.phrases)
├── REST API (для тестирования)
│   └── POST /api/agent/process
├── External Clients
│   ├── Session Service Client
│   ├── Chat CRUD Client
│   ├── Knowledge Base CRUD Client
│   └── Redis Client (опционально)
├── LLM Integration
│   ├── LLM Client (настраиваемый base_url)
│   └── Prompt Templates
└── Configuration
    └── Pydantic Settings
```

### 1.2 Поток данных

```
Kafka (messages.full.data)
    ↓
Agent Service (Kafka Consumer)
    ↓
LangGraph State Machine
    ├── Получение истории (Redis/Kafka)
    ├── Получение программы (Session Service)
    ├── Получение теории (Knowledge Base)
    ├── Определение номера вопроса (LLM)
    ├── Оценка ответа (LLM)
    ├── Генерация вопроса (LLM)
    └── Сохранение результата
    ↓
Kafka (generated.phrases)
    ↓
Dialogue Aggregator → BFF → Frontend
```

---

## 2. LangGraph State Machine

### 2.1 State Schema

```python
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
    
    # История диалога
    dialogue_history: List[dict]  # [{role, content, timestamp}, ...]
    dialogue_state: Optional[dict]  # DialogueState из Redis
    
    # Программа интервью
    interview_program: Optional[dict]  # InterviewProgram
    total_questions: int
    current_question_index: Optional[int]  # Определяется агентом
    current_question: Optional[dict]  # QuestionItem
    current_theory: Optional[str]
    
    # Оценка ответа
    answer_evaluation: Optional[dict]:
        completeness_score: float  # 0.0 - 1.0
        accuracy_score: float  # 0.0 - 1.0
        overall_score: float  # 0.0 - 1.0
        is_complete: bool
        missing_points: List[str]
        evaluation_reasoning: str
    
    # Решение агента
    agent_decision: Optional[Literal[
        "ask_clarification",      # Задать уточняющий вопрос
        "next_question",          # Перейти к следующему вопросу
        "off_topic_reminder",     # Напомнить об интервью
        "thank_you",              # Благодарность за интервью
        "error"                   # Ошибка обработки
    ]]
    
    # Сгенерированный ответ
    generated_response: Optional[str]
    generated_question: Optional[str]
    response_metadata: Optional[dict]
    
    # Метаданные обработки
    processing_steps: List[str]  # Для отладки
    error: Optional[str]
    retry_count: int
```

### 2.2 Nodes (Состояния/Шаги обработки)

#### **Node 1: `receive_message`**
**Вход:** Событие из Kafka `messages.full.data`  
**Выход:** AgentState с заполненными входными данными

**Логика:**
- Парсинг события Kafka
- Извлечение `chat_id`, `session_id`, `user_id`, `message_id`, `content`
- Проверка типа сообщения (только `role == "user"`)
- Инициализация состояния

**Код:**
```python
def receive_message(state: AgentState) -> AgentState:
    """Получение и парсинг сообщения из Kafka"""
    # state уже содержит данные из события
    state["processing_steps"].append("message_received")
    return state
```

#### **Node 2: `load_context`**
**Вход:** AgentState с `chat_id`, `session_id`  
**Выход:** AgentState с загруженной историей и программой

**Логика:**
- Загрузка истории диалога из Redis (`dialogue:{chat_id}:messages`)
- Загрузка состояния диалога из Redis (`dialogue:{chat_id}:state`)
- Загрузка программы интервью из Session Service (`GET /sessions/:id/program`)
- Заполнение `dialogue_history`, `dialogue_state`, `interview_program`

**Код:**
```python
async def load_context(state: AgentState, clients: ServiceClients) -> AgentState:
    """Загрузка контекста диалога"""
    chat_id = state["chat_id"]
    session_id = state["session_id"]
    
    # Загрузка истории из Redis
    history = await clients.redis.get_messages(chat_id, limit=50)
    state["dialogue_history"] = [msg.model_dump() for msg in history]
    
    # Загрузка состояния
    dialogue_state = await clients.redis.get_dialogue_state(chat_id)
    if dialogue_state:
        state["dialogue_state"] = dialogue_state.model_dump()
    
    # Загрузка программы интервью
    program = await clients.session_service.get_program(session_id)
    if program:
        state["interview_program"] = program
        state["total_questions"] = len(program.get("questions", []))
    
    state["processing_steps"].append("context_loaded")
    return state
```

#### **Node 3: `check_off_topic`**
**Вход:** AgentState с `user_message`  
**Выход:** AgentState с решением `agent_decision`

**Логика:**
- Проверка, является ли сообщение пользователя вопросом вне интервью
- Использование LLM для классификации
- Если вне темы → `agent_decision = "off_topic_reminder"`

**Промпт:**
```
Ты - интервьюер на техническом ML-интервью. Пользователь отправил сообщение.

Сообщение пользователя: {user_message}

История диалога:
{recent_messages}

Определи, является ли сообщение:
1. Ответом на вопрос интервьюера
2. Вопросом пользователя о чем-то, не связанном с интервью
3. Комментарием или репликой, не относящейся к интервью

Ответь только одним словом: "answer", "off_topic", или "comment"
```

**Код:**
```python
async def check_off_topic(
    state: AgentState, 
    llm_client: LLMClient,
    config: AgentConfig
) -> AgentState:
    """Проверка, является ли сообщение вне темы интервью"""
    user_message = state["user_message"]
    recent_messages = state["dialogue_history"][-5:]  # Последние 5 сообщений
    
    prompt = build_off_topic_prompt(user_message, recent_messages)
    response = await llm_client.generate(prompt, temperature=0.1)
    
    decision = response.strip().lower()
    if decision in ["off_topic", "comment"]:
        state["agent_decision"] = "off_topic_reminder"
        state["processing_steps"].append("off_topic_detected")
    else:
        state["processing_steps"].append("on_topic")
    
    return state
```

#### **Node 4: `determine_question_index`**
**Вход:** AgentState с историей и программой  
**Выход:** AgentState с `current_question_index`

**Логика:**
- Анализ истории диалога для определения текущего вопроса
- Использование LLM для определения номера вопроса
- Учет уже заданных вопросов и уточнений

**Промпт:**
```
Ты - интервьюер на техническом ML-интервью. Проанализируй историю диалога и определи, на каком вопросе из программы интервью сейчас находится диалог.

Программа интервью:
{interview_program}

История диалога:
{dialogue_history}

Определи:
1. Номер текущего вопроса из программы (order) - если вопрос еще не задан, верни None
2. Был ли задан уточняющий вопрос после последнего ответа пользователя

Ответь в формате JSON:
{{
    "current_question_order": <int или null>,
    "clarification_asked": <true/false>,
    "reasoning": "<объяснение>"
}}
```

**Код:**
```python
async def determine_question_index(
    state: AgentState,
    llm_client: LLMClient,
    config: AgentConfig
) -> AgentState:
    """Определение номера текущего вопроса"""
    program = state["interview_program"]
    history = state["dialogue_history"]
    
    if not program or "questions" not in program:
        state["error"] = "Interview program not loaded"
        state["agent_decision"] = "error"
        return state
    
    prompt = build_question_index_prompt(program, history)
    response = await llm_client.generate(prompt, temperature=0.1, response_format="json")
    
    try:
        result = json.loads(response)
        state["current_question_index"] = result.get("current_question_order")
        
        # Установка текущего вопроса
        if state["current_question_index"] is not None:
            questions = program["questions"]
            for q in questions:
                if q["order"] == state["current_question_index"]:
                    state["current_question"] = q
                    state["current_theory"] = q.get("theory")
                    break
        
        state["processing_steps"].append(f"question_index_determined: {state['current_question_index']}")
    except Exception as e:
        state["error"] = f"Failed to parse question index: {str(e)}"
        state["agent_decision"] = "error"
    
    return state
```

#### **Node 5: `evaluate_answer`**
**Вход:** AgentState с `user_message`, `current_question`, `current_theory`  
**Выход:** AgentState с `answer_evaluation`

**Логика:**
- Оценка полноты и точности ответа пользователя
- Сравнение с референсом (теорией)
- Генерация оценки с объяснением

**Промпт:**
```
Ты - эксперт по машинному обучению, оценивающий ответ кандидата на техническом интервью.

Вопрос интервьюера: {current_question}

Теория (референс для оценки): {current_theory}

Ответ кандидата: {user_message}

Оцени ответ кандидата по следующим критериям:
1. Полнота (completeness) - насколько полно раскрыт вопрос (0.0 - 1.0)
2. Точность (accuracy) - насколько правильно ответ (0.0 - 1.0)
3. Общая оценка (overall) - среднее между полнотой и точностью

Также определи:
- Является ли ответ полным (is_complete: true/false)
- Какие аспекты не раскрыты (missing_points: список строк)
- Краткое объяснение оценки (evaluation_reasoning: строка)

Ответь в формате JSON:
{{
    "completeness_score": <float 0.0-1.0>,
    "accuracy_score": <float 0.0-1.0>,
    "overall_score": <float 0.0-1.0>,
    "is_complete": <true/false>,
    "missing_points": ["<пункт 1>", "<пункт 2>", ...],
    "evaluation_reasoning": "<объяснение>"
}}
```

**Код:**
```python
async def evaluate_answer(
    state: AgentState,
    llm_client: LLMClient,
    config: AgentConfig
) -> AgentState:
    """Оценка ответа пользователя"""
    user_message = state["user_message"]
    current_question = state.get("current_question")
    current_theory = state.get("current_theory")
    
    if not current_question:
        state["error"] = "Current question not determined"
        state["agent_decision"] = "error"
        return state
    
    prompt = build_evaluation_prompt(
        current_question["question"],
        current_theory or "",
        user_message
    )
    
    response = await llm_client.generate(prompt, temperature=0.1, response_format="json")
    
    try:
        evaluation = json.loads(response)
        state["answer_evaluation"] = evaluation
        state["processing_steps"].append("answer_evaluated")
    except Exception as e:
        state["error"] = f"Failed to parse evaluation: {str(e)}"
        state["agent_decision"] = "error"
    
    return state
```

#### **Node 6: `make_decision`**
**Вход:** AgentState с `answer_evaluation`, `current_question_index`, `total_questions`  
**Выход:** AgentState с `agent_decision`

**Логика:**
- Принятие решения на основе оценки
- Проверка, был ли уже задан уточняющий вопрос
- Проверка, является ли это последним вопросом

**Код:**
```python
def make_decision(state: AgentState) -> AgentState:
    """Принятие решения о следующем действии"""
    evaluation = state.get("answer_evaluation")
    current_index = state.get("current_question_index")
    total = state.get("total_questions", 0)
    history = state.get("dialogue_history", [])
    
    if not evaluation:
        state["agent_decision"] = "error"
        return state
    
    # Проверка, был ли задан уточняющий вопрос после последнего ответа
    clarification_asked = check_clarification_in_history(history, current_index)
    
    # Если ответ неполный и уточнение еще не задано
    if not evaluation.get("is_complete", False) and not clarification_asked:
        state["agent_decision"] = "ask_clarification"
    # Если ответ полный или уточнение уже было
    elif evaluation.get("is_complete", False) or clarification_asked:
        # Проверка, последний ли это вопрос
        if current_index is not None and current_index >= total:
            state["agent_decision"] = "thank_you"
        else:
            state["agent_decision"] = "next_question"
    else:
        state["agent_decision"] = "error"
    
    state["processing_steps"].append(f"decision_made: {state['agent_decision']}")
    return state

def check_clarification_in_history(history: List[dict], question_index: int) -> bool:
    """Проверка, был ли задан уточняющий вопрос"""
    # Анализ последних сообщений ассистента
    for msg in reversed(history[-10:]):  # Последние 10 сообщений
        if msg.get("role") == "assistant":
            # Простая эвристика: если в сообщении есть слова "уточни", "расскажи подробнее"
            content = msg.get("content", "").lower()
            if any(word in content for word in ["уточни", "расскажи подробнее", "можешь пояснить"]):
                return True
    return False
```

#### **Node 7: `generate_response`**
**Вход:** AgentState с `agent_decision`, `answer_evaluation`, `current_question`  
**Выход:** AgentState с `generated_response`

**Логика:**
- Генерация ответа в зависимости от решения
- Разные промпты для разных типов ответов

**Промпты:**

**Для уточняющего вопроса:**
```
Ты - интервьюер на техническом ML-интервью. Кандидат дал неполный ответ на вопрос.

Вопрос: {current_question}

Ответ кандидата: {user_message}

Недостающие аспекты: {missing_points}

Сгенерируй уточняющий вопрос, который поможет кандидату дополнить ответ. Вопрос должен быть вежливым и конструктивным.

Формат ответа: только текст вопроса, без дополнительных комментариев.
```

**Для следующего вопроса:**
```
Ты - интервьюер на техническом ML-интервью. Кандидат дал полный ответ на вопрос.

Вопрос: {current_question}

Ответ кандидата: {user_message}

Оценка: {evaluation_reasoning}

Следующий вопрос из программы: {next_question}

Сгенерируй переход к следующему вопросу. Сначала кратко подтверди, что ответ хороший (1-2 предложения), затем задай следующий вопрос.

Формат ответа: текст перехода и вопроса.
```

**Для напоминания об интервью:**
```
Ты - интервьюер на техническом ML-интервью. Кандидат задал вопрос или написал сообщение, не относящееся к интервью.

Сообщение кандидата: {user_message}

Вежливо напомни кандидату, что сейчас идет техническое интервью, и попроси его отвечать на вопросы интервьюера. Затем повтори последний заданный вопрос.

Формат ответа: вежливое напоминание и повтор вопроса.
```

**Для благодарности:**
```
Ты - интервьюер на техническом ML-интервью. Интервью завершено, все вопросы программы заданы.

Поблагодари кандидата за участие в интервью. Сообщение должно быть дружелюбным и профессиональным.

Формат ответа: только текст благодарности.
```

**Код:**
```python
async def generate_response(
    state: AgentState,
    llm_client: LLMClient,
    config: AgentConfig
) -> AgentState:
    """Генерация ответа агента"""
    decision = state.get("agent_decision")
    
    if decision == "ask_clarification":
        prompt = build_clarification_prompt(state)
    elif decision == "next_question":
        prompt = build_next_question_prompt(state)
    elif decision == "off_topic_reminder":
        prompt = build_off_topic_reminder_prompt(state)
    elif decision == "thank_you":
        prompt = build_thank_you_prompt()
    else:
        state["error"] = f"Unknown decision: {decision}"
        return state
    
    response = await llm_client.generate(prompt, temperature=0.7)
    state["generated_response"] = response.strip()
    state["processing_steps"].append("response_generated")
    
    return state
```

#### **Node 8: `publish_response`**
**Вход:** AgentState с `generated_response`  
**Выход:** AgentState (финальное состояние)

**Логика:**
- Публикация сгенерированного ответа в Kafka (`generated.phrases`)
- Формирование события `phrase.agent.generated`

**Код:**
```python
async def publish_response(
    state: AgentState,
    kafka_producer: KafkaProducer,
    config: AgentConfig
) -> AgentState:
    """Публикация ответа в Kafka"""
    from uuid import uuid4
    from datetime import datetime
    
    event = {
        "event_id": str(uuid4()),
        "event_type": "phrase.agent.generated",
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "service": config.service_name,
        "version": config.service_version,
        "payload": {
            "chat_id": state["chat_id"],
            "message_id": str(uuid4()),
            "generated_text": state["generated_response"],
            "confidence": state.get("answer_evaluation", {}).get("overall_score", 0.0),
            "intermediate_steps": state.get("processing_steps", []),
            "metadata": {
                "current_question_index": state.get("current_question_index"),
                "agent_decision": state.get("agent_decision"),
                "evaluation": state.get("answer_evaluation"),
            },
            "timestamp": datetime.utcnow().isoformat() + "Z",
        },
        "metadata": {
            "correlation_id": state.get("message_id"),
        }
    }
    
    await kafka_producer.publish(config.kafka_topic_generated, event)
    state["processing_steps"].append("response_published")
    
    return state
```

### 2.3 Edges (Переходы между состояниями)

```python
from langgraph.graph import StateGraph, END

def create_agent_graph(
    llm_client: LLMClient,
    clients: ServiceClients,
    kafka_producer: KafkaProducer,
    config: AgentConfig
) -> StateGraph:
    """Создание графа состояний агента"""
    
    workflow = StateGraph(AgentState)
    
    # Добавление узлов
    workflow.add_node("receive_message", receive_message)
    workflow.add_node("load_context", lambda s: load_context(s, clients))
    workflow.add_node("check_off_topic", lambda s: check_off_topic(s, llm_client, config))
    workflow.add_node("determine_question_index", lambda s: determine_question_index(s, llm_client, config))
    workflow.add_node("evaluate_answer", lambda s: evaluate_answer(s, llm_client, config))
    workflow.add_node("make_decision", make_decision)
    workflow.add_node("generate_response", lambda s: generate_response(s, llm_client, config))
    workflow.add_node("publish_response", lambda s: publish_response(s, kafka_producer, config))
    
    # Определение потока
    workflow.set_entry_point("receive_message")
    
    workflow.add_edge("receive_message", "load_context")
    workflow.add_edge("load_context", "check_off_topic")
    
    # Условный переход после проверки off-topic
    workflow.add_conditional_edges(
        "check_off_topic",
        lambda s: "off_topic_reminder" if s.get("agent_decision") == "off_topic_reminder" else "determine_question_index",
        {
            "off_topic_reminder": "generate_response",
            "determine_question_index": "determine_question_index"
        }
    )
    
    workflow.add_edge("determine_question_index", "evaluate_answer")
    workflow.add_edge("evaluate_answer", "make_decision")
    workflow.add_edge("make_decision", "generate_response")
    workflow.add_edge("generate_response", "publish_response")
    workflow.add_edge("publish_response", END)
    
    return workflow.compile()
```

---

## 3. Конфигурация (Pydantic Settings)

### 3.1 AgentConfig

```python
from pydantic_settings import BaseSettings
from pydantic import Field
from typing import Optional

class AgentConfig(BaseSettings):
    """Конфигурация Agent Service"""
    
    # Service
    service_name: str = Field(default="agent-service", alias="AGENT_SERVICE_NAME")
    service_version: str = Field(default="1.0.0", alias="AGENT_SERVICE_VERSION")
    log_level: str = Field(default="INFO", alias="LOG_LEVEL")
    log_format: str = Field(default="json", alias="LOG_FORMAT")
    
    # Kafka
    kafka_bootstrap_servers: str = Field(
        default="localhost:9092", 
        alias="KAFKA_BOOTSTRAP_SERVERS"
    )
    kafka_topic_messages_full: str = Field(
        default="messages.full.data",
        alias="KAFKA_TOPIC_MESSAGES_FULL"
    )
    kafka_topic_generated: str = Field(
        default="generated.phrases",
        alias="KAFKA_TOPIC_GENERATED"
    )
    kafka_consumer_group: str = Field(
        default="agent-service-group",
        alias="KAFKA_CONSUMER_GROUP"
    )
    kafka_consumer_auto_offset_reset: str = Field(
        default="earliest",
        alias="KAFKA_CONSUMER_AUTO_OFFSET_RESET"
    )
    
    # LLM
    llm_provider: str = Field(
        default="openai",
        alias="LLM_PROVIDER"  # openai, anthropic, local
    )
    llm_base_url: Optional[str] = Field(
        default=None,
        alias="LLM_BASE_URL"  # Для локальных моделей или прокси
    )
    llm_api_key: Optional[str] = Field(
        default=None,
        alias="LLM_API_KEY"
    )
    llm_model: str = Field(
        default="gpt-4",
        alias="LLM_MODEL"
    )
    llm_temperature: float = Field(
        default=0.7,
        alias="LLM_TEMPERATURE"
    )
    llm_max_tokens: int = Field(
        default=2000,
        alias="LLM_MAX_TOKENS"
    )
    llm_timeout: int = Field(
        default=60,
        alias="LLM_TIMEOUT"
    )
    
    # External Services
    session_service_url: str = Field(
        default="http://session-service:8083",
        alias="SESSION_SERVICE_URL"
    )
    chat_crud_service_url: str = Field(
        default="http://chat-crud-service:8087",
        alias="CHAT_CRUD_SERVICE_URL"
    )
    knowledge_base_crud_service_url: str = Field(
        default="http://knowledge-base-crud-service:8090",
        alias="KNOWLEDGE_BASE_CRUD_SERVICE_URL"
    )
    redis_host: str = Field(default="localhost", alias="REDIS_HOST")
    redis_port: int = Field(default=6379, alias="REDIS_PORT")
    redis_db: int = Field(default=0, alias="REDIS_DB")
    redis_password: Optional[str] = Field(default=None, alias="REDIS_PASSWORD")
    
    # Processing
    max_retries: int = Field(default=3, alias="MAX_RETRIES")
    processing_timeout: int = Field(default=120, alias="PROCESSING_TIMEOUT")
    enable_redis_cache: bool = Field(default=True, alias="ENABLE_REDIS_CACHE")
    
    # REST API
    rest_api_host: str = Field(default="0.0.0.0", alias="REST_API_HOST")
    rest_api_port: int = Field(default=8093, alias="REST_API_PORT")
    enable_rest_api: bool = Field(default=True, alias="ENABLE_REST_API")
    
    # Metrics
    metrics_port: int = Field(default=9092, alias="METRICS_PORT")
    enable_prometheus: bool = Field(default=True, alias="ENABLE_PROMETHEUS")
    
    class Config:
        env_file = ".env"
        case_sensitive = False
```

---

## 4. REST API для тестирования

### 4.1 Endpoint: `POST /api/agent/process`

**Описание:** Принимает запрос в том же формате, что и события Kafka, для локального тестирования

**Request Body:**
```json
{
  "event_id": "uuid",
  "event_type": "message.full",
  "timestamp": "2025-01-17T10:30:00.123Z",
  "service": "test-service",
  "version": "1.0.0",
  "payload": {
    "chat_id": "uuid",
    "message_id": "uuid",
    "role": "user",
    "content": "L1 регуляризация добавляет сумму абсолютных значений весов",
    "metadata": {
      "user_id": "uuid",
      "message_index": 5,
      "dialogue_context": {
        "total_messages": 10,
        "dialogue_type": "ml_interview",
        "status": "active",
        "topic": "ml",
        "difficulty": "middle",
        "current_question_index": 3
      }
    },
    "source": "user_input",
    "timestamp": "2025-01-17T10:30:00.123Z",
    "processed_at": "2025-01-17T10:30:00.500Z"
  }
}
```

**Response:**
```json
{
  "success": true,
  "event_id": "uuid",
  "generated_response": "Отличный ответ! Вы правильно описали L1 регуляризацию...",
  "agent_state": {
    "current_question_index": 3,
    "agent_decision": "next_question",
    "answer_evaluation": {
      "completeness_score": 0.85,
      "accuracy_score": 0.90,
      "overall_score": 0.875,
      "is_complete": true
    },
    "processing_steps": [
      "message_received",
      "context_loaded",
      "on_topic",
      "question_index_determined: 3",
      "answer_evaluated",
      "decision_made: next_question",
      "response_generated",
      "response_published"
    ]
  }
}
```

**Код:**
```python
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import Optional

app = FastAPI(title="Agent Service API")

class ProcessRequest(BaseModel):
    """Запрос на обработку сообщения (формат Kafka события)"""
    event_id: str
    event_type: str
    timestamp: str
    service: str
    version: str
    payload: dict
    metadata: Optional[dict] = None

class ProcessResponse(BaseModel):
    """Ответ на запрос обработки"""
    success: bool
    event_id: str
    generated_response: Optional[str] = None
    agent_state: Optional[dict] = None
    error: Optional[str] = None

@app.post("/api/agent/process", response_model=ProcessResponse)
async def process_message(request: ProcessRequest):
    """Обработка сообщения (для тестирования)"""
    try:
        # Преобразование запроса в AgentState
        payload = request.payload
        initial_state: AgentState = {
            "chat_id": payload["chat_id"],
            "session_id": payload["metadata"]["dialogue_context"].get("session_id", ""),
            "user_id": payload["metadata"]["user_id"],
            "message_id": payload["message_id"],
            "user_message": payload["content"],
            "message_timestamp": datetime.fromisoformat(payload["timestamp"].replace("Z", "+00:00")),
            "dialogue_history": [],
            "dialogue_state": None,
            "interview_program": None,
            "total_questions": 0,
            "current_question_index": None,
            "current_question": None,
            "current_theory": None,
            "answer_evaluation": None,
            "agent_decision": None,
            "generated_response": None,
            "generated_question": None,
            "response_metadata": None,
            "processing_steps": [],
            "error": None,
            "retry_count": 0,
        }
        
        # Запуск графа
        graph = get_agent_graph()  # Получение скомпилированного графа
        final_state = await graph.ainvoke(initial_state)
        
        return ProcessResponse(
            success=True,
            event_id=request.event_id,
            generated_response=final_state.get("generated_response"),
            agent_state={
                "current_question_index": final_state.get("current_question_index"),
                "agent_decision": final_state.get("agent_decision"),
                "answer_evaluation": final_state.get("answer_evaluation"),
                "processing_steps": final_state.get("processing_steps"),
            }
        )
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
```

---

## 5. Интеграция с внешними сервисами

### 5.1 Session Service Client

```python
import httpx
from typing import Optional, Dict

class SessionServiceClient:
    """Клиент для работы с Session Service"""
    
    def __init__(self, base_url: str, timeout: int = 10):
        self.base_url = base_url
        self.timeout = timeout
        self.client = httpx.AsyncClient(timeout=timeout)
    
    async def get_program(self, session_id: str) -> Optional[Dict]:
        """Получение программы интервью"""
        try:
            response = await self.client.get(
                f"{self.base_url}/sessions/{session_id}/program"
            )
            response.raise_for_status()
            data = response.json()
            return data.get("program")
        except Exception as e:
            logger.error(f"Failed to get program: {e}")
            return None
    
    async def close_session(self, session_id: str) -> bool:
        """Закрытие сессии"""
        try:
            response = await self.client.put(
                f"{self.base_url}/sessions/{session_id}/close"
            )
            response.raise_for_status()
            return True
        except Exception as e:
            logger.error(f"Failed to close session: {e}")
            return False
```

### 5.2 Redis Client

```python
import redis.asyncio as redis
from typing import List, Optional
from ..models.messages import Message
from ..models.dialogue import DialogueState

class RedisClient:
    """Клиент для работы с Redis"""
    
    def __init__(self, host: str, port: int, db: int, password: Optional[str] = None):
        self.redis = redis.Redis(
            host=host,
            port=port,
            db=db,
            password=password,
            decode_responses=True
        )
    
    async def get_messages(self, chat_id: str, limit: int = 50) -> List[Message]:
        """Получение истории сообщений"""
        key = f"dialogue:{chat_id}:messages"
        messages_data = await self.redis.lrange(key, -limit, -1)
        
        messages = []
        for msg_data in messages_data:
            try:
                msg_dict = json.loads(msg_data)
                messages.append(Message(**msg_dict))
            except Exception as e:
                logger.warning(f"Failed to parse message: {e}")
        
        return messages
    
    async def get_dialogue_state(self, chat_id: str) -> Optional[DialogueState]:
        """Получение состояния диалога"""
        key = f"dialogue:{chat_id}:state"
        data = await self.redis.get(key)
        
        if data:
            try:
                state_dict = json.loads(data)
                return DialogueState(**state_dict)
            except Exception as e:
                logger.warning(f"Failed to parse dialogue state: {e}")
        
        return None
```

### 5.3 LLM Client

```python
from openai import AsyncOpenAI
from typing import Optional, Literal

class LLMClient:
    """Клиент для работы с LLM"""
    
    def __init__(self, config: AgentConfig):
        self.config = config
        self.client = None
        self._initialize_client()
    
    def _initialize_client(self):
        """Инициализация клиента в зависимости от провайдера"""
        if self.config.llm_provider == "openai":
            self.client = AsyncOpenAI(
                api_key=self.config.llm_api_key,
                base_url=self.config.llm_base_url,
                timeout=self.config.llm_timeout
            )
        elif self.config.llm_provider == "anthropic":
            # Инициализация Anthropic клиента
            pass
        elif self.config.llm_provider == "local":
            # Инициализация локального клиента (например, через Ollama)
            self.client = AsyncOpenAI(
                base_url=self.config.llm_base_url or "http://localhost:11434/v1",
                api_key="not-needed",
                timeout=self.config.llm_timeout
            )
    
    async def generate(
        self,
        prompt: str,
        temperature: Optional[float] = None,
        response_format: Optional[Literal["json", "text"]] = "text"
    ) -> str:
        """Генерация ответа"""
        temperature = temperature or self.config.llm_temperature
        
        response_format_param = None
        if response_format == "json":
            response_format_param = {"type": "json_object"}
        
        response = await self.client.chat.completions.create(
            model=self.config.llm_model,
            messages=[{"role": "user", "content": prompt}],
            temperature=temperature,
            max_tokens=self.config.llm_max_tokens,
            response_format=response_format_param
        )
        
        return response.choices[0].message.content
```

---

## 6. Kafka Integration

### 6.1 Consumer

```python
from confluent_kafka import Consumer, KafkaError
import json
from typing import Callable

class KafkaConsumer:
    """Kafka consumer для messages.full.data"""
    
    def __init__(self, config: AgentConfig, process_callback: Callable):
        self.config = config
        self.process_callback = process_callback
        
        consumer_config = {
            "bootstrap.servers": config.kafka_bootstrap_servers,
            "group.id": config.kafka_consumer_group,
            "auto.offset.reset": config.kafka_consumer_auto_offset_reset,
            "enable.auto.commit": False,
        }
        
        self.consumer = Consumer(consumer_config)
        self.consumer.subscribe([config.kafka_topic_messages_full])
    
    async def consume(self):
        """Основной цикл потребления сообщений"""
        while True:
            try:
                msg = self.consumer.poll(timeout=1.0)
                
                if msg is None:
                    continue
                
                if msg.error():
                    if msg.error().code() == KafkaError._PARTITION_EOF:
                        continue
                    logger.error(f"Consumer error: {msg.error()}")
                    continue
                
                # Парсинг события
                event_dict = json.loads(msg.value().decode("utf-8"))
                
                # Обработка через callback
                await self.process_callback(event_dict)
                
                # Commit offset
                self.consumer.commit()
                
            except Exception as e:
                logger.error(f"Error processing message: {e}", exc_info=True)
```

### 6.2 Producer

```python
from confluent_kafka import Producer
import json

class KafkaProducer:
    """Kafka producer для generated.phrases"""
    
    def __init__(self, config: AgentConfig):
        self.config = config
        
        producer_config = {
            "bootstrap.servers": config.kafka_bootstrap_servers,
            "acks": "all",
            "retries": 3,
        }
        
        self.producer = Producer(producer_config)
    
    async def publish(self, topic: str, event: dict):
        """Публикация события"""
        event_json = json.dumps(event).encode("utf-8")
        key = event["payload"]["chat_id"].encode("utf-8")
        
        self.producer.produce(
            topic,
            value=event_json,
            key=key,
            callback=self._delivery_callback
        )
        
        self.producer.poll(0)
        self.producer.flush()
    
    def _delivery_callback(self, err, msg):
        """Callback для доставки сообщения"""
        if err:
            logger.error(f"Message delivery failed: {err}")
        else:
            logger.debug(f"Message delivered to {msg.topic()}")
```

---

## 7. Структура проекта

```
backend/agent-service/
├── src/
│   ├── __init__.py
│   ├── main.py                 # Точка входа, FastAPI app
│   ├── config.py               # Pydantic Settings
│   ├── graph/
│   │   ├── __init__.py
│   │   ├── state.py            # AgentState TypedDict
│   │   ├── nodes.py            # Все узлы графа
│   │   ├── edges.py            # Переходы между узлами
│   │   └── builder.py          # Создание графа
│   ├── llm/
│   │   ├── __init__.py
│   │   ├── client.py           # LLMClient
│   │   └── prompts.py          # Шаблоны промптов
│   ├── clients/
│   │   ├── __init__.py
│   │   ├── session_client.py   # Session Service Client
│   │   ├── redis_client.py    # Redis Client
│   │   └── kafka_client.py    # Kafka Producer/Consumer
│   ├── api/
│   │   ├── __init__.py
│   │   └── endpoints.py        # REST API endpoints
│   ├── models/
│   │   ├── __init__.py
│   │   └── events.py           # Модели событий Kafka
│   ├── logger/
│   │   ├── __init__.py
│   │   └── setup.py
│   └── metrics/
│       ├── __init__.py
│       └── collector.py
├── tests/
│   ├── unit/
│   │   ├── test_nodes.py
│   │   └── test_graph.py
│   └── integration/
│       └── test_api.py
├── Dockerfile
├── requirements.txt
└── README.md
```

---

## 8. Обработка ошибок и retry

### 8.1 Retry механизм

```python
from tenacity import retry, stop_after_attempt, wait_exponential

@retry(
    stop=stop_after_attempt(3),
    wait=wait_exponential(multiplier=1, min=4, max=10)
)
async def process_with_retry(state: AgentState, graph: StateGraph) -> AgentState:
    """Обработка с повторными попытками"""
    try:
        return await graph.ainvoke(state)
    except Exception as e:
        state["retry_count"] += 1
        if state["retry_count"] >= 3:
            state["error"] = str(e)
            state["agent_decision"] = "error"
        raise
```

### 8.2 Обработка ошибок в узлах

```python
def safe_node(node_func):
    """Декоратор для безопасной обработки ошибок в узлах"""
    async def wrapper(state: AgentState) -> AgentState:
        try:
            return await node_func(state)
        except Exception as e:
            logger.error(f"Error in node {node_func.__name__}: {e}", exc_info=True)
            state["error"] = str(e)
            state["agent_decision"] = "error"
            return state
    return wrapper
```

---

## 9. Метрики и мониторинг

### 9.1 Метрики Prometheus

```python
from prometheus_client import Counter, Histogram, Gauge

# Метрики обработки
messages_processed_total = Counter(
    "agent_messages_processed_total",
    "Total messages processed",
    ["status", "decision"]
)

processing_duration = Histogram(
    "agent_processing_duration_seconds",
    "Time spent processing message",
    buckets=[0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0]
)

llm_calls_total = Counter(
    "agent_llm_calls_total",
    "Total LLM API calls",
    ["provider", "model", "status"]
)

llm_call_duration = Histogram(
    "agent_llm_call_duration_seconds",
    "Time spent on LLM call",
    buckets=[0.5, 1.0, 2.0, 5.0, 10.0, 30.0]
)

# Метрики состояния
active_dialogues = Gauge(
    "agent_active_dialogues",
    "Number of active dialogues being processed"
)

current_question_index = Histogram(
    "agent_current_question_index",
    "Distribution of current question indices",
    buckets=list(range(0, 11))
)
```

---

## 10. Развертывание

### 10.1 Dockerfile

```dockerfile
FROM python:3.11-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY src/ ./src/

ENV PYTHONPATH=/app
ENV LOG_LEVEL=INFO

CMD ["python", "-m", "src.main"]
```

### 10.2 requirements.txt

```
fastapi==0.104.1
uvicorn==0.24.0
pydantic==2.5.0
pydantic-settings==2.1.0
langgraph==0.0.20
langchain==0.1.0
openai==1.3.0
confluent-kafka==2.3.0
redis==5.0.1
httpx==0.25.0
prometheus-client==0.19.0
structlog==23.2.0
tenacity==8.2.3
```

### 10.3 docker-compose.yml (добавление)

```yaml
agent-service:
  build:
    context: ./backend/agent-service
  environment:
    # Service
    AGENT_SERVICE_NAME: agent-service
    AGENT_SERVICE_VERSION: 1.0.0
    LOG_LEVEL: INFO
    
    # Kafka
    KAFKA_BOOTSTRAP_SERVERS: kafka:9092
    KAFKA_TOPIC_MESSAGES_FULL: messages.full.data
    KAFKA_TOPIC_GENERATED: generated.phrases
    KAFKA_CONSUMER_GROUP: agent-service-group
    
    # LLM
    LLM_PROVIDER: openai
    LLM_BASE_URL: ${LLM_BASE_URL:-}
    LLM_API_KEY: ${LLM_API_KEY:-}
    LLM_MODEL: gpt-4
    LLM_TEMPERATURE: 0.7
    
    # External Services
    SESSION_SERVICE_URL: http://session-service:8083
    CHAT_CRUD_SERVICE_URL: http://chat-crud-service:8087
    KNOWLEDGE_BASE_CRUD_SERVICE_URL: http://knowledge-base-crud-service:8090
    
    # Redis
    REDIS_HOST: redis
    REDIS_PORT: 6379
    REDIS_DB: 0
    
    # REST API
    REST_API_HOST: 0.0.0.0
    REST_API_PORT: 8093
    ENABLE_REST_API: "true"
    
    # Metrics
    METRICS_PORT: 9092
    ENABLE_PROMETHEUS: "true"
  depends_on:
    - kafka
    - redis
    - session-service
  ports:
    - "8093:8093"  # REST API
    - "9092:9092"  # Metrics
```

---

## 11. Тестирование

### 11.1 Unit тесты для узлов

```python
import pytest
from src.graph.nodes import evaluate_answer, make_decision

@pytest.mark.asyncio
async def test_evaluate_answer_complete():
    """Тест оценки полного ответа"""
    state = {
        "user_message": "L1 регуляризация добавляет сумму абсолютных значений весов...",
        "current_question": {"question": "Что такое L1 регуляризация?", "theory": "..."},
        "current_theory": "L1 регуляризация (Lasso)...",
        "answer_evaluation": None,
        "processing_steps": [],
    }
    
    # Моки LLM клиента
    llm_client = MockLLMClient()
    config = MockConfig()
    
    result = await evaluate_answer(state, llm_client, config)
    
    assert result["answer_evaluation"] is not None
    assert result["answer_evaluation"]["is_complete"] is True
    assert result["answer_evaluation"]["overall_score"] > 0.7
```

### 11.2 Integration тесты для API

```python
import pytest
from fastapi.testclient import TestClient
from src.main import app

client = TestClient(app)

def test_process_message_endpoint():
    """Тест REST API endpoint"""
    request = {
        "event_id": "test-123",
        "event_type": "message.full",
        "timestamp": "2025-01-17T10:30:00Z",
        "service": "test",
        "version": "1.0.0",
        "payload": {
            "chat_id": "chat-123",
            "message_id": "msg-123",
            "role": "user",
            "content": "Тестовый ответ",
            "metadata": {
                "user_id": "user-123",
                "message_index": 1,
                "dialogue_context": {
                    "total_messages": 2,
                    "dialogue_type": "ml_interview",
                    "status": "active"
                }
            },
            "source": "user_input",
            "timestamp": "2025-01-17T10:30:00Z",
            "processed_at": "2025-01-17T10:30:00Z"
        }
    }
    
    response = client.post("/api/agent/process", json=request)
    assert response.status_code == 200
    data = response.json()
    assert data["success"] is True
    assert "generated_response" in data
```

---

## 12. Заключение

Данная архитектура обеспечивает:

1. ✅ **Оценку ответов** на основе референса (теории)
2. ✅ **Генерацию уточняющих вопросов** при неполных ответах
3. ✅ **Переход к следующим вопросам** при полных ответах
4. ✅ **Обработку внеинтервьюных сообщений** с напоминанием
5. ✅ **Благодарность** после последнего вопроса
6. ✅ **Реализацию на LangGraph** с четким state management
7. ✅ **Настройки через Pydantic** с поддержкой различных LLM
8. ✅ **REST API для тестирования** с тем же форматом, что и Kafka
9. ✅ **Отслеживание номера вопроса** через LLM анализ
10. ✅ **Интеграцию с существующей инфраструктурой**

Архитектура готова к реализации.
