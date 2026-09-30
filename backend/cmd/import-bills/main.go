// Command import-bills creates the recurring Servicios bills from the note of the
// Servicios category in ~/finances/config.json (megacable, agua, luz, telcel,
// gas LP). It is a one-time import: it refuses to run when the bills table
// already has rows unless --force is given (which appends and never deletes).
// The real due days are unknown, so every bill starts due on the 1st of next
// month and the user adjusts the dates in the app.
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

	billspg "github.com/valium69mg/finances-app/backend/internal/bills/adapters/postgres"
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
	path := flag.String("file", filepath.Join(home, "finances", "config.json"), "path to the legacy config.json")
	force := flag.Bool("force", false, "import even when the bills table already has rows (appends)")
	flag.Parse()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	data, err := os.ReadFile(*path)
	if err != nil {
		return fmt.Errorf("read %s: %w", *path, err)
	}
	list, err := parseServices(data, time.Now())
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	n, err := billspg.NewRepo(pool).Import(ctx, list, *force)
	if errors.Is(err, billspg.ErrNotEmpty) {
		return errors.New("bills already exist; nothing imported (use FORCE=1 make import-bills to append anyway)")
	}
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	fmt.Printf("imported %d monthly bills (category Servicios, reminder %d days before):\n", n, reminderLeadDays)
	for _, b := range list {
		fmt.Printf("  %-10s %s MXN, due %s\n", b.Name, b.Amount.StringFixed(2), b.NextDueDate)
	}
	fmt.Println("NOTE: the due dates are placeholders (the 1st of next month). Open Bills & Subscriptions in the app and set the real due day of each bill.")
	return nil
}
