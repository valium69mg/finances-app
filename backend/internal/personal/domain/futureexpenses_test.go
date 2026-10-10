package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

func TestFutureErrorsAreTheOwnersValues(t *testing.T) {
	if _, err := domain.ValidateFuture(domain.FutureInput{Name: " ", Target: decimal.NewFromInt(1), DueDate: "2026-12-01"}); !errors.Is(err, domain.ErrFutureInvalid) {
		t.Errorf("empty name error = %v; want ErrFutureInvalid", err)
	}
	if err := domain.ValidateMoney("amount", decimal.RequireFromString("0.001")); !errors.Is(err, domain.ErrFutureInvalid) {
		t.Errorf("3 decimals error = %v; want ErrFutureInvalid", err)
	}
	if domain.SavingsCategory != "Gastos futuros" {
		t.Errorf("savings category = %q", domain.SavingsCategory)
	}
}

func TestReleaseKeepsTheOwnersRule(t *testing.T) {
	linked, rem := domain.Release(decimal.NewFromInt(500), decimal.NewFromInt(300))
	if !linked.Equal(decimal.NewFromInt(500)) || !rem.Equal(decimal.NewFromInt(200)) {
		t.Errorf("saved 500 paid 300: linked %s remainder %s; want 500 and 200", linked, rem)
	}
	linked, rem = domain.Release(decimal.NewFromInt(100), decimal.NewFromInt(300))
	if !linked.Equal(decimal.NewFromInt(100)) || !rem.IsZero() {
		t.Errorf("saved 100 paid 300: linked %s remainder %s; want 100 and 0", linked, rem)
	}
}

func TestTrimDescription(t *testing.T) {
	if got := domain.TrimDescription("  Viaje  "); got != "Viaje" {
		t.Errorf("got %q", got)
	}
	long := strings.Repeat("é", 200)
	if got := domain.TrimDescription(long); len([]rune(got)) != domain.MaxDescriptionLength {
		t.Errorf("trimmed to %d runes, want %d", len([]rune(got)), domain.MaxDescriptionLength)
	}
}

func TestValidateSavingDescription(t *testing.T) {
	if got, err := domain.ValidateSavingDescription("  hola "); err != nil || got != "hola" {
		t.Errorf("got %q, %v", got, err)
	}
	if got, err := domain.ValidateSavingDescription(""); err != nil || got != "" {
		t.Errorf("empty must be allowed, got %q, %v", got, err)
	}
	if _, err := domain.ValidateSavingDescription(strings.Repeat("a", 121)); !errors.Is(err, domain.ErrFutureInvalid) {
		t.Errorf("121 chars error = %v", err)
	}
}
