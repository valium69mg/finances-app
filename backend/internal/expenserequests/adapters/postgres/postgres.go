// Package postgres implements the expense requests repository on a pgx pool.
//
// Money is NUMERIC. It crosses the driver as text (`$n::text::numeric` in,
// `col::text` out) so no precision is lost through a float.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/expenserequests/app"
	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	futureexpensespg "github.com/valium69mg/finances-app/backend/internal/futureexpenses/adapters/postgres"
	ledgerpg "github.com/valium69mg/finances-app/backend/internal/ledger/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

// Repo stores the requests in expense_requests.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const columns = `r.id, r.requester_id::text, u.email, r.amount::text, r.description, r.suggested_category,
	r.expense_date::text, r.status, r.decision_comment, r.decided_by::text, r.decided_at, r.result_kind,
	r.result_movement_id, r.result_future_expense_id, r.created_at, r.updated_at`

const from = ` FROM expense_requests r JOIN users u ON u.id = r.requester_id`

func scan(row pgx.Row) (domain.Request, error) {
	var (
		r                    domain.Request
		id                   int64
		amount, status       string
		category, comment    *string
		decidedBy, kind      *string
		decidedAt            *time.Time
		movementID, futureID *int64
		createdAt, updatedAt time.Time
	)
	if err := row.Scan(&id, &r.RequesterID, &r.RequesterEmail, &amount, &r.Description, &category, &r.ExpenseDate,
		&status, &comment, &decidedBy, &decidedAt, &kind, &movementID, &futureID, &createdAt, &updatedAt); err != nil {
		return domain.Request{}, err
	}
	r.ID, r.Status, r.DecidedAt, r.CreatedAt, r.UpdatedAt = int(id), domain.Status(status), decidedAt, createdAt, updatedAt
	var err error
	if r.Amount, err = decimal.NewFromString(amount); err != nil {
		return domain.Request{}, fmt.Errorf("parse amount: %w", err)
	}
	if category != nil {
		r.SuggestedCategory = *category
	}
	if comment != nil {
		r.DecisionComment = *comment
	}
	if decidedBy != nil {
		r.DecidedBy = *decidedBy
	}
	if kind != nil {
		r.ResultKind = domain.Destination(*kind)
	}
	if movementID != nil {
		v := int(*movementID)
		r.ResultMovementID = &v
	}
	if futureID != nil {
		v := int(*futureID)
		r.ResultFutureExpenseID = &v
	}
	return r, nil
}

// Create stores a pending request.
func (r *Repo) Create(ctx context.Context, requesterID string, v domain.Validated) (domain.Request, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO expense_requests (requester_id, amount, description, suggested_category, expense_date)
		VALUES ($1::text::uuid, $2::text::numeric, $3, NULLIF($4, ''), $5::date) RETURNING id`,
		requesterID, v.Amount.String(), v.Description, v.SuggestedCategory, v.Date).Scan(&id)
	if err != nil {
		return domain.Request{}, err
	}
	return r.Get(ctx, int(id))
}

// Get returns the request with the email of its requester.
func (r *Repo) Get(ctx context.Context, id int) (domain.Request, error) {
	req, err := scan(r.pool.QueryRow(ctx, `SELECT `+columns+from+` WHERE r.id = $1`, int64(id)))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Request{}, domain.ErrNotFound
	}
	return req, err
}

