package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config aggregates all runtime configuration for admin-bff-service (§10.1).
type Config struct {
	Server                  ServerConfig   `mapstructure:"server"`
	AuthService             ServiceConfig  `mapstructure:"auth_service"`
	KnowledgeProducerSvc    ServiceConfig  `mapstructure:"knowledge_producer_service"`
	KnowledgeBaseCrudSvc    ServiceConfig  `mapstructure:"knowledge_base_crud_service"`
	QuestionsCrudSvc        ServiceConfig  `mapstructure:"questions_crud_service"`
	UserCrudSvc             ServiceConfig  `mapstructure:"user_crud_service"`
	ResultsCrudSvc          ServiceConfig  `mapstructure:"results_crud_service"`
	PrometheusURL           string         `mapstructure:"prometheus_url"`
	JWT                     JWTConfig      `mapstructure:"jwt"`
	AllowedRoles            []string       `mapstructure:"allowed_roles"`
	AdminSecret             string         `mapstructure:"admin_secret"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// ServiceConfig holds downstream service connection parameters.
type ServiceConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// JWTConfig holds JWT validation parameters.
type JWTConfig struct {
	Secret   string `mapstructure:"secret"`
	Issuer   string `mapstructure:"issuer"`
	Audience string `mapstructure:"audience"`
}

// Load reads configuration from config.yaml and environment variables.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")

	v.SetEnvPrefix("ADMIN_BFF")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	// Defaults
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8096
	}
	if len(cfg.AllowedRoles) == 0 {
		cfg.AllowedRoles = []string{"admin", "content_editor"}
	}

	return cfg, nil
}
