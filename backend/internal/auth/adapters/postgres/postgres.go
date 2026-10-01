// Package postgres implements the auth repositories on a pgx pool.
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
)

// UserRepo stores users in the users table.
type UserRepo struct{ pool *pgxpool.Pool }

// NewUserRepo builds a UserRepo.
func NewUserRepo(pool *pgxpool.Pool) *UserRepo { return &UserRepo{pool: pool} }

const userColumns = `id::text, email, password_hash, verified, role, active`

func (r *UserRepo) find(ctx context.Context, where string, arg any) (domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE `+where, arg).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Verified, &u.Role, &u.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	return u, err
}

// FindByEmail returns the user with the (already normalized) email.
func (r *UserRepo) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.find(ctx, `email = $1`, email)
}

// FindByID returns the user with the given id. A malformed id is reported as not found.
func (r *UserRepo) FindByID(ctx context.Context, id string) (domain.User, error) {
	u, err := r.find(ctx, `id = $1::uuid`, id)
	if isInvalidText(err) {
		return domain.User{}, domain.ErrNotFound
	}
	return u, err
}

// SetPasswordAndVerify stores the new password hash and marks the user verified.
func (r *UserRepo) SetPasswordAndVerify(ctx context.Context, userID, passwordHash string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, verified = true WHERE id = $1::uuid`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}

// CreateIfMissing inserts an unverified user and reports whether a row was
// created. An existing email is left untouched.
func (r *UserRepo) CreateIfMissing(ctx context.Context, email, passwordHash string) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, verified) VALUES ($1, $2, false)
		 ON CONFLICT (email) DO NOTHING`, email, passwordHash)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// RefreshTokenRepo stores hashed refresh tokens.
type RefreshTokenRepo struct{ pool *pgxpool.Pool }

// NewRefreshTokenRepo builds a RefreshTokenRepo.
func NewRefreshTokenRepo(pool *pgxpool.Pool) *RefreshTokenRepo { return &RefreshTokenRepo{pool: pool} }

// Create stores a refresh token.
func (r *RefreshTokenRepo) Create(ctx context.Context, t domain.RefreshToken) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO refresh_tokens (token_hash, user_id, expires_at) VALUES ($1, $2::uuid, $3)`,
		t.Hash, t.UserID, t.ExpiresAt)
	return err
}

// Find returns the token with the given hash.
func (r *RefreshTokenRepo) Find(ctx context.Context, hash string) (domain.RefreshToken, error) {
	var t domain.RefreshToken
	err := r.pool.QueryRow(ctx,
		`SELECT token_hash, user_id::text, expires_at, used_at, revoked_at
		 FROM refresh_tokens WHERE token_hash = $1`, hash).
		Scan(&t.Hash, &t.UserID, &t.ExpiresAt, &t.UsedAt, &t.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RefreshToken{}, domain.ErrNotFound
	}
	return t, err
}

// MarkUsed atomically marks a live token as used; false means it was not live.
func (r *RefreshTokenRepo) MarkUsed(ctx context.Context, hash string, now time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET used_at = $2
		 WHERE token_hash = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > $2`,
		hash, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// Revoke revokes one token.
func (r *RefreshTokenRepo) Revoke(ctx context.Context, hash string, now time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2 WHERE token_hash = $1 AND revoked_at IS NULL`, hash, now)
	return err
}

// RevokeAllForUser revokes every non-revoked refresh token of the user.
func (r *RefreshTokenRepo) RevokeAllForUser(ctx context.Context, userID string, now time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = $2 WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID, now)
	return err
}

// VerificationTokenRepo stores hashed verification tokens.
type VerificationTokenRepo struct{ pool *pgxpool.Pool }

// NewVerificationTokenRepo builds a VerificationTokenRepo.
func NewVerificationTokenRepo(pool *pgxpool.Pool) *VerificationTokenRepo {
	return &VerificationTokenRepo{pool: pool}
}

// Create stores a verification token.
func (r *VerificationTokenRepo) Create(ctx context.Context, t domain.VerificationToken) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO verification_tokens (token_hash, user_id, expires_at) VALUES ($1, $2::uuid, $3)`,
		t.Hash, t.UserID, t.ExpiresAt)
	return err
}

// Consume atomically marks an unused, unexpired token as used and returns its user id.
func (r *VerificationTokenRepo) Consume(ctx context.Context, hash string, now time.Time) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx,
		`UPDATE verification_tokens SET used_at = $2
		 WHERE token_hash = $1 AND used_at IS NULL AND expires_at > $2
		 RETURNING user_id::text`, hash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return userID, err
}
