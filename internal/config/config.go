// Package config loads and validates the application configuration from
// environment variables. Validation runs at startup so that a misconfigured
// instance fails fast instead of serving traffic in a broken state.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Config is the root configuration of the service.
type Config struct {
	App  AppConfig
	Log  LogConfig
	HTTP HTTPConfig
	DB   DBConfig
}

// DBConfig holds database settings.
type DBConfig struct {
	URL string
}

// AppConfig holds process-level settings.
type AppConfig struct {
	Name       string
	Env        string
	InstanceID string
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level slog.Level
}

// HTTPConfig holds the HTTP server settings.
type HTTPConfig struct {
	Addr              string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// LookupFunc resolves an environment variable. It allows tests to inject
// values without mutating the process environment.
type LookupFunc func(key string) (string, bool)

// Load reads the configuration from the process environment.
func Load() (Config, error) {
	return LoadFrom(os.LookupEnv)
}

// LoadFrom reads the configuration using the given lookup function and
// validates the result. All problems are reported together.
func LoadFrom(lookup LookupFunc) (Config, error) {
	r := reader{lookup: lookup}

	hostname, _ := os.Hostname()

	cfg := Config{
		App: AppConfig{
			Name:       r.string("APP_NAME", "wallet-service"),
			Env:        r.string("APP_ENV", "local"),
			InstanceID: r.string("APP_INSTANCE_ID", hostname),
		},
		Log: LogConfig{
			Level: r.logLevel("LOG_LEVEL", slog.LevelInfo),
		},
		HTTP: HTTPConfig{
			Addr:              r.string("HTTP_ADDR", ":8080"),
			ReadHeaderTimeout: r.duration("HTTP_READ_HEADER_TIMEOUT", 5*time.Second),
			ReadTimeout:       r.duration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:      r.duration("HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:       r.duration("HTTP_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout:   r.duration("HTTP_SHUTDOWN_TIMEOUT", 20*time.Second),
		},
		DB: DBConfig{
			URL: r.string("DATABASE_URL", ""),
		},
	}

	if err := errors.Join(append(r.errs, cfg.validate())...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration: %w", err)
	}
	return cfg, nil
}

func (c Config) validate() error {
	var errs []error
	if strings.TrimSpace(c.App.Name) == "" {
		errs = append(errs, errors.New("APP_NAME must not be empty"))
	}
	if strings.TrimSpace(c.App.InstanceID) == "" {
		errs = append(errs, errors.New("APP_INSTANCE_ID must not be empty"))
	}
	if strings.TrimSpace(c.HTTP.Addr) == "" {
		errs = append(errs, errors.New("HTTP_ADDR must not be empty"))
	}
	if strings.TrimSpace(c.DB.URL) == "" {
		errs = append(errs, errors.New("DATABASE_URL must not be empty"))
	}
	positive := map[string]time.Duration{
		"HTTP_READ_HEADER_TIMEOUT": c.HTTP.ReadHeaderTimeout,
		"HTTP_READ_TIMEOUT":        c.HTTP.ReadTimeout,
		"HTTP_WRITE_TIMEOUT":       c.HTTP.WriteTimeout,
		"HTTP_IDLE_TIMEOUT":        c.HTTP.IdleTimeout,
		"HTTP_SHUTDOWN_TIMEOUT":    c.HTTP.ShutdownTimeout,
	}
	for key, d := range positive {
		if d <= 0 {
			errs = append(errs, fmt.Errorf("%s must be greater than zero", key))
		}
	}
	return errors.Join(errs...)
}

// reader accumulates parsing errors so they can be reported at once.
type reader struct {
	lookup LookupFunc
	errs   []error
}

func (r *reader) raw(key string) (string, bool) {
	v, ok := r.lookup(key)
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	return v, v != ""
}

func (r *reader) string(key, def string) string {
	if v, ok := r.raw(key); ok {
		return v
	}
	return def
}

func (r *reader) duration(key string, def time.Duration) time.Duration {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: invalid duration %q", key, v))
		return def
	}
	return d
}

func (r *reader) logLevel(key string, def slog.Level) slog.Level {
	v, ok := r.raw(key)
	if !ok {
		return def
	}
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(v)); err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s: invalid log level %q", key, v))
		return def
	}
	return lvl
}
