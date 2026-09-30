package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	authhttp "github.com/valium69mg/finances-app/backend/internal/auth/adapters/http"
	authpg "github.com/valium69mg/finances-app/backend/internal/auth/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/ratelimit"
	"github.com/valium69mg/finances-app/backend/internal/auth/adapters/resend"
	authapp "github.com/valium69mg/finances-app/backend/internal/auth/app"
	dashboardhttp "github.com/valium69mg/finances-app/backend/internal/dashboard/adapters/http"
	dashboardapp "github.com/valium69mg/finances-app/backend/internal/dashboard/app"
	expenseshttp "github.com/valium69mg/finances-app/backend/internal/expenses/adapters/http"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	incomehttp "github.com/valium69mg/finances-app/backend/internal/income/adapters/http"
	incomeapp "github.com/valium69mg/finances-app/backend/internal/income/app"
	ledgerpg "github.com/valium69mg/finances-app/backend/internal/ledger/adapters/postgres"
	"github.com/valium69mg/finances-app/backend/internal/platform/config"
	"github.com/valium69mg/finances-app/backend/internal/platform/cors"
	"github.com/valium69mg/finances-app/backend/internal/platform/health"
	"github.com/valium69mg/finances-app/backend/internal/platform/postgres"
	savingshttp "github.com/valium69mg/finances-app/backend/internal/savings/adapters/http"
	savingspg "github.com/valium69mg/finances-app/backend/internal/savings/adapters/postgres"
	savingsapp "github.com/valium69mg/finances-app/backend/internal/savings/app"
	settingshttp "github.com/valium69mg/finances-app/backend/internal/settings/adapters/http"
	settingspg "github.com/valium69mg/finances-app/backend/internal/settings/adapters/postgres"
	settingsapp "github.com/valium69mg/finances-app/backend/internal/settings/app"
)

const shutdownTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	authSvc := authapp.NewService(authapp.Deps{
		Users:              authpg.NewUserRepo(pool),
		RefreshTokens:      authpg.NewRefreshTokenRepo(pool),
		VerificationTokens: authpg.NewVerificationTokenRepo(pool),
		Mailer:             resend.New(cfg.ResendAPIKey, cfg.ResendFrom, "", nil),
		Limiter:            ratelimit.New(nil),
		JWTSecret:          []byte(cfg.JWTSecret),
		AppBaseURL:         cfg.AppBaseURL,
	})

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", health.Handler(pool))
	auth := authhttp.New(authSvc, slog.Default())
	auth.Register(mux)

	settingsSvc := settingsapp.NewService(settingspg.NewRepo(pool))
	settingshttp.New(settingsSvc, slog.Default()).Register(mux, auth.RequireAuth)

	movements := ledgerpg.NewRepo(pool)
	expensesSvc := expensesapp.NewService(movements, settingsSvc, nil)
	expenseshttp.New(expensesSvc, slog.Default()).Register(mux, auth.RequireAuth)

	incomeSvc := incomeapp.NewService(movements, settingsSvc, nil)
	incomehttp.New(incomeSvc, slog.Default()).Register(mux, auth.RequireAuth)

	savingsSvc := savingsapp.NewService(movements, savingspg.NewRepo(pool), settingsSvc, nil)
	savingshttp.New(savingsSvc, slog.Default()).Register(mux, auth.RequireAuth)

	dashboardSvc := dashboardapp.NewService(movements, settingsSvc, incomeSvc, nil)
	dashboardhttp.New(dashboardSvc, slog.Default()).Register(mux, auth.RequireAuth)

	corsOrigin, err := cors.OriginFromURL(cfg.AppBaseURL)
	if err != nil {
		return fmt.Errorf("derive CORS origin: %w", err)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           cors.Middleware(corsOrigin)(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", cfg.HTTPAddr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
