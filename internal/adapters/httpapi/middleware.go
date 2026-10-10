package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/mthpedrosa/backend-challenge-go/internal/platform/correlation"
)

// maxCorrelationIDLength bounds client supplied ids to avoid log abuse.
const maxCorrelationIDLength = 128

// CorrelationID reuses a valid X-Correlation-ID header or generates a new
// one, stores it in the request context and echoes it in the response.
func CorrelationID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			id := c.Request().Header.Get(correlation.Header)
			if id == "" || len(id) > maxCorrelationIDLength {
				id = uuid.NewString()
			}
			req := c.Request()
			c.SetRequest(req.WithContext(correlation.WithID(req.Context(), id)))
			c.Response().Header().Set(correlation.Header, id)
			return next(c)
		}
	}
}

// RequestLogger logs one structured line per request. Bodies and headers are
// never logged to avoid leaking credentials or financial payloads.
func RequestLogger(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			if err != nil {
				// Let the error handler write the response so the status is known.
				c.Error(err)
			}
			req := c.Request()
			status := c.Response().Status
			level := slog.LevelInfo
			switch {
			case status >= http.StatusInternalServerError:
				level = slog.LevelError
			case status >= http.StatusBadRequest:
				level = slog.LevelWarn
			}
			logger.LogAttrs(req.Context(), level, "http request",
				slog.String("method", req.Method),
				slog.String("route", c.Path()),
				slog.Int("status", status),
				slog.Duration("latency", time.Since(start)),
			)
			return nil
		}
	}
}

// Recover converts panics into 500 responses and logs the stack trace.
func Recover(logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					if r == http.ErrAbortHandler {
						panic(r)
					}
					logger.ErrorContext(c.Request().Context(), "panic recovered",
						slog.Any("panic", r),
						slog.String("stack", string(debug.Stack())),
					)
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			return next(c)
		}
	}
}
