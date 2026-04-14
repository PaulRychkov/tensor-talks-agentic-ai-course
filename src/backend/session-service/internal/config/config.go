package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config агрегирует все опции рантайма для session-service.
type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Redis          RedisConfig          `mapstructure:"redis"`
	SessionCRUD    SessionCRUDConfig    `mapstructure:"session_crud"`
	Kafka          KafkaConfig          `mapstructure:"kafka"`
	SessionManager SessionManagerConfig `mapstructure:"session_manager"`
}

// ServerConfig описывает настройки HTTP-сервера.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// RedisConfig содержит параметры подключения к Redis.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
	TTLHours int    `mapstructure:"ttl_hours"` // TTL для сессий в часах
}

// SessionCRUDConfig содержит параметры подключения к session-crud-service.
type SessionCRUDConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// KafkaConfig содержит параметры подключения к Kafka.
type KafkaConfig struct {
	Brokers       []string `mapstructure:"brokers"`
	TopicRequest  string   `mapstructure:"topic_request"`
	TopicResponse string   `mapstructure:"topic_response"`
	ConsumerGroup string   `mapstructure:"consumer_group"`
}

// SessionManagerConfig содержит параметры управления сессиями.
type SessionManagerConfig struct {
	MaxActiveSessions     int `mapstructure:"max_active_sessions"`     // Максимальное количество активных сессий
	ProgramTimeoutSeconds int `mapstructure:"program_timeout_seconds"` // Таймаут ожидания программы интервью от builder-service (через Kafka) для HTTP ответа bff-service
}

// Load загружает конфигурацию из файла и переменных окружения.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")

	v.SetEnvPrefix("SESSION")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	// Парсим Kafka brokers из строки или массива
	if brokersStr := v.GetString("kafka.brokers"); brokersStr != "" {
		cfg.Kafka.Brokers = strings.Split(brokersStr, ",")
		for i, broker := range cfg.Kafka.Brokers {
			cfg.Kafka.Brokers[i] = strings.TrimSpace(broker)
		}
	} else {
		cfg.Kafka.Brokers = v.GetStringSlice("kafka.brokers")
	}

	if len(cfg.Kafka.Brokers) == 0 {
		cfg.Kafka.Brokers = []string{"kafka:9092"}
	}

	// Значения по умолчанию
	if cfg.Redis.Addr == "" {
		cfg.Redis.Addr = "localhost:6379"
	}
	if cfg.Redis.TTLHours == 0 {
		cfg.Redis.TTLHours = 24
	}
	if cfg.SessionCRUD.BaseURL == "" {
		cfg.SessionCRUD.BaseURL = "http://session-crud-service:8085"
	}
	if cfg.SessionCRUD.TimeoutSeconds == 0 {
		cfg.SessionCRUD.TimeoutSeconds = 5
	}
	if cfg.Kafka.TopicRequest == "" {
		cfg.Kafka.TopicRequest = "tensor-talks-interview.build.request"
	}
	if cfg.Kafka.TopicResponse == "" {
		cfg.Kafka.TopicResponse = "tensor-talks-interview.build.response"
	}
	if cfg.SessionManager.MaxActiveSessions == 0 {
		cfg.SessionManager.MaxActiveSessions = 100
	}
	if cfg.SessionManager.ProgramTimeoutSeconds == 0 {
		cfg.SessionManager.ProgramTimeoutSeconds = 30 // 30 секунд на получение программы интервью от builder-service
	}

	return cfg, nil
}
