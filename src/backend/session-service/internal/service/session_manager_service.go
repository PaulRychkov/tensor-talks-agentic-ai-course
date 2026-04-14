package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tensor-talks/session-service/internal/client"
	"github.com/tensor-talks/session-service/internal/kafka"
	"github.com/tensor-talks/session-service/internal/models"
	"github.com/tensor-talks/session-service/internal/redis"
	"go.uber.org/zap"
)

// SessionManagerService управляет жизненным циклом сессий.
type SessionManagerService struct {
	crudClient     *client.SessionCRUDClient
	redisCache     *redis.Cache
	kafkaProducer  *kafka.Producer
	logger         *zap.Logger
	maxActive      int
	programTimeout time.Duration // Таймаут ожидания программы интервью от builder-service (для HTTP ответа bff-service)

	// Каналы для ожидания ответов от interview builder
	pendingSessions sync.Map // map[string]chan *programResult
	mu              sync.RWMutex
}

// NewSessionManagerService создаёт новый сервис управления сессиями.
func NewSessionManagerService(
	crudClient *client.SessionCRUDClient,
	redisCache *redis.Cache,
	kafkaProducer *kafka.Producer,
	maxActive int,
	programTimeoutSeconds int,
	logger *zap.Logger,
) *SessionManagerService {
	return &SessionManagerService{
		crudClient:     crudClient,
		redisCache:     redisCache,
		kafkaProducer:  kafkaProducer,
		logger:         logger,
		maxActive:      maxActive,
		programTimeout: time.Duration(programTimeoutSeconds) * time.Second,
	}
}

