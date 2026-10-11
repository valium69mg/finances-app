package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const (
	// AccessTokenTTL is the lifetime of the JWT access token.
	AccessTokenTTL = 15 * time.Minute
	// RefreshTokenTTL is the lifetime of an opaque refresh token.
	RefreshTokenTTL = 7 * 24 * time.Hour
	// VerificationTokenTTL is the lifetime of an email verification token.
	VerificationTokenTTL = time.Hour

	tokenBytes = 32
)

// RefreshToken is the stored (hashed) form of an opaque refresh token.
type RefreshToken struct {
	Hash      string
	UserID    string
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

// Expired reports whether the token is past its lifetime at now.
func (t RefreshToken) Expired(now time.Time) bool { return !now.Before(t.ExpiresAt) }

// TokenPurpose says what a single-use email token may be redeemed for. Verification
// and reset tokens share the table, the format, the lifetime and the hashing, but a
// token only ever works for the purpose it was issued for.
type TokenPurpose string

const (
	// PurposeVerification verifies the address and sets the first password (also used by invitations).
	PurposeVerification TokenPurpose = "verification"
	// PurposeReset sets a new password on an already verified account.
	PurposeReset TokenPurpose = "reset"
)

// VerificationToken is the stored (hashed) form of an email verification or
// password reset token.
type VerificationToken struct {
	Hash      string
	UserID    string
	Purpose   TokenPurpose
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// GenerateToken returns a random 32-byte token (base64url) and its storage hash.
func GenerateToken() (raw, hash string, err error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashToken(raw), nil
}

// HashToken returns the hex SHA-256 of a raw token; only this value is stored.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
