package domain

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// IssueAccessToken signs an HS256 JWT for userID valid for AccessTokenTTL from now.
func IssueAccessToken(secret []byte, userID string, now time.Time) (string, error) {
	claims := jwt.RegisteredClaims{
		Subject:   userID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// ParseAccessToken validates signature (HS256 only) and expiry, returning the user id.
func ParseAccessToken(secret []byte, token string, now time.Time) (string, error) {
	var claims jwt.RegisteredClaims
	parsed, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return "", errors.Join(ErrInvalidToken, err)
	}
	return claims.Subject, nil
}
