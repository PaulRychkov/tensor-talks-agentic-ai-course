package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

/*
Пакет config отвечает за загрузку и валидацию конфигурации auth-service.

Источник настроек:
  - YAML-файл `config/config.yaml`;
  - переменные окружения с префиксом `AUTH_` (имеют приоритет над файлом).

Через конфигурацию настраиваются:
  - параметры HTTP-сервера;
  - подключение к user-crud-service;
  - параметры JWT (issuer, audience, TTL, секрет).
*/

// Config агрегирует все опции рантайма для auth-service.
type Config struct {
	Server    ServerConfig   `mapstructure:"server"`
	UserCrud  UserCrudConfig `mapstructure:"user_store"`
	JWT       JWTConfig      `mapstructure:"jwt"`
	Redis     RedisConfig    `mapstructure:"redis"`
}

// ServerConfig описывает настройки HTTP-сервера.
type ServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// UserCrudConfig описывает параметры подключения к user-crud-service.
type UserCrudConfig struct {
	BaseURL        string `mapstructure:"base_url"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds"`
}

// JWTConfig инкапсулирует настройки выпуска JWT-токенов.
type JWTConfig struct {
	Issuer          string        `mapstructure:"issuer"`
	Audience        string        `mapstructure:"audience"`
	AccessTokenTTL  time.Duration `mapstructure:"-"`
	RefreshTokenTTL time.Duration `mapstructure:"-"`
	Secret          string        `mapstructure:"secret"`

	AccessTokenTTLRaw  string `mapstructure:"access_token_ttl"`
	RefreshTokenTTLRaw string `mapstructure:"refresh_token_ttl"`
}

// RedisConfig содержит параметры подключения к Redis для управления логин-сессиями.
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// Load загружает конфигурацию из файла и переменных окружения.
// Порядок приоритетов:
//  1. значения из окружения `AUTH_*`,
//  2. значения из `config/config.yaml`.
//
// Здесь же происходит парсинг строковых TTL для токенов и проверка, что секрет задан.
func Load() (Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")

	v.SetEnvPrefix("AUTH")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}

	if err := cfg.JWT.parseDurations(); err != nil {
		return Config{}, err
	}

	if cfg.JWT.Secret == "" {
		return Config{}, fmt.Errorf("jwt.secret must be provided")
	}

	return cfg, nil
}

// parseDurations парсит текстовые значения TTL в тип time.Duration.
// Ошибки парсинга считаются критическими и приводят к невозможности запуска сервиса,
// чтобы не допустить работы с неверной конфигурацией безопасности.
func (j *JWTConfig) parseDurations() error {
	access, err := time.ParseDuration(j.AccessTokenTTLRaw)
	if err != nil {
		return fmt.Errorf("parse jwt.access_token_ttl: %w", err)
	}
	refresh, err := time.ParseDuration(j.RefreshTokenTTLRaw)
	if err != nil {
		return fmt.Errorf("parse jwt.refresh_token_ttl: %w", err)
	}
	j.AccessTokenTTL = access
	j.RefreshTokenTTL = refresh
	return nil
}

