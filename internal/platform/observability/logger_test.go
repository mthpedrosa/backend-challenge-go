package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/mthpedrosa/backend-challenge-go/internal/config"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/correlation"
)

func TestLogger_EmitsJSONWithCorrelationID(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.Config{
		App: config.AppConfig{Name: "wallet-service", Env: "test", InstanceID: "i-1"},
		Log: config.LogConfig{Level: slog.LevelInfo},
	}
	logger := newLogger(&buf, cfg)

	ctx := correlation.WithID(context.Background(), "corr-123")
	logger.With("component", "test").InfoContext(ctx, "hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, buf.String())
	}
	want := map[string]string{
		"msg":           "hello",
		"service":       "wallet-service",
		"instanceId":    "i-1",
		"component":     "test",
		"correlationId": "corr-123",
	}
	for k, v := range want {
		if rec[k] != v {
			t.Errorf("%s = %v, want %q", k, rec[k], v)
		}
	}
}
