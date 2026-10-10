package money

import (
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/mthpedrosa/backend-challenge-go/internal/domain"
)

func TestParse_ValidAmounts(t *testing.T) {
	tests := []struct {
		amountStr string
		currency  string
		wantMinor int64
		wantStr   string
	}{
		{"0.00", CurrencyBRL, 0, "0.00"},
		{"0.01", CurrencyBRL, 1, "0.01"},
		{"0.99", CurrencyBRL, 99, "0.99"},
		{"1.00", CurrencyBRL, 100, "1.00"},
		{"25.00", CurrencyBRL, 2500, "25.00"},
		{"123456.78", CurrencyBRL, 12345678, "123456.78"},
		{"92233720368547758.07", CurrencyBRL, 9223372036854775807, "92233720368547758.07"}, // MaxInt64
	}

	for _, tt := range tests {
		t.Run(tt.amountStr, func(t *testing.T) {
			m, err := Parse(tt.amountStr, tt.currency)
			if err != nil {
				t.Fatalf("unexpected error parsing %q: %v", tt.amountStr, err)
			}
			if m.MinorUnits() != tt.wantMinor {
				t.Errorf("MinorUnits() = %d, want %d", m.MinorUnits(), tt.wantMinor)
			}
			if m.Currency() != tt.currency {
				t.Errorf("Currency() = %q, want %q", m.Currency(), tt.currency)
			}
			if m.FormatDecimal() != tt.wantStr {
				t.Errorf("FormatDecimal() = %q, want %q", m.FormatDecimal(), tt.wantStr)
			}
		})
	}
}

func TestParse_InvalidInputs(t *testing.T) {
	invalid := []string{
		"",
		" ",
		"-25.00",
		"+25.00",
		"25",
		"25.",
		"25.0",
		"25.000",
		"25,00",
		"abc",
		"NaN",
		"Infinity",
		"-Infinity",
		"1e3",
		"1.5e2",
		"999999999999999999999999999999999999.00", // overflow
	}

	for _, s := range invalid {
		t.Run(s, func(t *testing.T) {
			_, err := Parse(s, CurrencyBRL)
			if err == nil {
				t.Fatalf("expected error parsing invalid amount %q, got nil", s)
			}
			var domErr *domain.DomainError
			if !errors.As(err, &domErr) {
				t.Fatalf("expected *domain.DomainError, got %T: %v", err, err)
			}
			if domErr.Code != domain.CodeInvalidAmount {
				t.Errorf("error code = %q, want %q", domErr.Code, domain.CodeInvalidAmount)
			}
		})
	}
}

func TestParse_UnsupportedCurrency(t *testing.T) {
	_, err := Parse("25.00", "EUR")
	if err == nil {
		t.Fatal("expected error for unsupported currency EUR, got nil")
	}
	var domErr *domain.DomainError
	if !errors.As(err, &domErr) || domErr.Code != domain.CodeUnsupportedCurrency {
		t.Errorf("expected error code %q, got %v", domain.CodeUnsupportedCurrency, err)
	}
}

func TestArithmetic_AddAndSub(t *testing.T) {
	m10, _ := Parse("10.00", CurrencyBRL)
	m25, _ := Parse("25.00", CurrencyBRL)

	sum, err := m10.Add(m25)
	if err != nil {
		t.Fatalf("unexpected error adding: %v", err)
	}
	if sum.FormatDecimal() != "35.00" {
		t.Errorf("sum = %s, want 35.00", sum.FormatDecimal())
	}

	sub, err := m25.Sub(m10)
	if err != nil {
		t.Fatalf("unexpected error subtracting: %v", err)
	}
	if sub.FormatDecimal() != "15.00" {
		t.Errorf("sub = %s, want 15.00", sub.FormatDecimal())
	}
}

func TestArithmetic_Overflow(t *testing.T) {
	maxM, err := FromMinor(math.MaxInt64, CurrencyBRL)
	if err != nil {
		t.Fatalf("failed to create max money: %v", err)
	}
	one, _ := FromMinor(1, CurrencyBRL)

	// Overflow on Add
	_, err = maxM.Add(one)
	if err == nil {
		t.Fatal("expected overflow on Add, got nil")
	}

	// Overflow on Sub
	minM, err := FromMinor(math.MinInt64, CurrencyBRL)
	if err != nil {
		t.Fatalf("failed to create min money: %v", err)
	}
	_, err = minM.Sub(one)
	if err == nil {
		t.Fatal("expected overflow on Sub, got nil")
	}

	// Overflow on Neg
	_, err = minM.Neg()
	if err == nil {
		t.Fatal("expected overflow on Neg(MinInt64), got nil")
	}
}

func TestArithmetic_CurrencyMismatch(t *testing.T) {
	brl, _ := Parse("10.00", CurrencyBRL)
	usd, _ := Parse("10.00", CurrencyUSD)

	if _, err := brl.Add(usd); err == nil {
		t.Error("expected currency mismatch on Add")
	}
	if _, err := brl.Sub(usd); err == nil {
		t.Error("expected currency mismatch on Sub")
	}
	if _, err := brl.Compare(usd); err == nil {
		t.Error("expected currency mismatch on Compare")
	}
}

func TestComparisonAndPredicates(t *testing.T) {
	zero, _ := Zero(CurrencyBRL)
	pos, _ := Parse("10.00", CurrencyBRL)
	neg, _ := FromMinor(-1000, CurrencyBRL)

	if !zero.IsZero() || pos.IsZero() || neg.IsZero() {
		t.Error("incorrect IsZero result")
	}
	if !pos.IsPositive() || zero.IsPositive() || neg.IsPositive() {
		t.Error("incorrect IsPositive result")
	}
	if !neg.IsNegative() || zero.IsNegative() || pos.IsNegative() {
		t.Error("incorrect IsNegative result")
	}

	cmp, err := pos.Compare(zero)
	if err != nil || cmp != 1 {
		t.Errorf("Compare pos > zero: cmp=%d, err=%v", cmp, err)
	}

	cmp, err = neg.Compare(zero)
	if err != nil || cmp != -1 {
		t.Errorf("Compare neg < zero: cmp=%d, err=%v", cmp, err)
	}

	cmp, err = zero.Compare(zero)
	if err != nil || cmp != 0 {
		t.Errorf("Compare zero == zero: cmp=%d, err=%v", cmp, err)
	}
}

func TestJSONSerialization(t *testing.T) {
	m, _ := Parse("25.00", CurrencyBRL)

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal money: %v", err)
	}
	wantJSON := `{"amount":"25.00","currency":"BRL"}`
	if string(data) != wantJSON {
		t.Errorf("JSON = %s, want %s", string(data), wantJSON)
	}

	var decoded Money
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal money: %v", err)
	}
	if !decoded.Equal(m) {
		t.Errorf("decoded %v != original %v", decoded, m)
	}
}
