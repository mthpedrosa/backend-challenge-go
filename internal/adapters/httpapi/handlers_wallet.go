package httpapi

import (
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/mthpedrosa/backend-challenge-go/internal/app/usecase"
	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
)

type WalletHandler struct {
	openWalletUC      *usecase.OpenWalletUseCase
	readWalletUC      *usecase.ReadWalletUseCase
	readLedgerUC      *usecase.ReadLedgerUseCase
	reconcileWalletUC *usecase.ReconcileWalletUseCase
}

func NewWalletHandler(openUC *usecase.OpenWalletUseCase, readUC *usecase.ReadWalletUseCase, ledgerUC *usecase.ReadLedgerUseCase, reconcileUC *usecase.ReconcileWalletUseCase) *WalletHandler {
	return &WalletHandler{
		openWalletUC:      openUC,
		readWalletUC:      readUC,
		readLedgerUC:      ledgerUC,
		reconcileWalletUC: reconcileUC,
	}
}

func (h *WalletHandler) RegisterRoutes(e *echo.Echo) {
	e.POST("/wallets", h.OpenWallet)
	e.GET("/wallets/:walletId", h.GetWallet)
	e.GET("/wallets/:walletId/ledger", h.GetLedger)
	e.POST("/wallets/:walletId/reconciliation", h.ReconcileWallet)
}

type InitialBalanceDTO struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type OpenWalletRequest struct {
	PlayerID       string `json:"playerId"`
	Currency       string `json:"currency"`
	InitialBalance any    `json:"initialBalance"`
}

type OpenWalletResponse struct {
	ID       string        `json:"id"`
	WalletID string        `json:"walletId"`
	PlayerID string        `json:"playerId"`
	Balance  MoneyResponse `json:"balance"`
	Version  int64         `json:"version"`
}

func (h *WalletHandler) OpenWallet(c echo.Context) error {
	caller := GetCallerInfo(c)
	if !caller.IsInternal {
		return domain.WrapDomainError(domain.CodeForbidden, "wallet operations are restricted to internal clients", nil)
	}

	var raw map[string]any
	if err := c.Bind(&raw); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	playerID, _ := raw["playerId"].(string)
	currency, _ := raw["currency"].(string)
	initialBalanceStr := "0.00"

	if ib, ok := raw["initialBalance"]; ok {
		switch v := ib.(type) {
		case string:
			initialBalanceStr = v
		case map[string]any:
			if a, ok := v["amount"].(string); ok {
				initialBalanceStr = a
			}
			if cur, ok := v["currency"].(string); ok && cur != "" {
				currency = cur
			}
		}
	}

	if currency == "" {
		currency = "BRL"
	}

	w, err := h.openWalletUC.Execute(c.Request().Context(), usecase.OpenWalletInput{
		PlayerID:       playerID,
		Currency:       currency,
		InitialBalance: initialBalanceStr,
	})
	if err != nil {
		return err // handled by ErrorHandler
	}

	return c.JSON(http.StatusCreated, OpenWalletResponse{
		ID:       w.ID(),
		WalletID: w.ID(),
		PlayerID: w.PlayerID(),
		Balance: MoneyResponse{
			Amount:   w.Balance().FormatDecimal(),
			Currency: w.Currency(),
		},
		Version: w.Version(),
	})
}

type WalletResponse struct {
	ID       string        `json:"id"`
	WalletID string        `json:"walletId"`
	PlayerID string        `json:"playerId"`
	Balance  MoneyResponse `json:"balance"`
	Currency string        `json:"currency"`
	Version  int64         `json:"version"`
}

func (h *WalletHandler) GetWallet(c echo.Context) error {
	caller := GetCallerInfo(c)
	if !caller.IsInternal {
		return domain.WrapDomainError(domain.CodeForbidden, "wallet operations are restricted to internal clients", nil)
	}

	id := c.Param("walletId")
	w, err := h.readWalletUC.Execute(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, WalletResponse{
		ID:       w.ID(),
		WalletID: w.ID(),
		PlayerID: w.PlayerID(),
		Balance: MoneyResponse{
			Amount:   w.Balance().FormatDecimal(),
			Currency: w.Currency(),
		},
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
	caller := GetCallerInfo(c)
	if !caller.IsInternal {
		return domain.WrapDomainError(domain.CodeForbidden, "wallet operations are restricted to internal clients", nil)
	}

	id := c.Param("walletId")
	cursor := c.QueryParam("cursor")
	limit := 50
	if lStr := c.QueryParam("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			if l > 100 {
				limit = 100
			} else {
				limit = l
			}
		}
	}

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

	var nextCursor *string
	if len(entries) == limit && limit > 0 {
		lastEntry := entries[len(entries)-1]
		// Opaque base64 cursor encoding timestamp + id
		rawToken := lastEntry.CreatedAt().Format("2006-01-02T15:04:05.999999999Z07:00") + "|" + lastEntry.ID()
		b64 := base64.URLEncoding.EncodeToString([]byte(rawToken))
		nextCursor = &b64
	}

	if res == nil {
		res = []LedgerEntryResponse{}
	}

	return c.JSON(http.StatusOK, LedgerResponse{
		Items:      res,
		NextCursor: nextCursor,
	})
}

type MoneyResponse struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type ReconcileWalletResponse struct {
	WalletID          string        `json:"walletId"`
	StoredBalance     MoneyResponse `json:"storedBalance"`
	CalculatedBalance MoneyResponse `json:"calculatedBalance"`
	Difference        MoneyResponse `json:"difference"`
	Consistent        bool          `json:"consistent"`
	CheckedEntries    int64         `json:"checkedEntries"`
}

func (h *WalletHandler) ReconcileWallet(c echo.Context) error {
	caller := GetCallerInfo(c)
	if !caller.IsInternal {
		return domain.WrapDomainError(domain.CodeForbidden, "wallet operations are restricted to internal clients", nil)
	}

	id := c.Param("walletId")
	out, err := h.reconcileWalletUC.Execute(c.Request().Context(), id)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, ReconcileWalletResponse{
		WalletID: out.WalletID,
		StoredBalance: MoneyResponse{
			Amount:   out.StoredBalance.FormatDecimal(),
			Currency: out.StoredBalance.Currency(),
		},
		CalculatedBalance: MoneyResponse{
			Amount:   out.CalculatedBalance.FormatDecimal(),
			Currency: out.CalculatedBalance.Currency(),
		},
		Difference: MoneyResponse{
			Amount:   out.Difference.FormatDecimal(),
			Currency: out.Difference.Currency(),
		},
		Consistent:     out.Consistent,
		CheckedEntries: out.CheckedEntries,
	})
}
