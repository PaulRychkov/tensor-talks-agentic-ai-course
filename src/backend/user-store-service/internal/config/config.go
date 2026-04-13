package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config wraps all runtime configuration knobs.
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// DatabaseConfig contains PostgreSQL connection parameters.
type DatabaseConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Name     string `mapstructure:"name"`
	SSLMode  string `mapstructure:"ssl_mode"`
	Schema   string `mapstructure:"schema"` // PostgreSQL schema name
}

// DSN строит строку подключения к PostgreSQL в формате, совместимом с GORM/pgx.
// Включает хост, порт, имя пользователя, пароль, имя базы, режим SSL и search_path для схемы.
func (c DatabaseConfig) DSN() string {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host,
		c.Port,
		c.User,
		c.Password,
		c.Name,
		c.SSLMode,
	)
	if c.Schema != "" {
		dsn += fmt.Sprintf(" search_path=%s", c.Schema)
	}
	return dsn
}

// Load читает конфигурацию из файла и переменных окружения.
// При ошибке чтения/разбора конфигурации сервис не запускается, чтобы не работать
// с потенциально некорректными параметрами подключения к базе.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")

	v.SetEnvPrefix("USER_STORE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	return cfg, nil
}
