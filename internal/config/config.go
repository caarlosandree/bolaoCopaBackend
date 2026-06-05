package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost             string
	DBPort             string
	DBUser             string
	DBPassword         string
	DBName             string
	JWTSecret          string
	AppEnv             string
	LogLevel           string
	LogFormat          string
	AuditEnabled       bool
	AuditRetentionDays int
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		JWTSecret:  os.Getenv("JWT_SECRET"),
	}
	cfg.AppEnv = getenvDefault("APP_ENV", "development")
	cfg.LogLevel = getenvDefault("LOG_LEVEL", defaultLogLevel(cfg.AppEnv))
	cfg.LogFormat = getenvDefault("LOG_FORMAT", defaultLogFormat(cfg.AppEnv))
	cfg.AuditEnabled = getenvBoolDefault("AUDIT_ENABLED", true)
	cfg.AuditRetentionDays = getenvIntDefault("AUDIT_RETENTION_DAYS", 90)

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET não definido no .env")
	}
	if cfg.DBName == "" {
		return nil, fmt.Errorf("DB_NAME não definido no .env")
	}

	return cfg, nil
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName,
	)
}

func getenvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getenvBoolDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvIntDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func defaultLogLevel(appEnv string) string {
	if appEnv == "production" {
		return "info"
	}
	return "debug"
}

func defaultLogFormat(appEnv string) string {
	if appEnv == "production" {
		return "json"
	}
	return "text"
}
