package config

import (
	"testing"
	"time"
)

func TestLoadFromDefaults(t *testing.T) {
	t.Setenv("unused", "")
	values := map[string]string{"DATABASE_URL": "postgres://example", "APP_OFFLINE_SIGNING_KEY": "test-offline-signing-key-at-least-32-characters"}

	cfg, err := LoadFrom(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.SessionTTL != 12*time.Hour {
		t.Fatalf("SessionTTL = %v", cfg.SessionTTL)
	}
	if cfg.FrontendDir != "" {
		t.Fatalf("FrontendDir = %q", cfg.FrontendDir)
	}
}

func TestLoadFromFrontendDirectory(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":            "postgres://example",
		"APP_FRONTEND_DIR":        "/app/frontend",
		"APP_OFFLINE_SIGNING_KEY": "test-offline-signing-key-at-least-32-characters",
	}

	cfg, err := LoadFrom(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("LoadFrom() error = %v", err)
	}
	if cfg.FrontendDir != "/app/frontend" {
		t.Fatalf("FrontendDir = %q", cfg.FrontendDir)
	}
}

func TestLoadFromRejectsUnsafeTTL(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":            "postgres://example",
		"APP_SESSION_TTL":         "10m",
		"APP_OFFLINE_SIGNING_KEY": "test-offline-signing-key-at-least-32-characters",
	}

	if _, err := LoadFrom(func(key string) string { return values[key] }); err == nil {
		t.Fatal("LoadFrom() expected error")
	}
}
