// Command seed inserts the admin user. It is idempotent: an existing admin is
// left untouched. The admin gets a random password that is hashed and never
// printed or stored; the user sets a real one through the email verification flow.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	authpg "github.com/valium69mg/finances-app/backend/internal/auth/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/auth/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/postgres"
)

const adminEmail = "carlostranquilino.cr@gmail.com"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("generate password: %w", err)
	}
	hash, err := domain.HashPassword(base64.RawURLEncoding.EncodeToString(buf))
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	created, err := authpg.NewUserRepo(pool).CreateIfMissing(ctx, adminEmail, hash)
	if err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}
	if created {
		fmt.Println("admin user created (unverified); log in with its email to receive the verification email")
	} else {
		fmt.Println("admin user already exists; nothing to do")
	}
	return nil
}
