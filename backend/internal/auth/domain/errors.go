// Package domain holds the pure authentication rules: users, password policy,
// opaque token generation/hashing and access-token claims. No I/O.
package domain

import "errors"

var (
	// ErrNotFound is returned by repositories when a record does not exist (or is not usable).
	ErrNotFound = errors.New("not found")
	// ErrInvalidEmail means the email is not a plain, valid address.
	ErrInvalidEmail = errors.New("invalid email")
	// ErrInvalidCredentials means a verified user presented a wrong password.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidToken means a token is unknown, expired, used, revoked or malformed.
	ErrInvalidToken = errors.New("invalid token")
	// ErrWeakPassword means the password does not satisfy the policy.
	ErrWeakPassword = errors.New("weak password")
	// ErrRateLimited means too many attempts were made.
	ErrRateLimited = errors.New("rate limited")
)
