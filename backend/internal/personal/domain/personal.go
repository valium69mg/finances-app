// Package domain holds the pure model of the personal finances module: the
// own ledger of a household user (income, expenses and savings in MXN), fully
// separate from the family movements of the owner. Categories are rows with an
// id; income and savings categories are fixed, expense categories are hers.
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
	ErrInvalidInput = errors.New("invalid personal finance input")
	ErrNotFound     = errors.New("personal finance record not found")
	// ErrForbidden is returned to any role other than household.
	ErrForbidden = errors.New("forbidden")
	// ErrCategoryFixed is returned when a fixed (Ingreso or Ahorro) category is renamed.
	ErrCategoryFixed = errors.New("category is fixed")
	// ErrDuplicateCategory is returned when the name already exists for the kind.
	ErrDuplicateCategory = errors.New("category name already exists")
)

// Kind is the type of a category or movement.
type Kind string

const (
	KindIncome  Kind = "Ingreso"
	KindExpense Kind = "Gasto"
	KindSavings Kind = "Ahorro"
)

// Limits.
const (
	MaxNameLength        = 80
	MaxDescriptionLength = 120
	DateLayout           = "2006-01-02"
)

// Kinds is every kind, in the order they are seeded and listed.
var Kinds = []Kind{KindIncome, KindExpense, KindSavings}

// Valid reports whether k is one of the three kinds.
func (k Kind) Valid() bool { return k == KindIncome || k == KindExpense || k == KindSavings }

// Editable reports whether the user can add and rename categories of the kind.
func (k Kind) Editable() bool { return k == KindExpense }

// Defaults are the categories seeded once per user for each kind.
var Defaults = map[Kind][]string{
	KindIncome:  {"Trabajo", "Transferencia", "Otros"},
	KindExpense: {"Belleza", "Salud", "Tratamientos", "Tienda de conveniencia", "Ropa", "Pintura", "Maquillaje", "Compras en línea"},
	KindSavings: {"Ahorro", "Inversión", ledger.CategoryFutureExpenses},
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, fmt.Sprintf(format, args...))
}

// Category is one category of a user.
type Category struct {
	ID        int
	Kind      Kind
	Name      string
	CreatedAt time.Time
}

// Movement is one row of the personal ledger. Amount is positive except for an
// Ahorro, where a negative amount is a withdrawal.
type Movement struct {
	ID           int
	Kind         Kind
	CategoryID   int
	CategoryName string
	Amount       decimal.Decimal
	Description  string
	Date         string // YYYY-MM-DD
	// FutureExpenseID is the future expense an Ahorro feeds, nil otherwise.
	FutureExpenseID *int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ParseKind checks a kind name.
func ParseKind(s string) (Kind, error) {
	k := Kind(strings.TrimSpace(s))
	if !k.Valid() {
		return "", invalid("kind must be Ingreso, Gasto or Ahorro")
	}
	return k, nil
}

// ValidateCategoryName trims and checks the name of a category.
func ValidateCategoryName(name string) (string, error) {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return "", invalid("name is required")
	case utf8.RuneCountInString(name) > MaxNameLength:
		return "", invalid("name must be at most %d characters", MaxNameLength)
	}
	return name, nil
}

// MovementInput is the raw data of a new or updated movement.
type MovementInput struct {
	Kind        string
	CategoryID  int
	Amount      decimal.Decimal
	Description string
	Date        string
}

// ValidMovement is a MovementInput after validation.
type ValidMovement struct {
	Kind        Kind
	CategoryID  int
	Amount      decimal.Decimal
	Description string
	Date        string
}

// ValidateAmount checks a user-supplied amount: in range first (before any
// comparison or rounding), at most 2 decimals, positive, except for an Ahorro
// where any non-zero amount is allowed (negative is a withdrawal).
func ValidateAmount(kind Kind, v decimal.Decimal) error {
	if err := ledger.CheckAmount(v); err != nil {
		return invalid("amount is out of range, it must be below %s", ledger.MaxAmount)
	}
	if !v.Equal(v.Round(2)) {
		return invalid("amount must have at most 2 decimals")
	}
	if kind == KindSavings {
		if v.IsZero() {
			return invalid("amount must not be zero")
		}
		return nil
	}
	if !v.IsPositive() {
		return invalid("amount must be greater than zero")
	}
	return nil
}

// ParseDate checks a YYYY-MM-DD date.
func ParseDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if _, err := time.Parse(DateLayout, s); err != nil {
		return "", invalid("date must be like YYYY-MM-DD")
	}
	return s, nil
}

// ValidateMovement checks the input of a create or an update.
func ValidateMovement(in MovementInput) (ValidMovement, error) {
	kind, err := ParseKind(in.Kind)
	if err != nil {
		return ValidMovement{}, err
	}
	if in.CategoryID <= 0 {
		return ValidMovement{}, invalid("category_id is required")
	}
	if err := ValidateAmount(kind, in.Amount); err != nil {
		return ValidMovement{}, err
	}
	description := strings.TrimSpace(in.Description)
	switch {
	case description == "":
		return ValidMovement{}, invalid("description is required")
	case utf8.RuneCountInString(description) > MaxDescriptionLength:
		return ValidMovement{}, invalid("description must be at most %d characters", MaxDescriptionLength)
	}
	date, err := ParseDate(in.Date)
	if err != nil {
		return ValidMovement{}, err
	}
	return ValidMovement{Kind: kind, CategoryID: in.CategoryID, Amount: in.Amount, Description: description, Date: date}, nil
}