// List returns the requests newest first.
func (r *Repo) List(ctx context.Context, f app.ListFilter) ([]domain.Request, error) {
	var (
		where []string
		args  []any
	)
	if f.RequesterID != "" {
		args = append(args, f.RequesterID)
		where = append(where, fmt.Sprintf("r.requester_id = $%d::text::uuid", len(args)))
	}
	if f.Status != "" {
		args = append(args, string(f.Status))
		where = append(where, fmt.Sprintf("r.status = $%d", len(args)))
	}
	query := `SELECT ` + columns + from
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	rows, err := r.pool.Query(ctx, query+` ORDER BY r.created_at DESC, r.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Request{}
	for rows.Next() {
		req, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

// notPending tells why a guarded update touched no row: the request does not
// exist or it is already decided.
func (r *Repo) notPending(ctx context.Context, id int) error {
	if _, err := r.Get(ctx, id); err != nil {
		return err
	}
	return domain.ErrInvalidState
}

// Cancel moves a pending request of the requester to cancelada. The state is
// part of the UPDATE, so two concurrent decisions cannot both win.
func (r *Repo) Cancel(ctx context.Context, id int, requesterID string) (domain.Request, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE expense_requests SET status = 'cancelada', decided_at = now(), updated_at = now()
		WHERE id = $1 AND requester_id = $2::text::uuid AND status = 'solicitada'`,
		int64(id), requesterID)
	if err != nil {
		return domain.Request{}, err
	}
	if tag.RowsAffected() == 0 {
		req, err := r.Get(ctx, id)
		if err != nil {
			return domain.Request{}, err
		}
		if req.RequesterID != requesterID {
			return domain.Request{}, domain.ErrForbidden
		}
		return domain.Request{}, domain.ErrInvalidState
	}
	return r.Get(ctx, id)
}

// Reject moves a pending request to rechazada.
func (r *Repo) Reject(ctx context.Context, id int, decidedBy, comment string) (domain.Request, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE expense_requests
		SET status = 'rechazada', decided_by = $2::text::uuid, decided_at = now(), decision_comment = $3, updated_at = now()
		WHERE id = $1 AND status = 'solicitada'`,
		int64(id), decidedBy, comment)
	if err != nil {
		return domain.Request{}, err
	}
	if tag.RowsAffected() == 0 {
		return domain.Request{}, r.notPending(ctx, id)
	}
	return r.Get(ctx, id)
}

// Approve is one transaction. The request row is locked FOR UPDATE, so a
// second approval (or a concurrent cancel or reject) waits and then finds it
// decided. The Gasto (attributed to the requester through the session context
// the ledger insert reads) or the future expense and the status change commit
// together or not at all.
func (r *Repo) Approve(ctx context.Context, id int, decidedBy string, a app.Approval) (domain.Request, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Request{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status, requesterID string
	if err := tx.QueryRow(ctx, `SELECT status, requester_id::text FROM expense_requests WHERE id = $1 FOR UPDATE`, int64(id)).Scan(&status, &requesterID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Request{}, domain.ErrNotFound
		}
		return domain.Request{}, err
	}
	if domain.Status(status) != domain.StatusPending {
		return domain.Request{}, domain.ErrInvalidState
	}

	var movementID, futureID *int64
	switch a.Destination {
	case domain.DestinationExpense:
		if a.Expense == nil {
			return domain.Request{}, errors.New("approval without a Gasto")
		}
		asRequester := session.With(ctx, session.Identity{UserID: requesterID, Role: session.RoleHousehold})
		m, err := ledgerpg.Insert(asRequester, tx, *a.Expense)
		if err != nil {
			return domain.Request{}, fmt.Errorf("register the expense: %w", err)
		}
		v := int64(m.ID)
		movementID = &v
	case domain.DestinationFuture:
		if a.Future == nil {
			return domain.Request{}, errors.New("approval without a future expense")
		}
		fid, err := futureexpensespg.Insert(ctx, tx, *a.Future)
		if err != nil {
			return domain.Request{}, fmt.Errorf("create the future expense: %w", err)
		}
		v := int64(fid)
		futureID = &v
	default:
		return domain.Request{}, fmt.Errorf("unknown destination %q", a.Destination)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE expense_requests
		SET status = 'aprobada', decided_by = $2::text::uuid, decided_at = now(), result_kind = $3,
		    result_movement_id = $4, result_future_expense_id = $5, updated_at = now()
		WHERE id = $1`,
		int64(id), decidedBy, string(a.Destination), movementID, futureID); err != nil {
		return domain.Request{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Request{}, err
	}
	return r.Get(ctx, id)
}

// OwnerEmails returns the email of every active, verified owner.
func (r *Repo) OwnerEmails(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT email FROM users WHERE role = 'owner' AND active AND verified ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
