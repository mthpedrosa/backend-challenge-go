package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
)

// ErrorBody is the JSON envelope returned for every error response.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries a stable machine readable code and a human message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Stable error codes owned by the transport layer. Domain codes are mapped
// here in later phases.
const (
	CodeNotFound         = "NOT_FOUND"
	CodeMethodNotAllowed = "METHOD_NOT_ALLOWED"
	CodeBadRequest       = "BAD_REQUEST"
	CodeInternal         = "INTERNAL_ERROR"
)

// NewErrorHandler returns the central Echo error handler, translating errors
// into the public error contract.
func NewErrorHandler(logger *slog.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		status, body := http.StatusInternalServerError, ErrorBody{Error: ErrorDetail{
			Code:    CodeInternal,
			Message: "internal server error",
		}}

		var he *echo.HTTPError
		var de *domain.DomainError
		if errors.As(err, &de) {
			status = http.StatusUnprocessableEntity // default 422

			// Custom mappings based on domain rules
			switch de.Code {
			case domain.CodeInvalidAmount, domain.CodeCurrencyMismatch, domain.CodeInvalidKind, "PLAYER_WALLET_MISMATCH", "INVALID_WALLET_STATE":
				status = http.StatusBadRequest
			case "WALLET_ALREADY_EXISTS", "IDEMPOTENCY_CONFLICT":
				status = http.StatusConflict
			case "WALLET_NOT_FOUND", "TRANSACTION_NOT_FOUND":
				status = http.StatusNotFound
			}

			// If it's a rejected transaction, we return 200 OK with the rejection data
			// Wait, the challenge says REJECTED is a 200 OK with `{"status":"REJECTED","failureCode":...}` but this is for wager endpoint.
			// Let's just map domain errors normally here. The handler will return 200 OK for REJECTED transactions.
			body.Error = ErrorDetail{Code: de.Code, Message: de.Message}
		} else if errors.As(err, &he) {
			status = he.Code
			body.Error = ErrorDetail{Code: codeForStatus(he.Code), Message: http.StatusText(he.Code)}
		} else {
			logger.ErrorContext(c.Request().Context(), "unhandled error", slog.Any("error", err))
		}

		var writeErr error
		if c.Request().Method == http.MethodHead {
			writeErr = c.NoContent(status)
		} else {
			writeErr = c.JSON(status, body)
		}
		if writeErr != nil {
			logger.ErrorContext(c.Request().Context(), "failed to write error response", slog.Any("error", writeErr))
		}
	}
}

func codeForStatus(status int) string {
	switch status {
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusMethodNotAllowed:
		return CodeMethodNotAllowed
	case http.StatusBadRequest:
		return CodeBadRequest
	default:
		if status >= http.StatusInternalServerError {
			return CodeInternal
		}
		return http.StatusText(status)
	}
}
