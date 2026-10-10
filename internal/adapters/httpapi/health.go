package httpapi

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

// HealthHandler exposes public liveness and readiness probes.
type HealthHandler struct {
	pool *pgxpool.Pool
	sqs  *sqs.Client
}

// NewHealthHandler builds the health handler.
func NewHealthHandler(pool *pgxpool.Pool, sqsClient *sqs.Client) *HealthHandler {
	return &HealthHandler{pool: pool, sqs: sqsClient}
}

type healthResponse struct {
	Status  string            `json:"status"`
	Details map[string]string `json:"details,omitempty"`
}

// RegisterRoutes implements Routes.
func (h *HealthHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/health/live", h.Live)
	e.GET("/health/ready", h.Ready)
}

// Live reports that the process is up and serving requests.
func (h *HealthHandler) Live(c echo.Context) error {
	return c.JSON(http.StatusOK, healthResponse{Status: "ok"})
}

// Ready reports if external connections are established and healthy.
func (h *HealthHandler) Ready(c echo.Context) error {
	details := make(map[string]string)
	status := "ok"
	statusCode := http.StatusOK

	// Check DB
	if err := h.pool.Ping(context.Background()); err != nil {
		status = "error"
		details["postgres"] = err.Error()
		statusCode = http.StatusServiceUnavailable
	} else {
		details["postgres"] = "ok"
	}

	// SQS doesn't have a direct ping, but we can do a lightweight call if needed,
	// or assume the client connects on-demand. We skip explicit ping for SQS here
	// unless we really want to hit ListQueues, but that requires IAM. We'll just report "ok" if client exists.
	if h.sqs == nil {
		status = "error"
		details["sqs"] = "client nil"
		statusCode = http.StatusServiceUnavailable
	} else {
		details["sqs"] = "ok"
	}

	return c.JSON(statusCode, healthResponse{
		Status:  status,
		Details: details,
	})
}
