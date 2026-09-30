package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata" // embed the zone database: the distroless image ships none

	"github.com/jackc/pgx/v5/pgxpool"

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
	"github.com/valium69mg/finances-app/backend/internal/platform/clientip"
	"github.com/valium69mg/finances-app/backend/internal/platform/config"
	"github.com/valium69mg/finances-app/backend/internal/platform/cors"
	"github.com/valium69mg/finances-app/backend/internal/platform/health"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
	"github.com/valium69mg/finances-app/backend/internal/platform/postgres"
	remindersdisk "github.com/valium69mg/finances-app/backend/internal/reminders/adapters/disk"
	reminderspg "github.com/valium69mg/finances-app/backend/internal/reminders/adapters/postgres"
	remindersapp "github.com/valium69mg/finances-app/backend/internal/reminders/app"
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
	maxHeaderBytes        = 32 << 10
)

// setupLogger installs the default logger: JSON lines for the production stack
// (easy to ship and filter), text for local development.
func setupLogger(json bool) {
	var h slog.Handler = slog.NewTextHandler(os.Stderr, nil)
	if json {
		h = slog.NewJSONHandler(os.Stderr, nil)
	}
	slog.SetDefault(slog.New(h))
}

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

// reminderRunner wires the email reminders. The disk alert is left out, with a
// warning, when the probe directory cannot be measured (for example in local
// development, where /probe does not exist).
func reminderRunner(cfg config.Config, pool *pgxpool.Pool, lister remindersapp.BillLister, mailer remindersapp.Mailer, now func() time.Time) *remindersapp.Runner {
	repo := reminderspg.NewRepo(pool)
	deps := remindersapp.Deps{
		Bills: lister, Mailer: mailer, Log: repo, Owner: repo,
		AppBaseURL: cfg.AppBaseURL, DiskAlertPct: cfg.DiskAlertPct, Now: now, Logger: slog.Default(),
	}
	probe := remindersdisk.NewProbe(cfg.DiskProbePath)
	if used, err := probe.UsedPercent(); err != nil {
		slog.Warn("the disk alert is off: the probe path cannot be measured", "path", cfg.DiskProbePath, "error", err)
	} else {
		deps.Disk = probe
		slog.Info("disk alert armed", "path", cfg.DiskProbePath, "used_percent", int(used), "threshold_percent", cfg.DiskAlertPct)
	}
	slog.Info("email reminders enabled", "timezone", cfg.TZName, "interval", remindersapp.DefaultInterval.String())
	return remindersapp.NewRunner(deps)
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	setupLogger(cfg.LogJSON)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// "Today" is the calendar day in the configured zone, not in the container's
	// UTC: it drives the bill flags, the default dates and the reminders.
	loc, err := time.LoadLocation(cfg.TZName)
	if err != nil {
		return fmt.Errorf("load TZ_NAME: %w", err)
	}
	now := func() time.Time { return time.Now().In(loc) }

	resendMailer := resend.New(cfg.ResendAPIKey, cfg.ResendFrom, "", nil)
	authSvc := authapp.NewService(authapp.Deps{
		Users:              authpg.NewUserRepo(pool),
		RefreshTokens:      authpg.NewRefreshTokenRepo(pool),
		VerificationTokens: authpg.NewVerificationTokenRepo(pool),
		Mailer:             resendMailer,
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
	expensesSvc := expensesapp.NewService(movements, settingsSvc, now)
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

	billsSvc := billsapp.NewService(billspg.NewRepo(pool), expensesSvc, settingsSvc, now, slog.Default())
	billshttp.New(billsSvc, slog.Default()).Register(mux, auth.RequireAuth)

	// Stopped before the pool is closed (deferred after it, so it runs first).
	remindersCtx, stopReminders := context.WithCancel(ctx)
	var reminders sync.WaitGroup
	defer func() {
		stopReminders()
		reminders.Wait()
	}()
	if cfg.RemindersEnabled {
		runner := reminderRunner(cfg, pool, billsSvc, resendMailer, now)
		reminders.Add(1)
		go func() {
			defer reminders.Done()
			runner.Run(remindersCtx)
		}()
	} else {
		slog.Info("email reminders are disabled (REMINDERS_ENABLED=false)")
	}

	dashboardSvc := dashboardapp.NewService(movements, settingsSvc, incomeSvc, taxfilingSvc, now)
	dashboardhttp.New(dashboardSvc, slog.Default()).Register(mux, auth.RequireAuth)

	monthcloseSvc := monthcloseapp.NewService(monthclosepg.NewRepo(pool), movements, settingsSvc, taxfilingSvc, nil, slog.Default())
	monthclosehttp.New(monthcloseSvc, slog.Default()).Register(mux, auth.RequireAuth)

	corsOrigin, err := cors.OriginFromURL(cfg.AppBaseURL)
	if err != nil {
		return fmt.Errorf("derive CORS origin: %w", err)
	}

	// Outermost first: resolve the client IP, then log and observe every request
	// (including the panics Recover turns into 500s), then CORS and the routes.
	logger := slog.Default()
	resolver := clientip.NewResolver(cfg.TrustedProxies)
	handler := httpmw.Chain(mux,
		resolver.Middleware,
		httpmw.Observe(logger),
		httpmw.Recover(logger),
		cors.Middleware(corsOrigin),
	)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		MaxHeaderBytes:    maxHeaderBytes,
		ReadHeaderTimeout: 5 * time.Second,
		// Generous enough for a 12 MiB invoice upload on a slow uplink and a 10 MiB
		// download; nginx applies its own, tighter per-route limits in front.
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", cfg.HTTPAddr, "trusted_proxies", len(cfg.TrustedProxies), "production", cfg.Production)
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
