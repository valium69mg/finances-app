package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestParseKind(t *testing.T) {
	for in, want := range map[string]domain.Kind{"Ingreso": domain.KindIncome, " Gasto ": domain.KindExpense, "Ahorro": domain.KindSavings} {
		got, err := domain.ParseKind(in)
		if err != nil || got != want {
			t.Errorf("ParseKind(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "gasto", "Otro"} {
		if _, err := domain.ParseKind(in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("ParseKind(%q) error = %v; want ErrInvalidInput", in, err)
		}
	}
}

func TestOnlyExpenseCategoriesAreEditable(t *testing.T) {
	if domain.KindIncome.Editable() || domain.KindSavings.Editable() || !domain.KindExpense.Editable() {
		t.Error("only Gasto categories must be editable")
	}
}

func TestDefaults(t *testing.T) {
	if got := len(domain.Defaults[domain.KindIncome]); got != 3 {
		t.Errorf("income defaults = %d, want 3", got)
	}
	if got := len(domain.Defaults[domain.KindExpense]); got != 8 {
		t.Errorf("expense defaults = %d, want 8", got)
	}
	if got := domain.Defaults[domain.KindSavings]; len(got) != 3 || got[0] != "Ahorro" || got[1] != "Inversión" || got[2] != "Gastos futuros" {
		t.Errorf("savings defaults = %v", got)
	}
}

func TestValidateCategoryName(t *testing.T) {
	if got, err := domain.ValidateCategoryName("  Cafetería "); err != nil || got != "Cafetería" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, in := range []string{"", "   ", strings.Repeat("a", domain.MaxNameLength+1)} {
		if _, err := domain.ValidateCategoryName(in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("ValidateCategoryName(%q) error = %v; want ErrInvalidInput", in, err)
		}
	}
	if _, err := domain.ValidateCategoryName(strings.Repeat("é", domain.MaxNameLength)); err != nil {
		t.Errorf("a name of exactly %d characters must be valid: %v", domain.MaxNameLength, err)
	}
}

func TestValidateAmount(t *testing.T) {
	cases := []struct {
		name   string
		kind   domain.Kind
		amount string
		ok     bool
	}{
		{"expense positive", domain.KindExpense, "10.50", true},
		{"expense zero", domain.KindExpense, "0", false},
		{"expense negative", domain.KindExpense, "-1", false},
		{"income negative", domain.KindIncome, "-1", false},
		{"savings contribution", domain.KindSavings, "100", true},
		{"savings withdrawal", domain.KindSavings, "-100", true},
		{"savings zero", domain.KindSavings, "0", false},
		{"three decimals", domain.KindExpense, "1.001", false},
		{"at the limit", domain.KindExpense, "1000000000000", false},
		{"below the limit", domain.KindExpense, "999999999999.99", true},
		{"absurd exponent", domain.KindExpense, "1e999999999", false},
		{"absurd negative savings", domain.KindSavings, "-1e999999999", false},
	}
	for _, c := range cases {
		err := domain.ValidateAmount(c.kind, d(c.amount))
		if c.ok && err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
		if !c.ok && !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: error = %v; want ErrInvalidInput", c.name, err)
		}
	}
}

func TestValidateMovement(t *testing.T) {
	good := domain.MovementInput{Kind: "Gasto", CategoryID: 3, Amount: d("25.5"), Description: "  Crema ", Date: "2026-10-03"}
	got, err := domain.ValidateMovement(good)
	if err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if got.Kind != domain.KindExpense || got.Description != "Crema" || got.CategoryID != 3 || got.Date != "2026-10-03" {
		t.Errorf("got %+v", got)
	}

	bad := map[string]func(*domain.MovementInput){
		"kind":              func(in *domain.MovementInput) { in.Kind = "x" },
		"category":          func(in *domain.MovementInput) { in.CategoryID = 0 },
		"amount":            func(in *domain.MovementInput) { in.Amount = d("0") },
		"empty description": func(in *domain.MovementInput) { in.Description = " " },
		"long description":  func(in *domain.MovementInput) { in.Description = strings.Repeat("a", domain.MaxDescriptionLength+1) },
		"date":              func(in *domain.MovementInput) { in.Date = "03/10/2026" },
		"empty date":        func(in *domain.MovementInput) { in.Date = "" },
	}
	for name, mutate := range bad {
		in := good
		mutate(&in)
		if _, err := domain.ValidateMovement(in); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: error = %v; want ErrInvalidInput", name, err)
		}
	}
}
