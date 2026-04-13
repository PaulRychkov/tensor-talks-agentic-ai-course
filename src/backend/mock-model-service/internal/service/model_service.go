package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tensor-talks/mock-model-service/internal/client"
	"github.com/tensor-talks/mock-model-service/internal/kafka"
	"github.com/tensor-talks/mock-model-service/internal/model"
	"github.com/tensor-talks/mock-model-service/internal/redisclient"
	"go.uber.org/zap"
)

// ModelService обрабатывает события чата и генерирует ответы модели.
// В будущем это будет marking-service.
type ModelService struct {
	producer         *kafka.Producer
	sessionMgr       *model.SessionManager
	sessionManagerCl *client.SessionManagerClient
	chatCRUDCl       *client.ChatCRUDClient
	resultsCRUDCl    *client.ResultsCRUDClient
	redisCl          *redisclient.Client
	agentBridge      *kafka.AgentBridge
	logger           *zap.Logger
	questionDelay    time.Duration
}

// NewModelService создаёт новый сервис модели.
func NewModelService(
	producer *kafka.Producer,
	sessionManagerCl *client.SessionManagerClient,
	chatCRUDCl *client.ChatCRUDClient,
	resultsCRUDCl *client.ResultsCRUDClient,
	redisCl *redisclient.Client,
	agentBridge *kafka.AgentBridge,
	questionDelaySeconds int,
	logger *zap.Logger,
) *ModelService {
	return &ModelService{
		producer:         producer,
		sessionMgr:       model.NewSessionManager(),
		sessionManagerCl: sessionManagerCl,
		chatCRUDCl:       chatCRUDCl,
		resultsCRUDCl:    resultsCRUDCl,
		redisCl:          redisCl,
		agentBridge:      agentBridge,
		logger:           logger,
		questionDelay:    time.Duration(questionDelaySeconds) * time.Second,
	}
}

