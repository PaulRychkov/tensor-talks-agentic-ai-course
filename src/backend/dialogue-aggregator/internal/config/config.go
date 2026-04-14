package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config агрегирует все опции конфигурации dialogue-aggregator.
type Config struct {
	Server         ServerConfig         `mapstructure:"server"`
	Kafka          KafkaConfig          `mapstructure:"kafka"`
	Model          ModelConfig          `mapstructure:"model"`
	SessionManager SessionManagerConfig `mapstructure:"session_manager"`
	ChatCRUD       ChatCRUDConfig       `mapstructure:"chat_crud"`
	ResultsCRUD    ResultsCRUDConfig    `mapstructure:"results_crud"`
	Redis          RedisConfig          `mapstructure:"redis"`
}

// ServerConfig описывает настройки HTTP-сервера.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// KafkaConfig содержит параметры подключения к Kafka.
type KafkaConfig struct {
	Brokers        []string `mapstructure:"brokers"`
	TopicChatOut   string   `mapstructure:"topic_chat_out"`
	TopicChatIn    string   `mapstructure:"topic_chat_in"`
	ConsumerGroup  string   `mapstructure:"consumer_group"`
	TopicSessionCompleted string `mapstructure:"topic_session_completed"`
	TopicMessages  string   `mapstructure:"topic_messages_full"`
	TopicGenerated string   `mapstructure:"topic_generated_phrases"`
	AgentGroupID   string   `mapstructure:"agent_consumer_group"`
}

// ModelConfig содержит параметры модели.
type ModelConfig struct {
	QuestionDelaySeconds int `mapstructure:"question_delay_seconds"`
}

// SessionManagerConfig содержит параметры подключения к session-manager-service.
type SessionManagerConfig struct {
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

// RedisConfig содержит параметры подключения к Redis.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// Load загружает конфигурацию из файла и переменных окружения.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")

	v.SetEnvPrefix("DIALOGUE_AGGREGATOR")
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
	if cfg.Model.QuestionDelaySeconds == 0 {
		cfg.Model.QuestionDelaySeconds = 2
	}
	if cfg.SessionManager.BaseURL == "" {
		cfg.SessionManager.BaseURL = "http://session-service:8083"
	}
	if cfg.SessionManager.TimeoutSeconds == 0 {
		cfg.SessionManager.TimeoutSeconds = 5
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
