// Package postgres implements the personal finances repository on a pgx pool.
//
// Money is NUMERIC. It crosses the driver as text (`$n::text::numeric` in,
// `col::text` out) so no precision is lost through a float. Every query is
// scoped by user_id, so a row of another user is indistinguishable from a
// missing one.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/personal/app"
	"github.com/valium69mg/finances-app/backend/internal/personal/domain"
)

// Postgres error codes handled here.
const (
	codeInvalidText = "22P02" // malformed uuid
	codeForeignKey  = "23503"
	codeUnique      = "23505"
	codeCheck       = "23514"
)

// Repo stores the categories and movements in personal_categories and
// personal_movements.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// --- categories ---------------------------------------------------------

const categoryColumns = `c.id, c.kind, c.name, c.created_at`

func scanCategory(row pgx.Row) (domain.Category, error) {
	var (
		c    domain.Category
		id   int64
		kind string
	)
	if err := row.Scan(&id, &kind, &c.Name, &c.CreatedAt); err != nil {
		return domain.Category{}, err
	}
	c.ID, c.Kind = int(id), domain.Kind(kind)
	return c, nil
}

// SeedDefaults inserts, in one transaction, the default categories of every
// kind. The fixed kinds (Ingreso and Ahorro) always ensure every fixed name
// exists, so a category added later (Gastos futuros) reaches users seeded
// before; they cannot be renamed, so this never re-adds a renamed one. The
// editable kind (Gasto) is seeded only while the user has none, so a renamed
// default is never re-added. ON CONFLICT DO NOTHING makes it idempotent and
// concurrent first uses converge on the same rows.
func (r *Repo) SeedDefaults(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, kind := range domain.Kinds {
		if _, err := tx.Exec(ctx, `
			INSERT INTO personal_categories (user_id, kind, name)
			SELECT $1::text::uuid, $2::text, t.name
			FROM unnest($3::text[]) WITH ORDINALITY AS t(name, ord)
			WHERE NOT $4::boolean
			   OR NOT EXISTS (SELECT 1 FROM personal_categories WHERE user_id = $1::text::uuid AND kind = $2::text)
			ORDER BY t.ord
			ON CONFLICT DO NOTHING`,
			userID, string(kind), domain.Defaults[kind], kind.Editable()); err != nil {
			return fmt.Errorf("seed %s categories: %w", kind, err)
		}
	}
	return tx.Commit(ctx)
}

// ListCategories returns the categories ordered Ingreso, Gasto, Ahorro, then id.
func (r *Repo) ListCategories(ctx context.Context, userID string, kind domain.Kind) ([]domain.Category, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+categoryColumns+` FROM personal_categories c
		WHERE c.user_id = $1::text::uuid AND ($2::text = '' OR c.kind = $2::text)
		ORDER BY CASE c.kind WHEN 'Ingreso' THEN 1 WHEN 'Gasto' THEN 2 ELSE 3 END, c.id`,
		userID, string(kind))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Category{}
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCategory returns one category of the user.
func (r *Repo) GetCategory(ctx context.Context, userID string, id int) (domain.Category, error) {
	c, err := scanCategory(r.pool.QueryRow(ctx, `
		SELECT `+categoryColumns+` FROM personal_categories c
		WHERE c.id = $1 AND c.user_id = $2::text::uuid`, int64(id), userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Category{}, domain.ErrNotFound
	}
	return c, err
}

// CreateCategory stores a Gasto category.
func (r *Repo) CreateCategory(ctx context.Context, userID, name string) (domain.Category, error) {
	c, err := scanCategory(r.pool.QueryRow(ctx, `
		INSERT INTO personal_categories AS c (user_id, kind, name)
		VALUES ($1::text::uuid, 'Gasto', $2)
		RETURNING `+categoryColumns, userID, name))
	if pgCode(err) == codeUnique {
		return domain.Category{}, domain.ErrDuplicateCategory
	}
	return c, err
}

// RenameCategory renames a Gasto category of the user.
func (r *Repo) RenameCategory(ctx context.Context, userID string, id int, name string) (domain.Category, error) {
	c, err := scanCategory(r.pool.QueryRow(ctx, `
		UPDATE personal_categories AS c SET name = $3
		WHERE c.id = $1 AND c.user_id = $2::text::uuid AND c.kind = 'Gasto'
		RETURNING `+categoryColumns, int64(id), userID, name))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.Category{}, domain.ErrNotFound
	case pgCode(err) == codeUnique:
		return domain.Category{}, domain.ErrDuplicateCategory
	}
	return c, err
}

