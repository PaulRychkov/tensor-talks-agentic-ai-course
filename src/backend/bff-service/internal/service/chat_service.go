package service

import (
	"context"
	"encoding/json"
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
	sessionClient   *client.SessionClient
	sessionCRUDCl   *client.SessionCRUDClient
	chatCRUDCl      *client.ChatCRUDClient
	resultsCRUDCl   *client.ResultsCRUDClient
	kafkaProducer   *kafka.Producer
	redisStepClient *client.RedisStepClient // читает реальный шаг агента из Redis
	logger          *zap.Logger
	// Хранилище активных сессий (только для активных чатов)
	sessions sync.Map // map[string]*Session
	// Текущий шаг обработки агентом (session_id → step label), fallback когда Redis недоступен
	processingSteps sync.Map // map[string]string
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
	Question          string
	QuestionID        string
	Timestamp         time.Time
	QuestionNumber    int
	TotalQuestions    int
	PIIMaskedContent  string // если не пусто — оригинальное сообщение пользователя было заменено этим текстом
}

// ChatResults представляет результаты завершенного чата.
type ChatResults struct {
	Score           int
	Feedback        string
	Recommendations []string
	CompletedAt     time.Time
	Pending         bool // true = чат завершён, отчёт ещё формируется аналитиком
}

// IsSessionCompleted проверяет, завершён ли чат (chat.completed получен), без проверки отчёта.
func (s *ChatService) IsSessionCompleted(sessionID string) bool {
	if state, ok := s.sessions.Load(sessionID); ok {
		session := state.(*Session)
		session.mu.RLock()
		defer session.mu.RUnlock()
		return session.Results != nil
	}
	return false
}

// NewChatService создаёт новый сервис для работы с чатами.
func NewChatService(
	sessionClient *client.SessionClient,
	sessionCRUDCl *client.SessionCRUDClient,
	chatCRUDCl *client.ChatCRUDClient,
	resultsCRUDCl *client.ResultsCRUDClient,
	kafkaProducer *kafka.Producer,
	redisStepClient *client.RedisStepClient,
	logger *zap.Logger,
) *ChatService {
	return &ChatService{
		sessionClient:   sessionClient,
		sessionCRUDCl:   sessionCRUDCl,
		chatCRUDCl:      chatCRUDCl,
		resultsCRUDCl:   resultsCRUDCl,
		kafkaProducer:   kafkaProducer,
		redisStepClient: redisStepClient,
		logger:          logger,
	}
}

