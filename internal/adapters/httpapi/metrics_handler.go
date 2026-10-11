package httpapi

import (
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricsHandler struct{}

func NewMetricsHandler() *MetricsHandler {
	return &MetricsHandler{}
}

func (h *MetricsHandler) RegisterRoutes(e *echo.Echo) {
	// Expose prometheus metrics
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
}
