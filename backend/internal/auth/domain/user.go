package domain

import (
	"net/mail"
	"strings"

	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

const maxEmailLength = 254

// User is an account that can log in.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	Verified     bool
	// Role is what the account may do; Active=false blocks login and refresh.
	Role   session.Role
	Active bool
}

// NormalizeEmail trims and lower-cases an email and rejects anything that is not a bare address.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > maxEmailLength {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", ErrInvalidEmail
	}
	return email, nil
}
