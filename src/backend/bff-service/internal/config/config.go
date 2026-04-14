package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

/*
Пакет config отвечает за загрузку конфигурации BFF-сервиса.

Источники:
  - YAML-файл `config/config.yaml`;
  - переменные окружения с префиксом `BFF_` (перекрывают значения из файла).

Настройки:
  - адрес HTTP-сервера;
  - параметры подключения к auth-service;
  - CORS (разрешённые origin и заголовки).
*/

// Config агрегирует все опции конфигурации BFF.
// Используется на этапе старта сервиса для настройки HTTP-сервера, CORS и клиента auth-service.
type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	AuthService    AuthServiceConfig    `mapstructure:"auth_service"`
	SessionService SessionServiceConfig `mapstructure:"session_service"`
	SessionCRUD    SessionCRUDConfig    `mapstructure:"session_crud"`
	ChatCRUD       ChatCRUDConfig       `mapstructure:"chat_crud"`
	ResultsCRUD    ResultsCRUDConfig    `mapstructure:"results_crud"`
	UserCRUD       UserCRUDConfig       `mapstructure:"user_crud"`
	Kafka          KafkaConfig          `mapstructure:"kafka"`
	CORS           CORSConfig           `mapstructure:"cors"`
	Redis          RedisConfig          `mapstructure:"redis"`
}

// RedisConfig содержит параметры подключения к Redis (для чтения шагов агента).
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// UserCRUDConfig содержит параметры подключения к user-crud-service.
type UserCRUDConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// SessionCRUDConfig содержит параметры подключения к session-crud-service.
type SessionCRUDConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// ChatCRUDConfig содержит параметры подключения к chat-crud-service.
type ChatCRUDConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// ResultsCRUDConfig содержит параметры подключения к results-crud-service.
type ResultsCRUDConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// ServerConfig описывает настройки HTTP-сервера BFF.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// AuthServiceConfig содержит параметры подключения к auth-service.
type AuthServiceConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// SessionServiceConfig содержит параметры подключения к session-service.
type SessionServiceConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// KafkaConfig содержит параметры подключения к Kafka.
type KafkaConfig struct {
	Brokers       []string `mapstructure:"brokers"`
	TopicChatOut  string   `mapstructure:"topic_chat_out"`
	TopicChatIn   string   `mapstructure:"topic_chat_in"`
	ConsumerGroup string   `mapstructure:"consumer_group"`
}

// CORSConfig описывает настройки CORS.
type CORSConfig struct {
	AllowOrigins []string `mapstructure:"allow_origins"`
	AllowHeaders []string `mapstructure:"allow_headers"`
}

// Load загружает конфигурацию из файла и окружения.
// Порядок приоритетов:
//  1. переменные окружения с префиксом BFF_,
//  2. значения из `config/config.yaml`.
//
// После чтения конфигурации дополнительно заполняются списки CORS-оригинов/заголовков.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")

	v.SetEnvPrefix("BFF")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	// Парсим CORS AllowOrigins из строки или массива (аналогично Kafka brokers)
	if originsStr := v.GetString("cors.allow_origins"); originsStr != "" {
		cfg.CORS.AllowOrigins = strings.Split(originsStr, ",")
		for i, origin := range cfg.CORS.AllowOrigins {
			cfg.CORS.AllowOrigins[i] = strings.TrimSpace(origin)
		}
	} else {
		cfg.CORS.AllowOrigins = v.GetStringSlice("cors.allow_origins")
	}

	// Парсим CORS AllowHeaders из строки или массива
	if headersStr := v.GetString("cors.allow_headers"); headersStr != "" {
		cfg.CORS.AllowHeaders = strings.Split(headersStr, ",")
		for i, header := range cfg.CORS.AllowHeaders {
			cfg.CORS.AllowHeaders[i] = strings.TrimSpace(header)
		}
	} else {
		cfg.CORS.AllowHeaders = v.GetStringSlice("cors.allow_headers")
	}

	// Парсим Kafka brokers из строки или массива
	// Сначала пробуем получить как строку (из переменной окружения)
	if brokersStr := v.GetString("kafka.brokers"); brokersStr != "" {
		// Разделяем по запятой, если это строка с несколькими брокерами
		cfg.Kafka.Brokers = strings.Split(brokersStr, ",")
		// Убираем пробелы
		for i, broker := range cfg.Kafka.Brokers {
			cfg.Kafka.Brokers[i] = strings.TrimSpace(broker)
		}
	} else {
		// Если не строка, пробуем получить как массив
		cfg.Kafka.Brokers = v.GetStringSlice("kafka.brokers")
	}

	// Если brokers все еще пустой, используем значение по умолчанию
	if len(cfg.Kafka.Brokers) == 0 {
		cfg.Kafka.Brokers = []string{"kafka:9092"}
	}

	// Значения по умолчанию для новых сервисов
	if cfg.UserCRUD.BaseURL == "" {
		cfg.UserCRUD.BaseURL = "http://tensor-talks-user-crud-service:8082"
	}
	if cfg.UserCRUD.TimeoutSeconds == 0 {
		cfg.UserCRUD.TimeoutSeconds = 5
	}
	if cfg.SessionCRUD.BaseURL == "" {
		cfg.SessionCRUD.BaseURL = "http://session-crud-service:8085"
	}
	if cfg.SessionCRUD.TimeoutSeconds == 0 {
		cfg.SessionCRUD.TimeoutSeconds = 5
	}
	if cfg.ChatCRUD.BaseURL == "" {
		cfg.ChatCRUD.BaseURL = "http://chat-crud-service:8087"
	}
	if cfg.ChatCRUD.TimeoutSeconds == 0 {
		cfg.ChatCRUD.TimeoutSeconds = 5
	}
	if cfg.ResultsCRUD.BaseURL == "" {
		cfg.ResultsCRUD.BaseURL = "http://results-crud-service:8088"
	}
	if cfg.ResultsCRUD.TimeoutSeconds == 0 {
		cfg.ResultsCRUD.TimeoutSeconds = 5
	}

	return cfg, nil
}
