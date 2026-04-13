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
	pendingSessions sync.Map // map[string]chan *models.InterviewProgram
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
	programChan := make(chan *models.InterviewProgram, 1)
	s.pendingSessions.Store(sessionID.String(), programChan)
	defer s.pendingSessions.Delete(sessionID.String())

	// Отправляем запрос в Kafka
	if err := s.kafkaProducer.SendInterviewBuildRequest(sessionID, params); err != nil {
		return nil, fmt.Errorf("send interview build request: %w", err)
	}

	s.logger.Info("Interview build request sent",
		zap.String("session_id", sessionID.String()),
		zap.Duration("timeout", s.programTimeout),
	)

	// Ждём программу интервью от builder-service через Kafka.
	// Это таймаут для HTTP ответа bff-service, а не для результатов интервью.
	// Результаты интервью приходят асинхронно через Kafka от mock-model-service.
	select {
	case program := <-programChan:
		if program == nil {
			return nil, fmt.Errorf("received nil program")
		}

		// Сохраняем программу в CRUD
		if err := s.crudClient.UpdateProgram(ctx, sessionID, program); err != nil {
			return nil, fmt.Errorf("update program in crud: %w", err)
		}

		// Сохраняем в Redis кэш
		cachedSession := &models.CachedSession{
			SessionID:        sessionID,
			UserID:           userID,
			Params:           params,
			InterviewProgram: *program,
			CachedAt:         time.Now(),
		}
		if err := s.redisCache.SetSession(ctx, cachedSession); err != nil {
			s.logger.Warn("Failed to cache session in Redis",
				zap.String("session_id", sessionID.String()),
				zap.Error(err),
			)
			// Не возвращаем ошибку, продолжаем
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

// GetInterviewProgram возвращает программу интервью для сессии (сначала из Redis, потом из CRUD).
func (s *SessionManagerService) GetInterviewProgram(ctx context.Context, sessionID uuid.UUID) (*models.InterviewProgram, error) {
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
		return &cachedSession.InterviewProgram, nil
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

	return crudResp.Session.InterviewProgram, nil
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

// HandleInterviewBuildResponse обрабатывает ответ от interview builder (реализует kafka.EventHandler).
func (s *SessionManagerService) HandleInterviewBuildResponse(ctx context.Context, sessionID string, programPayload map[string]interface{}) error {
	// Конвертируем payload в InterviewProgram
	// programPayload имеет структуру: {"questions": [...]}
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
		if o, ok := qMap["order"].(float64); ok {
			question.Order = int(o)
		}

		program.Questions = append(program.Questions, question)
	}

	// Находим канал для этой сессии
	if ch, ok := s.pendingSessions.Load(sessionID); ok {
		programChan := ch.(chan *models.InterviewProgram)
		select {
		case programChan <- &program:
			s.logger.Info("Interview program sent to waiting channel",
				zap.String("session_id", sessionID),
				zap.Int("questions_count", len(program.Questions)),
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