// --- movements ----------------------------------------------------------

const movementColumns = `m.id, m.kind, m.category_id, c.name, m.amount::text, m.description, m.date::text, m.future_expense_id, m.created_at, m.updated_at`

func scanMovement(row pgx.Row) (domain.Movement, error) {
	var (
		m                    domain.Movement
		id, categoryID       int64
		kind, amount         string
		futureID             *int64
		createdAt, updatedAt time.Time
	)
	if err := row.Scan(&id, &kind, &categoryID, &m.CategoryName, &amount, &m.Description, &m.Date, &futureID, &createdAt, &updatedAt); err != nil {
		return domain.Movement{}, err
	}
	if futureID != nil {
		f := int(*futureID)
		m.FutureExpenseID = &f
	}
	m.ID, m.Kind, m.CategoryID, m.CreatedAt, m.UpdatedAt = int(id), domain.Kind(kind), int(categoryID), createdAt, updatedAt
	var err error
	if m.Amount, err = decimal.NewFromString(amount); err != nil {
		return domain.Movement{}, fmt.Errorf("parse amount: %w", err)
	}
	return m, nil
}

// mapWriteError turns the constraint violations of a write into the domain
// errors: the composite foreign key rejects a category of another user or kind,
// the CHECK rejects a wrong amount sign.
func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.ConstraintName == "personal_movements_future_expense_savings_only" {
		return fmt.Errorf("%w: a saving linked to a future expense must stay an Ahorro", domain.ErrInvalidInput)
	}
	switch pgCode(err) {
	case codeForeignKey:
		return fmt.Errorf("%w: category does not match the movement", domain.ErrInvalidInput)
	case codeCheck:
		return fmt.Errorf("%w: amount is not allowed for this kind", domain.ErrInvalidInput)
	}
	return err
}

