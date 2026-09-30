// Package domain holds the pure model of the future expenses module: the items
// the owner chooses to save for, their validation and the plan that says how
// much to put aside each pay cycle. A future expense is explicit: it is never
// derived from a recurring bill.
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// Errors of the module. ErrInvalidInput is wrapped with a message saying what
// is wrong.
var (
	ErrInvalidInput = errors.New("invalid future expense")
	ErrNotFound     = errors.New("future expense not found")
	// ErrAlreadyPaid is returned when an item that is already paid is paid,
	// edited or given savings again.
	ErrAlreadyPaid = errors.New("future expense is already paid")
	// ErrInsufficientFreeBalance is returned when an assignment asks for more
	// than the free balance holds.
	ErrInsufficientFreeBalance = errors.New("not enough free balance")
)

// Status of an item.
type Status string

const (
	StatusActive Status = "active"
	StatusPaid   Status = "paid"
)

// Limits and fixed names.
const (
	MaxNameLength = 120
	DateLayout    = "2006-01-02"
	// SavingsCategory is the Ahorro category every saving of this module uses.
	SavingsCategory = ledger.CategoryFutureExpenses
	// DefaultExpenseCategory is the Gasto category of a payment when none is
	// given and none can be inferred from the name of the item.
	DefaultExpenseCategory = "Otros"
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// FutureExpense is one item. Saved is the net of the Ahorro movements linked to
// it (zero for a paid item, whose release row cancels its savings).
type FutureExpense struct {
	ID        int
	Name      string
	Target    decimal.Decimal
	DueDate   string // YYYY-MM-DD
	Status    Status
	Saved     decimal.Decimal
	CreatedAt time.Time
	UpdatedAt time.Time
	// Set only when the item is paid.
	PaidAt            string // YYYY-MM-DD
	AmountPaid        *decimal.Decimal
	ExpenseMovementID *int
}

// Input is the raw data of a new or updated item.
type Input struct {
	Name    string
	Target  decimal.Decimal
	DueDate string
}

// Validated is an Input after validation: the name is trimmed, the target is a
// positive MXN amount with at most 2 decimals and the due date is YYYY-MM-DD.
type Validated struct {
	Name    string
	Target  decimal.Decimal
	DueDate string
}

// ValidateMoney checks a user-supplied amount of this module: in range first
// (before any comparison or rounding), then positive with at most 2 decimals.
func ValidateMoney(field string, v decimal.Decimal) error {
	if err := ledger.CheckAmount(v); err != nil {
		return invalid("%s is out of range, it must be below %s", field, ledger.MaxAmount)
	}
	if !v.IsPositive() {
		return invalid("%s must be greater than zero", field)
	}
	if !v.Equal(v.Round(2)) {
		return invalid("%s must have at most 2 decimals", field)
	}
	return nil
}

// ParseDate checks a YYYY-MM-DD date.
func ParseDate(field, s string) (string, error) {
	if _, err := time.Parse(DateLayout, s); err != nil {
		return "", invalid("%s must be a date like YYYY-MM-DD", field)
	}
	return s, nil
}

// Validate checks the input of a create or an update.
func Validate(in Input) (Validated, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return Validated{}, invalid("name is required")
	case utf8.RuneCountInString(name) > MaxNameLength:
		return Validated{}, invalid("name must be at most %d characters", MaxNameLength)
	}
	if err := ValidateMoney("target amount", in.Target); err != nil {
		return Validated{}, err
	}
	due, err := ParseDate("due date", strings.TrimSpace(in.DueDate))
	if err != nil {
		return Validated{}, err
	}
	return Validated{Name: name, Target: in.Target, DueDate: due}, nil
}

// Planned is an item with what is still missing and what to put aside each pay
// cycle to have it on time. CyclesLeft is the number of pay cycles from the
// current one up to the cycle of the due date (at least 1, also when overdue),
// the divisor of Suggested. A paid item has no plan: its Remaining, Suggested
// and CyclesLeft are zero.
type Planned struct {
	FutureExpense
	Remaining  decimal.Decimal
	Suggested  decimal.Decimal
	CyclesLeft int
}

