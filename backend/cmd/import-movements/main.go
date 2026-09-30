// Command import-movements loads ~/finances/movimientos.csv into the movements
// table. It is a one-time import: it refuses to run when the table already has
// rows unless --force is given (which appends and never deletes). It works for
// Ingreso, Gasto and Ahorro rows alike.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	ledgerpg "github.com/valium69mg/finances-app/backend/internal/ledger/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/platform/postgres"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	path := flag.String("file", filepath.Join(home, "finances", "movimientos.csv"), "path to the legacy movimientos.csv")
	force := flag.Bool("force", false, "import even when the movements table already has rows (appends)")
	flag.Parse()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	f, err := os.Open(*path)
	if err != nil {
		return fmt.Errorf("read %s: %w", *path, err)
	}
	rows, err := parseMovements(f, time.Local)
	_ = f.Close()
	if err != nil {
		return fmt.Errorf("parse %s: %w", *path, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	n, err := ledgerpg.NewRepo(pool).ImportMovements(ctx, rows, *force)
	if errors.Is(err, ledgerpg.ErrNotEmpty) {
		return errors.New("movements already exist; nothing imported (use FORCE=1 make import-movements to append anyway)")
	}
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	fmt.Printf("imported %d movements\n", n)
	return nil
}
