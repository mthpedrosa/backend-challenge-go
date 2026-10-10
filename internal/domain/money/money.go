package money

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
)

// Strict decimal regex requiring exactly two decimal places.
// Rejects signs (+/-), scientific notation, comma, whitespace, and any scale other than 2.
var decimalPattern = regexp.MustCompile(`^[0-9]+\.[0-9]{2}$`)

// Supported ISO 4217 currencies.
// BRL is the primary currency; USD is supported for currency mismatch testing.
const (
	CurrencyBRL = "BRL"
	CurrencyUSD = "USD"
)

var supportedCurrencies = map[string]struct{}{
	CurrencyBRL: {},
	CurrencyUSD: {},
}

// Money is an immutable Value Object representing monetary amounts with exact precision.
// It uses int64 minor units (cents), ensuring values never pass through floating-point types.
type Money struct {
	minorUnits int64
	currency   string
}

// IsValidCurrency checks whether the provided ISO 4217 currency code is supported by the domain.
func IsValidCurrency(c string) bool {
	_, ok := supportedCurrencies[c]
	return ok
}

// Zero returns a zero monetary amount for the given currency.
func Zero(currency string) (Money, error) {
	if !IsValidCurrency(currency) {
		return Money{}, domain.WrapDomainError(
			domain.CodeUnsupportedCurrency,
			fmt.Sprintf("unsupported currency: %s", currency),
			nil,
		)
	}
	return Money{minorUnits: 0, currency: currency}, nil
}

// FromMinor constructs Money directly from minor units (cents).
// Used during rehydration from storage and for internal calculations.
func FromMinor(minorUnits int64, currency string) (Money, error) {
	if !IsValidCurrency(currency) {
		return Money{}, domain.WrapDomainError(
			domain.CodeUnsupportedCurrency,
			fmt.Sprintf("unsupported currency: %s", currency),
			nil,
		)
	}
	return Money{minorUnits: minorUnits, currency: currency}, nil
}

// Parse parses a decimal string (e.g. "25.00") into Money without using floats.
// It rejects empty strings, negatives, scale != 2, scientific notation, NaN, and arithmetic overflow.
func Parse(amountStr, currency string) (Money, error) {
	if !IsValidCurrency(currency) {
		return Money{}, domain.WrapDomainError(
			domain.CodeUnsupportedCurrency,
			fmt.Sprintf("unsupported currency: %s", currency),
			nil,
		)
	}

	if !decimalPattern.MatchString(amountStr) {
		return Money{}, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			fmt.Sprintf("invalid decimal amount format %q: must match ^[0-9]+\\.[0-9]{2}$", amountStr),
			nil,
		)
	}

	parts := strings.Split(amountStr, ".")
	wholePartStr, centsPartStr := parts[0], parts[1]

	wholeUnits, err := strconv.ParseInt(wholePartStr, 10, 64)
	if err != nil {
		return Money{}, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			fmt.Sprintf("overflow parsing whole units %q", wholePartStr),
			err,
		)
	}

	centsUnits, err := strconv.ParseInt(centsPartStr, 10, 64)
	if err != nil {
		return Money{}, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			fmt.Sprintf("invalid cents %q", centsPartStr),
			err,
		)
	}

	// Guard against overflow when multiplying whole units by 100
	if wholeUnits > math.MaxInt64/100 {
		return Money{}, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			fmt.Sprintf("overflow multiplying %d by 100", wholeUnits),
			nil,
		)
	}

	minorUnits := wholeUnits * 100
	if minorUnits > math.MaxInt64-centsUnits {
		return Money{}, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			fmt.Sprintf("overflow adding cents %d to %d", centsUnits, minorUnits),
			nil,
		)
	}
	minorUnits += centsUnits

	return Money{minorUnits: minorUnits, currency: currency}, nil
}

// MinorUnits returns total minor units (cents).
func (m Money) MinorUnits() int64 {
	return m.minorUnits
}

// Currency returns the ISO 4217 currency code.
func (m Money) Currency() string {
	return m.currency
}

// IsZero reports whether the amount is zero.
func (m Money) IsZero() bool {
	return m.minorUnits == 0
}

// IsPositive reports whether the amount is strictly positive (> 0).
func (m Money) IsPositive() bool {
	return m.minorUnits > 0
}

// IsNegative reports whether the amount is strictly negative (< 0).
func (m Money) IsNegative() bool {
	return m.minorUnits < 0
}

// FormatDecimal returns the decimal string representation with two places (e.g. "25.00" or "-5.00").
// Computed purely using integer arithmetic without floating-point conversion.
func (m Money) FormatDecimal() string {
	sign := ""
	val := m.minorUnits
	if val < 0 {
		sign = "-"
		if val == math.MinInt64 {
			// Special handling for overflow on -math.MinInt64
			// math.MinInt64 is -9223372036854775808
			return "-92233720368547758.08"
		}
		val = -val
	}
	whole := val / 100
	cents := val % 100
	return fmt.Sprintf("%s%d.%02d", sign, whole, cents)
}

// Equal compares value and currency equality.
func (m Money) Equal(other Money) bool {
	return m.minorUnits == other.minorUnits && m.currency == other.currency
}

// Add adds two monetary amounts of the same currency with overflow checking.
func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, domain.WrapDomainError(
			domain.CodeCurrencyMismatch,
			fmt.Sprintf("cannot add different currencies: %s and %s", m.currency, other.currency),
			nil,
		)
	}

	a := m.minorUnits
	b := other.minorUnits

	// Check for addition overflow
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return Money{}, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			fmt.Sprintf("arithmetic overflow adding %d to %d", b, a),
			nil,
		)
	}

	return Money{minorUnits: a + b, currency: m.currency}, nil
}

// Sub subtracts another monetary amount of the same currency with overflow checking.
func (m Money) Sub(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, domain.WrapDomainError(
			domain.CodeCurrencyMismatch,
			fmt.Sprintf("cannot subtract different currencies: %s and %s", m.currency, other.currency),
			nil,
		)
	}

	a := m.minorUnits
	b := other.minorUnits

	// Check for subtraction overflow
	if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
		return Money{}, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			fmt.Sprintf("arithmetic overflow subtracting %d from %d", b, a),
			nil,
		)
	}

	return Money{minorUnits: a - b, currency: m.currency}, nil
}

// Neg negates the amount with overflow protection against MinInt64.
func (m Money) Neg() (Money, error) {
	if m.minorUnits == math.MinInt64 {
		return Money{}, domain.WrapDomainError(
			domain.CodeInvalidAmount,
			"arithmetic overflow negating MinInt64",
			nil,
		)
	}
	return Money{minorUnits: -m.minorUnits, currency: m.currency}, nil
}

// Compare compares two monetary amounts of the same currency:
// returns -1 if m < other, 0 if m == other, and 1 if m > other.
func (m Money) Compare(other Money) (int, error) {
	if m.currency != other.currency {
		return 0, domain.WrapDomainError(
			domain.CodeCurrencyMismatch,
			fmt.Sprintf("cannot compare different currencies: %s and %s", m.currency, other.currency),
			nil,
		)
	}
	if m.minorUnits < other.minorUnits {
		return -1, nil
	}
	if m.minorUnits > other.minorUnits {
		return 1, nil
	}
	return 0, nil
}

// JSON DTO for Money: {"amount": "25.00", "currency": "BRL"}
type moneyJSON struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(moneyJSON{
		Amount:   m.FormatDecimal(),
		Currency: m.currency,
	})
}

func (m *Money) UnmarshalJSON(data []byte) error {
	var j moneyJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return err
	}
	parsed, err := Parse(j.Amount, j.Currency)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}
