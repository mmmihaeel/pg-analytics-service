package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env                 string
	Port                string
	Version             string
	DatabaseURL         string
	RedisURL            string
	ManagementAPIKey    string
	AutoMigrate         bool
	AutoSeed            bool
	MaxReportRangeDays  int
	RecomputeLockTTL    time.Duration
	RecomputeWorkers    int
	RecomputeQueueSize  int
	HTTPReadTimeout     time.Duration
	HTTPWriteTimeout    time.Duration
	HTTPShutdownTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Env:                 getString("APP_ENV", "development"),
		Port:                getString("APP_PORT", "3004"),
		Version:             getString("APP_VERSION", "dev"),
		DatabaseURL:         getString("DATABASE_URL", ""),
		RedisURL:            getString("REDIS_URL", ""),
		ManagementAPIKey:    getString("MANAGEMENT_API_KEY", "change-me"),
		AutoMigrate:         getBool("APP_AUTO_MIGRATE", true),
		AutoSeed:            getBool("APP_AUTO_SEED", false),
		MaxReportRangeDays:  getInt("MAX_REPORT_RANGE_DAYS", 366),
		RecomputeLockTTL:    getDuration("RECOMPUTE_LOCK_TTL", 15*time.Minute),
		RecomputeWorkers:    getInt("RECOMPUTE_WORKERS", 1),
		RecomputeQueueSize:  getInt("RECOMPUTE_QUEUE_SIZE", 64),
		HTTPReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 10*time.Second),
		HTTPWriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 20*time.Second),
		HTTPShutdownTimeout: getDuration("HTTP_SHUTDOWN_TIMEOUT", 15*time.Second),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RedisURL == "" {
		return Config{}, fmt.Errorf("REDIS_URL is required")
	}

	if cfg.MaxReportRangeDays < 1 {
		return Config{}, fmt.Errorf("MAX_REPORT_RANGE_DAYS must be positive")
	}
	if cfg.RecomputeWorkers < 1 {
		return Config{}, fmt.Errorf("RECOMPUTE_WORKERS must be at least 1")
	}
	if cfg.RecomputeQueueSize < 1 {
		return Config{}, fmt.Errorf("RECOMPUTE_QUEUE_SIZE must be at least 1")
	}

	return cfg, nil
}

func getString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