// HandleChatStarted обрабатывает начало чата - получает программу интервью и отправляет первый вопрос.
func (s *ModelService) HandleChatStarted(ctx context.Context, sessionID, userID string) error {
	s.logger.Info("Chat started, getting interview program",
		zap.String("session_id", sessionID),
		zap.String("user_id", userID),
	)

	// Парсим sessionID
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session_id: %w", err)
	}

	// Получаем программу интервью от session-manager
	program, err := s.sessionManagerCl.GetInterviewProgram(ctx, sessionUUID)
	if err != nil {
		return fmt.Errorf("get interview program: %w", err)
	}

	s.logger.Info("Interview program received",
		zap.String("session_id", sessionID),
		zap.Int("questions_count", len(program.Questions)),
	)

	// DEBUG: Log first question structure
	if len(program.Questions) > 0 {
		firstQ := program.Questions[0]
		s.logger.Warn("DEBUG: First question in program",
			zap.String("session_id", sessionID),
			zap.String("question", firstQ.Question),
			zap.Int("question_length", len(firstQ.Question)),
			zap.String("theory", firstQ.Theory),
			zap.Int("theory_length", len(firstQ.Theory)),
			zap.Int("order", firstQ.Order),
		)
	}

	// Создаём или получаем состояние сессии и устанавливаем программу
	s.sessionMgr.GetOrCreate(sessionID, userID)
	s.sessionMgr.SetProgram(sessionID, program)

	// Инициализируем состояние диалога в Redis (dialogue:{chat_id}:state и messages)
	chatID := sessionID // пока используем sessionID как chatID
	if s.redisCl != nil {
		state := map[string]any{
			"chat_id":       chatID,
			"session_id":    sessionID,
			"user_id":       userID,
			"status":        "active",
			"awaiting_llm":  true,
			"awaiting_user": false,
			"started_at":    time.Now().UTC().Format(time.RFC3339),
		}
		if err := s.redisCl.SetState(ctx, chatID, state); err != nil {
			s.logger.Warn("Failed to set initial dialogue state in Redis",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	// Отправляем стартовое message.full событие в agent-service, чтобы он сгенерировал
	// приветствие и первый вопрос.
	if s.agentBridge != nil {
		now := time.Now().UTC()
		messageID := uuid.New().String()
		firstQuestionID := ""
		if program != nil && len(program.Questions) > 0 {
			firstQuestionID = program.Questions[0].ID
		}
		if firstQuestionID == "" {
			s.logger.Error("Missing question_id for first question",
				zap.String("session_id", sessionID),
			)
			return fmt.Errorf("missing question_id for first question")
		}
		event := kafka.MessageFullEvent{
			EventID:   uuid.New().String(),
			EventType: "message.full",
			Timestamp: now.Format(time.RFC3339),
			Service:   "dialogue-aggregator",
			Version:   "1.0.0",
			Payload: map[string]interface{}{
				"chat_id":     chatID,
				"message_id":  messageID,
				"question_id": firstQuestionID,
				"role":        "system",
				"content":     "",
				"metadata": map[string]interface{}{
					"user_id": userID,
					"dialogue_context": map[string]interface{}{
						"session_id": sessionID,
						"status":     "started",
					},
				},
				"source":       "dialogue_aggregator",
				"timestamp":    now.Format(time.RFC3339),
				"processed_at": now.Format(time.RFC3339),
			},
			Metadata: map[string]interface{}{
				"correlation_id": messageID,
			},
		}

		if err := s.agentBridge.SendMessageFull(ctx, event, chatID); err != nil {
			s.logger.Error("Failed to send start message.full to agent-service",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
			// Не падаем, просто продолжаем без агента
			return nil
		}

		// Ждём первый ответ от агента
		waitStart := time.Now()
		phraseEvent, err := s.agentBridge.ReceiveOnePhrase(ctx, chatID, messageID, 60*time.Second)
		if err != nil {
			s.logger.Error("Failed to receive first phrase from agent-service",
				zap.String("session_id", sessionID),
				zap.Duration("wait_duration", time.Since(waitStart)),
				zap.Error(err),
			)
			return nil
		}
		s.logger.Info("First phrase received from agent-service",
			zap.String("session_id", sessionID),
			zap.Duration("wait_duration", time.Since(waitStart)),
		)

		// Извлекаем сгенерированный текст
		generatedText, _ := phraseEvent.Payload["generated_text"].(string)
		if generatedText == "" {
			s.logger.Warn("Empty generated_text from agent-service",
				zap.String("session_id", sessionID),
			)
			return nil
		}

		// Извлекаем текущий индекс вопроса (если агент его прислал)
		currentQuestionIndex := 0
		currentQuestionID := ""
		if md, ok := phraseEvent.Payload["metadata"].(map[string]interface{}); ok {
			if v, ok := md["current_question_index"].(float64); ok {
				currentQuestionIndex = int(v)
			}
			if v, ok := md["question_id"].(string); ok {
				currentQuestionID = v
			}
		}
		if v, ok := phraseEvent.Payload["question_id"].(string); ok && v != "" {
			currentQuestionID = v
		}

		// Сохраняем ответ агента в Redis и chat-crud
		if s.redisCl != nil {
			msg := map[string]any{
				"role":         "assistant",
				"content":      generatedText,
				"timestamp":    time.Now().UTC().Format(time.RFC3339),
				"message_id":   phraseEvent.Payload["message_id"],
				"question_id":  currentQuestionID,
				"message_kind": "question",
			}
			if err := s.redisCl.AppendMessage(ctx, chatID, msg); err != nil {
				s.logger.Warn("Failed to append assistant message to Redis",
					zap.String("session_id", sessionID),
					zap.Error(err),
				)
			}

			// Обновляем состояние диалога: фиксируем текущий вопрос для следующего ответа
			stateUpdate := map[string]any{
				"current_question_index": currentQuestionIndex,
				"current_question_id":    currentQuestionID,
				"current_message_id":     phraseEvent.Payload["message_id"],
			}
			if err := s.redisCl.SetState(ctx, chatID, stateUpdate); err != nil {
				s.logger.Warn("Failed to update dialogue state in Redis",
					zap.String("session_id", sessionID),
					zap.Error(err),
				)
			}
		}

		if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, generatedText, map[string]any{
			"message_id":   phraseEvent.Payload["message_id"],
			"question_id":  currentQuestionID,
			"message_kind": "question",
		}); err != nil {
			s.logger.Warn("Failed to save first generated question to chat-crud",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}

		// Отправляем первый вопрос пользователю через chat.events.in
		if err := s.producer.SendModelQuestion(sessionID, userID, generatedText); err != nil {
			s.logger.Error("Failed to send first generated question to Kafka",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
			return nil
		}

		s.logger.Info("First question generated by agent-service and sent",
			zap.String("session_id", sessionID),
		)

		return nil
	}

	// Проверяем, нужно ли отправлять следующий вопрос
	if !s.sessionMgr.HasMoreQuestions(sessionID) {
		s.logger.Info("No more questions to ask, session already completed",
			zap.String("session_id", sessionID),
		)
		// Все вопросы уже заданы, но без agent-service не можем сформировать результаты
		s.logger.Error("Cannot complete chat without agent-service evaluation",
			zap.String("session_id", sessionID),
		)
		return fmt.Errorf("cannot complete chat: agent-service required for results")
	}

	// Небольшая задержка перед отправкой следующего вопроса
	time.Sleep(s.questionDelay)

	// Получаем следующий вопрос из программы
	firstQuestion, ok := s.sessionMgr.GetNextQuestion(sessionID)
	if !ok {
		return fmt.Errorf("no questions in program")
	}

	// Сначала сохраняем сообщение в chat-crud
	systemMsg := fmt.Sprintf("Вопрос: %s", firstQuestion.Question)
	systemMessageID := uuid.New().String()
	if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, systemMsg, map[string]any{
		"message_id":   systemMessageID,
		"question_id":  firstQuestion.ID,
		"message_kind": "question",
	}); err != nil {
		s.logger.Warn("Failed to save system message to chat-crud, continuing",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		// Продолжаем, даже если не удалось сохранить
	} else {
		s.logger.Info("System message saved to chat-crud",
			zap.String("session_id", sessionID),
		)
	}

	// Затем отправляем вопрос в Kafka
	if err := s.producer.SendModelQuestion(sessionID, userID, firstQuestion.Question); err != nil {
		return fmt.Errorf("send first question: %w", err)
	}

	// Увеличиваем счётчик вопросов
	s.sessionMgr.IncrementQuestion(sessionID)

	s.logger.Info("First question sent",
		zap.String("session_id", sessionID),
		zap.String("question", firstQuestion.Question),
		zap.Int("question_number", 1),
	)

	return nil
}

// HandleUserMessage обрабатывает сообщение пользователя через agent-service.
func (s *ModelService) HandleUserMessage(ctx context.Context, sessionID, userID, content, messageID string) error {
	s.logger.Info("User message received",
		zap.String("session_id", sessionID),
		zap.String("user_id", userID),
		zap.String("message_id", messageID),
		zap.String("content", content),
	)

	// Парсим sessionID
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session_id: %w", err)
	}

	chatID := sessionID // используем sessionID как chatID
	currentQuestionID := ""
	if s.redisCl != nil {
		if state, err := s.redisCl.GetState(ctx, chatID); err == nil && state != nil {
			if v, ok := state["current_question_id"].(string); ok {
				currentQuestionID = v
			}
		}
	}
	if currentQuestionID == "" {
		s.logger.Error("Missing current_question_id for user message",
			zap.String("session_id", sessionID),
		)
		return fmt.Errorf("missing current_question_id")
	}

	// Сохраняем сообщение пользователя в Redis и chat-crud
	if s.redisCl != nil {
		msg := map[string]any{
			"role":         "user",
			"content":      content,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
			"message_id":   messageID,
			"question_id":  currentQuestionID,
			"message_kind": "answer",
		}
		if err := s.redisCl.AppendMessage(ctx, chatID, msg); err != nil {
			s.logger.Warn("Failed to append user message to Redis",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeUser, content, map[string]any{
		"message_id":   messageID,
		"question_id":  currentQuestionID,
		"message_kind": "answer",
	}); err != nil {
		s.logger.Warn("Failed to save user message to chat-crud, continuing",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}

	// Обновляем состояние диалога в Redis
	if s.redisCl != nil {
		state := map[string]any{}
		if existingState, err := s.redisCl.GetState(ctx, chatID); err == nil && existingState != nil {
			for k, v := range existingState {
				state[k] = v
			}
		}
		state["last_activity"] = time.Now().UTC().Format(time.RFC3339)
		state["awaiting_llm"] = true
		state["awaiting_user"] = false
		if err := s.redisCl.SetState(ctx, chatID, state); err != nil {
			s.logger.Warn("Failed to update dialogue state in Redis",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	// Отправляем message.full в agent-service
	if s.agentBridge == nil {
		s.logger.Warn("Agent bridge not available, falling back to old logic",
			zap.String("session_id", sessionID),
		)
		return s.handleUserMessageFallback(ctx, sessionID, userID, content, messageID)
	}

	now := time.Now().UTC()
	msgID := uuid.New().String()
	event := kafka.MessageFullEvent{
		EventID:   uuid.New().String(),
		EventType: "message.full",
		Timestamp: now.Format(time.RFC3339),
		Service:   "dialogue-aggregator",
		Version:   "1.0.0",
		Payload: map[string]interface{}{
			"chat_id":     chatID,
			"message_id":  msgID,
			"question_id": currentQuestionID,
			"role":        "user",
			"content":     content,
			"metadata": map[string]interface{}{
				"user_id": userID,
				"dialogue_context": map[string]interface{}{
					"session_id": sessionID,
					"status":     "active",
				},
			},
			"source":       "user_input",
			"timestamp":    now.Format(time.RFC3339),
			"processed_at": now.Format(time.RFC3339),
		},
		Metadata: map[string]interface{}{
			"correlation_id": msgID,
		},
	}

	if err := s.agentBridge.SendMessageFull(ctx, event, chatID); err != nil {
		s.logger.Error("Failed to send message.full to agent-service",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		return s.handleUserMessageFallback(ctx, sessionID, userID, content, messageID)
	}

	// Ждём ответ от агента
	waitStart := time.Now()
	phraseEvent, err := s.agentBridge.ReceiveOnePhrase(ctx, chatID, msgID, 60*time.Second)
	if err != nil {
		s.logger.Error("Failed to receive phrase.agent.generated from agent-service",
			zap.String("session_id", sessionID),
			zap.Duration("wait_duration", time.Since(waitStart)),
			zap.Error(err),
		)
		// Fallback: отправляем вежливое сообщение с просьбой повторить ответ
		// НЕ показываем техническую ошибку пользователю
		fallbackMsg := "Извините, не удалось обработать ваш ответ. Можете, пожалуйста, попробовать ответить еще раз или переформулировать ваш ответ?"
		fallbackMessageID := uuid.New().String()
		if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, fallbackMsg, map[string]any{
			"message_id":   fallbackMessageID,
			"question_id":  currentQuestionID,
			"message_kind": "system",
		}); err == nil {
			_ = s.producer.SendModelQuestion(sessionID, userID, fallbackMsg)
		}
		return nil
	}
	s.logger.Info("phrase.agent.generated received",
		zap.String("session_id", sessionID),
		zap.Duration("wait_duration", time.Since(waitStart)),
	)

	generatedText, _ := phraseEvent.Payload["generated_text"].(string)
	if generatedText == "" {
		s.logger.Warn("Empty generated_text from agent-service",
			zap.String("session_id", sessionID),
		)
		return nil
	}

	// Проверяем, завершил ли агент интервью
	metadata, _ := phraseEvent.Payload["metadata"].(map[string]interface{})
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	agentDecision := ""
	if v, ok := metadata["agent_decision"].(string); ok {
		agentDecision = v
	}
	messageKind := "question"
	if v, ok := metadata["message_kind"].(string); ok && v != "" {
		messageKind = v
	}
	scoreDelta := metadata["score_delta"]

	currentQuestionIndex := 0
	if v, ok := metadata["current_question_index"].(float64); ok {
		currentQuestionIndex = int(v)
	}
	currentQuestionID = ""
	if v, ok := phraseEvent.Payload["question_id"].(string); ok {
		currentQuestionID = v
	} else if v, ok := metadata["question_id"].(string); ok {
		currentQuestionID = v
	}

	// Если агент перешел к следующему вопросу, продвигаем индекс
	nextQuestionIndex := currentQuestionIndex
	if agentDecision == "next_question" && currentQuestionIndex > 0 {
		nextQuestionIndex = currentQuestionIndex + 1
	}

	// Сохраняем ответ агента в Redis и chat-crud
	if s.redisCl != nil {
		msg := map[string]any{
			"role":         "assistant",
			"content":      generatedText,
			"timestamp":    time.Now().UTC().Format(time.RFC3339),
			"message_id":   phraseEvent.Payload["message_id"],
			"question_id":  currentQuestionID,
			"message_kind": messageKind,
		}
		if scoreDelta != nil {
			msg["score_delta"] = scoreDelta
		}
		if err := s.redisCl.AppendMessage(ctx, chatID, msg); err != nil {
			s.logger.Warn("Failed to append assistant message to Redis",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}

		// Обновляем состояние
		stateUpdate := map[string]any{}
		if existingState, err := s.redisCl.GetState(ctx, chatID); err == nil && existingState != nil {
			for k, v := range existingState {
				stateUpdate[k] = v
			}
		}
		stateUpdate["awaiting_llm"] = false
		stateUpdate["awaiting_user"] = agentDecision != "thank_you"
		if nextQuestionIndex > 0 {
			stateUpdate["current_question_index"] = nextQuestionIndex
		}
		if currentQuestionID != "" {
			stateUpdate["current_question_id"] = currentQuestionID
		}
		if phraseEvent.Payload["message_id"] != nil {
			stateUpdate["current_message_id"] = phraseEvent.Payload["message_id"]
		}
		if messageKind == "clarification" && currentQuestionID != "" {
			history, _ := stateUpdate["clarification_history"].(map[string]any)
			if history == nil {
				history = map[string]any{}
			}
			list, _ := history[currentQuestionID].([]any)
			list = append(list, phraseEvent.Payload["message_id"])
			history[currentQuestionID] = list
			stateUpdate["clarification_history"] = history
		}
		if agentDecision == "thank_you" {
			stateUpdate["status"] = "finished"
		}
		if err := s.redisCl.SetState(ctx, chatID, stateUpdate); err != nil {
			s.logger.Warn("Failed to update dialogue state after agent response",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	metadataMap := map[string]any{
		"message_id":   phraseEvent.Payload["message_id"],
		"question_id":  currentQuestionID,
		"message_kind": messageKind,
	}
	if scoreDelta != nil {
		metadataMap["score_delta"] = scoreDelta
	}
	if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, generatedText, metadataMap); err != nil {
		s.logger.Warn("Failed to save assistant message to chat-crud",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}

	// Если агент решил завершить интервью
	if agentDecision == "thank_you" {
		// Сохраняем финальное сообщение
		completionMsg := "Интервью завершено. Результаты будут доступны в разделе результатов."
		if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, completionMsg, map[string]any{
			"message_id":   uuid.New().String(),
			"question_id":  currentQuestionID,
			"message_kind": "system",
		}); err != nil {
			s.logger.Warn("Failed to save completion message",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}

		if err := s.chatCRUDCl.CreateChatDump(ctx, sessionUUID); err != nil {
			s.logger.Warn("Failed to create chat dump",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}

		// Извлекаем оценку из metadata.evaluation от agent-service
		score, feedback, recommendations := s.extractResultsFromEvaluation(metadata)

		if err := s.resultsCRUDCl.SaveResult(ctx, sessionUUID, score, feedback, false); err != nil {
			s.logger.Error("Failed to save result",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}

		if err := s.sessionManagerCl.CloseSession(ctx, sessionUUID); err != nil {
			s.logger.Warn("Failed to close session",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}

		if err := s.producer.SendChatCompleted(sessionID, userID, score, feedback, recommendations); err != nil {
			return fmt.Errorf("send chat completed: %w", err)
		}

		s.sessionMgr.Delete(sessionID)
		s.logger.Info("Chat completed by agent",
			zap.String("session_id", sessionID),
			zap.Int("score", score),
		)
		return nil
	}

	// Отправляем сгенерированный вопрос/ответ в chat.events.in
	if err := s.producer.SendModelQuestion(sessionID, userID, generatedText); err != nil {
		return fmt.Errorf("send model question: %w", err)
	}

	s.logger.Info("Agent response sent",
		zap.String("session_id", sessionID),
		zap.String("decision", agentDecision),
	)

	return nil
}

// handleUserMessageFallback старая логика на случай недоступности agent-service.
func (s *ModelService) handleUserMessageFallback(ctx context.Context, sessionID, userID, content, messageID string) error {
	sessionUUID, _ := uuid.Parse(sessionID)
	state := s.sessionMgr.GetOrCreate(sessionID, userID)

	if state.Program == nil {
		return fmt.Errorf("session program not found")
	}

	if s.sessionMgr.ShouldComplete(sessionID) {
		// Без agent-service не можем сформировать результаты
		s.logger.Error("Cannot complete chat without agent-service evaluation",
			zap.String("session_id", sessionID),
		)
		return fmt.Errorf("cannot complete chat: agent-service required for results")
	}

	time.Sleep(s.questionDelay)
	nextQuestion, ok := s.sessionMgr.GetNextQuestion(sessionID)
	if !ok {
		return fmt.Errorf("failed to get next question")
	}

	_ = s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, fmt.Sprintf("Вопрос: %s", nextQuestion.Question), map[string]any{
		"message_id":   messageID,
		"question_id":  nextQuestion.ID,
		"message_kind": "question",
	})
	if err := s.producer.SendModelQuestion(sessionID, userID, nextQuestion.Question); err != nil {
		return err
	}

	s.sessionMgr.IncrementQuestion(sessionID)
	return nil
}

// extractResultsFromEvaluation извлекает score, feedback и recommendations из evaluation от agent-service.
func (s *ModelService) extractResultsFromEvaluation(metadata map[string]interface{}) (int, string, []string) {
	// Извлекаем evaluation из metadata
	evaluation, _ := metadata["evaluation"].(map[string]interface{})
	if evaluation == nil {
		s.logger.Warn("No evaluation found in metadata, using default values")
		return 0, "Не удалось получить оценку от агента.", []string{"Попробуйте пройти интервью еще раз"}
	}

	// Извлекаем overall_score (0.0-1.0) и конвертируем в 0-100
	overallScoreFloat, ok := evaluation["overall_score"].(float64)
	if !ok {
		s.logger.Warn("overall_score not found or invalid in evaluation")
		overallScoreFloat = 0.0
	}
	score := int(overallScoreFloat * 100)
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	// Извлекаем feedback из evaluation_reasoning
	feedback := ""
	if reasoning, ok := evaluation["evaluation_reasoning"].(string); ok && reasoning != "" {
		feedback = reasoning
	}

	// Извлекаем рекомендации из metadata.recommendations (генерируются при thank_you)
	recommendations := []string{}
	if recs, ok := metadata["recommendations"].([]interface{}); ok && len(recs) > 0 {
		for _, rec := range recs {
			if recStr, ok := rec.(string); ok && recStr != "" {
				recommendations = append(recommendations, recStr)
			}
		}
	}

	// Fallback: если рекомендаций нет, используем missing_points из последнего ответа
	if len(recommendations) == 0 {
		if missingPoints, ok := evaluation["missing_points"].([]interface{}); ok && len(missingPoints) > 0 {
			for _, point := range missingPoints {
				if pointStr, ok := point.(string); ok && pointStr != "" {
					recommendations = append(recommendations, fmt.Sprintf("Изучить: %s", pointStr))
				}
			}
		}
	}

	return score, feedback, recommendations
}

// HandleChatResumed обрабатывает восстановление активной сессии чата.
func (s *ModelService) HandleChatResumed(ctx context.Context, sessionID, userID string) error {
	s.logger.Info("Chat resumed, restoring session state",
		zap.String("session_id", sessionID),
		zap.String("user_id", userID),
	)

	// Парсим sessionID
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session_id: %w", err)
	}

	// Получаем программу интервью от session-manager
	program, err := s.sessionManagerCl.GetInterviewProgram(ctx, sessionUUID)
	if err != nil {
		return fmt.Errorf("get interview program: %w", err)
	}

	s.logger.Info("Interview program received for resume",
		zap.String("session_id", sessionID),
		zap.Int("questions_count", len(program.Questions)),
	)

	chatID := sessionID

	// Восстанавливаем состояние из chat-crud и заполняем Redis
	messages, err := s.chatCRUDCl.GetMessages(ctx, sessionUUID)
	if err != nil {
		s.logger.Warn("Failed to get chat history for resume, assuming new session state",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		s.sessionMgr.GetOrCreate(sessionID, userID)
		s.sessionMgr.SetProgram(sessionID, program)
		return nil
	}

	// Восстанавливаем историю в Redis из chat-crud (если Redis пуст)
	if s.redisCl != nil && len(messages) > 0 {
		// TODO: можно добавить проверку, есть ли уже данные в Redis
		// Пока просто заполняем Redis из chat-crud
		for _, msg := range messages {
			role := "user"
			if msg.Type == "system" {
				role = "assistant"
			}
			// Используем CreatedAt как есть (это строка) или текущее время
			timestamp := msg.CreatedAt
			if timestamp == "" {
				timestamp = time.Now().UTC().Format(time.RFC3339)
			}
			redisMsg := map[string]any{
				"role":      role,
				"content":   msg.Content,
				"timestamp": timestamp,
			}
			if err := s.redisCl.AppendMessage(ctx, chatID, redisMsg); err != nil {
				s.logger.Warn("Failed to restore message to Redis",
					zap.String("session_id", sessionID),
					zap.Error(err),
				)
			}
		}

		// Восстанавливаем состояние
		systemMessagesCount := 0
		userMessagesCount := 0
		for _, msg := range messages {
			if msg.Type == "system" {
				if msg.Metadata != nil {
					if kind, ok := msg.Metadata["message_kind"].(string); ok && kind == "question" {
						systemMessagesCount++
					}
				}
			} else if msg.Type == "user" {
				userMessagesCount++
			}
		}

		state := map[string]any{
			"last_activity": time.Now().UTC().Format(time.RFC3339),
			"awaiting_llm":  userMessagesCount == systemMessagesCount && s.sessionMgr.HasMoreQuestions(sessionID),
			"awaiting_user": userMessagesCount < systemMessagesCount,
		}
		if err := s.redisCl.SetState(ctx, chatID, state); err != nil {
			s.logger.Warn("Failed to restore dialogue state in Redis",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	// Подсчитываем количество системных сообщений (вопросов) и пользовательских сообщений
	systemMessagesCount := 0
	userMessagesCount := 0
	for _, msg := range messages {
		if msg.Type == "system" {
			if msg.Metadata != nil {
				if kind, ok := msg.Metadata["message_kind"].(string); ok && kind == "question" {
					systemMessagesCount++
				}
			}
		} else if msg.Type == "user" {
			userMessagesCount++
		}
	}

	// Восстанавливаем состояние на основе истории
	s.logger.Info("Restoring session state from chat history",
		zap.String("session_id", sessionID),
		zap.Int("system_messages_count", systemMessagesCount),
		zap.Int("user_messages_count", userMessagesCount),
		zap.Int("total_messages_count", len(messages)),
	)

	s.sessionMgr.RestoreStateFromChatHistory(sessionID, program, systemMessagesCount)

	s.logger.Info("Session restored successfully",
		zap.String("session_id", sessionID),
		zap.Int("questions_asked", s.sessionMgr.GetQuestionCount(sessionID)),
	)

	// Определяем текущий question_id и message_id из истории (по сохраненным метаданным)
	currentQuestionID := ""
	currentMessageID := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Metadata != nil {
			if v, ok := messages[i].Metadata["question_id"].(string); ok && v != "" {
				currentQuestionID = v
				if mid, ok := messages[i].Metadata["message_id"].(string); ok {
					currentMessageID = mid
				}
				break
			}
		}
	}
	if currentQuestionID == "" {
		s.logger.Error("Missing question_id in chat history for resume",
			zap.String("session_id", sessionID),
		)
		return fmt.Errorf("missing question_id for resume")
	}

	if s.redisCl != nil {
		stateUpdate := map[string]any{}
		if existingState, err := s.redisCl.GetState(ctx, chatID); err == nil && existingState != nil {
			for k, v := range existingState {
				stateUpdate[k] = v
			}
		}
		stateUpdate["current_question_id"] = currentQuestionID
		if currentMessageID != "" {
			stateUpdate["current_message_id"] = currentMessageID
		}
		if err := s.redisCl.SetState(ctx, chatID, stateUpdate); err != nil {
			s.logger.Warn("Failed to update current question in Redis after resume",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	// Если пользователь уже ответил на последний вопрос и есть еще вопросы,
	// инициируем вызов agent-service для генерации следующего вопроса
	if s.sessionMgr.HasMoreQuestions(sessionID) && userMessagesCount == systemMessagesCount && len(messages) > 0 && s.agentBridge != nil {
		// Отправляем message.full с контекстом resume
		now := time.Now().UTC()
		msgID := uuid.New().String()
		event := kafka.MessageFullEvent{
			EventID:   uuid.New().String(),
			EventType: "message.full",
			Timestamp: now.Format(time.RFC3339),
			Service:   "dialogue-aggregator",
			Version:   "1.0.0",
			Payload: map[string]interface{}{
				"chat_id":     chatID,
				"message_id":  msgID,
				"question_id": currentQuestionID,
				"role":        "system",
				"content":     "resume_dialogue",
				"metadata": map[string]interface{}{
					"user_id": userID,
					"dialogue_context": map[string]interface{}{
						"session_id": sessionID,
						"status":     "resumed",
					},
				},
				"source":       "dialogue_aggregator",
				"timestamp":    now.Format(time.RFC3339),
				"processed_at": now.Format(time.RFC3339),
			},
			Metadata: map[string]interface{}{
				"correlation_id": msgID,
			},
		}

		if err := s.agentBridge.SendMessageFull(ctx, event, chatID); err != nil {
			s.logger.Error("Failed to send resume message to agent-service",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		} else {
			// Ждём ответ от агента
			waitStart := time.Now()
			phraseEvent, err := s.agentBridge.ReceiveOnePhrase(ctx, chatID, msgID, 60*time.Second)
			if err != nil {
				s.logger.Error("Failed to receive phrase after resume",
					zap.String("session_id", sessionID),
					zap.Duration("wait_duration", time.Since(waitStart)),
					zap.Error(err),
				)
			} else {
				s.logger.Info("Phrase received after resume",
					zap.String("session_id", sessionID),
					zap.Duration("wait_duration", time.Since(waitStart)),
				)
				generatedText, _ := phraseEvent.Payload["generated_text"].(string)
				if generatedText != "" {
					// Сохраняем в Redis и chat-crud
					if s.redisCl != nil {
						msg := map[string]any{
							"role":      "assistant",
							"content":   generatedText,
							"timestamp": time.Now().UTC().Format(time.RFC3339),
						}
						_ = s.redisCl.AppendMessage(ctx, chatID, msg)
					}
					questionID := ""
					if v, ok := phraseEvent.Payload["question_id"].(string); ok {
						questionID = v
					}
					_ = s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, generatedText, map[string]any{
						"message_id":   phraseEvent.Payload["message_id"],
						"question_id":  questionID,
						"message_kind": "question",
					})
					_ = s.producer.SendModelQuestion(sessionID, userID, generatedText)
					s.sessionMgr.IncrementQuestion(sessionID)
					s.logger.Info("Next question generated by agent after resume",
						zap.String("session_id", sessionID),
					)
				}
			}
		}
	}

	return nil
}

// HandleChatTerminated обрабатывает досрочное завершение чата пользователем.
func (s *ModelService) HandleChatTerminated(ctx context.Context, sessionID, userID string) error {
	s.logger.Info("Chat terminated by user",
		zap.String("session_id", sessionID),
		zap.String("user_id", userID),
	)

	// Парсим sessionID
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session_id: %w", err)
	}

	chatID := sessionID

	// Сохраняем финальное сообщение о досрочном завершении в Redis и chat-crud
	terminationMsg := "Чат завершен пользователем."
	if s.redisCl != nil {
		msg := map[string]any{
			"role":      "system",
			"content":   terminationMsg,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}
		if err := s.redisCl.AppendMessage(ctx, chatID, msg); err != nil {
			s.logger.Warn("Failed to append termination message to Redis",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}
	if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, terminationMsg, map[string]any{
		"message_id":   uuid.New().String(),
		"message_kind": "system",
	}); err != nil {
		s.logger.Warn("Failed to save termination message to chat-crud",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}

	// Создаём дамп чата
	if err := s.chatCRUDCl.CreateChatDump(ctx, sessionUUID); err != nil {
		s.logger.Warn("Failed to create chat dump",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}

	// При досрочном завершении используем минимальную оценку (нет данных от агента)
	score := 0
	feedback := "Интервью было досрочно завершено пользователем."
	recommendations := []string{
		"Рекомендуется пройти интервью полностью для получения детальной оценки",
		"Попробуйте ответить на все вопросы программы интервью",
	}

	// Сохраняем результаты с флагом terminated_early = true
	if err := s.resultsCRUDCl.SaveResult(ctx, sessionUUID, score, feedback, true); err != nil {
		s.logger.Error("Failed to save result to results-crud",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		// Продолжаем, даже если не удалось сохранить результат
	}

	// Закрываем сессию в session-manager
	if err := s.sessionManagerCl.CloseSession(ctx, sessionUUID); err != nil {
		s.logger.Warn("Failed to close session in session-manager",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}

	// Отправляем событие завершения в Kafka (с результатами)
	if err := s.producer.SendChatCompleted(sessionID, userID, score, feedback, recommendations); err != nil {
		return fmt.Errorf("send chat completed: %w", err)
	}

	// Удаляем сессию из локального менеджера
	s.sessionMgr.Delete(sessionID)

	s.logger.Info("Chat terminated successfully",
		zap.String("session_id", sessionID),
		zap.Int("score", score),
	)

	return nil
}
