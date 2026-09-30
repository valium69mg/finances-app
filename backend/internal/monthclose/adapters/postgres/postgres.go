// Package postgres implements the month close repository on a pgx pool: the
// month_closes table. The totals are NUMERIC columns and the per-category rows,
// the suggestion and the hints are JSONB documents whose money values are
// decimal strings. Money crosses the driver as text (`$n::text::numeric` in,
// `col::text` out) so no precision is lost through a float.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/monthclose/app"
	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

// Repo stores month closes in the month_closes table.
type Repo struct{ pool *pgxpool.Pool }

var _ app.Repo = (*Repo)(nil)

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const (
	pgUniqueViolation = "23505"
	periodConstraint  = "month_closes_pkey"
)

const closeColumns = `period, closed_at, income::text, expenses::text, savings::text, available::text,
	emergency_accumulated::text, emergency_goal::text, tax_filing_status, categories, suggestion, adjustments`

// The JSON documents keep their own field names, independent of the domain
// structs, so renaming a Go field never rewrites what is already stored.
// decimal.Decimal marshals as a quoted decimal string.
type categoryDoc struct {
	Name       string           `json:"name"`
	Budget     *decimal.Decimal `json:"budget"`
	Spent      decimal.Decimal  `json:"spent"`
	Remaining  *decimal.Decimal `json:"remaining"`
	OverBudget bool             `json:"over_budget"`
}

type suggestionDoc struct {
	ToEmergencyFund   decimal.Decimal `json:"to_emergency_fund"`
	ToInvestments     decimal.Decimal `json:"to_investments"`
	ToFutureExpenses  decimal.Decimal `json:"to_future_expenses"`
	InvestmentsPaused bool            `json:"investments_paused"`
}

type adjustmentDoc struct {
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	Budget       decimal.Decimal `json:"budget"`
	Real         decimal.Decimal `json:"real"`
	DeviationPct decimal.Decimal `json:"deviation_pct"`
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == periodConstraint
}

// Create stores the close. closed_at is taken from the close (now when zero).
func (r *Repo) Create(ctx context.Context, c monthclose.Close) (monthclose.Close, error) {
	cats := make([]categoryDoc, len(c.Categories))
	for i, x := range c.Categories {
		cats[i] = categoryDoc{Name: x.Name, Budget: x.Budget, Spent: x.Spent, Remaining: x.Remaining, OverBudget: x.OverBudget}
	}
	adjs := make([]adjustmentDoc, len(c.Adjustments))
	for i, a := range c.Adjustments {
		adjs[i] = adjustmentDoc{Name: a.Name, Kind: string(a.Kind), Budget: a.Budget, Real: a.Real, DeviationPct: a.DeviationPct}
	}
	catsJSON, err := json.Marshal(cats)
	if err != nil {
		return monthclose.Close{}, err
	}
	adjsJSON, err := json.Marshal(adjs)
	if err != nil {
		return monthclose.Close{}, err
	}
	var suggestionJSON []byte
	if s := c.Suggestion; s != nil {
		if suggestionJSON, err = json.Marshal(suggestionDoc{
			ToEmergencyFund: s.ToEmergencyFund, ToInvestments: s.ToInvestments,
			ToFutureExpenses: s.ToFutureExpenses, InvestmentsPaused: s.InvestmentsPaused,
		}); err != nil {
			return monthclose.Close{}, err
		}
	}
	var status *string
	if c.FilingStatus != "" {
		s := string(c.FilingStatus)
		status = &s
	}
	var closedAt *time.Time
	if !c.ClosedAt.IsZero() {
		closedAt = &c.ClosedAt
	}

	saved, err := scanClose(r.pool.QueryRow(ctx, `
		INSERT INTO month_closes (period, closed_at, income, expenses, savings, available,
		                          emergency_accumulated, emergency_goal, tax_filing_status,
		                          categories, suggestion, adjustments)
		VALUES ($1, COALESCE($2, now()), $3::text::numeric, $4::text::numeric, $5::text::numeric, $6::text::numeric,
		        $7::text::numeric, $8::text::numeric, $9, $10::jsonb, $11::jsonb, $12::jsonb)
		RETURNING `+closeColumns,
		c.Period, closedAt, c.Income.String(), c.Expenses.String(), c.Savings.String(), c.Available.String(),
		c.Emergency.Accumulated.String(), c.Emergency.Goal.String(), status, catsJSON, suggestionJSON, adjsJSON))
	if isUniqueViolation(err) {
		return monthclose.Close{}, fmt.Errorf("%w: %s", monthclose.ErrAlreadyClosed, c.Period)
	}
	return saved, err
}

