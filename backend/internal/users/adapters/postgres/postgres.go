// Package postgres implements the users repository on a pgx pool.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/valium69mg/finances-app/backend/internal/users/domain"
)

// Repo stores accounts in the users table.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// The password hash is deliberately not selected: it never leaves the table.
const columns = `id::text, email, role, active, verified, created_at`

func scan(row pgx.Row) (domain.User, error) {
	var u domain.User
	err := row.Scan(&u.ID, &u.Email, &u.Role, &u.Active, &u.Verified, &u.CreatedAt)
	return u, err
}

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// List returns every account, oldest first.
func (r *Repo) List(ctx context.Context) ([]domain.User, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+columns+` FROM users ORDER BY created_at, email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.User{}
	for rows.Next() {
		u, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// Get returns the account; a malformed id is reported as not found.
func (r *Repo) Get(ctx context.Context, id string) (domain.User, error) {
	u, err := scan(r.pool.QueryRow(ctx, `SELECT `+columns+` FROM users WHERE id = $1::text::uuid`, id))
	if errors.Is(err, pgx.ErrNoRows) || pgCode(err) == "22P02" {
		return domain.User{}, domain.ErrNotFound
	}
	return u, err
}

// Create inserts an unverified, active account.
func (r *Repo) Create(ctx context.Context, acc domain.NewAccount) (domain.User, error) {
	u, err := scan(r.pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, verified, role, active)
		VALUES ($1, $2, false, $3, true)
		RETURNING `+columns, acc.Email, acc.PasswordHash, string(acc.Role)))
	if pgCode(err) == "23505" {
		return domain.User{}, domain.ErrEmailTaken
	}
	return u, err
}

// SetActive flips the active flag of the account.
func (r *Repo) SetActive(ctx context.Context, id string, active bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE users SET active = $2 WHERE id = $1::text::uuid`, id, active)
	if pgCode(err) == "22P02" {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}
