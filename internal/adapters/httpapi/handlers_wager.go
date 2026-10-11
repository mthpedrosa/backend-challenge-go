package httpapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/mthpedrosa/backend-challenge-go/internal/app/usecase"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain/wager"
)

type WagerHandler struct {
	processWagerUC *usecase.ProcessWagerUseCase
	readWagerUC    *usecase.ReadWagerUseCase
}

func NewWagerHandler(processUC *usecase.ProcessWagerUseCase, readUC *usecase.ReadWagerUseCase) *WagerHandler {
	return &WagerHandler{processWagerUC: processUC, readWagerUC: readUC}
}

func (h *WagerHandler) RegisterRoutes(e *echo.Echo) {
	e.POST("/wagering/transactions", h.ProcessWager)
	e.GET("/wagering/transactions/:transactionId", h.GetTransaction)
	e.GET("/providers/:providerId/wagering/transactions/:externalTransactionId", h.GetExternalTransaction)
}

type ProcessWagerRequest struct {
	ProviderID            string  `json:"providerId"`
	ExternalTransactionID string  `json:"externalTransactionId"`
	IdempotencyKey        string  `json:"idempotencyKey"`
	PlayerID              string  `json:"playerId"`
	WalletID              string  `json:"walletId"`
	Currency              string  `json:"currency"`
	Amount                string  `json:"amount"`
	Kind                  string  `json:"kind"`
	RoundID               string  `json:"roundId"`
	GameID                string  `json:"gameId"`
	ReferenceExternalID   *string `json:"referenceExternalTransactionId,omitempty"`
}

type ProcessWagerResponse struct {
	TransactionID    string         `json:"transactionId"`
	Status           string         `json:"status"`
	Balance          *MoneyResponse `json:"balance,omitempty"`
	BalanceAfter     *string        `json:"balanceAfter,omitempty"`
	FailureCode      *string        `json:"failureCode,omitempty"`
	IdempotentReplay bool           `json:"idempotentReplay"`
}

func (h *WagerHandler) ProcessWager(c echo.Context) error {
	var req ProcessWagerRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	caller := GetCallerInfo(c)
	if !caller.IsInternal && caller.ClientID != "" && caller.ClientID != req.ProviderID {
		return domain.WrapDomainError(domain.CodeForbidden, "provider ID mismatch with authenticated client", nil)
	}

	result, err := h.processWagerUC.Execute(c.Request().Context(), usecase.ProcessWagerInput{
		ProviderID:            req.ProviderID,
		ExternalTransactionID: req.ExternalTransactionID,
		IdempotencyKey:        req.IdempotencyKey,
		PlayerID:              req.PlayerID,
		WalletID:              req.WalletID,
		Currency:              req.Currency,
		Amount:                req.Amount,
		Kind:                  req.Kind,
		RoundID:               req.RoundID,
		GameID:                req.GameID,
		ReferenceExternalID:   req.ReferenceExternalID,
	})
	if err != nil {
		return err // Handled by ErrorHandler (400, 404, 409, 500)
	}

	tx := result.Transaction
	res := ProcessWagerResponse{
		TransactionID:    tx.ID(),
		Status:           string(tx.Status()),
		IdempotentReplay: result.IdempotentReplay,
	}

	if tx.Status() == wager.StatusProcessed {
		bal := tx.BalanceAfter().FormatDecimal()
		res.BalanceAfter = &bal
		res.Balance = &MoneyResponse{
			Amount:   bal,
			Currency: tx.Amount().Currency(),
		}
		return c.JSON(http.StatusOK, res)
	}

	if tx.Status() == wager.StatusRejected {
		res.FailureCode = tx.FailureCode()
		return c.JSON(http.StatusOK, res) // Business rejections return 200 OK
	}

	if tx.Status() == wager.StatusPendingReference {
		return c.JSON(http.StatusAccepted, res)
	}

	return c.JSON(http.StatusOK, res)
}

type TransactionResponse struct {
	ID                             string  `json:"id"`
	WalletID                       string  `json:"walletId"`
	PlayerID                       string  `json:"playerId"`
	ProviderID                     *string `json:"providerId,omitempty"`
	ExternalTransactionID          *string `json:"externalTransactionId,omitempty"`
	Kind                           string  `json:"kind"`
	Amount                         string  `json:"amount"`
	Currency                       string  `json:"currency"`
	Status                         string  `json:"status"`
	FailureCode                    *string `json:"failureCode,omitempty"`
	ReferenceExternalTransactionID *string `json:"referenceExternalTransactionId,omitempty"`
	CreatedAt                      string  `json:"createdAt"`
}

func toTransactionResponse(tx *wager.Transaction) TransactionResponse {
	return TransactionResponse{
		ID:                             tx.ID(),
		WalletID:                       tx.WalletID(),
		PlayerID:                       tx.PlayerID(),
		ProviderID:                     tx.ProviderID(),
		ExternalTransactionID:          tx.ExternalTransactionID(),
		Kind:                           string(tx.Kind()),
		Amount:                         tx.Amount().FormatDecimal(),
		Currency:                       tx.Amount().Currency(),
		Status:                         string(tx.Status()),
		FailureCode:                    tx.FailureCode(),
		ReferenceExternalTransactionID: tx.ReferenceExternalTransactionID(),
		CreatedAt:                      tx.CreatedAt().Format("2006-01-02T15:04:05.999Z"),
	}
}

func (h *WagerHandler) GetTransaction(c echo.Context) error {
	id := c.Param("transactionId")
	tx, err := h.readWagerUC.GetByID(c.Request().Context(), id)
	if err != nil {
		return err
	}

	caller := GetCallerInfo(c)
	if !caller.IsInternal && caller.ClientID != "" && tx.ProviderID() != nil && *tx.ProviderID() != caller.ClientID {
		return domain.WrapDomainError(domain.CodeNotFound, "transaction not found", nil)
	}

	return c.JSON(http.StatusOK, toTransactionResponse(tx))
}

func (h *WagerHandler) GetExternalTransaction(c echo.Context) error {
	providerID := c.Param("providerId")
	extID := c.Param("externalTransactionId")

	caller := GetCallerInfo(c)
	if !caller.IsInternal && caller.ClientID != "" && caller.ClientID != providerID {
		return domain.WrapDomainError(domain.CodeForbidden, "forbidden provider access", nil)
	}

	tx, err := h.readWagerUC.GetByExternalID(c.Request().Context(), providerID, extID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toTransactionResponse(tx))
}
