package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tensor-talks/bff-service/internal/client"
	"github.com/tensor-talks/bff-service/internal/kafka"
	"go.uber.org/zap"
)

// ChatService управляет чатами и сессиями.
type ChatService struct {
	sessionClient *client.SessionClient
	sessionCRUDCl *client.SessionCRUDClient
	chatCRUDCl    *client.ChatCRUDClient
	resultsCRUDCl *client.ResultsCRUDClient
	kafkaProducer *kafka.Producer
	logger        *zap.Logger
	// Хранилище активных сессий (только для активных чатов)
	sessions sync.Map // map[string]*Session
}

// Session представляет активную сессию чата.
type Session struct {
	SessionID string
	UserID    string
	// Очередь новых вопросов от модели
	Questions chan QuestionUpdate
	// Результаты чата (если завершён)
	Results *ChatResults
	mu      sync.RWMutex
}

// QuestionUpdate представляет обновление с вопросом от модели.
type QuestionUpdate struct {
	Question   string
	QuestionID string
	Timestamp  time.Time
}

// ChatResults представляет результаты завершенного чата.
type ChatResults struct {
	Score           int
	Feedback        string
	Recommendations []string
	CompletedAt     time.Time
}

// NewChatService создаёт новый сервис для работы с чатами.
func NewChatService(
	sessionClient *client.SessionClient,
	sessionCRUDCl *client.SessionCRUDClient,
	chatCRUDCl *client.ChatCRUDClient,
	resultsCRUDCl *client.ResultsCRUDClient,
	kafkaProducer *kafka.Producer,
	logger *zap.Logger,
) *ChatService {
	return &ChatService{
		sessionClient: sessionClient,
		sessionCRUDCl: sessionCRUDCl,
		chatCRUDCl:    chatCRUDCl,
		resultsCRUDCl: resultsCRUDCl,
		kafkaProducer: kafkaProducer,
		logger:        logger,
	}
}

// StartChat создаёт новую сессию с параметрами интервью и отправляет событие начала чата в Kafka.
func (s *ChatService) StartChat(ctx context.Context, userID uuid.UUID, params client.SessionParams) (uuid.UUID, error) {
	// Создаём сессию через session-manager-service с параметрами
	sessionResp, err := s.sessionClient.CreateSession(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to create session",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return uuid.Nil, fmt.Errorf("create session: %w", err)
	}

	sessionID := sessionResp.SessionID
	sessionIDStr := sessionID.String()
	userIDStr := userID.String()

	// Сохраняем сессию в локальное хранилище (для активных чатов)
	session := &Session{
		SessionID: sessionIDStr,
		UserID:    userIDStr,
		Questions: make(chan QuestionUpdate, 10), // Буферизованный канал для вопросов
		Results:   nil,
	}
	s.sessions.Store(sessionIDStr, session)

	// Отправляем событие начала чата в Kafka
	requestID := uuid.New().String()
	if err := s.kafkaProducer.SendChatStarted(sessionIDStr, userIDStr, requestID); err != nil {
		s.logger.Error("Failed to send chat started event",
			zap.String("session_id", sessionIDStr),
			zap.String("user_id", userIDStr),
			zap.Error(err),
		)
		// Не возвращаем ошибку, так как сессия уже создана
	}

	s.logger.Info("Chat started",
		zap.String("session_id", sessionIDStr),
		zap.String("user_id", userIDStr),
	)

	return sessionID, nil
}

