package domain

import (
	"errors"
	"fmt"
)

// Stable failure codes for audit logs, API error contracts, and messaging envelopes.
const (
	// Recoverable input validation errors (rejected at the perimeter, transaction not persisted)
	CodeInvalidAmount          = "INVALID_AMOUNT"
	CodeCurrencyMismatch       = "CURRENCY_MISMATCH"
	CodeUnsupportedCurrency    = "UNSUPPORTED_CURRENCY"
	CodeInvalidKind            = "INVALID_KIND"
	CodeInvalidStateTransition = "INVALID_STATE_TRANSITION"
	CodeInvalidLedgerEntry     = "INVALID_LEDGER_ENTRY"
	CodeInvalidWalletState     = "INVALID_WALLET_STATE"
	CodeUnauthorized           = "UNAUTHORIZED"

	// Permanent business rejections (persisted as REJECTED with stable failureCode)
	CodeInsufficientFunds         = "INSUFFICIENT_FUNDS"
	CodeReversalInsufficientFunds = "REVERSAL_INSUFFICIENT_FUNDS"
	CodeReferenceNotFound         = "REFERENCE_NOT_FOUND"
	CodeReferenceMismatch         = "REFERENCE_MISMATCH"
	CodeAmountMismatch            = "AMOUNT_MISMATCH"
	CodeAlreadyReversed           = "ALREADY_REVERSED"
	CodeInvalidReferenceKind      = "INVALID_REFERENCE_KIND"
	CodeReferenceNotProcessed     = "REFERENCE_NOT_PROCESSED"
)

// DomainError represents a strongly-typed and classifiable domain error.
type DomainError struct {
	Code    string
	Message string
	Err     error
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

// Is enables errors.Is support by comparing the stable error code.
func (e *DomainError) Is(target error) bool {
	var t *DomainError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// NewDomainError constructs a new DomainError instance.
func NewDomainError(code, message string) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
	}
}

// WrapDomainError constructs a new DomainError wrapping an underlying cause.
func WrapDomainError(code, message string, err error) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}
