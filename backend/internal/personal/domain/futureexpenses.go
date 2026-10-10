package domain

import (
	"fmt"
	"strings"
	"unicode/utf8"

	fe "github.com/valium69mg/finances-app/backend/internal/futureexpenses/domain"
)

// The future expenses of a household user follow the owner's model exactly, so
// the pure pieces (validation, suggested monthly amount, cycle plan, release
// of a payment) are the owner's, imported rather than copied. Only the storage
// differs (her own tables, see the postgres adapter).
type (
	FutureExpense   = fe.FutureExpense
	FutureInput     = fe.Input
	FutureValidated = fe.Validated
	FuturePlanned   = fe.Planned
	FuturePlan      = fe.Plan
	FutureStatus    = fe.Status
)

// Statuses of an item.
const (
	FutureActive = fe.StatusActive
	FuturePaid   = fe.StatusPaid
)

// SavingsCategory is the fixed Ahorro category every saving of the module uses.
const SavingsCategory = fe.SavingsCategory

// Errors of the future expenses. They are the owner's values so a handler can
// map them with errors.Is; ErrFutureInvalid is the 400 invalid_future_expense.
var (
	ErrFutureInvalid           = fe.ErrInvalidInput
	ErrAlreadyPaid             = fe.ErrAlreadyPaid
	ErrInsufficientFreeBalance = fe.ErrInsufficientFreeBalance
)

// Re-exported rules of the owner's module.
var (
	ValidateFuture = fe.Validate
	ValidateMoney  = fe.ValidateMoney
	BuildPlan      = fe.BuildPlan
	PlanItem       = fe.PlanItem
	Release        = fe.Release
)

// TrimDescription cuts a generated description to the movement limit, so a
// long item name never makes a saving or an expense fail its CHECK.
func TrimDescription(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= MaxDescriptionLength {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:MaxDescriptionLength]))
}

// ValidateSavingDescription trims a user-supplied description of a
// contribution and checks its length. Empty is allowed (a default is used).
func ValidateSavingDescription(s string) (string, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxDescriptionLength {
		return "", fmt.Errorf("%w: description must be at most %d characters", fe.ErrInvalidInput, MaxDescriptionLength)
	}
	return s, nil
}
