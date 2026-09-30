// Package domain holds the pure rules of Bills & Subscriptions: recurrences and
// the computation of the next due date (with month-end clamping), the
// validation of a bill, and the overdue and due-soon flags of its pending
// occurrence. Only the next occurrence of a bill exists at a time; paying or
// skipping it generates the following one.
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

// Errors returned by the bills rules and repositories.
var (
	ErrInvalidInput = errors.New("invalid bill input")
	// ErrNotFound is returned when a bill does not exist.
	ErrNotFound = errors.New("bill not found")
	// ErrNotPending is returned when the occurrence to resolve is already paid or
	// skipped (for example a double click or two tabs).
	ErrNotPending = errors.New("the occurrence is already resolved")
	// ErrInactive is returned when an inactive bill would be paid or skipped.
	ErrInactive = errors.New("the bill is inactive")
)

// Recurrence is how often a bill repeats. There are no custom intervals.
type Recurrence string

const (
	Weekly    Recurrence = "weekly"
	Biweekly  Recurrence = "biweekly"
	Monthly   Recurrence = "monthly"
	Bimonthly Recurrence = "bimonthly"
	Yearly    Recurrence = "yearly"
)

// Recurrences lists every valid recurrence.
var Recurrences = []Recurrence{Weekly, Biweekly, Monthly, Bimonthly, Yearly}

// IsValid reports whether r is one of the supported recurrences.
func (r Recurrence) IsValid() bool {
	for _, v := range Recurrences {
		if r == v {
			return true
		}
	}
	return false
}

// Status is the state of an occurrence.
type Status string

const (
	StatusPending Status = "pending"
	StatusPaid    Status = "paid"
	StatusSkipped Status = "skipped"
)

// Currency codes of a bill, the same the ledger accepts.
const (
	CurrencyMXN = "MXN"
	CurrencyUSD = "USD"
)

// Limits and defaults.
const (
	DefaultReminderLeadDays = 3
	MaxReminderLeadDays     = 365
	MaxNameLength           = 120
	MaxNotesLength          = 1000
	DateLayout              = "2006-01-02"
)

// Occurrence is one due date of a bill. A pending occurrence has no payment
// data; a paid one has PaidOn, AmountPaid and Currency (and the expense it
// registered, unless that expense was deleted later); a skipped one has none.
type Occurrence struct {
	ID         int
	BillID     int
	DueDate    string // YYYY-MM-DD
	Status     Status
	PaidOn     string // YYYY-MM-DD, empty unless paid
	ExpenseID  *int
	AmountPaid *decimal.Decimal
	Currency   string // currency of AmountPaid, empty unless paid
	ResolvedAt *time.Time
}

// Bill is a recurring bill or subscription. Amount is nil for a variable bill.
// AnchorDay is the day of the month the schedule is based on: it is what keeps
// a bill due on the 31st from drifting to the 28th after February. Pending is
// the next occurrence, nil only for a bill loaded without it.
type Bill struct {
	ID               int
	Name             string
	Category         string
	Amount           *decimal.Decimal
	Currency         string
	Recurrence       Recurrence
	NextDueDate      string // due date of the pending occurrence
	AnchorDay        int
	ReminderLeadDays int
	Active           bool
	Notes            string
	CreatedAt        time.Time
	Pending          *Occurrence
}

// Input is the raw data of a new or updated bill.
type Input struct {
	Name             string
	Category         string
	Amount           *decimal.Decimal
	Currency         string
	Recurrence       Recurrence
	NextDueDate      string
	ReminderLeadDays *int
	Active           bool
	Notes            string
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// ParseDate parses a YYYY-MM-DD date.
func ParseDate(s string) (time.Time, error) {
	t, err := time.Parse(DateLayout, s)
	if err != nil {
		return time.Time{}, invalid("invalid date %q, use YYYY-MM-DD", s)
	}
	return t, nil
}

// AnchorDayOf returns the day of the month of a YYYY-MM-DD date.
func AnchorDayOf(date string) (int, error) {
	t, err := ParseDate(date)
	if err != nil {
		return 0, err
	}
	return t.Day(), nil
}

func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// addMonths adds n months to the year and month of t and lands on anchorDay,
// clamped to the last day of the target month.
func addMonths(t time.Time, n, anchorDay int) time.Time {
	first := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, n, 0)
	day := min(anchorDay, daysIn(first.Year(), first.Month()))
	return time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC)
}

// NextDue returns the due date after from for a recurrence. Monthly, bimonthly
// and yearly dates land on anchorDay of the target month, or on its last day
// when the month is shorter, so the schedule does not drift: from the 31st,
// Jan 31 -> Feb 28 -> Mar 31. A yearly bill anchored on Feb 29 falls on Feb 28
// in a common year and on Feb 29 again in the next leap year. Weekly and
// biweekly steps add 7 and 14 days and ignore anchorDay.
func NextDue(r Recurrence, anchorDay int, from string) (string, error) {
	t, err := ParseDate(from)
	if err != nil {
		return "", err
	}
	if anchorDay < 1 || anchorDay > 31 {
		return "", invalid("anchor day %d must be between 1 and 31", anchorDay)
	}
	var next time.Time
	switch r {
	case Weekly:
		next = t.AddDate(0, 0, 7)
	case Biweekly:
		next = t.AddDate(0, 0, 14)
	case Monthly:
		next = addMonths(t, 1, anchorDay)
	case Bimonthly:
		next = addMonths(t, 2, anchorDay)
	case Yearly:
		next = addMonths(t, 12, anchorDay)
	default:
		return "", invalid("recurrence %q must be one of weekly, biweekly, monthly, bimonthly, yearly", r)
	}
	return next.Format(DateLayout), nil
}

