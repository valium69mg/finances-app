package domain

import (
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	// MinPasswordLength is the minimum password length in characters.
	MinPasswordLength = 12
	// maxPasswordBytes is bcrypt's input limit; longer input would be silently truncated.
	maxPasswordBytes = 72
)

// ValidatePassword enforces the password policy.
func ValidatePassword(password string) error {
	if utf8.RuneCountInString(password) < MinPasswordLength || len(password) > maxPasswordBytes {
		return ErrWeakPassword
	}
	return nil
}

// HashPassword returns the bcrypt hash of password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches the bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
