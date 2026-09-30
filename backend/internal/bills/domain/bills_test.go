package domain_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/bills/domain"
)

func d(s string) *decimal.Decimal {
	v := decimal.RequireFromString(s)
	return &v
}

func TestNextDue(t *testing.T) {
	tests := []struct {
		name   string
		rec    domain.Recurrence
		anchor int
		from   string
		want   string
	}{
		{"weekly", domain.Weekly, 10, "2026-10-10", "2026-10-17"},
		{"weekly crosses month", domain.Weekly, 28, "2026-10-28", "2026-11-04"},
		{"weekly crosses year", domain.Weekly, 28, "2026-12-28", "2027-01-04"},
		{"weekly ignores the anchor day", domain.Weekly, 5, "2026-10-10", "2026-10-17"},
		{"weekly over a leap day", domain.Weekly, 24, "2028-02-24", "2028-03-02"},
		{"biweekly", domain.Biweekly, 10, "2026-10-10", "2026-10-24"},
		{"biweekly crosses month", domain.Biweekly, 25, "2026-10-25", "2026-11-08"},
		{"biweekly crosses year", domain.Biweekly, 25, "2026-12-25", "2027-01-08"},
		{"monthly", domain.Monthly, 15, "2026-10-15", "2026-11-15"},
		{"monthly december rolls the year", domain.Monthly, 15, "2026-12-15", "2027-01-15"},
		{"monthly 31st to a 30-day month", domain.Monthly, 31, "2026-10-31", "2026-11-30"},
		{"monthly 31st from a 30-day month recovers", domain.Monthly, 31, "2026-11-30", "2026-12-31"},
		{"monthly 31st to February", domain.Monthly, 31, "2027-01-31", "2027-02-28"},
		{"monthly 31st to February in a leap year", domain.Monthly, 31, "2028-01-31", "2028-02-29"},
		{"monthly 31st from February recovers", domain.Monthly, 31, "2027-02-28", "2027-03-31"},
		{"monthly 30th to February", domain.Monthly, 30, "2027-01-30", "2027-02-28"},
		{"monthly 29th to February common year", domain.Monthly, 29, "2027-01-29", "2027-02-28"},
		{"monthly 29th to February leap year", domain.Monthly, 29, "2028-01-29", "2028-02-29"},
		{"monthly 28th never clamps", domain.Monthly, 28, "2027-01-28", "2027-02-28"},
		{"monthly first", domain.Monthly, 1, "2026-10-01", "2026-11-01"},
		{"monthly from a clamped date with anchor 29", domain.Monthly, 29, "2027-02-28", "2027-03-29"},
		{"bimonthly", domain.Bimonthly, 10, "2026-10-10", "2026-12-10"},
		{"bimonthly rolls the year", domain.Bimonthly, 10, "2026-11-10", "2027-01-10"},
		{"bimonthly 31st to a 30-day month", domain.Bimonthly, 31, "2026-08-31", "2026-10-31"},
		{"bimonthly 31st lands on a short month", domain.Bimonthly, 31, "2026-10-31", "2026-12-31"},
		{"bimonthly 31st to February", domain.Bimonthly, 31, "2026-12-31", "2027-02-28"},
		{"bimonthly 31st from February recovers", domain.Bimonthly, 31, "2027-02-28", "2027-04-30"},
		{"yearly", domain.Yearly, 15, "2026-03-15", "2027-03-15"},
		{"yearly Feb 29 to a common year", domain.Yearly, 29, "2028-02-29", "2029-02-28"},
		{"yearly Feb 29 recovers in the next leap year", domain.Yearly, 29, "2031-02-28", "2032-02-29"},
		{"yearly Feb 28 anchor stays on the 28th", domain.Yearly, 28, "2027-02-28", "2028-02-28"},
		{"yearly 31st keeps the month", domain.Yearly, 31, "2026-12-31", "2027-12-31"},
		{"yearly 31st in a 30-day month clamps", domain.Yearly, 31, "2026-04-30", "2027-04-30"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.NextDue(tc.rec, tc.anchor, tc.from)
			if err != nil {
				t.Fatalf("NextDue: %v", err)
			}
			if got != tc.want {
				t.Errorf("NextDue(%s, %d, %s) = %s, want %s", tc.rec, tc.anchor, tc.from, got, tc.want)
			}
		})
	}
}

// A schedule stepped many times must land on the anchor day whenever the month
// is long enough: the clamp of a short month never sticks.
func TestNextDueDoesNotDrift(t *testing.T) {
	date := "2026-01-31"
	want := []string{
		"2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31", "2026-06-30", "2026-07-31",
		"2026-08-31", "2026-09-30", "2026-10-31", "2026-11-30", "2026-12-31", "2027-01-31",
		"2027-02-28", "2027-03-31",
	}
	for i, w := range want {
		next, err := domain.NextDue(domain.Monthly, 31, date)
		if err != nil {
			t.Fatal(err)
		}
		if next != w {
			t.Fatalf("step %d = %s, want %s", i+1, next, w)
		}
		date = next
	}
}

func TestNextDueErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rec    domain.Recurrence
		anchor int
		from   string
	}{
		{"unknown recurrence", "daily", 1, "2026-10-01"},
		{"empty recurrence", "", 1, "2026-10-01"},
		{"bad date", domain.Monthly, 1, "2026-13-01"},
		{"empty date", domain.Monthly, 1, ""},
		{"anchor zero", domain.Monthly, 0, "2026-10-01"},
		{"anchor 32", domain.Monthly, 32, "2026-10-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.NextDue(tc.rec, tc.anchor, tc.from); !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestAnchorDayOf(t *testing.T) {
	if got, err := domain.AnchorDayOf("2026-10-31"); err != nil || got != 31 {
		t.Errorf("AnchorDayOf = %d, %v; want 31", got, err)
	}
	if _, err := domain.AnchorDayOf("nope"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestRecurrenceIsValid(t *testing.T) {
	for _, r := range domain.Recurrences {
		if !r.IsValid() {
			t.Errorf("%s should be valid", r)
		}
	}
	for _, r := range []domain.Recurrence{"", "daily", "Monthly", "quarterly"} {
		if r.IsValid() {
			t.Errorf("%q should be invalid", r)
		}
	}
}

func TestOverdueAndDueSoon(t *testing.T) {
	tests := []struct {
		name     string
		due      string
		today    string
		lead     int
		overdue  bool
		dueSoon  bool
		daysLeft int
	}{
		{"overdue by one day", "2026-10-09", "2026-10-10", 3, true, false, -1},
		{"due today is not overdue and is due soon", "2026-10-10", "2026-10-10", 3, false, true, 0},
		{"due today with zero lead is due soon", "2026-10-10", "2026-10-10", 0, false, true, 0},
		{"tomorrow with zero lead is not due soon", "2026-10-11", "2026-10-10", 0, false, false, 1},
		{"exactly the lead days ahead", "2026-10-13", "2026-10-10", 3, false, true, 3},
		{"one day beyond the lead", "2026-10-14", "2026-10-10", 3, false, false, 4},
		{"across a month end", "2026-11-01", "2026-10-30", 2, false, true, 2},
		{"long overdue", "2026-01-01", "2026-10-10", 3, true, false, -282},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := domain.IsOverdue(tc.due, tc.today); got != tc.overdue {
				t.Errorf("IsOverdue = %v, want %v", got, tc.overdue)
			}
			if got := domain.IsDueSoon(tc.due, tc.today, tc.lead); got != tc.dueSoon {
				t.Errorf("IsDueSoon = %v, want %v", got, tc.dueSoon)
			}
			if n, err := domain.DaysUntil(tc.due, tc.today); err != nil || n != tc.daysLeft {
				t.Errorf("DaysUntil = %d, %v; want %d", n, err, tc.daysLeft)
			}
		})
	}
}

func TestFlagsWithBadDates(t *testing.T) {
	if domain.IsOverdue("bad", "2026-10-10") || domain.IsDueSoon("bad", "2026-10-10", 3) {
		t.Error("a bad date must not raise flags")
	}
	if _, err := domain.DaysUntil("2026-10-10", "bad"); err == nil {
		t.Error("want an error for a bad today")
	}
}

func validInput() domain.Input {
	return domain.Input{
		Name: "Megacable", Category: "Servicios", Amount: d("550"), Recurrence: domain.Monthly,
		NextDueDate: "2026-11-01", Active: true,
	}
}

func TestValidateDefaultsAndTrims(t *testing.T) {
	in := validInput()
	in.Name = "  Megacable  "
	in.Notes = "  contrato 123 "
	v, err := domain.Validate(in)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if v.Name != "Megacable" || v.Notes != "contrato 123" {
		t.Errorf("not trimmed: %q %q", v.Name, v.Notes)
	}
	if v.Currency != "MXN" || v.ReminderLeadDays != 3 || v.AnchorDay != 1 {
		t.Errorf("defaults = %s %d %d", v.Currency, v.ReminderLeadDays, v.AnchorDay)
	}
	if !v.Active {
		t.Error("active must be kept")
	}
}

func TestValidateVariableBillAndAnchor(t *testing.T) {
	in := validInput()
	in.Amount = nil
	in.NextDueDate = "2026-10-31"
	zero := 0
	in.ReminderLeadDays = &zero
	v, err := domain.Validate(in)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if v.Amount != nil || v.AnchorDay != 31 || v.ReminderLeadDays != 0 {
		t.Errorf("got amount=%v anchor=%d lead=%d", v.Amount, v.AnchorDay, v.ReminderLeadDays)
	}
}

func TestValidateRejects(t *testing.T) {
	neg, over := -1, 366
	tests := []struct {
		name string
		mut  func(*domain.Input)
	}{
		{"empty name", func(i *domain.Input) { i.Name = "  " }},
		{"long name", func(i *domain.Input) { i.Name = string(make([]rune, 121)) + "x" }},
		{"empty category", func(i *domain.Input) { i.Category = "" }},
		{"bad recurrence", func(i *domain.Input) { i.Recurrence = "daily" }},
		{"bad currency", func(i *domain.Input) { i.Currency = "EUR" }},
		{"zero amount", func(i *domain.Input) { i.Amount = d("0") }},
		{"negative amount", func(i *domain.Input) { i.Amount = d("-5") }},
		{"three decimals", func(i *domain.Input) { i.Amount = d("10.123") }},
		{"huge exponent", func(i *domain.Input) { i.Amount = d("1e2000000000") }},
		{"huge exponent 999999999", func(i *domain.Input) { i.Amount = d("1e999999999") }},
		{"above numeric(14,2)", func(i *domain.Input) { i.Amount = d("10000000000000") }},
		{"exactly the exclusive maximum", func(i *domain.Input) { i.Amount = d("1000000000000") }},
		{"bad date", func(i *domain.Input) { i.NextDueDate = "2026-02-30" }},
		{"empty date", func(i *domain.Input) { i.NextDueDate = "" }},
		{"negative lead", func(i *domain.Input) { i.ReminderLeadDays = &neg }},
		{"too long lead", func(i *domain.Input) { i.ReminderLeadDays = &over }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mut(&in)
			if _, err := domain.Validate(in); !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestValidateAcceptsUSD(t *testing.T) {
	in := validInput()
	in.Currency = "USD"
	v, err := domain.Validate(in)
	if err != nil || v.Currency != "USD" {
		t.Errorf("got %v, %v", v.Currency, err)
	}
}

func TestResolvePayment(t *testing.T) {
	fixed := domain.Bill{Name: "Megacable", Category: "Servicios", Amount: d("550")}
	variable := domain.Bill{Name: "Luz", Category: "Servicios"}
	const today = "2026-10-05"
	tests := []struct {
		name string
		bill domain.Bill
		in   domain.PaymentInput
		want domain.Payment
	}{
		{"fixed bill takes every default", fixed, domain.PaymentInput{},
			domain.Payment{Date: today, Amount: *d("550"), Category: "Servicios", Description: "Megacable"}},
		{"amount can be overridden", fixed, domain.PaymentInput{Amount: d("499.50")},
			domain.Payment{Date: today, Amount: *d("499.50"), Category: "Servicios", Description: "Megacable"}},
		{"variable bill with an amount", variable, domain.PaymentInput{Amount: d("312.10")},
			domain.Payment{Date: today, Amount: *d("312.10"), Category: "Servicios", Description: "Luz"}},
		{"category, date and description overrides", fixed,
			domain.PaymentInput{Date: "2026-10-01", Category: " Hogar ", Description: " recibo octubre "},
			domain.Payment{Date: "2026-10-01", Amount: *d("550"), Category: "Hogar", Description: "recibo octubre"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.ResolvePayment(tc.bill, tc.in, today)
			if err != nil {
				t.Fatalf("ResolvePayment: %v", err)
			}
			if got.Date != tc.want.Date || !got.Amount.Equal(tc.want.Amount) || got.Category != tc.want.Category || got.Description != tc.want.Description {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolvePaymentRejects(t *testing.T) {
	fixed := domain.Bill{Name: "Megacable", Category: "Servicios", Amount: d("550")}
	variable := domain.Bill{Name: "Luz", Category: "Servicios"}
	tests := []struct {
		name string
		bill domain.Bill
		in   domain.PaymentInput
	}{
		{"variable bill needs an amount", variable, domain.PaymentInput{}},
		{"zero amount", fixed, domain.PaymentInput{Amount: d("0")}},
		{"negative amount", fixed, domain.PaymentInput{Amount: d("-1")}},
		{"too many decimals", fixed, domain.PaymentInput{Amount: d("1.005")}},
		{"bad date", fixed, domain.PaymentInput{Date: "yesterday"}},
		{"huge exponent", fixed, domain.PaymentInput{Amount: d("1e2000000000")}},
		{"huge exponent 999999999", fixed, domain.PaymentInput{Amount: d("1e999999999")}},
		{"above numeric(14,2)", fixed, domain.PaymentInput{Amount: d("10000000000000")}},
		{"exactly the exclusive maximum", fixed, domain.PaymentInput{Amount: d("1000000000000")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := domain.ResolvePayment(tc.bill, tc.in, "2026-10-05"); !errors.Is(err, domain.ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}