// Get returns the close of a period.
func (r *Repo) Get(ctx context.Context, period string) (monthclose.Close, error) {
	c, err := scanClose(r.pool.QueryRow(ctx, `SELECT `+closeColumns+` FROM month_closes WHERE period = $1`, period))
	if errors.Is(err, pgx.ErrNoRows) {
		return monthclose.Close{}, monthclose.ErrNotFound
	}
	return c, err
}

// List returns every close, newest period first.
func (r *Repo) List(ctx context.Context) ([]monthclose.Close, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+closeColumns+` FROM month_closes ORDER BY period DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []monthclose.Close{}
	for rows.Next() {
		c, err := scanClose(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Delete removes the close of a period. Movements are never touched.
func (r *Repo) Delete(ctx context.Context, period string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM month_closes WHERE period = $1`, period)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return monthclose.ErrNotFound
	}
	return nil
}

func scanClose(row pgx.Row) (monthclose.Close, error) {
	var (
		c                                                     monthclose.Close
		income, expenses, saved, available, accumulated, goal string
		status                                                *string
		cats, suggestion, adjs                                []byte
	)
	if err := row.Scan(&c.Period, &c.ClosedAt, &income, &expenses, &saved, &available,
		&accumulated, &goal, &status, &cats, &suggestion, &adjs); err != nil {
		return monthclose.Close{}, err
	}
	c.ClosedAt = c.ClosedAt.UTC()
	var err error
	for _, v := range []struct {
		name string
		raw  string
		dst  *decimal.Decimal
	}{
		{"income", income, &c.Income}, {"expenses", expenses, &c.Expenses}, {"savings", saved, &c.Savings},
		{"available", available, &c.Available}, {"emergency_accumulated", accumulated, &c.Emergency.Accumulated},
		{"emergency_goal", goal, &c.Emergency.Goal},
	} {
		if *v.dst, err = decimal.NewFromString(v.raw); err != nil {
			return monthclose.Close{}, fmt.Errorf("parse %s: %w", v.name, err)
		}
	}
	if status != nil {
		c.FilingStatus = taxfiling.PaymentStatus(*status)
	}

	var catDocs []categoryDoc
	if err := json.Unmarshal(cats, &catDocs); err != nil {
		return monthclose.Close{}, fmt.Errorf("parse categories: %w", err)
	}
	c.Categories = make([]monthclose.Category, len(catDocs))
	for i, x := range catDocs {
		c.Categories[i] = monthclose.Category{Name: x.Name, Budget: x.Budget, Spent: x.Spent, Remaining: x.Remaining, OverBudget: x.OverBudget}
	}
	if suggestion != nil {
		var doc suggestionDoc
		if err := json.Unmarshal(suggestion, &doc); err != nil {
			return monthclose.Close{}, fmt.Errorf("parse suggestion: %w", err)
		}
		c.Suggestion = &monthclose.Suggestion{
			ToEmergencyFund: doc.ToEmergencyFund, ToInvestments: doc.ToInvestments,
			ToFutureExpenses: doc.ToFutureExpenses, InvestmentsPaused: doc.InvestmentsPaused,
		}
	}
	var adjDocs []adjustmentDoc
	if err := json.Unmarshal(adjs, &adjDocs); err != nil {
		return monthclose.Close{}, fmt.Errorf("parse adjustments: %w", err)
	}
	c.Adjustments = make([]monthclose.Adjustment, len(adjDocs))
	for i, a := range adjDocs {
		c.Adjustments[i] = monthclose.Adjustment{Name: a.Name, Kind: ledger.Kind(a.Kind), Budget: a.Budget, Real: a.Real, DeviationPct: a.DeviationPct}
	}
	return c, nil
}
