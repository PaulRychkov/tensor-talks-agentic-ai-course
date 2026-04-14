package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/tensor-talks/auth-service/internal/config"
)

/*
Пакет client реализует HTTP-клиент для взаимодействия auth-service с user-crud-service.

Через этот клиент:
  - создаются пользователи;
  - запрашивается пользователь по логину;
  - запрашивается пользователь по внешнему GUID.

Клиент инкапсулирует:
  - построение URL и HTTP-запросов;
  - парсинг ответов и ошибок;
  - настройки таймаутов и транспорта.
*/

// User описывает "санитизированного" пользователя, возвращаемого user-crud-service.
type User struct {
	ID              uuid.UUID `json:"id"`
	Login           string    `json:"login"`
	PasswordHash    string    `json:"password_hash"`
	RecoveryKeyHash *string   `json:"recovery_key_hash,omitempty"` // §10.10
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// UserCrudClient предоставляет методы для обращения к HTTP API user-crud-service.
type UserCrudClient struct {
	baseURL *url.URL
	client  *http.Client
}

// APIError представляет ошибку, возвращённую user-crud-service.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("user store api error: status=%d message=%s", e.Status, e.Message)
}

// NewUserCrudClient создаёт новый клиент user-store на основе конфигурации.
// Настраивает базовый URL сервиса, таймауты HTTP-клиента и сетевые параметры
// (таймауты подключения, TLS-handshake и ожидания заголовков ответа).
func NewUserCrudClient(cfg config.UserCrudConfig) (*UserCrudClient, error) {
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse user store base URL: %w", err)
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &UserCrudClient{
		baseURL: parsed,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
				TLSHandshakeTimeout:   3 * time.Second,
				ResponseHeaderTimeout: timeout,
			},
		},
	}, nil
}

// CreateUser создаёт нового пользователя в user-crud-service.
// Ожидает уже нормализованный логин и захешированный пароль. При конфликте логина
// сервер вернёт ошибку APIError со статусом 409.
func (c *UserCrudClient) CreateUser(ctx context.Context, login, passwordHash string) (*User, error) {
	payload := map[string]string{
		"login":         login,
		"password_hash": passwordHash,
	}
	return c.doRequest(ctx, http.MethodPost, "/users", payload)
}

// GetUserByLogin возвращает пользователя по логину.
// Используется auth-service при логине пользователя для получения хеша пароля и GUID.
func (c *UserCrudClient) GetUserByLogin(ctx context.Context, login string) (*User, error) {
	endpoint := path.Join("/users/by-login", url.PathEscape(login))
	return c.doRequest(ctx, http.MethodGet, endpoint, nil)
}

// GetUserByID возвращает пользователя по внешнему GUID.
// Применяется при выдаче новых токенов и в эндпоинте `/auth/me`, когда по данным
// из токена нужно получить актуальное состояние пользователя.
func (c *UserCrudClient) GetUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	return c.doRequest(ctx, http.MethodGet, path.Join("/users", id.String()), nil)
}

// DeleteUser удаляет пользователя по внешнему GUID (§10.14/6).
func (c *UserCrudClient) DeleteUser(ctx context.Context, id uuid.UUID) error {
	return c.doNoContent(ctx, http.MethodDelete, path.Join("/users", id.String()), nil)
}

// SetRecoveryKeyHash сохраняет bcrypt-хеш ключа восстановления (§10.10).
func (c *UserCrudClient) SetRecoveryKeyHash(ctx context.Context, id uuid.UUID, hash string) error {
	return c.doNoContent(ctx, http.MethodPatch, path.Join("/users", id.String(), "recovery-key-hash"), map[string]string{"recovery_key_hash": hash})
}

// UpdatePasswordHash обновляет хеш пароля пользователя (§10.10, recovery flow).
func (c *UserCrudClient) UpdatePasswordHash(ctx context.Context, id uuid.UUID, passwordHash string) error {
	_, err := c.doRequest(ctx, http.MethodPut, path.Join("/users", id.String()), map[string]string{"password_hash": passwordHash})
	return err
}

// doNoContent выполняет запрос, не читая тело ответа (для 204 No Content).
func (c *UserCrudClient) doNoContent(ctx context.Context, method, endpoint string, body any) error {
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, endpoint)

	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := extractErrorMessage(resp.Body)
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	return nil
}

// doRequest — вспомогательный метод, который собирает URL, добавляет JSON-тело (если есть),
// выполняет HTTP-запрос и декодирует ответ в структуру пользователя. При кодах >= 400
// возвращает APIError с текстом ошибки из тела ответа.
func (c *UserCrudClient) doRequest(ctx context.Context, method, endpoint string, body any) (*User, error) {
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, endpoint)

	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		msg := extractErrorMessage(resp.Body)
		return nil, &APIError{Status: resp.StatusCode, Message: msg}
	}

	var wrapper struct {
		User User `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &wrapper.User, nil
}

// extractErrorMessage пытается вытащить поле `error` из JSON-ответа, а если это не удалось,
// возвращает сырой текст тела ответа для последующей диагностики.
func extractErrorMessage(body io.Reader) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err == nil && payload.Error != "" {
		return payload.Error
	}
	bytes, err := io.ReadAll(body)
	if err != nil {
		return ""
	}
	return string(bytes)
}

