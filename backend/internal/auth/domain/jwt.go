package domain

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/valium69mg/finances-app/backend/internal/platform/session"
)

// accessClaims are the registered claims plus the role of the account.
type accessClaims struct {
	jwt.RegisteredClaims
	Role session.Role `json:"role"`
}

// IssueAccessToken signs an HS256 JWT for userID with the given role, valid for
// AccessTokenTTL from now. An unknown role is refused so a token never carries
// a privilege the backend cannot interpret.
func IssueAccessToken(secret []byte, userID string, role session.Role, now time.Time) (string, error) {
	if !role.Valid() {
		return "", errors.New("unknown role")
	}
	claims := accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
		},
		Role: role,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
}

// ParseAccessToken validates signature (HS256 only), expiry and the role claim,
// returning the identity. A token without a known role is invalid (for example
// one issued before roles existed): the client refreshes and gets a fresh one.
func ParseAccessToken(secret []byte, token string, now time.Time) (session.Identity, error) {
	var claims accessClaims
	parsed, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid || claims.Subject == "" || !claims.Role.Valid() {
		return session.Identity{}, errors.Join(ErrInvalidToken, err)
	}
	return session.Identity{UserID: claims.Subject, Role: claims.Role}, nil
}