// ListMovements returns the movements of the user in the inclusive date range,
// newest first (date, then id).
func (r *Repo) ListMovements(ctx context.Context, userID string, f app.Filter) ([]domain.Movement, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+movementColumns+`
		FROM personal_movements m JOIN personal_categories c ON c.id = m.category_id
		WHERE m.user_id = $1::text::uuid AND m.date BETWEEN $2::date AND $3::date
		  AND ($4::text = '' OR m.kind = $4::text) AND ($5::bigint = 0 OR m.category_id = $5::bigint)
		ORDER BY m.date DESC, m.id DESC`,
		userID, f.From, f.To, string(f.Kind), int64(f.CategoryID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Movement{}
	for rows.Next() {
		m, err := scanMovement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// CreateMovement stores a movement.
func (r *Repo) CreateMovement(ctx context.Context, userID string, v domain.ValidMovement) (domain.Movement, error) {
	return insertMovement(ctx, r.pool, userID, v, nil)
}

// rowQuerier is the QueryRow half of a pool or a transaction.
type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// insertMovement is the one INSERT of personal_movements. futureID links an
// Ahorro to a future expense of the same user (the composite foreign key
// rejects any other).
func insertMovement(ctx context.Context, q rowQuerier, userID string, v domain.ValidMovement, futureID *int64) (domain.Movement, error) {
	m, err := scanMovement(q.QueryRow(ctx, `
		WITH m AS (
			INSERT INTO personal_movements (user_id, kind, category_id, amount, description, date, future_expense_id)
			VALUES ($1::text::uuid, $2, $3, $4::text::numeric, $5, $6::date, $7::bigint)
			RETURNING *)
		SELECT `+movementColumns+` FROM m JOIN personal_categories c ON c.id = m.category_id`,
		userID, string(v.Kind), int64(v.CategoryID), v.Amount.String(), v.Description, v.Date, futureID))
	if err != nil {
		return domain.Movement{}, mapWriteError(err)
	}
	return m, nil
}

// UpdateMovement replaces a movement of the user.
func (r *Repo) UpdateMovement(ctx context.Context, userID string, id int, v domain.ValidMovement) (domain.Movement, error) {
	m, err := scanMovement(r.pool.QueryRow(ctx, `
		WITH m AS (
			UPDATE personal_movements SET kind = $3, category_id = $4, amount = $5::text::numeric,
				description = $6, date = $7::date, updated_at = now()
			WHERE id = $1 AND user_id = $2::text::uuid
			RETURNING *)
		SELECT `+movementColumns+` FROM m JOIN personal_categories c ON c.id = m.category_id`,
		int64(id), userID, string(v.Kind), int64(v.CategoryID), v.Amount.String(), v.Description, v.Date))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Movement{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Movement{}, mapWriteError(err)
	}
	return m, nil
}

// DeleteMovement removes a movement of the user.
func (r *Repo) DeleteMovement(ctx context.Context, userID string, id int) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM personal_movements WHERE id = $1 AND user_id = $2::text::uuid`, int64(id), userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// --- dashboard ----------------------------------------------------------

// KindTotals sums the movements of the user by kind in the inclusive range.
// Savings are net: a withdrawal is a negative row.
func (r *Repo) KindTotals(ctx context.Context, userID, from, to string) (domain.KindTotals, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT kind, SUM(amount)::text FROM personal_movements
		WHERE user_id = $1::text::uuid AND date BETWEEN $2::date AND $3::date
		GROUP BY kind`, userID, from, to)
	if err != nil {
		return domain.KindTotals{}, err
	}
	defer rows.Close()
	var t domain.KindTotals
	for rows.Next() {
		var kind, sum string
		if err := rows.Scan(&kind, &sum); err != nil {
			return domain.KindTotals{}, err
		}
		v, err := decimal.NewFromString(sum)
		if err != nil {
			return domain.KindTotals{}, fmt.Errorf("parse sum: %w", err)
		}
		switch domain.Kind(kind) {
		case domain.KindIncome:
			t.Income = v
		case domain.KindExpense:
			t.Expenses = v
		case domain.KindSavings:
			t.Savings = v
		}
	}
	return t, rows.Err()
}

// SpentByCategory returns the Gasto categories with spending in the range,
// biggest first (name breaks ties).
func (r *Repo) SpentByCategory(ctx context.Context, userID, from, to string) ([]domain.CategorySpend, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT c.id, c.name, SUM(m.amount)::text AS spent
		FROM personal_movements m JOIN personal_categories c ON c.id = m.category_id
		WHERE m.user_id = $1::text::uuid AND m.kind = 'Gasto' AND m.date BETWEEN $2::date AND $3::date
		GROUP BY c.id, c.name
		HAVING SUM(m.amount) > 0
		ORDER BY SUM(m.amount) DESC, c.name, c.id`, userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CategorySpend{}
	for rows.Next() {
		var (
			s     domain.CategorySpend
			id    int64
			spent string
		)
		if err := rows.Scan(&id, &s.Category, &spent); err != nil {
			return nil, err
		}
		s.CategoryID = int(id)
		if s.Spent, err = decimal.NewFromString(spent); err != nil {
			return nil, fmt.Errorf("parse spent: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

var _ app.Repo = (*Repo)(nil)
