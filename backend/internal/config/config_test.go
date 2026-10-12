package config

import (
	"testing"
	"time"
)

func TestLoadFromDefaults(t *testing.T) {
	t.Setenv("unused", "")
	values := map[string]string{"DATABASE_URL": "postgres://example"}

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
}

func TestLoadFromRejectsUnsafeTTL(t *testing.T) {
	values := map[string]string{
		"DATABASE_URL":    "postgres://example",
		"APP_SESSION_TTL": "10m",
	}

	if _, err := LoadFrom(func(key string) string { return values[key] }); err == nil {
		t.Fatal("LoadFrom() expected error")
	}
}