// DaysUntil returns the whole days from today to due (negative when past).
func DaysUntil(due, today string) (int, error) {
	d, err := ParseDate(due)
	if err != nil {
		return 0, err
	}
	t, err := ParseDate(today)
	if err != nil {
		return 0, err
	}
	return int(d.Sub(t).Hours() / 24), nil
}

// IsOverdue reports whether a pending occurrence is past its due date. A bill
// due today is not overdue yet.
func IsOverdue(due, today string) bool {
	n, err := DaysUntil(due, today)
	return err == nil && n < 0
}

// IsDueSoon reports whether a pending occurrence is not overdue and falls
// within leadDays of today (leadDays 0 means only on the due day).
func IsDueSoon(due, today string, leadDays int) bool {
	n, err := DaysUntil(due, today)
	return err == nil && n >= 0 && n <= leadDays
}

// checkAmount is the range guard of every user amount of this module: below
// ledger.MaxAmount (the numeric(14,2) columns) and with a sane exponent.
func checkAmount(v decimal.Decimal) error {
	if err := ledger.CheckAmount(v); err != nil {
		return invalid("amount is out of range, it must be below %s", ledger.MaxAmount)
	}
	return nil
}

// Validated is a bill input after validation and defaults.
type Validated struct {
	Name             string
	Category         string
	Amount           *decimal.Decimal
	Currency         string
	Recurrence       Recurrence
	NextDueDate      string
	AnchorDay        int
	ReminderLeadDays int
	Active           bool
	Notes            string
}

// Validate checks an input and applies the defaults: currency MXN and a
// reminder lead time of DefaultReminderLeadDays. The anchor day is that of the
// next due date; an update that keeps the due date keeps its stored anchor
// instead (see the service). Category existence is checked by the service,
// against the settings.
func Validate(in Input) (Validated, error) {
	name := strings.TrimSpace(in.Name)
	switch {
	case name == "":
		return Validated{}, invalid("name is required")
	case utf8.RuneCountInString(name) > MaxNameLength:
		return Validated{}, invalid("name must be at most %d characters", MaxNameLength)
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		return Validated{}, invalid("category is required")
	}
	if !in.Recurrence.IsValid() {
		return Validated{}, invalid("recurrence %q must be one of weekly, biweekly, monthly, bimonthly, yearly", in.Recurrence)
	}
	currency := strings.TrimSpace(in.Currency)
	if currency == "" {
		currency = CurrencyMXN
	}
	if currency != CurrencyMXN && currency != CurrencyUSD {
		return Validated{}, invalid("currency %q must be MXN or USD", currency)
	}
	var amount *decimal.Decimal
	if in.Amount != nil {
		// Range first: the comparisons and the rounding below rescale the
		// decimal, which is only safe once its exponent is bounded.
		if err := checkAmount(*in.Amount); err != nil {
			return Validated{}, err
		}
		if !in.Amount.IsPositive() {
			return Validated{}, invalid("amount must be greater than zero, or empty for a variable bill")
		}
		if !in.Amount.Equal(in.Amount.Round(2)) {
			return Validated{}, invalid("amount must have at most 2 decimals")
		}
		a := *in.Amount
		amount = &a
	}
	anchor, err := AnchorDayOf(in.NextDueDate)
	if err != nil {
		return Validated{}, err
	}
	lead := DefaultReminderLeadDays
	if in.ReminderLeadDays != nil {
		lead = *in.ReminderLeadDays
	}
	if lead < 0 || lead > MaxReminderLeadDays {
		return Validated{}, invalid("reminder lead days must be between 0 and %d", MaxReminderLeadDays)
	}
	notes := strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(notes) > MaxNotesLength {
		return Validated{}, invalid("notes must be at most %d characters", MaxNotesLength)
	}
	return Validated{
		Name: name, Category: category, Amount: amount, Currency: currency, Recurrence: in.Recurrence,
		NextDueDate: in.NextDueDate, AnchorDay: anchor, ReminderLeadDays: lead, Active: in.Active, Notes: notes,
	}, nil
}

// PaymentInput is the raw data of a payment. Empty fields take the defaults of
// ResolvePayment.
type PaymentInput struct {
	Date        string
	Amount      *decimal.Decimal
	Category    string
	Description string
}

// Payment is a validated payment with its defaults applied.
type Payment struct {
	Date        string
	Amount      decimal.Decimal
	Category    string
	Description string
}

// ResolvePayment applies the payment defaults of a bill: date today, amount
// the bill's fixed amount (required for a variable bill and always
// overridable), category the bill's default and description the bill's name.
// Whether the category exists and the currency rules are the expenses module's.
func ResolvePayment(b Bill, in PaymentInput, today string) (Payment, error) {
	date := strings.TrimSpace(in.Date)
	if date == "" {
		date = today
	}
	if _, err := ParseDate(date); err != nil {
		return Payment{}, err
	}
	var amount decimal.Decimal
	switch {
	case in.Amount != nil:
		amount = *in.Amount
	case b.Amount != nil:
		amount = *b.Amount
	default:
		return Payment{}, invalid("amount is required: the bill has no fixed amount")
	}
	if err := checkAmount(amount); err != nil {
		return Payment{}, err
	}
	if !amount.IsPositive() {
		return Payment{}, invalid("amount must be greater than zero")
	}
	if !amount.Equal(amount.Round(2)) {
		return Payment{}, invalid("amount must have at most 2 decimals")
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		category = b.Category
	}
	description := strings.TrimSpace(in.Description)
	if description == "" {
		description = b.Name
	}
	return Payment{Date: date, Amount: amount, Category: category, Description: description}, nil
}
