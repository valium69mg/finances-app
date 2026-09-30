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
	billshttp "github.com/valium69mg/finances-app/backend/internal/bills/adapters/http"
	billspg "github.com/valium69mg/finances-app/backend/internal/bills/adapters/postgres"
	billsapp "github.com/valium69mg/finances-app/backend/internal/bills/app"
	dashboardhttp "github.com/valium69mg/finances-app/backend/internal/dashboard/adapters/http"
	dashboardapp "github.com/valium69mg/finances-app/backend/internal/dashboard/app"
	expenseshttp "github.com/valium69mg/finances-app/backend/internal/expenses/adapters/http"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	incomehttp "github.com/valium69mg/finances-app/backend/internal/income/adapters/http"
	incomeapp "github.com/valium69mg/finances-app/backend/internal/income/app"
	invoiceshttp "github.com/valium69mg/finances-app/backend/internal/invoices/adapters/http"
	invoicespg "github.com/valium69mg/finances-app/backend/internal/invoices/adapters/postgres"
	invoicess3 "github.com/valium69mg/finances-app/backend/internal/invoices/adapters/s3"
	invoicesapp "github.com/valium69mg/finances-app/backend/internal/invoices/app"
	ledgerpg "github.com/valium69mg/finances-app/backend/internal/ledger/adapters/postgres"
	monthclosehttp "github.com/valium69mg/finances-app/backend/internal/monthclose/adapters/http"
	monthclosepg "github.com/valium69mg/finances-app/backend/internal/monthclose/adapters/postgres"
	monthcloseapp "github.com/valium69mg/finances-app/backend/internal/monthclose/app"
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
	taxfilinghttp "github.com/valium69mg/finances-app/backend/internal/taxfiling/adapters/http"
	taxfilingpg "github.com/valium69mg/finances-app/backend/internal/taxfiling/adapters/postgres"
	taxfilingapp "github.com/valium69mg/finances-app/backend/internal/taxfiling/app"
)

const (
	shutdownTimeout       = 10 * time.Second
	storageStartupTimeout = 10 * time.Second
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// invoiceStore builds the object store of the invoice files. The storage is
// optional at boot: a missing configuration or an unreachable MinIO never stops
// the API. Missing credentials disable the storage; otherwise the bucket is
// ensured lazily (with backoff) on first use, so the file routes answer 503
// storage_unavailable while it is down and work again as soon as it is back,
// without a restart.
func invoiceStore(ctx context.Context, cfg config.S3) invoicesapp.ObjectStore {
	if !cfg.Configured() {
		slog.Warn("object storage is not configured (set S3_ACCESS_KEY and S3_SECRET_KEY, or MINIO_ROOT_USER and MINIO_ROOT_PASSWORD): " +
			"invoice file routes answer 503 storage_unavailable, every other route works")
		return invoicess3.NewDisabled("S3_ACCESS_KEY and S3_SECRET_KEY (or MINIO_ROOT_USER and MINIO_ROOT_PASSWORD) are not set")
	}
	store, err := invoicess3.New(invoicess3.Config{
		Endpoint: cfg.Endpoint, AccessKey: cfg.AccessKey, SecretKey: cfg.SecretKey,
		Bucket: cfg.Bucket, Region: cfg.Region, UseSSL: cfg.UseSSL,
	})
	if err != nil {
		slog.Warn("object storage client could not be built: invoice file routes answer 503 storage_unavailable", "error", err)
		return invoicess3.NewDisabled("the storage client could not be built")
	}
	lazy := invoicess3.NewLazy(store, store.EnsureBucket, slog.Default())
	// An early, non-fatal check so a misconfigured or stopped MinIO is visible in
	// the log at startup; the failed attempt is logged by the store itself.
	checkCtx, cancel := context.WithTimeout(ctx, storageStartupTimeout)
	defer cancel()
	if err := lazy.Prepare(checkCtx); err != nil {
		slog.Warn("object storage is not reachable yet: the API starts anyway and retries on demand (is MinIO up? run make db-up)")
	}
	return lazy
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

	store := invoiceStore(ctx, cfg.S3)
	invoicesSvc := invoicesapp.NewService(invoicespg.NewRepo(pool), store, movements, settingsSvc, nil, slog.Default())
	invoiceshttp.New(invoicesSvc, slog.Default()).Register(mux, auth.RequireAuth)

	taxfilingSvc := taxfilingapp.NewService(taxfilingpg.NewRepo(pool), invoicesSvc, expensesSvc, settingsSvc, nil, slog.Default())
	taxfilinghttp.New(taxfilingSvc, slog.Default()).Register(mux, auth.RequireAuth)
	// Issuing an invoice in an already filed period warns (period_already_filed).
	invoicesSvc.WithFilings(taxfilingSvc)

	billsSvc := billsapp.NewService(billspg.NewRepo(pool), expensesSvc, settingsSvc, nil, slog.Default())
	billshttp.New(billsSvc, slog.Default()).Register(mux, auth.RequireAuth)

	dashboardSvc := dashboardapp.NewService(movements, settingsSvc, incomeSvc, taxfilingSvc, nil)
	dashboardhttp.New(dashboardSvc, slog.Default()).Register(mux, auth.RequireAuth)

	monthcloseSvc := monthcloseapp.NewService(monthclosepg.NewRepo(pool), movements, settingsSvc, taxfilingSvc, nil, slog.Default())
	monthclosehttp.New(monthcloseSvc, slog.Default()).Register(mux, auth.RequireAuth)

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
