// Package domain holds the expense request rules: what a household user may
// ask for, the states a request goes through and what a decision must carry.
// Amounts are MXN only in v1.
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

var (
	// ErrInvalidInput wraps every validation failure (400).
	ErrInvalidInput = errors.New("invalid expense request")
	// ErrNotFound is returned for an unknown request id (404).
	ErrNotFound = errors.New("expense request not found")
	// ErrInvalidState is returned when a transition starts from a state other
	// than solicitada: a decided request is immutable (409).
	ErrInvalidState = errors.New("the request is not pending")
	// ErrForbidden is returned when the caller may not do the action (403).
	ErrForbidden = errors.New("forbidden")
	// ErrFutureExpensePaid is returned when a revert finds the future expense of
	// the approval already paid: it became a real expense, so the owner has to
	// undo that payment first (409).
	ErrFutureExpensePaid = errors.New("the future expense is already paid")
	// ErrRateLimited is returned when a user creates too many requests (429).
	ErrRateLimited = errors.New("too many requests")
)

// Status of a request.
type Status string

const (
	StatusPending   Status = "solicitada"
	StatusApproved  Status = "aprobada"
	StatusRejected  Status = "rechazada"
	StatusCancelled Status = "cancelada"
)

// Statuses lists every state, in the order the UI shows them.
var Statuses = []Status{StatusPending, StatusApproved, StatusRejected, StatusCancelled}

// ParseStatus validates a status filter.
func ParseStatus(s string) (Status, error) {
	for _, st := range Statuses {
		if string(st) == s {
			return st, nil
		}
	}
	return "", invalid("status must be one of solicitada, aprobada, rechazada, cancelada")
}

// Destination is what the owner decides to do with an approved request.
type Destination string

const (
	// DestinationExpense registers the request as a real Gasto.
	DestinationExpense Destination = "gasto"
	// DestinationFuture moves it to the Future Expenses module.
	DestinationFuture Destination = "gasto_futuro"
)

// Limits.
const (
	MaxDescriptionLength = 120 // the name of a future expense is at most 120 too
	MaxCategoryLength    = 80
	MaxCommentLength     = 500
	DateLayout           = "2006-01-02"
)

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// Request is one expense request.
type Request struct {
	ID                    int
	RequesterID           string
	RequesterEmail        string
	Amount                decimal.Decimal
	Description           string
	SuggestedCategory     string // empty when none
	ExpenseDate           string // YYYY-MM-DD
	Status                Status
	DecisionComment       string // set when rejected
	DecidedBy             string // empty for a cancellation
	DecidedAt             *time.Time
	ResultKind            Destination // empty unless approved
	ResultMovementID      *int
	ResultFutureExpenseID *int
	CreatedAt             time.Time
	UpdatedAt             time.Time
	// RevertCount and RevertedAt are the audit trail of the owner reverting an
	// approval back to solicitada (zero and nil when it never happened).
	RevertCount int
	RevertedAt  *time.Time
}

// Input is the raw data of a new request.
type Input struct {
	Amount            decimal.Decimal
	Description       string
	SuggestedCategory string
	Date              string // empty means today
}

// Validated is an Input after validation.
type Validated struct {
	Amount            decimal.Decimal
	Description       string
	SuggestedCategory string
	Date              string
}

// ValidateAmount checks a user-supplied amount: in range first (before any
// comparison or rounding), then positive with at most 2 decimals.
func ValidateAmount(v decimal.Decimal) error {
	if err := ledger.CheckAmount(v); err != nil {
		return invalid("amount is out of range, it must be below %s", ledger.MaxAmount)
	}
	if !v.IsPositive() {
		return invalid("amount must be greater than zero")
	}
	if !v.Equal(v.Round(2)) {
		return invalid("amount must have at most 2 decimals")
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

// Validate checks a new request. gastoCategories are the names of the Gasto
// categories; a suggested category must be one of them, with the exact spelling.
func Validate(in Input, gastoCategories []string, today string) (Validated, error) {
	if err := ValidateAmount(in.Amount); err != nil {
		return Validated{}, err
	}
	description := strings.TrimSpace(in.Description)
	switch {
	case description == "":
		return Validated{}, invalid("description is required")
	case utf8.RuneCountInString(description) > MaxDescriptionLength:
		return Validated{}, invalid("description must be at most %d characters", MaxDescriptionLength)
	}
	category := in.SuggestedCategory
	if category != "" {
		if utf8.RuneCountInString(category) > MaxCategoryLength || !contains(gastoCategories, category) {
			return Validated{}, invalid("suggested category must be one of the Gasto categories")
		}
	}
	date := strings.TrimSpace(in.Date)
	if date == "" {
		date = today
	} else if _, err := ParseDate("date", date); err != nil {
		return Validated{}, err
	}
	return Validated{Amount: in.Amount, Description: description, SuggestedCategory: category, Date: date}, nil
}

// ValidateComment checks the comment of a rejection: required and bounded.
func ValidateComment(s string) (string, error) {
	c := strings.TrimSpace(s)
	switch {
	case c == "":
		return "", invalid("comment is required")
	case utf8.RuneCountInString(c) > MaxCommentLength:
		return "", invalid("comment must be at most %d characters", MaxCommentLength)
	}
	return c, nil
}

// ValidateCategory checks that name is exactly the name of a Gasto category.
func ValidateCategory(name string, gastoCategories []string) error {
	if name == "" || !contains(gastoCategories, name) {
		return invalid("category must be one of the Gasto categories")
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// BudgetCheck is the owner's view of what approving a request as a Gasto does
// to the budget of its category in the pay cycle of the date. Budget,
// Remaining, ProjectedRemaining and Fits are nil when the category has no
// budget.
type BudgetCheck struct {
	Category           string
	Budget             *decimal.Decimal
	Spent              decimal.Decimal
	Remaining          *decimal.Decimal
	Amount             decimal.Decimal
	ProjectedSpent     decimal.Decimal
	ProjectedRemaining *decimal.Decimal
	Fits               *bool
	OverBy             decimal.Decimal
}

// NewBudgetCheck projects the request amount onto the spent total of a
// category. budget is nil when the category has none.
func NewBudgetCheck(category string, budget *decimal.Decimal, spent, amount decimal.Decimal) BudgetCheck {
	c := BudgetCheck{Category: category, Budget: budget, Spent: spent, Amount: amount, ProjectedSpent: spent.Add(amount), OverBy: decimal.Zero}
	if budget == nil {
		return c
	}
	remaining := budget.Sub(spent)
	projected := budget.Sub(c.ProjectedSpent)
	fits := !c.ProjectedSpent.GreaterThan(*budget)
	c.Remaining, c.ProjectedRemaining, c.Fits = &remaining, &projected, &fits
	if !fits {
		c.OverBy = c.ProjectedSpent.Sub(*budget)
	}
	return c
}
