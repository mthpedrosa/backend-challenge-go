package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func lookupFrom(m map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		v, ok := m[key]
		return v, ok
	}
}

func TestLoadFrom_Defaults(t *testing.T) {
	cfg, err := LoadFrom(lookupFrom(map[string]string{
		"APP_INSTANCE_ID":      "test-1",
		"DATABASE_URL":         "postgres://u:p@localhost:5432/db",
		"JWKS_URL":             "http://mock",
		"SQS_OUTBOX_QUEUE_URL": "http://mock-sqs",
		"SQS_INBOX_QUEUE_URL":  "http://mock-inbox-sqs",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTP.Addr != ":8080" {
		t.Errorf("HTTP.Addr = %q, want :8080", cfg.HTTP.Addr)
	}
	if cfg.Log.Level != slog.LevelInfo {
		t.Errorf("Log.Level = %v, want INFO", cfg.Log.Level)
	}
	if cfg.HTTP.ShutdownTimeout != 20*time.Second {
		t.Errorf("HTTP.ShutdownTimeout = %v, want 20s", cfg.HTTP.ShutdownTimeout)
	}
	if cfg.App.InstanceID != "test-1" {
		t.Errorf("App.InstanceID = %q, want test-1", cfg.App.InstanceID)
	}
}

func TestLoadFrom_Overrides(t *testing.T) {
	cfg, err := LoadFrom(lookupFrom(map[string]string{
		"HTTP_ADDR":             "127.0.0.1:9090",
		"LOG_LEVEL":             "debug",
		"HTTP_SHUTDOWN_TIMEOUT": "3s",
		"DATABASE_URL":          "postgres://override",
		"JWKS_URL":              "http://mock-override",
		"SQS_OUTBOX_QUEUE_URL":  "http://mock-sqs-override",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTP.Addr != "127.0.0.1:9090" || cfg.Log.Level != slog.LevelDebug || cfg.HTTP.ShutdownTimeout != 3*time.Second {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadFrom_ReportsAllErrors(t *testing.T) {
	_, err := LoadFrom(lookupFrom(map[string]string{
		"LOG_LEVEL":          "verbose",
		"HTTP_READ_TIMEOUT":  "ten seconds",
		"HTTP_WRITE_TIMEOUT": "-1s",
	}))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, want := range []string{"LOG_LEVEL", "HTTP_READ_TIMEOUT", "HTTP_WRITE_TIMEOUT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
