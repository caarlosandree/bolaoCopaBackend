package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost                       string
	DBPort                       string
	DBUser                       string
	DBPassword                   string
	DBName                       string
	JWTSecret                    string
	AppEnv                       string
	LogLevel                     string
	LogFormat                    string
	AuditEnabled                 bool
	AuditRetentionDays           int
	MatchSyncEnabled             bool
	MatchResultRetryMinutes      int
	MatchResultCheckAfterMinutes int
	OpenFootballURL              string
	WorldCup26BaseURL            string
	BaseURL                      string
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
	cfg.MatchSyncEnabled = getenvBoolDefault("MATCH_SYNC_ENABLED", true)
	cfg.MatchResultRetryMinutes = getenvIntDefault("MATCH_RESULT_RETRY_MINUTES", legacySyncIntervalMinutes())
	cfg.MatchResultCheckAfterMinutes = getenvIntDefault("MATCH_RESULT_CHECK_AFTER_MINUTES", 120)
	cfg.OpenFootballURL = getenvDefault("OPENFOOTBALL_WORLD_CUP_URL", "https://raw.githubusercontent.com/openfootball/worldcup.json/master/2026/worldcup.json")
	cfg.WorldCup26BaseURL = strings.TrimRight(getenvDefault("WORLDCUP26_BASE_URL", "https://worldcup26.ir"), "/")
	cfg.BaseURL = strings.TrimRight(getenvDefault("BASE_URL", "http://localhost:1323"), "/")

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

func legacySyncIntervalMinutes() int {
	minutesValue := strings.TrimSpace(os.Getenv("MATCH_SYNC_INTERVAL_MINUTES"))
	if minutesValue != "" {
		minutes, err := strconv.Atoi(minutesValue)
		if err == nil && minutes > 0 {
			return minutes
		}
	}

	hoursValue := strings.TrimSpace(os.Getenv("MATCH_SYNC_INTERVAL_HOURS"))
	if hoursValue == "" {
		return 15
	}
	hours, err := strconv.Atoi(hoursValue)
	if err != nil || hours <= 0 {
		return 15
	}
	return hours * 60
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
