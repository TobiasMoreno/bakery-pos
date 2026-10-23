package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultSessionTTL = 12 * time.Hour

type Config struct {
	Environment       string
	HTTPAddr          string
	DatabaseURL       string
	AllowedOrigin     string
	FrontendDir       string
	CookieSecure      bool
	SessionTTL        time.Duration
	OfflineSigningKey string
}

func Load() (Config, error) {
	return LoadFrom(os.Getenv)
}

func LoadFrom(getenv func(string) string) (Config, error) {
	cfg := Config{
		Environment:       valueOrDefault(getenv("APP_ENV"), "development"),
		HTTPAddr:          valueOrDefault(getenv("APP_HTTP_ADDR"), ":8080"),
		DatabaseURL:       strings.TrimSpace(getenv("DATABASE_URL")),
		AllowedOrigin:     valueOrDefault(getenv("APP_ALLOWED_ORIGIN"), "http://localhost:5173"),
		FrontendDir:       strings.TrimSpace(getenv("APP_FRONTEND_DIR")),
		SessionTTL:        defaultSessionTTL,
		OfflineSigningKey: strings.TrimSpace(getenv("APP_OFFLINE_SIGNING_KEY")),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if len(cfg.OfflineSigningKey) < 32 {
		return Config{}, errors.New("APP_OFFLINE_SIGNING_KEY must contain at least 32 characters")
	}

	if raw := strings.TrimSpace(getenv("APP_COOKIE_SECURE")); raw != "" {
		secure, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse APP_COOKIE_SECURE: %w", err)
		}
		cfg.CookieSecure = secure
	}

	if raw := strings.TrimSpace(getenv("APP_SESSION_TTL")); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse APP_SESSION_TTL: %w", err)
		}
		if ttl < 15*time.Minute || ttl > 7*24*time.Hour {
			return Config{}, errors.New("APP_SESSION_TTL must be between 15m and 168h")
		}
		cfg.SessionTTL = ttl
	}

	return cfg, nil
}

func valueOrDefault(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}
