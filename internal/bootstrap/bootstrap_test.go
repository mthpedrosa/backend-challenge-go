package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"

	"github.com/mthpedrosa/backend-challenge-go/internal/adapters/httpapi"
	"github.com/mthpedrosa/backend-challenge-go/internal/config"
	"github.com/mthpedrosa/backend-challenge-go/internal/platform/correlation"
)

func testConfig(cfg config.Config) config.Config {
	cfg.HTTP.Addr = "127.0.0.1:0"
	cfg.HTTP.ShutdownTimeout = 2 * time.Second
	return cfg
}

func TestOptions_GraphIsValid(t *testing.T) {
	if err := fx.ValidateApp(Options(fx.Decorate(testConfig))); err != nil {
		t.Fatalf("invalid fx graph: %v", err)
	}
}

func TestApp_StartServeAndStop(t *testing.T) {
	var server *httpapi.Server
	app := fxtest.New(t,
		Options(fx.Decorate(testConfig)),
		fx.Populate(&server),
	)
	app.RequireStart()

	addr := server.Addr()
	if addr == nil {
		t.Fatal("server did not bind an address")
	}
	base := "http://" + addr.String()

	t.Run("liveness", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/health/live", nil)
		req.Header.Set(correlation.Header, "corr-test")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if got := resp.Header.Get(correlation.Header); got != "corr-test" {
			t.Errorf("correlation header = %q, want corr-test", got)
		}
		var body map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body["status"] != "ok" {
			t.Errorf("body = %v, err = %v", body, err)
		}
	})

	t.Run("unknown route uses error contract", func(t *testing.T) {
		resp, err := http.Get(base + "/does-not-exist")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
		var body httpapi.ErrorBody
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil || body.Error.Code != httpapi.CodeNotFound {
			t.Errorf("body = %+v, err = %v", body, err)
		}
		if resp.Header.Get(correlation.Header) == "" {
			t.Error("expected generated correlation id")
		}
	})

	app.RequireStop()

	// After stop the listener must be released.
	client := http.Client{Timeout: time.Second}
	if _, err := client.Get(base + "/health/live"); err == nil {
		t.Error("expected connection error after shutdown")
	}
}