// SendMessage отправляет сообщение пользователя в Kafka.
func (s *ChatService) SendMessage(ctx context.Context, sessionID uuid.UUID, userID uuid.UUID, content string) error {
	sessionIDStr := sessionID.String()
	// Проверяем, что сессия существует (для активных чатов)
	if _, ok := s.sessions.Load(sessionIDStr); !ok {
		// Для завершенных чатов это нормально, продолжаем
		s.logger.Info("Session not in active sessions, continuing",
			zap.String("session_id", sessionIDStr),
		)
	}

	messageID := uuid.New().String()
	requestID := uuid.New().String()

	if err := s.kafkaProducer.SendUserMessage(sessionIDStr, userID.String(), content, messageID, requestID); err != nil {
		s.logger.Error("Failed to send user message",
			zap.String("session_id", sessionIDStr),
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("send message: %w", err)
	}

	s.logger.Info("User message sent",
		zap.String("session_id", sessionIDStr),
		zap.String("user_id", userID.String()),
		zap.String("message_id", messageID),
	)

	return nil
}

// HandleModelQuestion обрабатывает вопрос от модели (реализует kafka.EventHandler).
func (s *ChatService) HandleModelQuestion(ctx context.Context, sessionID, userID, question, questionID string) error {
	s.logger.Info("Received model question",
		zap.String("session_id", sessionID),
		zap.String("user_id", userID),
		zap.String("question_id", questionID),
	)

	// Находим сессию и добавляем вопрос в очередь
	if state, ok := s.sessions.Load(sessionID); ok {
		session := state.(*Session)
		update := QuestionUpdate{
			Question:   question,
			QuestionID: questionID,
			Timestamp:  time.Now(),
		}

		// Неблокирующая отправка в канал
		select {
		case session.Questions <- update:
			s.logger.Info("Question added to session queue",
				zap.String("session_id", sessionID),
			)
		default:
			s.logger.Warn("Session questions channel full, dropping question",
				zap.String("session_id", sessionID),
			)
		}
	} else {
		s.logger.Warn("Session not found for question",
			zap.String("session_id", sessionID),
		)
	}

	return nil
}

// HandleChatCompleted обрабатывает завершение чата (реализует kafka.EventHandler).
func (s *ChatService) HandleChatCompleted(ctx context.Context, sessionID, userID string, results kafka.ChatResults) error {
	s.logger.Info("Chat completed",
		zap.String("session_id", sessionID),
		zap.String("user_id", userID),
		zap.Int("score", results.Score),
	)

	// Сохраняем результаты в сессии
	if state, ok := s.sessions.Load(sessionID); ok {
		session := state.(*Session)
		session.mu.Lock()
		session.Results = &ChatResults{
			Score:           results.Score,
			Feedback:        results.Feedback,
			Recommendations: results.Recommendations,
			CompletedAt:     time.Now(),
		}
		session.mu.Unlock()

		// Закрываем канал вопросов
		close(session.Questions)
	} else {
		s.logger.Warn("Session not found for completion",
			zap.String("session_id", sessionID),
		)
	}

	return nil
}

// TerminateChat отправляет событие досрочного завершения чата пользователем.
func (s *ChatService) TerminateChat(ctx context.Context, sessionID uuid.UUID, userID uuid.UUID) error {
	sessionIDStr := sessionID.String()
	requestID := uuid.New().String()

	if err := s.kafkaProducer.SendChatTerminated(sessionIDStr, userID.String(), requestID); err != nil {
		s.logger.Error("Failed to send chat terminated event",
			zap.String("session_id", sessionIDStr),
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("send chat terminated: %w", err)
	}

	s.logger.Info("Chat terminated event sent",
		zap.String("session_id", sessionIDStr),
		zap.String("user_id", userID.String()),
	)

	return nil
}

// ResumeChat отправляет событие восстановления активной сессии чата в Kafka.
func (s *ChatService) ResumeChat(ctx context.Context, sessionID uuid.UUID, userID uuid.UUID) error {
	sessionIDStr := sessionID.String()

	// Проверяем, что сессия активна (нет end_time)
	// Получаем сессию из session-crud для проверки активности
	session, err := s.sessionCRUDCl.GetSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("Failed to get session for resume check",
			zap.String("session_id", sessionIDStr),
			zap.Error(err),
		)
		return fmt.Errorf("get session: %w", err)
	}

	// Если сессия уже завершена, не отправляем событие восстановления
	if session.EndTime != nil {
		s.logger.Info("Session already completed, skipping resume",
			zap.String("session_id", sessionIDStr),
		)
		return fmt.Errorf("session already completed")
	}

	// Создаём или получаем сессию в локальном хранилище
	if _, ok := s.sessions.Load(sessionIDStr); !ok {
		sessionState := &Session{
			SessionID: sessionIDStr,
			UserID:    userID.String(),
			Questions: make(chan QuestionUpdate, 10),
			Results:   nil,
		}
		s.sessions.Store(sessionIDStr, sessionState)
	}

	requestID := uuid.New().String()
	if err := s.kafkaProducer.SendChatResumed(sessionIDStr, userID.String(), requestID); err != nil {
		s.logger.Error("Failed to send chat resumed event",
			zap.String("session_id", sessionIDStr),
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return fmt.Errorf("send chat resumed: %w", err)
	}

	s.logger.Info("Chat resumed event sent",
		zap.String("session_id", sessionIDStr),
		zap.String("user_id", userID.String()),
	)

	return nil
}

// GetNextQuestion получает следующий вопрос для сессии (для polling).
func (s *ChatService) GetNextQuestion(sessionID string) (*QuestionUpdate, bool) {
	if state, ok := s.sessions.Load(sessionID); ok {
		session := state.(*Session)
		select {
		case question := <-session.Questions:
			return &question, true
		default:
			return nil, false
		}
	}
	return nil, false
}

// GetResults получает результаты чата (если завершён).
func (s *ChatService) GetResults(sessionID string) (*ChatResults, bool) {
	if state, ok := s.sessions.Load(sessionID); ok {
		session := state.(*Session)
		session.mu.RLock()
		defer session.mu.RUnlock()
		if session.Results != nil {
			return session.Results, true
		}
	}
	return nil, false
}

// GetInterviews получает список всех интервью пользователя (из session-crud и results-crud).
func (s *ChatService) GetInterviews(ctx context.Context, userID uuid.UUID) ([]InterviewInfo, error) {
	// Получаем все сессии пользователя
	sessions, err := s.sessionCRUDCl.GetSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get sessions: %w", err)
	}

	// Получаем session IDs для запроса результатов
	sessionIDs := make([]uuid.UUID, len(sessions))
	for i, session := range sessions {
		sessionIDs[i] = session.SessionID
	}

	// Получаем результаты для всех сессий
	results, err := s.resultsCRUDCl.GetResults(ctx, sessionIDs)
	if err != nil {
		s.logger.Warn("Failed to get results, continuing without them",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		results = []client.Result{} // Продолжаем без результатов
	} else {
		s.logger.Info("GetResults returned results",
			zap.String("user_id", userID.String()),
			zap.Int("sessions_count", len(sessions)),
			zap.Int("results_count", len(results)),
			zap.Strings("session_ids", func() []string {
				ids := make([]string, len(sessionIDs))
				for i, id := range sessionIDs {
					ids[i] = id.String()
				}
				return ids
			}()),
		)
	}

	// Создаём map для быстрого поиска результатов по session_id
	resultsMap := make(map[uuid.UUID]client.Result)
	for _, result := range results {
		resultsMap[result.SessionID] = result
		s.logger.Info("Result mapped",
			zap.String("session_id", result.SessionID.String()),
			zap.Int("score", result.Score),
		)
	}

	// Формируем ответ
	interviews := make([]InterviewInfo, len(sessions))
	for i, session := range sessions {
		result, hasResult := resultsMap[session.SessionID]
		interview := InterviewInfo{
			SessionID:       session.SessionID,
			StartTime:       session.StartTime,
			EndTime:         session.EndTime,
			Params:          session.Params,
			HasResults:      hasResult,
			TerminatedEarly: false,
		}
		// Заполняем данные результата только если результат существует
		if hasResult {
			scoreValue := result.Score
			interview.Score = &scoreValue
			interview.Feedback = result.Feedback
			if result.TerminatedEarly {
				interview.TerminatedEarly = true
			}
			s.logger.Info("Interview result data set",
				zap.String("session_id", session.SessionID.String()),
				zap.Int("score", scoreValue),
				zap.Bool("has_result", hasResult),
				zap.Bool("score_is_nil", interview.Score == nil),
			)
		} else {
			s.logger.Info("Interview has no result",
				zap.String("session_id", session.SessionID.String()),
			)
		}
		interviews[i] = interview
	}

	return interviews, nil
}

// InterviewInfo представляет информацию об интервью для списка.
type InterviewInfo struct {
	SessionID       uuid.UUID            `json:"session_id"`
	StartTime       time.Time            `json:"start_time"`
	EndTime         *time.Time           `json:"end_time,omitempty"`
	Params          client.SessionParams `json:"params"`
	HasResults      bool                 `json:"has_results"`
	Score           *int                 `json:"score"` // Убран omitempty чтобы поле всегда было в JSON (может быть null)
	Feedback        string               `json:"feedback,omitempty"`
	TerminatedEarly bool                 `json:"terminated_early,omitempty"`
}

// GetChatHistory получает историю чата по session_id (из chat-crud).
// Использует статус сессии для определения, какой endpoint вызывать.
func (s *ChatService) GetChatHistory(ctx context.Context, sessionID uuid.UUID) ([]client.ChatMessage, error) {
	// Получаем сессию для проверки статуса
	session, err := s.sessionCRUDCl.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Если сессия активна (нет end_time), используем GetActiveChatJSON
	if session.EndTime == nil {
		messages, err := s.chatCRUDCl.GetActiveChatJSON(ctx, sessionID)
		if err != nil {
			// Если активный чат не найден (нет сообщений), возвращаем пустой массив
			errStr := err.Error()
			if errStr == "active chat not found" || errStr == "chat crud service error: active chat not found" {
				return []client.ChatMessage{}, nil
			}
			return nil, fmt.Errorf("get active chat json: %w", err)
		}
		return messages, nil
	}

	// Если сессия завершена (есть end_time), используем GetChatDump
	dump, err := s.chatCRUDCl.GetChatDump(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get chat dump: %w", err)
	}

	// Извлекаем messages из JSONB структуры
	chatData, ok := dump.Chat["messages"]
	if !ok {
		return nil, fmt.Errorf("invalid chat dump structure: messages not found")
	}

	chatJSON, ok := chatData.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid chat dump structure: messages is not an array")
	}

	messages := make([]client.ChatMessage, 0, len(chatJSON))
	for _, msgInterface := range chatJSON {
		msgMap, ok := msgInterface.(map[string]interface{})
		if !ok {
			continue
		}

		msgType, _ := msgMap["type"].(string)
		msgContent, _ := msgMap["content"].(string)

		// Парсим время - может быть string или уже time.Time
		var msgCreatedAt time.Time
		if createdAtStr, ok := msgMap["created_at"].(string); ok {
			var parseErr error
			msgCreatedAt, parseErr = time.Parse(time.RFC3339, createdAtStr)
			if parseErr != nil {
				msgCreatedAt, _ = time.Parse("2006-01-02T15:04:05Z07:00", createdAtStr)
			}
		}

		messages = append(messages, client.ChatMessage{
			Type:      msgType,
			Content:   msgContent,
			CreatedAt: msgCreatedAt,
		})
	}

	return messages, nil
}

// GetChatResult получает результат интервью по session_id (из results-crud).
func (s *ChatService) GetChatResult(ctx context.Context, sessionID uuid.UUID) (*client.Result, error) {
	result, err := s.resultsCRUDCl.GetResult(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get result: %w", err)
	}
	return result, nil
}
