package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
	"go.uber.org/fx"

	"github.com/mthpedrosa/backend-challenge-go/internal/config"
)

// Routes is implemented by every handler that contributes endpoints. Handlers
// are collected through the "routes" Fx value group.
type Routes interface {
	RegisterRoutes(e *echo.Echo)
}

// AsRoutes annotates a constructor so its result joins the routes group.
func AsRoutes(constructor any) any {
	return fx.Annotate(constructor,
		fx.As(new(Routes)),
		fx.ResultTags(`group:"routes"`),
	)
}

// RouterParams are the dependencies needed to build the Echo router.
type RouterParams struct {
	fx.In

	Logger *slog.Logger
	Routes []Routes `group:"routes"`
}

// NewRouter builds the Echo instance with global middlewares and all routes.
func NewRouter(p RouterParams) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.HTTPErrorHandler = NewErrorHandler(p.Logger)

	// Order matters: correlation id first so every log line carries it, then
	// logging, then panic recovery closest to the handlers.
	e.Use(CorrelationID(), RequestLogger(p.Logger), Recover(p.Logger))

	for _, r := range p.Routes {
		r.RegisterRoutes(e)
	}
	return e
}

// Server owns the HTTP listener and its lifecycle.
type Server struct {
	cfg    config.HTTPConfig
	logger *slog.Logger
	srv    *http.Server

	mu   sync.Mutex
	addr net.Addr
	done chan struct{}
}

// NewServer wires the HTTP server into the Fx lifecycle: the listener is
// bound on start (so bind errors abort startup) and gracefully drained on
// stop, before dependencies such as database pools are closed.
func NewServer(lc fx.Lifecycle, cfg config.Config, logger *slog.Logger, router *echo.Echo) *Server {
	s := &Server{
		cfg:    cfg.HTTP,
		logger: logger.With(slog.String("component", "http")),
		srv: &http.Server{
			Handler:           router,
			ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
			ReadTimeout:       cfg.HTTP.ReadTimeout,
			WriteTimeout:      cfg.HTTP.WriteTimeout,
			IdleTimeout:       cfg.HTTP.IdleTimeout,
		},
		done: make(chan struct{}),
	}
	lc.Append(fx.Hook{OnStart: s.start, OnStop: s.stop})
	return s
}

func (s *Server) start(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("http listen on %s: %w", s.cfg.Addr, err)
	}

	s.mu.Lock()
	s.addr = ln.Addr()
	s.mu.Unlock()

	go func() {
		defer close(s.done)
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("http server stopped unexpectedly", slog.Any("error", err))
		}
	}()

	s.logger.Info("http server started", slog.String("addr", ln.Addr().String()))
	return nil
}

func (s *Server) stop(ctx context.Context) error {
	s.logger.Info("http server shutting down")

	// Bound the drain by our own timeout, without exceeding Fx's deadline.
	ctx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()

	// Shutdown stops accepting new connections and waits for in-flight
	// requests to finish.
	err := s.srv.Shutdown(ctx)
	if err != nil {
		_ = s.srv.Close()
		err = fmt.Errorf("http graceful shutdown: %w", err)
	}

	select {
	case <-s.done:
	case <-ctx.Done():
	}
	s.logger.Info("http server stopped")
	return err
}

// Addr returns the bound address, or nil before start. Useful in tests that
// listen on port 0.
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}