// StartChat создаёт новую сессию с параметрами интервью и отправляет событие начала чата в Kafka.
func (s *ChatService) StartChat(ctx context.Context, userID uuid.UUID, params client.SessionParams) (uuid.UUID, error) {
	// session-crud-service (ghcr.io) сохраняет в JSONB поле "type", а не "mode".
	// Дублируем Mode → Type чтобы mode не терялся при сохранении в БД.
	if params.Type == "" && params.Mode != "" {
		params.Type = params.Mode
	}
	s.logger.Info("StartChat params",
		zap.String("mode", params.Mode),
		zap.Strings("topics", params.Topics),
		zap.Strings("subtopics", params.Subtopics),
	)
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

	// Устанавливаем начальный шаг обработки
	s.processingSteps.Store(sessionIDStr, "processing")

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

// SetProcessingStep сохраняет текущий шаг обработки агентом для сессии.
func (s *ChatService) SetProcessingStep(sessionID, step string) {
	s.processingSteps.Store(sessionID, step)
}

// GetProcessingStep возвращает текущий шаг обработки агентом для сессии.
// Сначала проверяет Redis (реальный шаг от interviewer-agent-service),
// затем — внутренний sync.Map как fallback.
func (s *ChatService) GetProcessingStep(sessionID string) string {
	if s.redisStepClient != nil {
		if step := s.redisStepClient.GetAgentStep(sessionID); step != "" {
			return step
		}
	}
	if v, ok := s.processingSteps.Load(sessionID); ok {
		return v.(string)
	}
	return ""
}

// HandleModelQuestion обрабатывает вопрос от модели (реализует kafka.EventHandler).
func (s *ChatService) HandleModelQuestion(ctx context.Context, sessionID, userID, question, questionID string, questionNumber, totalQuestions int, piiMaskedContent string) error {
	s.logger.Info("Received model question",
		zap.String("session_id", sessionID),
		zap.String("user_id", userID),
		zap.String("question_id", questionID),
	)

	// Сбрасываем шаг обработки — агент завершил работу
	s.processingSteps.Delete(sessionID)

	// Находим сессию и добавляем вопрос в очередь
	if state, ok := s.sessions.Load(sessionID); ok {
		session := state.(*Session)
		update := QuestionUpdate{
			Question:         question,
			QuestionID:       questionID,
			Timestamp:        time.Now(),
			QuestionNumber:   questionNumber,
			TotalQuestions:   totalQuestions,
			PIIMaskedContent: piiMaskedContent,
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

	// Сбрасываем шаг обработки — сессия завершена
	s.processingSteps.Delete(sessionID)

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
// Всегда проверяет results-crud как источник истины (там хранит оценку аналитик).
// Если results-crud пуст — падает обратно на in-memory placeholder из chat.completed.
func (s *ChatService) GetResults(sessionID string) (*ChatResults, bool) {
	sessionUUID, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, false
	}

	// Проверяем results-crud — там финальная оценка от аналитика.
	// Аналитик всегда сохраняет report_json; dialogue-aggregator сохраняет без него.
	ctx := context.Background()
	result, err := s.resultsCRUDCl.GetResult(ctx, sessionUUID)
	hasReport := err == nil && result != nil &&
		len(result.ReportJSON) > 0 && string(result.ReportJSON) != "null"
	if hasReport {
		feedback := result.Feedback
		// Extract preparation_plan from report_json as recommendations
		var reportData struct {
			Summary         string   `json:"summary"`
			PreparationPlan []string `json:"preparation_plan"`
		}
		recommendations := []string{}
		if jsonErr := json.Unmarshal(result.ReportJSON, &reportData); jsonErr == nil {
			if feedback == "" {
				feedback = reportData.Summary
			}
			if len(reportData.PreparationPlan) > 0 {
				recommendations = reportData.PreparationPlan
			}
		}
		if feedback == "" {
			feedback = "Оценка формируется аналитиком."
		}
		return &ChatResults{
			Score:           result.Score,
			Feedback:        feedback,
			Recommendations: recommendations,
			CompletedAt:     result.UpdatedAt,
		}, true
	}

	// Запись в results-crud есть, но report_json ещё пуст — аналитик ещё работает.
	// Используем наличие записи как сигнал завершения сессии (dialogue-aggregator создаёт
	// placeholder сразу после chat.completed), не полагаясь на in-memory IsSessionCompleted
	// (который теряется при перезапуске пода BFF).
	if err == nil && result != nil {
		return &ChatResults{Pending: true}, true
	}

	// Нет записи в DB — fallback на in-memory флаг (короткое окно до сохранения dialogue-aggregator).
	if s.IsSessionCompleted(sessionID) {
		return &ChatResults{Pending: true}, true
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
		// Считаем результат "готовым" если аналитик сохранил report_json (новый формат)
		// ИЛИ score > 0 (старые данные без report_json — тоже от аналитика).
		// Placeholder из dialogue-aggregator всегда имеет score=0 и report_json=null.
		hasAnalystReport := len(result.ReportJSON) > 0 && string(result.ReportJSON) != "null"
		resultReady := hasResult && (hasAnalystReport || result.Score > 0)
		// session-service не всегда сохраняет params.Mode (legacy: хранит type вместо mode).
		// Восстанавливаем mode из results-crud.session_kind, если он пустой.
		params := session.Params
		if params.Mode == "" && hasResult && result.SessionKind != "" {
			params.Mode = result.SessionKind
		}
		interview := InterviewInfo{
			SessionID:       session.SessionID,
			StartTime:       session.StartTime,
			EndTime:         session.EndTime,
			Params:          params,
			HasResults:      resultReady,
			TerminatedEarly: false,
		}
		// Заполняем данные результата только если результат готов
		if resultReady {
			scoreValue := result.Score
			interview.Score = &scoreValue
			interview.Feedback = result.Feedback
			if result.TerminatedEarly {
				interview.TerminatedEarly = true
			}
			s.logger.Info("Interview result data set",
				zap.String("session_id", session.SessionID.String()),
				zap.Int("score", scoreValue),
				zap.Bool("result_ready", resultReady),
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

// SubmitSessionRating сохраняет оценку пользователя (1-5) для завершённой сессии.
func (s *ChatService) SubmitSessionRating(ctx context.Context, sessionID uuid.UUID, rating int, comment string) error {
	return s.resultsCRUDCl.SubmitRating(ctx, sessionID, rating, comment)
}

// ── Dashboard methods (§10.2) ────────────────────────────────────────────────

// DashboardSummary aggregated summary for the dashboard page.
type DashboardSummary struct {
	TotalSessions           int      `json:"total_sessions"`
	CompletedSessions       int      `json:"completed_sessions"`
	AvgScore                float64  `json:"avg_score"`
	StreakDays               int      `json:"streak_days"`
	CurrentLevel            string   `json:"current_level"`
	TrainingUnlockedTopics  []string `json:"training_unlocked_topics"`
	LastSessionDate         *string  `json:"last_session_date"`
}

// ActivityEntry represents a single day of activity.
type ActivityEntry struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// TopicProgress represents per-topic learning progress.
type TopicProgress struct {
	Topic    string  `json:"topic"`
	Score    float64 `json:"score"`
	Status   string  `json:"status"`
	Sessions int     `json:"sessions"`
}

// DashboardRecommendation is a single recommendation from the last report.
type DashboardRecommendation struct {
	Topic    string `json:"topic"`
	Action   string `json:"action"`
	Priority string `json:"priority"`
}

// getUserSessions fetches sessions for a user (helper for dashboard methods).
func (s *ChatService) getUserSessions(ctx context.Context, userID string) ([]client.Session, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user_id: %w", err)
	}
	return s.sessionCRUDCl.GetSessionsByUserID(ctx, uid)
}

// GetDashboardSummary returns aggregated dashboard statistics for a user.
func (s *ChatService) GetDashboardSummary(ctx context.Context, userID string) (*DashboardSummary, error) {
	sessions, err := s.getUserSessions(ctx, userID)
	if err != nil {
		s.logger.Warn("Failed to fetch sessions for dashboard summary",
			zap.String("user_id", userID),
			zap.Error(err),
		)
		sessions = nil
	}

	summary := &DashboardSummary{
		TrainingUnlockedTopics: []string{},
	}

	if len(sessions) == 0 {
		return summary, nil
	}

	summary.TotalSessions = len(sessions)

	var sessionIDs []uuid.UUID
	for _, sess := range sessions {
		sessionIDs = append(sessionIDs, sess.SessionID)
	}

	results, err := s.resultsCRUDCl.GetResults(ctx, sessionIDs)
	if err != nil {
		s.logger.Warn("Failed to fetch results for dashboard summary",
			zap.String("user_id", userID),
			zap.Error(err),
		)
	}

	var totalScore float64
	var scoreCount int
	summary.CompletedSessions = len(results)

	for _, r := range results {
		if r.Score > 0 {
			totalScore += float64(r.Score)
			scoreCount++
		}
	}

	if scoreCount > 0 {
		summary.AvgScore = totalScore / float64(scoreCount) / 100.0
	}

	if len(sessions) > 0 {
		lastDate := sessions[0].CreatedAt.Format(time.RFC3339)
		summary.LastSessionDate = &lastDate
	}

	summary.StreakDays = computeStreak(sessions)
	return summary, nil
}

// computeStreak calculates consecutive days of activity ending today.
func computeStreak(sessions []client.Session) int {
	if len(sessions) == 0 {
		return 0
	}
	today := time.Now().Truncate(24 * time.Hour)
	streak := 0
	for _, sess := range sessions {
		sessionDay := sess.CreatedAt.Truncate(24 * time.Hour)
		expected := today.Add(-time.Duration(streak) * 24 * time.Hour)
		if sessionDay.Equal(expected) {
			streak++
		} else if sessionDay.Before(expected) {
			break
		}
	}
	return streak
}

// GetDashboardActivity returns calendar activity for a user within a date range.
func (s *ChatService) GetDashboardActivity(ctx context.Context, userID, from, to string) ([]ActivityEntry, error) {
	sessions, err := s.getUserSessions(ctx, userID)
	if err != nil {
		return []ActivityEntry{}, nil
	}

	counts := map[string]int{}
	for _, sess := range sessions {
		day := sess.CreatedAt.Format("2006-01-02")
		if from != "" && day < from {
			continue
		}
		if to != "" && day > to {
			continue
		}
		counts[day]++
	}

	var entries []ActivityEntry
	for date, count := range counts {
		entries = append(entries, ActivityEntry{Date: date, Count: count})
	}
	return entries, nil
}

// GetDashboardTopicProgress returns per-topic progress for the user.
// Only study sessions contribute — topic scores are read from report_json.topic_scores.
func (s *ChatService) GetDashboardTopicProgress(ctx context.Context, userID string) ([]TopicProgress, error) {
	sessions, err := s.getUserSessions(ctx, userID)
	if err != nil || len(sessions) == 0 {
		return []TopicProgress{}, nil
	}

	var sessionIDs []uuid.UUID
	for _, sess := range sessions {
		sessionIDs = append(sessionIDs, sess.SessionID)
	}

	results, err := s.resultsCRUDCl.GetResults(ctx, sessionIDs)
	if err != nil || len(results) == 0 {
		return []TopicProgress{}, nil
	}

	// Only study sessions feed topic progress; scores come from report_json.topic_scores.
	topicScores := map[string][]float64{}
	for _, r := range results {
		if r.SessionKind != "study" {
			continue
		}
		if len(r.ReportJSON) == 0 {
			continue
		}
		var report struct {
			TopicScores map[string]float64 `json:"topic_scores"`
		}
		if err := json.Unmarshal(r.ReportJSON, &report); err != nil || len(report.TopicScores) == 0 {
			continue
		}
		for topic, score := range report.TopicScores {
			topicScores[topic] = append(topicScores[topic], score/100.0)
		}
	}

	var progress []TopicProgress
	for topic, scores := range topicScores {
		var sum float64
		for _, sc := range scores {
			sum += sc
		}
		avg := sum / float64(len(scores))
		status := "in_progress"
		if avg >= 0.8 {
			status = "completed"
		} else if avg == 0 {
			status = "not_started"
		}
		progress = append(progress, TopicProgress{
			Topic:    topic,
			Score:    avg,
			Status:   status,
			Sessions: len(scores),
		})
	}
	return progress, nil
}

// GetDashboardRecommendations returns recommendations from the user's last completed report.
func (s *ChatService) GetDashboardRecommendations(ctx context.Context, userID string) ([]DashboardRecommendation, error) {
	sessions, err := s.getUserSessions(ctx, userID)
	if err != nil || len(sessions) == 0 {
		return []DashboardRecommendation{}, nil
	}

	for _, sess := range sessions {
		result, err := s.resultsCRUDCl.GetResult(ctx, sess.SessionID)
		if err != nil || result == nil || len(result.ReportJSON) == 0 {
			continue
		}
		var report struct {
			PreparationPlan []struct {
				Topic  string `json:"topic"`
				Action string `json:"action"`
				Prio   string `json:"priority"`
			} `json:"preparation_plan"`
		}
		if err := json.Unmarshal(result.ReportJSON, &report); err != nil {
			continue
		}
		var recs []DashboardRecommendation
		for _, p := range report.PreparationPlan {
			recs = append(recs, DashboardRecommendation{
				Topic:    p.Topic,
				Action:   p.Action,
				Priority: p.Prio,
			})
		}
		return recs, nil
	}
	return []DashboardRecommendation{}, nil
}
