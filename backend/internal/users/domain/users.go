// Package domain holds the pure rules of the users module: who may be created,
// which roles exist and the errors the use cases report. No I/O.
package domain

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	authdomain "github.com/valium69mg/finances-app/backend/internal/auth/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

var (
	// ErrNotFound means the user does not exist.
	ErrNotFound = errors.New("user not found")
	// ErrInvalidEmail means the email is not a plain, valid address.
	ErrInvalidEmail = errors.New("invalid email")
	// ErrInvalidRole means the role is unknown or not creatable (v1 creates household accounts only).
	ErrInvalidRole = errors.New("invalid role")
	// ErrEmailTaken means another account already uses the email.
	ErrEmailTaken = errors.New("email already in use")
	// ErrForbidden means the caller is not the owner.
	ErrForbidden = errors.New("forbidden")
	// ErrSelf means the owner tried to deactivate themselves.
	ErrSelf = errors.New("cannot change your own account")
	// ErrAlreadyVerified means the user already chose a password, so there is nothing to invite.
	ErrAlreadyVerified = errors.New("user already verified")
	// ErrInactive means the user is deactivated and cannot be invited.
	ErrInactive = errors.New("user is inactive")
	// ErrRateLimited means too many invitations were requested.
	ErrRateLimited = errors.New("rate limited")
	// ErrSendFailed means the invitation email could not be sent.
	ErrSendFailed = errors.New("invitation email failed")
)

// User is an account as the owner sees it. The password hash never leaves the
// repository.
type User struct {
	ID        string
	Email     string
	Role      session.Role
	Active    bool
	Verified  bool
	CreatedAt time.Time
}

// NewAccount is a validated account ready to be stored.
type NewAccount struct {
	Email        string
	Role         session.Role
	PasswordHash string
}

// Validate normalizes the email and checks the role. Only household accounts
// can be created in v1: the owner is seeded.
func Validate(rawEmail string, role session.Role) (email string, err error) {
	email, err = authdomain.NormalizeEmail(rawEmail)
	if err != nil {
		return "", ErrInvalidEmail
	}
	if role != session.RoleHousehold {
		return "", ErrInvalidRole
	}
	return email, nil
}

// RandomPasswordHash returns the bcrypt hash of a random password that nobody
// ever sees, the same primitive as the admin seed. The account gets a real
// password through the verification link.
func RandomPasswordHash() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	return authdomain.HashPassword(base64.RawURLEncoding.EncodeToString(buf))
}
