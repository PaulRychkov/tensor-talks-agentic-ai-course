package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tensor-talks/dialogue-aggregator/internal/client"
	"github.com/tensor-talks/dialogue-aggregator/internal/kafka"
	"github.com/tensor-talks/dialogue-aggregator/internal/model"
	"github.com/tensor-talks/dialogue-aggregator/internal/models"
	"github.com/tensor-talks/dialogue-aggregator/internal/redisclient"
	"go.uber.org/zap"
)

// studyWrapQuestion wraps theory+question in study mode markers recognised by the frontend.
func studyWrapQuestion(theory, question string) string {
	if theory == "" {
		return question
	}
	return "[STUDY_THEORY]\n" + strings.TrimSpace(theory) + "\n[/STUDY_THEORY]\n[STUDY_QUESTION]\n" + strings.TrimSpace(question) + "\n[/STUDY_QUESTION]"
}

// prettyTopic превращает тег подтемы (theory_rag, machine_learning) в человекочитаемое
// название для отображения в плане ("RAG", "Machine Learning").
func prettyTopic(tag string) string {
	if tag == "" {
		return ""
	}
	t := strings.TrimPrefix(strings.TrimPrefix(tag, "theory_"), "practice_")
	t = strings.ReplaceAll(t, "_", " ")
	// Аббревиатуры — uppercase целиком; иначе title-case.
	known := map[string]string{
		"rag": "RAG", "llm": "LLM", "ml": "ML", "nlp": "NLP", "cv": "CV",
		"db": "БД", "sql": "SQL", "api": "API",
		"bert": "BERT", "gpt": "GPT", "rnn": "RNN", "lstm": "LSTM", "gru": "GRU",
		"t5": "T5", "elmo": "ELMo", "llama": "LLaMA", "roberta": "RoBERTa",
		"rlhf": "RLHF", "cot": "CoT", "svm": "SVM", "pca": "PCA",
	}
	parts := strings.Fields(t)
	for i, p := range parts {
		if up, ok := known[p]; ok {
			parts[i] = up
		} else if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// buildStudyPlanLines строит строки плана для study-режима в многоуровневом формате:
// верхний уровень — подтема (prettyTopic), под ней ПУНКТЫ — фрагменты теории
// (PointTitle). Вопросы к пунктам в план НЕ выводятся.
// Группировка: по (Subtopic или Topic) → уникальные PointID внутри группы.
// Fallback для старого формата (без point_*): по одному вопросу на строку.
func buildStudyPlanLines(questions []models.QuestionItem) []string {
	type pointEntry struct {
		id    string
		title string
	}
	subtopicOrder := []string{}
	subtopicPoints := map[string][]pointEntry{}
	seenPoints := map[string]bool{}
	hasPoints := false

	for _, q := range questions {
		subtopic := strings.TrimSpace(q.Subtopic)
		if subtopic == "" {
			subtopic = strings.TrimSpace(q.Topic)
		}
		pointID := strings.TrimSpace(q.PointID)
		pointTitle := strings.TrimSpace(q.PointTitle)
		if subtopic == "" || pointID == "" || pointTitle == "" {
			continue
		}
		hasPoints = true
		if _, seen := subtopicPoints[subtopic]; !seen {
			subtopicOrder = append(subtopicOrder, subtopic)
		}
		key := subtopic + "::" + pointID
		if seenPoints[key] {
			continue
		}
		seenPoints[key] = true
		subtopicPoints[subtopic] = append(subtopicPoints[subtopic], pointEntry{id: pointID, title: pointTitle})
	}

	shortenText := func(raw string, fallbackIdx int) string {
		text := strings.TrimSpace(raw)
		if idx := strings.IndexAny(text, "\n"); idx != -1 {
			text = strings.TrimSpace(text[:idx])
		}
		if len(text) > 160 {
			text = text[:157] + "..."
		}
		if text == "" {
			text = fmt.Sprintf("Пункт %d", fallbackIdx+1)
		}
		return text
	}

	if hasPoints {
		lines := make([]string, 0)
		for _, subtopic := range subtopicOrder {
			pts := subtopicPoints[subtopic]
			lines = append(lines, fmt.Sprintf("%s — %d %s", prettyTopic(subtopic), len(pts), pluralPoints(len(pts))))
			for i, p := range pts {
				lines = append(lines, "- "+shortenText(p.title, i))
			}
		}
		return lines
	}

	// Legacy fallback: group by Topic, one bullet per question.
	order := []string{}
	grouped := map[string][]models.QuestionItem{}
	hasTopic := false
	for _, q := range questions {
		topic := strings.TrimSpace(q.Topic)
		if topic == "" {
			continue
		}
		hasTopic = true
		if _, seen := grouped[topic]; !seen {
			order = append(order, topic)
		}
		grouped[topic] = append(grouped[topic], q)
	}
	if hasTopic {
		lines := make([]string, 0)
		for _, topic := range order {
			qs := grouped[topic]
			lines = append(lines, fmt.Sprintf("%s — %d %s", prettyTopic(topic), len(qs), pluralPoints(len(qs))))
			for i, q := range qs {
				lines = append(lines, "- "+shortenText(q.Question, i))
			}
		}
		return lines
	}

	// Last-resort fallback: flat list.
	lines := make([]string, 0, len(questions))
	for i, q := range questions {
		lines = append(lines, shortenText(q.Question, i))
	}
	return lines
}

// countStudyPoints returns the number of unique study points (PointID)
// in the program. Falls back to len(questions) when no point metadata exists.
func countStudyPoints(questions []models.QuestionItem) int {
	seen := map[string]bool{}
	for _, q := range questions {
		pid := strings.TrimSpace(q.PointID)
		if pid == "" {
			// no point metadata → fall back to raw question count
			return len(questions)
		}
		sub := strings.TrimSpace(q.Subtopic)
		if sub == "" {
			sub = strings.TrimSpace(q.Topic)
		}
		key := sub + "::" + pid
		seen[key] = true
	}
	if len(seen) == 0 {
		return len(questions)
	}
	return len(seen)
}

// studyPointNumber converts a 0-indexed question array index to a 1-indexed
// study point number. Questions sharing the same PointID belong to the same point.
func studyPointNumber(questions []models.QuestionItem, arrayIdx int) int {
	if arrayIdx < 0 || arrayIdx >= len(questions) {
		return 0
	}
	seen := map[string]int{} // pointKey → pointNumber (1-based)
	counter := 0
	for i, q := range questions {
		pid := strings.TrimSpace(q.PointID)
		if pid == "" {
			// no point metadata → use raw index
			if i == arrayIdx {
				return i + 1
			}
			continue
		}
		sub := strings.TrimSpace(q.Subtopic)
		if sub == "" {
			sub = strings.TrimSpace(q.Topic)
		}
		key := sub + "::" + pid
		if _, ok := seen[key]; !ok {
			counter++
			seen[key] = counter
		}
		if i == arrayIdx {
			return seen[key]
		}
	}
	return 0
}

func pluralPoints(n int) string {
	mod10 := n % 10
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return "пунктов"
	}
	switch mod10 {
	case 1:
		return "пункт"
	case 2, 3, 4:
		return "пункта"
	default:
		return "пунктов"
	}
}

// ModelService is the dialogue-aggregator: it routes messages between
// BFF (chat.events.*) and agent-service (messages.full.data /
// generated.phrases) via AgentBridge, persists messages to CRUD,
// manages Redis dialogue state, and relays agent evaluations to results.
// Business decisions (next/hint/clarify/skip) are made exclusively by
// agent-service; this service does NOT modify agent payload content.
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
	agentSemaphore   chan struct{}
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
		agentSemaphore:   make(chan struct{}, 3),
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

	// Получаем программу интервью + метаданные сессии (kind/topics/level)
	program, meta, err := s.sessionManagerCl.GetInterviewProgramWithMeta(ctx, sessionUUID)
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
	sessionKind := "interview"
	if meta != nil {
		sessionKind = meta.SessionKind
		if sessionKind == "" {
			sessionKind = "interview"
		}
		s.sessionMgr.SetMeta(sessionID, sessionKind, meta.Topics, meta.Level)
	}

	// Инициализируем состояние диалога в Redis (dialogue:{chat_id}:state и messages)
	chatID := sessionID // пока используем sessionID как chatID
	// For study mode, progress is measured in "points" (unique PointIDs), not questions.
	progressTotal := len(program.Questions)
	if sessionKind == "study" {
		progressTotal = countStudyPoints(program.Questions)
	}
	if s.redisCl != nil {
		state := map[string]any{
			"chat_id":         chatID,
			"session_id":      sessionID,
			"user_id":         userID,
			"status":          "active",
			"awaiting_llm":    true,
			"awaiting_user":   false,
			"started_at":      time.Now().UTC().Format(time.RFC3339),
			"total_questions":  progressTotal,
		}
		if err := s.redisCl.SetState(ctx, chatID, state); err != nil {
			s.logger.Warn("Failed to set initial dialogue state in Redis",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	if s.agentBridge == nil {
		return fmt.Errorf("agent-service bridge is required for chat operation")
	}

	// Для режима изучения — отправляем план до первого вопроса.
	// Группируем по q.Topic (подтеме): одна строка на подтему с количеством пунктов.
	// Если topic не заполнен (старый формат), fallback — по одной строке на вопрос.
	sessionKind, _, _ = s.sessionMgr.GetMeta(sessionID)
	if sessionKind == "study" && program != nil && len(program.Questions) > 0 {
		planLines := buildStudyPlanLines(program.Questions)
		planText := "[STUDY_PLAN]\n" + strings.Join(planLines, "\n") + "\n[/STUDY_PLAN]"
		if err := s.producer.SendModelQuestion(sessionID, userID, planText, 0, progressTotal); err != nil {
			s.logger.Warn("Failed to send study plan", zap.String("session_id", sessionID), zap.Error(err))
		}
		// Сохраняем план в chat-crud, чтобы он появлялся при resume-е сессии.
		// Frontend дедуплицирует live+history по content prefix [STUDY_PLAN].
		if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, planText, map[string]any{
			"message_kind": "study_plan",
		}); err != nil {
			s.logger.Warn("Failed to save study plan to chat-crud", zap.String("session_id", sessionID), zap.Error(err))
		}
	}

	if s.agentBridge != nil {
		now := time.Now().UTC()
		messageID := uuid.New().String()
		firstQuestionID := ""
		if program != nil && len(program.Questions) > 0 {
			firstQuestionID = program.Questions[0].ID
		}
		if firstQuestionID == "" {
			s.logger.Error("Missing question_id for first question — empty program, sending error to user",
				zap.String("session_id", sessionID),
			)
			errMsg := "К сожалению, не удалось подготовить программу сессии. Пожалуйста, вернитесь назад и попробуйте ещё раз."
			_ = s.producer.SendModelQuestion(sessionID, userID, errMsg, 0, 0)
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

		// Acquire semaphore to limit concurrent agent invocations (prevents LLM rate-limit pile-up)
		select {
		case s.agentSemaphore <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-s.agentSemaphore }()

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
		phraseEvent, err := s.agentBridge.ReceiveOnePhrase(ctx, chatID, messageID, 120*time.Second)
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
		if md, ok := phraseEvent.Payload["metadata"].(map[string]interface{}); ok { //nolint
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

		// Обновляем состояние диалога: фиксируем текущий вопрос для следующего ответа.
		// Содержимое сообщения (Redis history + chat-crud) сохраняется ниже — в зависимости
		// от режима: для study это greeting + theory+question, для interview/training — generatedText.
		if s.redisCl != nil {
			stateUpdate := map[string]any{}
			if existingState, err := s.redisCl.GetState(ctx, chatID); err == nil && existingState != nil {
				for k, v := range existingState {
					stateUpdate[k] = v
				}
			}
			stateUpdate["current_question_index"] = currentQuestionIndex
			stateUpdate["current_question_id"] = currentQuestionID
			stateUpdate["current_message_id"] = phraseEvent.Payload["message_id"]
			if err := s.redisCl.SetState(ctx, chatID, stateUpdate); err != nil {
				s.logger.Warn("Failed to update dialogue state in Redis",
					zap.String("session_id", sessionID),
					zap.Error(err),
				)
			}
		}

		// В режиме изучения: отправляем приветствие и первый вопрос+теорию отдельно.
		// Агент возвращает currentQuestionIndex как 1-indexed order → конвертируем в 0-indexed.
		if sessionKind == "study" {
			arrayIdx := currentQuestionIndex - 1
			if arrayIdx < 0 {
				arrayIdx = 0
			}
			if q, ok := s.sessionMgr.GetQuestionByIndex(sessionID, arrayIdx); ok {
				// 1) Короткое приветствие (не из LLM, т.к. LLM-текст содержит
				// сам первый вопрос и появится ДО theory-блока).
				greeting := "Привет! Начинаем изучение. Разберём первый пункт программы."
				if err := s.producer.SendModelQuestion(sessionID, userID, greeting, 0, progressTotal); err != nil {
					s.logger.Error("Failed to send study greeting", zap.String("session_id", sessionID), zap.Error(err))
					return nil
				}
				greetingMsgID := uuid.New().String()
				if s.redisCl != nil {
					_ = s.redisCl.AppendMessage(ctx, chatID, map[string]any{
						"role":         "assistant",
						"content":      greeting,
						"timestamp":    time.Now().UTC().Format(time.RFC3339),
						"message_id":   greetingMsgID,
						"message_kind": "system",
					})
				}
				if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, greeting, map[string]any{
					"message_id":   greetingMsgID,
					"message_kind": "system",
				}); err != nil {
					s.logger.Warn("Failed to save study greeting to chat-crud", zap.String("session_id", sessionID), zap.Error(err))
				}

				// 2) Теория + первый вопрос из программы
				theoryForWrap := q.PointTheory
				if theoryForWrap == "" {
					theoryForWrap = q.Theory
				}
				studyContent := studyWrapQuestion(theoryForWrap, q.Question)
				if err := s.producer.SendModelQuestion(sessionID, userID, studyContent, currentQuestionIndex, progressTotal); err != nil {
					s.logger.Error("Failed to send study theory+question", zap.String("session_id", sessionID), zap.Error(err))
				}
				if s.redisCl != nil {
					_ = s.redisCl.AppendMessage(ctx, chatID, map[string]any{
						"role":         "assistant",
						"content":      studyContent,
						"timestamp":    time.Now().UTC().Format(time.RFC3339),
						"message_id":   phraseEvent.Payload["message_id"],
						"question_id":  currentQuestionID,
						"message_kind": "study_theory",
					})
				}
				if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, studyContent, map[string]any{
					"message_id":   phraseEvent.Payload["message_id"],
					"question_id":  currentQuestionID,
					"message_kind": "study_theory",
				}); err != nil {
					s.logger.Warn("Failed to save study theory to chat-crud", zap.String("session_id", sessionID), zap.Error(err))
				}
				s.logger.Info("Study: sent greeting + theory+question as separate messages",
					zap.String("session_id", sessionID),
					zap.Int("array_idx", arrayIdx),
				)
				return nil
			}
		}

		// Non-study (interview/training): save generatedText as первый вопрос и отправляем.
		if s.redisCl != nil {
			_ = s.redisCl.AppendMessage(ctx, chatID, map[string]any{
				"role":         "assistant",
				"content":      generatedText,
				"timestamp":    time.Now().UTC().Format(time.RFC3339),
				"message_id":   phraseEvent.Payload["message_id"],
				"question_id":  currentQuestionID,
				"message_kind": "question",
			})
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
		if err := s.producer.SendModelQuestion(sessionID, userID, generatedText, currentQuestionIndex, progressTotal); err != nil {
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

	if s.agentBridge == nil {
		return fmt.Errorf("agent-service bridge is required for message processing")
	}

	now := time.Now().UTC()
	event := kafka.MessageFullEvent{
		EventID:   uuid.New().String(),
		EventType: "message.full",
		Timestamp: now.Format(time.RFC3339),
		Service:   "dialogue-aggregator",
		Version:   "1.0.0",
		Payload: map[string]interface{}{
			"chat_id":     chatID,
			"message_id":  messageID,
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
			"correlation_id": messageID,
		},
	}

	if err := s.agentBridge.SendMessageFull(ctx, event, chatID); err != nil {
		s.logger.Error("Failed to send message.full to agent-service",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
		return fmt.Errorf("send message.full to agent-service: %w", err)
	}

	// Ждём ответ от агента
	waitStart := time.Now()
	phraseEvent, err := s.agentBridge.ReceiveOnePhrase(ctx, chatID, messageID, 120*time.Second)
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
			_ = s.producer.SendModelQuestion(sessionID, userID, fallbackMsg, 0, 0)
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

	// Агент уже продвинул current_question_index на следующий вопрос в generate_response,
	// поэтому здесь просто сохраняем индекс как есть (без дополнительного инкремента).
	nextQuestionIndex := currentQuestionIndex

	// For progress indicator in UI: read total_questions from Redis state
	stateTotalQuestions := 0

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
			if tq, ok := existingState["total_questions"].(float64); ok && tq > 0 {
				stateTotalQuestions = int(tq)
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
		// Accumulate per-question evaluations keyed by question_id (latest wins).
		// Store on ANY decision that carries evaluation data — not just next_question/thank_you —
		// so that early termination can score the current in-progress question.
		evalData, _ := metadata["evaluation"].(map[string]interface{})
		if evalData != nil && currentQuestionID != "" {
			evalsMap, _ := stateUpdate["question_evaluations"].(map[string]any)
			if evalsMap == nil {
				evalsMap = map[string]any{}
			}
			evalsMap[currentQuestionID] = map[string]any{
				"question_id":    currentQuestionID,
				"question_index": currentQuestionIndex,
				"overall_score":  evalData["overall_score"],
				"decision":       agentDecision,
			}
			stateUpdate["question_evaluations"] = evalsMap
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

		// Определяем количество пройденных вопросов из Redis
		totalQuestions := 5
		answeredQuestions := totalQuestions // при нормальном завершении все вопросы пройдены
		if s.redisCl != nil {
			if rState, rErr := s.redisCl.GetState(ctx, chatID); rErr == nil && rState != nil {
				if tq, ok := rState["total_questions"].(float64); ok && tq > 0 {
					totalQuestions = int(tq)
					answeredQuestions = totalQuestions
				}
				if evalsMap, ok := rState["question_evaluations"].(map[string]any); ok && len(evalsMap) > 0 {
					answeredQuestions = len(evalsMap)
				}
			}
		}

		feedback := fmt.Sprintf(
			"Интервью завершено. Отвечено %d из %d вопросов. Оценка формируется аналитиком.",
			answeredQuestions, totalQuestions,
		)

		// Сохраняем placeholder-результат (score=0, аналитик обновит)
		if err := s.resultsCRUDCl.SaveResult(ctx, sessionUUID, 0, feedback, false); err != nil {
			s.logger.Error("Failed to save placeholder result",
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

		// Публикуем session.completed для analyst-agent-service
		kind, topics, level := s.sessionMgr.GetMeta(sessionID)
		if kind == "" {
			kind = "interview"
		}
		if err := s.producer.SendSessionCompleted(sessionID, kind, userID, chatID, topics, level, false, answeredQuestions, totalQuestions, kafka.TraceIDFromContext(ctx)); err != nil {
			s.logger.Error("Failed to publish session.completed",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}

		// Отправляем chat.completed в BFF для обновления UI
		if err := s.producer.SendChatCompleted(sessionID, userID, 0, feedback, []string{}); err != nil {
			return fmt.Errorf("send chat completed: %w", err)
		}

		s.sessionMgr.Delete(sessionID)
		s.logger.Info("Chat completed, session.completed published for analyst",
			zap.String("session_id", sessionID),
			zap.Int("answered_questions", answeredQuestions),
			zap.Int("total_questions", totalQuestions),
		)
		return nil
	}

	// Отправляем сгенерированный вопрос/ответ в chat.events.in
	piiMasked, _ := phraseEvent.Payload["pii_masked_content"].(string)

	// В режиме изучения при переходе к следующему вопросу:
	// 1) Отправляем комментарий LLM к ответу (plain)
	// 2a) Если следующий вопрос — новый пункт → отправляем теорию+вопрос (study-wrapped)
	// 2b) Если следующий вопрос — тот же пункт → отправляем только вопрос (без повтора теории)
	// nextQuestionIndex — 1-indexed order от агента → конвертируем в 0-indexed.
	if messageKind == "question" {
		kind, _, _ := s.sessionMgr.GetMeta(sessionID)
		if kind == "study" {
			arrayIdx := nextQuestionIndex - 1
			if arrayIdx < 0 {
				arrayIdx = 0
			}
			if nextQ, ok := s.sessionMgr.GetQuestionByIndex(sessionID, arrayIdx); ok {
				// Compare point_id of the previous question (arrayIdx-1) vs next question.
				samePoint := false
				if nextQ.PointID != "" && arrayIdx > 0 {
					if prevQ, ok := s.sessionMgr.GetQuestionByIndex(sessionID, arrayIdx-1); ok {
						samePoint = prevQ.PointID == nextQ.PointID
					}
				}

				// Вычисляем номер пункта (1-based) вместо raw question order.
				prog := s.sessionMgr.GetProgram(sessionID)
				var progQuestions []models.QuestionItem
				if prog != nil {
					progQuestions = prog.Questions
				}
				pointNum := studyPointNumber(progQuestions, arrayIdx)

				if samePoint {
					// Same point:
					// 1) Отправляем оценку агента (plain text)
					// 2) Отправляем следующий вопрос из программы (study-wrapped, без теории)
					if err := s.producer.SendModelQuestion(sessionID, userID, generatedText, 0, stateTotalQuestions, piiMasked); err != nil {
						return fmt.Errorf("send study evaluation: %w", err)
					}
					wrappedQ := "[STUDY_QUESTION]\n" + strings.TrimSpace(nextQ.Question) + "\n[/STUDY_QUESTION]"
					if err := s.producer.SendModelQuestion(sessionID, userID, wrappedQ, pointNum, stateTotalQuestions); err != nil {
						return fmt.Errorf("send study next question (same point): %w", err)
					}
					if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, wrappedQ, map[string]any{
						"message_id":   phraseEvent.Payload["message_id"],
						"question_id":  nextQ.ID,
						"message_kind": "study_question",
					}); err != nil {
						s.logger.Warn("Failed to save study question to chat-crud", zap.String("session_id", sessionID), zap.Error(err))
					}
					s.logger.Info("Study: sent evaluation + next question (same point, no theory)",
						zap.String("session_id", sessionID),
						zap.Int("array_idx", arrayIdx),
						zap.String("point_id", nextQ.PointID),
						zap.Int("point_num", pointNum),
					)
					return nil
				}

				// New point — evaluation, then transition, then theory+question block.
				theoryForWrap := nextQ.PointTheory
				if theoryForWrap == "" {
					theoryForWrap = nextQ.Theory
				}
				studyContent := studyWrapQuestion(theoryForWrap, nextQ.Question)
				// 1) Оценка агента
				if err := s.producer.SendModelQuestion(sessionID, userID, generatedText, 0, stateTotalQuestions, piiMasked); err != nil {
					return fmt.Errorf("send study evaluation (new point): %w", err)
				}
				// 2) Theory+question block (transition не нужен — теория сама разделяет пункты)
				if err := s.producer.SendModelQuestion(sessionID, userID, studyContent, pointNum, stateTotalQuestions); err != nil {
					return fmt.Errorf("send study theory+question: %w", err)
				}
				if err := s.chatCRUDCl.SaveMessage(ctx, sessionUUID, client.MessageTypeSystem, studyContent, map[string]any{
					"message_id":   phraseEvent.Payload["message_id"],
					"question_id":  nextQ.ID,
					"message_kind": "study_theory",
				}); err != nil {
					s.logger.Warn("Failed to save study theory to chat-crud", zap.String("session_id", sessionID), zap.Error(err))
				}
				s.logger.Info("Study: sent evaluation + theory+question (new point)",
					zap.String("session_id", sessionID),
					zap.Int("array_idx", arrayIdx),
					zap.String("point_id", nextQ.PointID),
				)
				return nil
			}
		}
	}

	if err := s.producer.SendModelQuestion(sessionID, userID, generatedText, nextQuestionIndex, stateTotalQuestions, piiMasked); err != nil {
		return fmt.Errorf("send model question: %w", err)
	}

	s.logger.Info("Agent response sent",
		zap.String("session_id", sessionID),
		zap.String("decision", agentDecision),
	)

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

	// Получаем программу интервью + метаданные сессии
	program, meta, err := s.sessionManagerCl.GetInterviewProgramWithMeta(ctx, sessionUUID)
	if err != nil {
		return fmt.Errorf("get interview program: %w", err)
	}
	if meta != nil {
		kind := meta.SessionKind
		if kind == "" {
			kind = "interview"
		}
		s.sessionMgr.GetOrCreate(sessionID, userID)
		s.sessionMgr.SetMeta(sessionID, kind, meta.Topics, meta.Level)
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

		state := map[string]any{}
		if existingState, err := s.redisCl.GetState(ctx, chatID); err == nil && existingState != nil {
			for k, v := range existingState {
				state[k] = v
			}
		}
		state["last_activity"] = time.Now().UTC().Format(time.RFC3339)
		state["awaiting_llm"] = userMessagesCount == systemMessagesCount && s.sessionMgr.HasMoreQuestions(sessionID)
		state["awaiting_user"] = userMessagesCount < systemMessagesCount
		if err := s.redisCl.SetState(ctx, chatID, state); err != nil {
			s.logger.Warn("Failed to restore dialogue state in Redis",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	// Подсчитываем количество системных сообщений (вопросов) и пользовательских сообщений.
	// Для study mode учитываем также study_theory (каждая теория идёт с вопросом).
	systemMessagesCount := 0
	userMessagesCount := 0
	for _, msg := range messages {
		if msg.Type == "system" {
			if msg.Metadata != nil {
				if kind, ok := msg.Metadata["message_kind"].(string); ok {
					if kind == "question" || kind == "study_theory" {
						systemMessagesCount++
					}
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
			// Short wait — agent silently drops system resume messages (main.py:115-122),
			// so a long wait just blocks the consumer. If agent responds, great; otherwise
			// user's next real message will drive the flow naturally.
			waitStart := time.Now()
			phraseEvent, err := s.agentBridge.ReceiveOnePhrase(ctx, chatID, msgID, 5*time.Second)
			if err != nil {
				s.logger.Warn("No phrase received after resume (expected for study mode)",
					zap.String("session_id", sessionID),
					zap.Duration("wait_duration", time.Since(waitStart)),
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
					_ = s.producer.SendModelQuestion(sessionID, userID, generatedText, 0, 0)
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

	// Определяем количество пройденных вопросов из Redis state
	answeredQuestions := 0
	totalQuestions := 5
	if s.redisCl != nil {
		if state, err := s.redisCl.GetState(ctx, chatID); err == nil && state != nil {
			if tq, ok := state["total_questions"].(float64); ok && tq > 0 {
				totalQuestions = int(tq)
			}
			// Считаем вопросы, на которые пользователь дал хотя бы 1 ответ
			if evalsMap, ok := state["question_evaluations"].(map[string]any); ok {
				answeredQuestions = len(evalsMap)
			}
		}
	}

	feedback := fmt.Sprintf(
		"Интервью было досрочно завершено. Отвечено %d из %d вопросов. Оценка формируется аналитиком.",
		answeredQuestions, totalQuestions,
	)

	// Сохраняем placeholder-результат с флагом terminated_early = true (score=0, аналитик обновит)
	if err := s.resultsCRUDCl.SaveResult(ctx, sessionUUID, 0, feedback, true); err != nil {
		s.logger.Error("Failed to save placeholder result to results-crud",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}

	// Закрываем сессию в session-manager
	if err := s.sessionManagerCl.CloseSession(ctx, sessionUUID); err != nil {
		s.logger.Warn("Failed to close session in session-manager",
			zap.String("session_id", sessionID),
			zap.Error(err),
		)
	}

	// Публикуем session.completed для analyst-agent-service.
	// If session was already completed naturally and deleted from memory,
	// skip sending a duplicate event (which would arrive with wrong kind).
	kind, topics, level := s.sessionMgr.GetMeta(sessionID)
	if kind == "" && topics == nil {
		s.logger.Info("Session already completed naturally, skipping duplicate session.completed",
			zap.String("session_id", sessionID),
		)
	} else {
		if kind == "" {
			kind = "interview"
		}
		if err := s.producer.SendSessionCompleted(sessionID, kind, userID, chatID, topics, level, true, answeredQuestions, totalQuestions, kafka.TraceIDFromContext(ctx)); err != nil {
			s.logger.Error("Failed to publish session.completed for early termination",
				zap.String("session_id", sessionID),
				zap.Error(err),
			)
		}
	}

	// Отправляем chat.completed в BFF для обновления UI
	if err := s.producer.SendChatCompleted(sessionID, userID, 0, feedback, []string{}); err != nil {
		return fmt.Errorf("send chat completed: %w", err)
	}

	// Удаляем сессию из локального менеджера
	s.sessionMgr.Delete(sessionID)

	s.logger.Info("Chat terminated, session.completed published for analyst",
		zap.String("session_id", sessionID),
		zap.Int("answered_questions", answeredQuestions),
		zap.Int("total_questions", totalQuestions),
	)

	return nil
}
