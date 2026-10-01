package main

import (
	"log/slog"
	"net/http"

	authhttp "github.com/valium69mg/finances-app/backend/internal/auth/adapters/http"
	billshttp "github.com/valium69mg/finances-app/backend/internal/bills/adapters/http"
	dashboardhttp "github.com/valium69mg/finances-app/backend/internal/dashboard/adapters/http"
	expenseshttp "github.com/valium69mg/finances-app/backend/internal/expenses/adapters/http"
	futureexpenseshttp "github.com/valium69mg/finances-app/backend/internal/futureexpenses/adapters/http"
	incomehttp "github.com/valium69mg/finances-app/backend/internal/income/adapters/http"
	invoiceshttp "github.com/valium69mg/finances-app/backend/internal/invoices/adapters/http"
	monthclosehttp "github.com/valium69mg/finances-app/backend/internal/monthclose/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
	savingshttp "github.com/valium69mg/finances-app/backend/internal/savings/adapters/http"
	settingshttp "github.com/valium69mg/finances-app/backend/internal/settings/adapters/http"
	systemhttp "github.com/valium69mg/finances-app/backend/internal/system/adapters/http"
	taxfilinghttp "github.com/valium69mg/finances-app/backend/internal/taxfiling/adapters/http"
	usershttp "github.com/valium69mg/finances-app/backend/internal/users/adapters/http"
)

// routeDeps are the use cases behind every HTTP route of the API. Keeping the
// whole route table in registerRoutes lets a test mount it on a recording
// router, so a route that forgets the authentication wrapper (and with it the
// role enforcement) cannot go unnoticed.
type routeDeps struct {
	Health        http.Handler
	Auth          *authhttp.Handler
	Settings      settingshttp.Service
	Expenses      expenseshttp.Service
	Income        incomehttp.Service
	Savings       savingshttp.Service
	Invoices      invoiceshttp.Service
	TaxFiling     taxfilinghttp.Service
	Bills         billshttp.Service
	FutureExpense futureexpenseshttp.Service
	Dashboard     dashboardhttp.Service
	MonthClose    monthclosehttp.Service
	System        systemhttp.Service
	Users         usershttp.Service
}

// registerRoutes mounts every route. Every authenticated route goes through
// d.Auth.RequireAuth (the requireAuth passed to the modules), which applies the
// role gate with default deny for the household role.
func registerRoutes(mux httpmw.Router, d routeDeps, requireAuth func(http.Handler) http.Handler, logger *slog.Logger) {
	mux.Handle("GET /healthz", d.Health)
	d.Auth.Register(mux)

	settingshttp.New(d.Settings, logger).Register(mux, requireAuth)
	expenseshttp.New(d.Expenses, logger).Register(mux, requireAuth)
	incomehttp.New(d.Income, logger).Register(mux, requireAuth)
	savingshttp.New(d.Savings, logger).Register(mux, requireAuth)
	invoiceshttp.New(d.Invoices, logger).Register(mux, requireAuth)
	taxfilinghttp.New(d.TaxFiling, logger).Register(mux, requireAuth)
	billshttp.New(d.Bills, logger).Register(mux, requireAuth)
	futureexpenseshttp.New(d.FutureExpense, logger).Register(mux, requireAuth)
	dashboardhttp.New(d.Dashboard, logger).Register(mux, requireAuth)
	monthclosehttp.New(d.MonthClose, logger).Register(mux, requireAuth)
	systemhttp.New(d.System, logger).Register(mux, requireAuth)
	usershttp.New(d.Users, logger).Register(mux, requireAuth)
}
