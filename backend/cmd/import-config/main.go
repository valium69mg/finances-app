// Command import-config loads ~/finances/config.json into the settings tables.
// It is a one-time import: it refuses to run when settings already exist
// unless --force is given. Only the configuration is imported, no movements.
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

	"github.com/valium69mg/finances-app/backend/internal/platform/postgres"
	"github.com/valium69mg/finances-app/backend/internal/settings/adapters/configfile"
	settingspg "github.com/valium69mg/finances-app/backend/internal/settings/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/settings/app"
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
	force := flag.Bool("force", false, "replace existing settings")
	flag.Parse()

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	data, err := os.ReadFile(*path)
	if err != nil {
		return fmt.Errorf("read %s: %w", *path, err)
	}
	cfg, err := configfile.Parse(data)
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

	err = app.NewService(settingspg.NewRepo(pool)).Import(ctx, cfg, *force)
	if errors.Is(err, app.ErrAlreadyImported) {
		return errors.New("settings already exist; nothing imported (use FORCE=1 make import-config to replace them)")
	}
	if err != nil {
		return fmt.Errorf("import: %w", err)
	}
	fmt.Printf("imported %d categories, %d clients, %d instruments, %d brackets, %d payment methods\n",
		len(cfg.Categories), len(cfg.Clients), len(cfg.Instruments), len(cfg.Brackets), len(cfg.PaymentMethods))
	return nil
}
