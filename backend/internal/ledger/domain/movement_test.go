package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func testCatalog() Catalog {
	return Catalog{
		Categories: []CatalogCategory{
			{Name: "Mandado", Kind: KindExpense},
			{Name: "Educación", Kind: KindExpense},
			{Name: "Sueldo", Kind: KindIncome},
			{Name: "Inversiones", Kind: KindSavings},
		},
		PaymentMethods: []string{"Efectivo", "Débito", "Crédito", "Transferencia"},
		DefaultRate:    dec("17.74"),
	}
}

func TestAmountMXN(t *testing.T) {
	rate := func(s string) *decimal.Decimal { d := dec(s); return &d }
	tests := []struct {
		name     string
		amount   string
		currency string
		rate     *decimal.Decimal
		want     string
	}{
		{"MXN passes through", "250.50", CurrencyMXN, nil, "250.5"},
		{"USD exact", "25.50", CurrencyUSD, rate("17.74"), "452.37"},
		{"USD rounds to cents", "3.33", CurrencyUSD, rate("17.745"), "59.09"},
		{"USD rounds half up", "1", CurrencyUSD, rate("17.745"), "17.75"},
		{"negative savings withdrawal", "-100", CurrencyUSD, rate("17.5"), "-1750"},
	}
	for _, tc := range tests {
		got := AmountMXN(dec(tc.amount), tc.currency, tc.rate)
		if !got.Equal(dec(tc.want)) {
			t.Errorf("%s: AmountMXN = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestNewMovementDefaults(t *testing.T) {
	m, err := NewMovement(MovementInput{Kind: KindExpense, Category: "mandado", Amount: dec("120.50"), Description: "  super  "},
		testCatalog(), "2026-10-05")
	if err != nil {
		t.Fatalf("NewMovement: %v", err)
	}
	if m.Date != "2026-10-05" || m.PaymentMethod != "Débito" || m.Currency != CurrencyMXN {
		t.Errorf("defaults not applied: %+v", m)
	}
	if m.Category != "Mandado" || m.Description != "super" || m.ExchangeRate != nil {
		t.Errorf("unexpected fields: %+v", m)
	}
	if !m.AmountMXN.Equal(dec("120.50")) {
		t.Errorf("AmountMXN = %s", m.AmountMXN)
	}
}

func TestNewMovementCategoryMatchesIgnoringAccents(t *testing.T) {
	m, err := NewMovement(MovementInput{Kind: KindExpense, Category: "educacion", Amount: dec("1")}, testCatalog(), "2026-10-05")
	if err != nil || m.Category != "Educación" {
		t.Fatalf("got %q, %v; want canonical Educación", m.Category, err)
	}
}

func TestNewMovementUSD(t *testing.T) {
	cat := testCatalog()
	explicit := dec("17.745")
	m, err := NewMovement(MovementInput{Kind: KindExpense, Category: "Mandado", Currency: "USD", Amount: dec("3.33"), ExchangeRate: &explicit},
		cat, "2026-10-05")
	if err != nil {
		t.Fatal(err)
	}
	if !m.AmountMXN.Equal(dec("59.09")) || m.ExchangeRate == nil || !m.ExchangeRate.Equal(explicit) {
		t.Errorf("explicit rate: %+v", m)
	}

	m, err = NewMovement(MovementInput{Kind: KindExpense, Category: "Mandado", Currency: "USD", Amount: dec("10")}, cat, "2026-10-05")
	if err != nil {
		t.Fatal(err)
	}
	if !m.AmountMXN.Equal(dec("177.40")) || !m.ExchangeRate.Equal(dec("17.74")) {
		t.Errorf("default rate: %+v", m)
	}

	cat.DefaultRate = decimal.Zero
	if _, err := NewMovement(MovementInput{Kind: KindExpense, Category: "Mandado", Currency: "USD", Amount: dec("10")}, cat, "2026-10-05"); !errors.Is(err, ErrInvalid) {
		t.Errorf("USD without any rate: err = %v, want ErrInvalid", err)
	}
}

func TestNewMovementMXNDropsRate(t *testing.T) {
	r := dec("17")
	m, err := NewMovement(MovementInput{Kind: KindExpense, Category: "Mandado", Amount: dec("5"), ExchangeRate: &r}, testCatalog(), "2026-10-05")
	if err != nil || m.ExchangeRate != nil {
		t.Errorf("MXN movement kept a rate: %+v, %v", m.ExchangeRate, err)
	}
}

func TestNewMovementOtherKindDefaults(t *testing.T) {
	m, err := NewMovement(MovementInput{Kind: KindSavings, Category: "Inversiones", Amount: dec("-500")}, testCatalog(), "2026-10-05")
	if err != nil {
		t.Fatalf("savings withdrawal: %v", err)
	}
	if m.PaymentMethod != "Transferencia" {
		t.Errorf("PaymentMethod = %q", m.PaymentMethod)
	}
}

func TestNewMovementIncome(t *testing.T) {
	m, err := NewMovement(MovementInput{Kind: KindIncome, Category: "sueldo", Amount: dec("3383.33"), Currency: "USD"},
		testCatalog(), "2026-10-05")
	if err != nil {
		t.Fatalf("income: %v", err)
	}
	// 3383.33 * 17.74 = 60020.2742, rounded to cents.
	if m.PaymentMethod != "Transferencia" || !m.AmountMXN.Equal(dec("60020.27")) {
		t.Errorf("unexpected income %+v", m)
	}
}

func TestNewMovementInvalid(t *testing.T) {
	ok := MovementInput{Kind: KindExpense, Category: "Mandado", Amount: dec("10"), Date: "2026-10-05"}
	tests := []struct {
		name string
		mut  func(*MovementInput)
	}{
		{"unknown kind", func(i *MovementInput) { i.Kind = "Otro" }},
		{"empty kind", func(i *MovementInput) { i.Kind = "" }},
		{"unknown category", func(i *MovementInput) { i.Category = "Nada" }},
		{"category of another kind", func(i *MovementInput) { i.Category = "Sueldo" }},
		{"empty category", func(i *MovementInput) { i.Category = "" }},
		{"unknown payment method", func(i *MovementInput) { i.PaymentMethod = "Cheque" }},
		{"unknown currency", func(i *MovementInput) { i.Currency = "EUR" }},
		{"bad date", func(i *MovementInput) { i.Date = "05/10/2026" }},
		{"impossible date", func(i *MovementInput) { i.Date = "2026-02-30" }},
		{"zero amount", func(i *MovementInput) { i.Amount = decimal.Zero }},
		{"negative expense", func(i *MovementInput) { i.Amount = dec("-1") }},
		{"zero income", func(i *MovementInput) { i.Kind, i.Category, i.Amount = KindIncome, "Sueldo", decimal.Zero }},
		{"negative income", func(i *MovementInput) { i.Kind, i.Category, i.Amount = KindIncome, "Sueldo", dec("-1") }},
		{"zero savings", func(i *MovementInput) { i.Kind, i.Category, i.Amount = KindSavings, "Inversiones", decimal.Zero }},
		{"USD with zero rate", func(i *MovementInput) { i.Currency = "USD"; z := decimal.Zero; i.ExchangeRate = &z }},
		{"huge exponent", func(i *MovementInput) { i.Amount = dec("1e2000000000") }},
		{"huge exponent 999999999", func(i *MovementInput) { i.Amount = dec("1e999999999") }},
		{"tiny exponent", func(i *MovementInput) { i.Amount = dec("1e-2000000000") }},
		{"amount above numeric(14,2)", func(i *MovementInput) { i.Amount = dec("10000000000000") }},
		{"savings above numeric(14,2)", func(i *MovementInput) {
			i.Kind, i.Category, i.Amount = KindSavings, "Inversiones", dec("-10000000000000")
		}},
		{"rate with huge exponent", func(i *MovementInput) { i.Currency = "USD"; r := dec("1e2000000000"); i.ExchangeRate = &r }},
		{"USD amount whose MXN value overflows", func(i *MovementInput) {
			i.Currency, i.Amount = "USD", dec("999999999999")
			r := dec("17.74")
			i.ExchangeRate = &r
		}},
	}
	for _, tc := range tests {
		in := ok
		tc.mut(&in)
		if _, err := NewMovement(in, testCatalog(), "2026-10-05"); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", tc.name, err)
		}
	}
}

func TestNewMovementTextLimits(t *testing.T) {
	base := func(desc, instr string) MovementInput {
		return MovementInput{Kind: KindSavings, Category: "Inversiones", Amount: dec("1"), Description: desc, Instrument: instr}
	}
	tests := []struct {
		name    string
		desc    string
		instr   string
		invalid bool
	}{
		{"description at the limit", strings.Repeat("é", MaxDescriptionLength), "", false},
		{"description over the limit", strings.Repeat("é", MaxDescriptionLength+1), "", true},
		{"description counts runes after trimming", "  " + strings.Repeat("é", MaxDescriptionLength) + "  ", "", false},
		{"instrument at the limit", "", strings.Repeat("é", MaxInstrumentLength), false},
		{"instrument over the limit", "", strings.Repeat("é", MaxInstrumentLength+1), true},
		{"NUL in description", "a\x00b", "", true},
		{"NUL in instrument", "", "a\x00b", true},
	}
	for _, tc := range tests {
		_, err := NewMovement(base(tc.desc, tc.instr), testCatalog(), "2026-10-05")
		if tc.invalid != errors.Is(err, ErrInvalid) || (!tc.invalid && err != nil) {
			t.Errorf("%s: err = %v, invalid want %v", tc.name, err, tc.invalid)
		}
	}
}
