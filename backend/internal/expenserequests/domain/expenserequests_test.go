package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

var cats = []string{"Comida", "Transporte"}

func TestValidate(t *testing.T) {
	good := domain.Input{Amount: d("250.50"), Description: "  Tacos  ", SuggestedCategory: "Comida", Date: "2026-10-03"}
	v, err := domain.Validate(good, cats, "2026-10-01")
	if err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if v.Description != "Tacos" || v.Date != "2026-10-03" || v.SuggestedCategory != "Comida" {
		t.Errorf("validated = %+v", v)
	}

	today, err := domain.Validate(domain.Input{Amount: d("1"), Description: "x"}, cats, "2026-10-01")
	if err != nil || today.Date != "2026-10-01" || today.SuggestedCategory != "" {
		t.Errorf("defaults = %+v, %v", today, err)
	}

	cases := map[string]domain.Input{
		"zero amount":         {Amount: d("0"), Description: "x"},
		"negative amount":     {Amount: d("-1"), Description: "x"},
		"three decimals":      {Amount: d("1.005"), Description: "x"},
		"out of range":        {Amount: d("1000000000000"), Description: "x"},
		"absurd exponent":     {Amount: d("1e999999999"), Description: "x"},
		"blank description":   {Amount: d("1"), Description: "   "},
		"long description":    {Amount: d("1"), Description: strings.Repeat("a", 121)},
		"unknown category":    {Amount: d("1"), Description: "x", SuggestedCategory: "Nope"},
		"category wrong case": {Amount: d("1"), Description: "x", SuggestedCategory: "comida"},
		"bad date":            {Amount: d("1"), Description: "x", Date: "03/10/2026"},
	}
	for name, in := range cases {
		if _, err := domain.Validate(in, cats, "2026-10-01"); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
}

func TestValidateComment(t *testing.T) {
	if c, err := domain.ValidateComment("  No este mes "); err != nil || c != "No este mes" {
		t.Errorf("comment = %q, %v", c, err)
	}
	for _, s := range []string{"", "  ", strings.Repeat("a", 501)} {
		if _, err := domain.ValidateComment(s); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("comment %q accepted", s)
		}
	}
}

func TestParseStatus(t *testing.T) {
	for _, s := range domain.Statuses {
		if got, err := domain.ParseStatus(string(s)); err != nil || got != s {
			t.Errorf("ParseStatus(%q) = %q, %v", s, got, err)
		}
	}
	if _, err := domain.ParseStatus("pending"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("unknown status accepted")
	}
}

func TestNewBudgetCheck(t *testing.T) {
	b := d("1000")
	cases := []struct {
		name                         string
		budget                       *decimal.Decimal
		spent, amount                string
		fits                         *bool
		projected, remaining, overBy string
	}{
		{name: "fits", budget: &b, spent: "700", amount: "200", fits: ptr(true), projected: "100", remaining: "300", overBy: "0"},
		{name: "exactly fits", budget: &b, spent: "700", amount: "300", fits: ptr(true), projected: "0", remaining: "300", overBy: "0"},
		{name: "exceeds", budget: &b, spent: "700", amount: "300.01", fits: ptr(false), projected: "-0.01", remaining: "300", overBy: "0.01"},
		{name: "already over", budget: &b, spent: "1100", amount: "50", fits: ptr(false), projected: "-150", remaining: "-100", overBy: "150"},
		{name: "no budget", budget: nil, spent: "700", amount: "300", fits: nil},
	}
	for _, c := range cases {
		got := domain.NewBudgetCheck("Comida", c.budget, d(c.spent), d(c.amount))
		if !got.ProjectedSpent.Equal(d(c.spent).Add(d(c.amount))) {
			t.Errorf("%s: projected spent = %s", c.name, got.ProjectedSpent)
		}
		if (got.Fits == nil) != (c.fits == nil) || (c.fits != nil && *got.Fits != *c.fits) {
			t.Errorf("%s: fits = %v, want %v", c.name, got.Fits, c.fits)
		}
		if c.budget == nil {
			if got.Budget != nil || got.Remaining != nil || got.ProjectedRemaining != nil || !got.OverBy.IsZero() {
				t.Errorf("%s: a category without budget must have nil budget figures: %+v", c.name, got)
			}
			continue
		}
		if !got.ProjectedRemaining.Equal(d(c.projected)) || !got.Remaining.Equal(d(c.remaining)) || !got.OverBy.Equal(d(c.overBy)) {
			t.Errorf("%s: projected remaining %s remaining %s over by %s", c.name, got.ProjectedRemaining, got.Remaining, got.OverBy)
		}
	}
}

func ptr(b bool) *bool { return &b }