// CreateSession создаёт новую сессию с проверкой лимита и ожиданием программы.
func (s *SessionManagerService) CreateSession(ctx context.Context, userID uuid.UUID, params models.SessionParams) (*SessionResponse, error) {
	// Проверяем, есть ли уже активная сессия у пользователя
	activeSession, err := s.crudClient.GetActiveSessionByUserID(ctx, userID)
	if err != nil && err.Error() != "active session not found" && err.Error() != "session crud service error: active session not found" {
		s.logger.Warn("Failed to check for active session, allowing session creation", zap.Error(err))
		// Продолжаем, если не можем проверить (кроме случая когда сессии нет)
	} else if err == nil && activeSession != nil {
		s.logger.Warn("User already has an active session",
			zap.String("user_id", userID.String()),
			zap.String("active_session_id", activeSession.Session.SessionID.String()),
		)
		return nil, fmt.Errorf("user already has an active session: %s", activeSession.Session.SessionID.String())
	}

	// Проверяем лимит активных сессий
	activeCount, err := s.redisCache.GetActiveSessionsCount(ctx)
	if err != nil {
		s.logger.Warn("Failed to get active sessions count, allowing session creation", zap.Error(err))
		// Продолжаем, если не можем получить количество
	} else if activeCount >= s.maxActive {
		s.logger.Warn("Max active sessions reached",
			zap.Int("active", activeCount),
			zap.Int("max", s.maxActive),
		)
		return nil, fmt.Errorf("max active sessions reached: %d", s.maxActive)
	}

	// Создаём сессию в CRUD
	s.logger.Info("Creating session in CRUD",
		zap.String("user_id", userID.String()),
	)
	crudResp, err := s.crudClient.CreateSession(ctx, userID, params)
	if err != nil {
		s.logger.Error("Failed to create session in CRUD",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		return nil, fmt.Errorf("create session in crud: %w", err)
	}
	s.logger.Info("Session created in CRUD",
		zap.String("session_id", crudResp.Session.SessionID.String()),
		zap.String("user_id", userID.String()),
	)

	sessionID := crudResp.Session.SessionID

	// Создаём канал для ожидания программы
	programChan := make(chan *programResult, 1)
	s.pendingSessions.Store(sessionID.String(), programChan)
	defer s.pendingSessions.Delete(sessionID.String())

	// Отправляем запрос в Kafka
	if err := s.kafkaProducer.SendInterviewBuildRequest(sessionID, userID, params); err != nil {
		return nil, fmt.Errorf("send interview build request: %w", err)
	}

	s.logger.Info("Interview build request sent",
		zap.String("session_id", sessionID.String()),
		zap.Duration("timeout", s.programTimeout),
	)

	// Ждём программу интервью от builder-service через Kafka.
	// Это таймаут для HTTP ответа bff-service, а не для результатов интервью.
	// Результаты интервью приходят асинхронно через Kafka от dialogue-aggregator.
	select {
	case res := <-programChan:
		if res == nil || res.Program == nil {
			return nil, fmt.Errorf("received nil program")
		}

		// Сохраняем программу в CRUD
		if err := s.crudClient.UpdateProgram(ctx, sessionID, res.Program); err != nil {
			return nil, fmt.Errorf("update program in crud: %w", err)
		}

		// Сохраняем в Redis кэш
		cachedSession := &models.CachedSession{
			SessionID:        sessionID,
			UserID:           userID,
			Params:           params,
			InterviewProgram: *res.Program,
			ProgramStatus:    res.Status,
			ProgramMeta:      res.Meta,
			ProgramVersion:   "1",
			CachedAt:         time.Now(),
		}
		if err := s.redisCache.SetSession(ctx, cachedSession); err != nil {
			s.logger.Warn("Failed to cache session in Redis",
				zap.String("session_id", sessionID.String()),
				zap.Error(err),
			)
		}

		return &SessionResponse{
			SessionID: sessionID,
			Ready:     true,
		}, nil

	case <-time.After(s.programTimeout):
		return nil, fmt.Errorf("timeout waiting for interview program from builder-service (timeout: %v)", s.programTimeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ProgramResponse bundles program with its meta/status for the handler.
type ProgramResponse struct {
	Program     *models.InterviewProgram
	Status      string
	Meta        *models.ProgramMeta
	SessionMode string
	Topics      []string
	Level       string
}

// GetInterviewProgram возвращает программу интервью для сессии (сначала из Redis, потом из CRUD).
func (s *SessionManagerService) GetInterviewProgram(ctx context.Context, sessionID uuid.UUID) (*ProgramResponse, error) {
	// Пробуем получить из Redis
	cachedSession, err := s.redisCache.GetSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("Failed to get session from Redis",
			zap.String("session_id", sessionID.String()),
			zap.Error(err),
		)
	}

	if cachedSession != nil && cachedSession.InterviewProgram.Questions != nil {
		s.logger.Info("Interview program found in Redis cache",
			zap.String("session_id", sessionID.String()),
		)
		status := cachedSession.ProgramStatus
		if status == "" {
			status = "ready"
		}
		sessionMode := cachedSession.Params.Mode
		if sessionMode == "" {
			sessionMode = cachedSession.Params.Type // ghcr.io session-crud stores mode as "type"
		}
		return &ProgramResponse{
			Program:     &cachedSession.InterviewProgram,
			Status:      status,
			Meta:        cachedSession.ProgramMeta,
			SessionMode: sessionMode,
			Topics:      cachedSession.Params.Topics,
			Level:       cachedSession.Params.Level,
		}, nil
	}

	// Если не в кэше, получаем из CRUD
	s.logger.Info("Interview program not in cache, fetching from CRUD",
		zap.String("session_id", sessionID.String()),
	)

	crudResp, err := s.crudClient.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session from crud: %w", err)
	}

	if crudResp.Session.InterviewProgram == nil {
		return nil, fmt.Errorf("interview program not found")
	}

	// Кэшируем для будущих запросов
	if cachedSession == nil {
		cachedSession = &models.CachedSession{
			SessionID: sessionID,
			UserID:    crudResp.Session.UserID,
		}
	}
	cachedSession.InterviewProgram = *crudResp.Session.InterviewProgram
	cachedSession.CachedAt = time.Now()

	// Пробуем получить params из CRUD response
	paramsBytes, _ := json.Marshal(crudResp.Session.Params)
	json.Unmarshal(paramsBytes, &cachedSession.Params)

	if err := s.redisCache.SetSession(ctx, cachedSession); err != nil {
		s.logger.Warn("Failed to cache session in Redis",
			zap.String("session_id", sessionID.String()),
			zap.Error(err),
		)
	}

	sessionMode := crudResp.Session.Params.Mode
	if sessionMode == "" {
		sessionMode = crudResp.Session.Params.Type // ghcr.io session-crud stores mode as "type"
	}
	return &ProgramResponse{
		Program:     crudResp.Session.InterviewProgram,
		Status:      "ready",
		Meta:        nil,
		SessionMode: sessionMode,
		Topics:      cachedSession.Params.Topics,
		Level:       cachedSession.Params.Level,
	}, nil
}

// CloseSession закрывает сессию.
func (s *SessionManagerService) CloseSession(ctx context.Context, sessionID uuid.UUID) error {
	// Удаляем из Redis
	if err := s.redisCache.DeleteSession(ctx, sessionID); err != nil {
		s.logger.Warn("Failed to delete session from Redis",
			zap.String("session_id", sessionID.String()),
			zap.Error(err),
		)
		// Не возвращаем ошибку, продолжаем
	}

	// Обновляем в CRUD
	if err := s.crudClient.CloseSession(ctx, sessionID); err != nil {
		return fmt.Errorf("close session in crud: %w", err)
	}

	s.logger.Info("Session closed",
		zap.String("session_id", sessionID.String()),
	)

	return nil
}

// programResult bundles the parsed program with its meta for the pending channel.
type programResult struct {
	Program *models.InterviewProgram
	Meta    *models.ProgramMeta
	Status  string // "ready" or "failed"
}

// HandleInterviewBuildResponse обрабатывает ответ от interview builder (реализует kafka.EventHandler).
func (s *SessionManagerService) HandleInterviewBuildResponse(ctx context.Context, sessionID string, programPayload map[string]interface{}, programMetaPayload map[string]interface{}) error {
	// Parse program_meta first to determine status
	meta := models.ProgramMetaFromMap(programMetaPayload)
	status := "ready"
	if meta != nil && !meta.ValidationPassed {
		status = "failed"
		s.logger.Warn("Interview builder reported validation failure",
			zap.String("session_id", sessionID),
			zap.Stringp("fallback_reason", meta.FallbackReason),
		)
	}

	// Конвертируем payload в InterviewProgram
	questionsRaw, ok := programPayload["questions"].([]interface{})
	if !ok {
		return fmt.Errorf("invalid questions format in program payload")
	}

	program := models.InterviewProgram{
		Questions: make([]models.QuestionItem, 0, len(questionsRaw)),
	}

	for _, qRaw := range questionsRaw {
		qMap, ok := qRaw.(map[string]interface{})
		if !ok {
			continue
		}

		question := models.QuestionItem{}
		if id, ok := qMap["id"].(string); ok {
			question.ID = id
		}
		if q, ok := qMap["question"].(string); ok {
			question.Question = q
		}
		if t, ok := qMap["theory"].(string); ok {
			question.Theory = t
		}
		if t, ok := qMap["topic"].(string); ok {
			question.Topic = t
		}
		if o, ok := qMap["order"].(float64); ok {
			question.Order = int(o)
		}
		// Study-mode hierarchy fields
		if st, ok := qMap["subtopic"].(string); ok {
			question.Subtopic = st
		}
		if pid, ok := qMap["point_id"].(string); ok {
			question.PointID = pid
		}
		if pt, ok := qMap["point_title"].(string); ok {
			question.PointTitle = pt
		}
		if pth, ok := qMap["point_theory"].(string); ok {
			question.PointTheory = pth
		}
		if qip, ok := qMap["question_in_point"].(float64); ok {
			question.QuestionInPoint = int(qip)
		}

		program.Questions = append(program.Questions, question)
	}

	// Находим канал для этой сессии
	if ch, ok := s.pendingSessions.Load(sessionID); ok {
		resChan := ch.(chan *programResult)
		res := &programResult{
			Program: &program,
			Meta:    meta,
			Status:  status,
		}
		select {
		case resChan <- res:
			s.logger.Info("Interview program sent to waiting channel",
				zap.String("session_id", sessionID),
				zap.Int("questions_count", len(program.Questions)),
				zap.String("program_status", status),
			)
		default:
			s.logger.Warn("Program channel full or closed",
				zap.String("session_id", sessionID),
			)
		}
	} else {
		s.logger.Warn("No pending session found for program response",
			zap.String("session_id", sessionID),
		)
	}

	return nil
}

// SessionResponse представляет ответ при создании сессии.
type SessionResponse struct {
	SessionID uuid.UUID `json:"session_id"`
	Ready     bool      `json:"ready"`
}
