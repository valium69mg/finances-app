// Package session carries the authenticated identity (user id and role) through
// the request context, so every module can read who is acting without
// depending on the auth module.
package session

import "context"

// Role is what an account may do. The role travels in the access token and is
// enforced by the HTTP layer with default deny for everything but the owner.
type Role string

const (
	// RoleOwner is the account holder: every route is allowed.
	RoleOwner Role = "owner"
	// RoleHousehold is a limited account: only an explicit route allowlist.
	RoleHousehold Role = "household"
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool { return r == RoleOwner || r == RoleHousehold }

// Identity is the authenticated caller.
type Identity struct {
	UserID string
	Role   Role
}

type ctxKey struct{}

// With returns ctx carrying the identity.
func With(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// From returns the identity stored by With.
func From(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// UserID returns the acting user id, or "" when the context has no identity
// (background jobs, imports).
func UserID(ctx context.Context) string {
	id, _ := From(ctx)
	return id.UserID
}
