// Package postgres implements the reminders persistence on a pgx pool: the log
// of sent emails and the lookup of the address they go to.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/valium69mg/finances-app/backend/internal/reminders/domain"
)

// Repo stores the reminder log and finds the owner.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

// Has reports whether the key was recorded.
func (r *Repo) Has(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM reminder_log WHERE key = $1)`, key).Scan(&exists)
	return exists, err
}

// Record stores the key. The primary key makes it race-safe: recording a key
// that already exists keeps the first row and is not an error.
func (r *Repo) Record(ctx context.Context, key string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO reminder_log (key) VALUES ($1) ON CONFLICT (key) DO NOTHING`, key)
	return err
}

// LastSent returns when the most recent key starting with prefix was recorded,
// or nil when there is none.
func (r *Repo) LastSent(ctx context.Context, prefix string) (*time.Time, error) {
	var last *time.Time
	err := r.pool.QueryRow(ctx, `SELECT max(sent_at) FROM reminder_log WHERE starts_with(key, $1)`, prefix).Scan(&last)
	return last, err
}

// OwnerEmail returns the email of the first verified user, by creation time: the
// app has a single owner. It returns domain.ErrNoOwner when nobody is verified.
func (r *Repo) OwnerEmail(ctx context.Context) (string, error) {
	var email string
	err := r.pool.QueryRow(ctx, `SELECT email FROM users WHERE verified ORDER BY created_at, id LIMIT 1`).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNoOwner
	}
	return email, err
}
