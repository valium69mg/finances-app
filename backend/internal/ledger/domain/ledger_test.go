package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestPreviousAndNextMonth(t *testing.T) {
	tests := []struct{ in, prev, next string }{
		{"2026-10", "2026-09", "2026-11"},
		{"2027-01", "2026-12", "2027-02"},
		{"2026-12", "2026-11", "2027-01"},
	}
	for _, tc := range tests {
		prev, err := PreviousMonth(tc.in)
		if err != nil || prev != tc.prev {
			t.Errorf("PreviousMonth(%s) = %q, %v; want %q", tc.in, prev, err, tc.prev)
		}
		next, err := NextMonth(tc.in)
		if err != nil || next != tc.next {
			t.Errorf("NextMonth(%s) = %q, %v; want %q", tc.in, next, err, tc.next)
		}
	}
	if _, err := PreviousMonth("2026-13"); err == nil {
		t.Error("PreviousMonth(2026-13) should fail")
	}
}

func TestCurrentMonth(t *testing.T) {
	if got := CurrentMonth(time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)); got != "2026-09" {
		t.Errorf("CurrentMonth = %q, want 2026-09", got)
	}
}

func TestParseDate(t *testing.T) {
	if _, err := ParseDate("2026-10-01"); err != nil {
		t.Errorf("valid date rejected: %v", err)
	}
	for _, bad := range []string{"2026-02-30", "01/10/2026", ""} {
		if _, err := ParseDate(bad); err == nil {
			t.Errorf("ParseDate(%q) should fail", bad)
		}
	}
}

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"  Educación ": "educacion",
		"CAFÉ":         "cafe",
		"Suscripción":  "suscripcion",
		"uber eats":    "uber eats",
	}
	for in, want := range tests {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMonthMovementsAndSumBy(t *testing.T) {
	movements := []Movement{
		{Date: "2026-10-01", Kind: KindIncome, Category: "Sueldo", AmountMXN: dec("60020.2742")},
		{Date: "2026-10-05", Kind: KindSavings, Category: CategorySATReserve, AmountMXN: dec("5775")},
		{Date: "2026-10-20", Kind: KindSavings, Category: CategorySATReserve, AmountMXN: dec("-1000")},
		{Date: "2026-11-02", Kind: KindSavings, Category: CategorySATReserve, AmountMXN: dec("300")},
	}

	oct := MonthMovements(movements, "2026-10")
	if len(oct) != 3 {
		t.Fatalf("October movements = %d, want 3", len(oct))
	}

	tests := []struct {
		name string
		in   []Movement
		f    Filter
		want string
	}{
		{"month income", oct, Filter{Kind: KindIncome}, "60020.2742"},
		{"month reserve nets withdrawals", oct, Filter{Kind: KindSavings, Category: CategorySATReserve}, "4775"},
		{"all-time reserve", movements, Filter{Kind: KindSavings, Category: CategorySATReserve}, "5075"},
		{"no filter", oct, Filter{}, "64795.2742"},
		{"no match", oct, Filter{Kind: KindExpense}, "0"},
	}
	for _, tc := range tests {
		if got := SumBy(tc.in, tc.f); !got.Equal(dec(tc.want)) {
			t.Errorf("%s: SumBy = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestKPIAmountAndSumKPI(t *testing.T) {
	gasto := func(amount, covered string) Movement {
		m := Movement{Kind: KindExpense, Category: "Casa", AmountMXN: dec(amount)}
		if covered != "" {
			m.CoveredBySavings = dec(covered)
		}
		return m
	}
	tests := []struct {
		name string
		m    Movement
		want string
	}{
		{"plain expense", gasto("100", ""), "100"},
		{"partly covered", gasto("100", "60"), "40"},
		{"fully covered", gasto("100", "100"), "0"},
		{"covered above an edited amount is capped", gasto("50", "100"), "0"},
		{"settlement release row", Movement{Kind: KindSavings, AmountMXN: dec("-60"), SavingsSettlement: true}, "0"},
		{"settlement remainder row", Movement{Kind: KindSavings, AmountMXN: dec("20"), SavingsSettlement: true}, "0"},
		{"ordinary saving", Movement{Kind: KindSavings, AmountMXN: dec("-60")}, "-60"},
		{"income", Movement{Kind: KindIncome, AmountMXN: dec("500")}, "500"},
	}
	for _, tc := range tests {
		if got := tc.m.KPIAmount(); !got.Equal(dec(tc.want)) {
			t.Errorf("%s: KPIAmount = %s, want %s", tc.name, got, tc.want)
		}
	}

	// A payment of 100 with 60 saved: the Gasto counts 40, the release row
	// counts nothing, and available (income - expenses - savings) is unchanged.
	month := []Movement{
		{Kind: KindIncome, AmountMXN: dec("1000")},
		gasto("100", "60"),
		{Kind: KindSavings, Category: "Gastos futuros", AmountMXN: dec("-60"), SavingsSettlement: true},
		{Kind: KindSavings, Category: "Inversiones", AmountMXN: dec("200")},
	}
	exp := SumKPI(month, Filter{Kind: KindExpense})
	sav := SumKPI(month, Filter{Kind: KindSavings})
	if !exp.Equal(dec("40")) || !sav.Equal(dec("200")) {
		t.Fatalf("expenses = %s, savings = %s; want 40 and 200", exp, sav)
	}
	oldAvail := dec("1000").Sub(SumBy(month, Filter{Kind: KindExpense})).Sub(SumBy(month, Filter{Kind: KindSavings}))
	newAvail := dec("1000").Sub(exp).Sub(sav)
	if !oldAvail.Equal(newAvail) {
		t.Errorf("available changed: %s -> %s", oldAvail, newAvail)
	}
	if got := SumKPI(month, Filter{Kind: KindExpense, Category: "Casa"}); !got.Equal(dec("40")) {
		t.Errorf("category spent = %s, want 40", got)
	}
	if got := SumBy(month, Filter{Kind: KindSavings, Category: "Gastos futuros"}); !got.Equal(dec("-60")) {
		t.Errorf("portfolio SumBy must keep the settlement row: %s", got)
	}
}
