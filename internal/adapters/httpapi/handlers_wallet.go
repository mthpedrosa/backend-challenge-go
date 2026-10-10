package httpapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/mthpedrosa/backend-challenge-go/internal/app/usecase"
)

type WalletHandler struct {
	openWalletUC *usecase.OpenWalletUseCase
	readWalletUC *usecase.ReadWalletUseCase
	readLedgerUC *usecase.ReadLedgerUseCase
}

func NewWalletHandler(openUC *usecase.OpenWalletUseCase, readUC *usecase.ReadWalletUseCase, ledgerUC *usecase.ReadLedgerUseCase) *WalletHandler {
	return &WalletHandler{openWalletUC: openUC, readWalletUC: readUC, readLedgerUC: ledgerUC}
}

func (h *WalletHandler) RegisterRoutes(e *echo.Echo) {
	e.POST("/wallets", h.OpenWallet)
	e.GET("/wallets/:walletId", h.GetWallet)
	e.GET("/wallets/:walletId/ledger", h.GetLedger)
}

type OpenWalletRequest struct {
	PlayerID       string `json:"playerId"`
	Currency       string `json:"currency"`
	InitialBalance string `json:"initialBalance"`
}

type OpenWalletResponse struct {
	WalletID string `json:"walletId"`
}

func (h *WalletHandler) OpenWallet(c echo.Context) error {
	var req OpenWalletRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	w, err := h.openWalletUC.Execute(c.Request().Context(), usecase.OpenWalletInput{
		PlayerID:       req.PlayerID,
		Currency:       req.Currency,
		InitialBalance: req.InitialBalance,
	})
	if err != nil {
		return err // handled by ErrorHandler
	}

	return c.JSON(http.StatusCreated, OpenWalletResponse{WalletID: w.ID()})
}

type WalletResponse struct {
	WalletID string `json:"walletId"`
	PlayerID string `json:"playerId"`
	Balance  string `json:"balance"`
	Currency string `json:"currency"`
	Version  int64  `json:"version"`
}

func (h *WalletHandler) GetWallet(c echo.Context) error {
	id := c.Param("walletId")
	w, err := h.readWalletUC.Execute(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, WalletResponse{
		WalletID: w.ID(),
		PlayerID: w.PlayerID(),
		Balance:  w.Balance().FormatDecimal(),
		Currency: w.Currency(),
		Version:  w.Version(),
	})
}

type LedgerEntryResponse struct {
	ID            string `json:"id"`
	TransactionID string `json:"transactionId"`
	Direction     string `json:"direction"`
	Amount        string `json:"amount"`
	BalanceBefore string `json:"balanceBefore"`
	BalanceAfter  string `json:"balanceAfter"`
	CreatedAt     string `json:"createdAt"`
}

type LedgerResponse struct {
	Items      []LedgerEntryResponse `json:"items"`
	NextCursor *string               `json:"nextCursor,omitempty"`
}

func (h *WalletHandler) GetLedger(c echo.Context) error {
	id := c.Param("walletId")
	cursor := c.QueryParam("cursor")
	// Use 50 as default
	limit := 50

	entries, err := h.readLedgerUC.Execute(c.Request().Context(), id, cursor, limit)
	if err != nil {
		return err
	}

	var res []LedgerEntryResponse
	for _, e := range entries {
		res = append(res, LedgerEntryResponse{
			ID:            e.ID(),
			TransactionID: e.TransactionID(),
			Direction:     string(e.Direction()),
			Amount:        e.Amount().FormatDecimal(),
			BalanceBefore: e.BalanceBefore().FormatDecimal(),
			BalanceAfter:  e.BalanceAfter().FormatDecimal(),
			CreatedAt:     e.CreatedAt().Format("2006-01-02T15:04:05.999Z"),
		})
	}

	// Basic implementation for cursor (using the last item ID as next cursor if we got the limit, though we'd typically fetch limit+1)
	var nextCursor *string
	if len(entries) == limit && limit > 0 {
		cursorStr := entries[len(entries)-1].ID()
		nextCursor = &cursorStr
	}

	if res == nil {
		res = []LedgerEntryResponse{}
	}

	return c.JSON(http.StatusOK, LedgerResponse{
		Items:      res,
		NextCursor: nextCursor,
	})
}