// Plan is the plan of every active item (earliest due date first) with its
// totals and the free balance: the Gastos futuros savings linked to no item.
type Plan struct {
	Items       []Planned
	Target      decimal.Decimal
	Saved       decimal.Decimal
	Remaining   decimal.Decimal
	Suggested   decimal.Decimal
	FreeBalance decimal.Decimal
}

// PlanItem computes the plan of one item. Remaining is what is missing (never
// negative: an item saved beyond its target needs nothing more) and Suggested
// is Remaining divided by the cycles left, rounded UP to the cent so the plan
// never falls short. today is a YYYY-MM-DD date in the configured time zone
// and the cycles follow the configured cycle start day.
func PlanItem(cycle ledger.Cycle, today string, it FutureExpense) (Planned, error) {
	p := Planned{FutureExpense: it}
	if it.Status == StatusPaid {
		return p, nil
	}
	currentIdx, err := cycleIndex(cycle.Of(today))
	if err != nil {
		return Planned{}, err
	}
	dueIdx, err := cycleIndex(cycle.Of(it.DueDate))
	if err != nil {
		return Planned{}, err
	}
	cycles := dueIdx - currentIdx
	if cycles < 1 {
		cycles = 1
	}
	p.CyclesLeft = cycles
	p.Remaining = decimal.Max(it.Target.Sub(it.Saved), decimal.Zero)
	p.Suggested = p.Remaining.Div(decimal.NewFromInt(int64(cycles))).RoundCeil(2)
	return p, nil
}

// BuildPlan plans the active items (paid ones are ignored), sorted by due date
// and then by id, and sums the totals.
func BuildPlan(cycle ledger.Cycle, today string, items []FutureExpense, freeBalance decimal.Decimal) (Plan, error) {
	plan := Plan{Items: []Planned{}, FreeBalance: freeBalance}
	for _, it := range items {
		if it.Status != StatusActive {
			continue
		}
		p, err := PlanItem(cycle, today, it)
		if err != nil {
			return Plan{}, err
		}
		plan.Items = append(plan.Items, p)
	}
	sortPlanned(plan.Items)
	for _, p := range plan.Items {
		plan.Target = plan.Target.Add(p.Target)
		plan.Saved = plan.Saved.Add(p.Saved)
		plan.Remaining = plan.Remaining.Add(p.Remaining)
		plan.Suggested = plan.Suggested.Add(p.Suggested)
	}
	return plan, nil
}

func sortPlanned(items []Planned) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && before(items[j], items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func before(a, b Planned) bool {
	if a.DueDate != b.DueDate {
		return a.DueDate < b.DueDate
	}
	return a.ID < b.ID
}

// cycleIndex turns a YYYY-MM cycle label into a month count so two labels can
// be subtracted.
func cycleIndex(label string) (int, error) {
	t, err := time.Parse("2006-01", label)
	if err != nil {
		return 0, fmt.Errorf("invalid cycle label %q", label)
	}
	return t.Year()*12 + int(t.Month()), nil
}

// Release computes the two Ahorro rows of a payment from what is saved and what
// was paid (all MXN). Linked is the amount of the negative row linked to the
// item (everything saved, so the item is left at zero); Remainder is the part of
// it that the payment did not use, returned to the free balance as a positive
// unlinked row. When less is saved than was paid only what exists is released
// (the rest of the payment comes from outside the savings). Nothing is
// released for an empty or negative saved amount.
func Release(saved, paid decimal.Decimal) (linked, remainder decimal.Decimal) {
	if !saved.IsPositive() {
		return decimal.Zero, decimal.Zero
	}
	remainder = decimal.Max(saved.Sub(paid), decimal.Zero)
	return saved, remainder
}
