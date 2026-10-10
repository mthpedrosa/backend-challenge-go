package httpapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HealthHandler exposes public liveness (and, in later phases, readiness)
// probes.
type HealthHandler struct{}

// NewHealthHandler builds the health handler.
func NewHealthHandler() *HealthHandler { return &HealthHandler{} }

type healthResponse struct {
	Status string `json:"status"`
}

// RegisterRoutes implements Routes.
func (h *HealthHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/health/live", h.Live)
}

// Live reports that the process is up and serving requests. It must not
// depend on external systems.
func (h *HealthHandler) Live(c echo.Context) error {
	return c.JSON(http.StatusOK, healthResponse{Status: "ok"})
}
