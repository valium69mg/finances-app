package app_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/expenserequests/app"
	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settingsapp "github.com/valium69mg/finances-app/backend/internal/settings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

// These tests wire the REAL expenses and settings services behind the request
// service, so the budget check is proved to resolve budgets exactly as the
// rest of the app does (computed Impuestos and Comisiones budgets, month
// overrides and the pause plan), not through a copy of that logic.

type settingsRepo struct{ cfg settings.Config }

func (r *settingsRepo) Load(context.Context) (settings.Config, error) { return r.cfg, nil }
func (r *settingsRepo) IsEmpty(context.Context) (bool, error)         { return false, nil }
func (r *settingsRepo) SaveGeneral(context.Context, settings.General) error {
	return nil
}
func (r *settingsRepo) SaveCategories(context.Context, []settings.Category) error { return nil }
func (r *settingsRepo) SaveClients(context.Context, []settings.Client) error      { return nil }
func (r *settingsRepo) SaveInstruments(context.Context, []settings.Instrument, map[string]string) error {
	return nil
}
func (r *settingsRepo) SaveBrackets(context.Context, []settings.Bracket) error { return nil }
func (r *settingsRepo) SavePaymentMethods(context.Context, []string) error     { return nil }
func (r *settingsRepo) SaveIssuer(context.Context, settings.Issuer) error      { return nil }
func (r *settingsRepo) SavePause(context.Context, *settings.PausePlan) error   { return nil }
func (r *settingsRepo) Import(context.Context, settings.Config) error          { return nil }

type ledgerRepo struct{ rows []ledger.Movement }

func (l *ledgerRepo) Create(_ context.Context, m ledger.Movement) (ledger.Movement, error) {
	m.ID = len(l.rows) + 1
	l.rows = append(l.rows, m)
	return m, nil
}
func (l *ledgerRepo) Update(context.Context, ledger.Movement) error { return nil }
func (l *ledgerRepo) Delete(context.Context, int) error             { return nil }
func (l *ledgerRepo) GetByID(context.Context, int) (ledger.Movement, error) {
	return ledger.Movement{}, ledger.ErrNotFound
}
func (l *ledgerRepo) ListAllByKind(context.Context, ledger.Kind) ([]ledger.Movement, error) {
	return l.rows, nil
}
func (l *ledgerRepo) ListByRange(_ context.Context, from, to string, kind ledger.Kind, _ int) ([]ledger.Movement, error) {
	var out []ledger.Movement
	for _, m := range l.rows {
		if m.Date >= from && m.Date <= to && (kind == "" || m.Kind == kind) {
			out = append(out, m)
		}
	}
	return out, nil
}

func realEnv(t *testing.T, cycleStartDay int) (*app.Service, *ledgerRepo, *settingsapp.Service) {
	t.Helper()
	cfg := settingstest.RealConfig()
	cfg.PaymentMethods = []string{"Efectivo", "Débito", "Crédito", "Transferencia"}
	cfg.CycleStartDay = cycleStartDay
	pause := settingstest.RealPause()
	cfg.Pause = &pause
	settingsSvc := settingsapp.NewService(&settingsRepo{cfg: cfg})
	ledgerR := &ledgerRepo{}
	now := func() time.Time { return time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC) }
	expensesSvc := expensesapp.NewService(ledgerR, settingsSvc, now)
	svc := app.NewService(app.Deps{
		Repo: newFakeRepo(), Expenses: expensesSvc, Settings: settingsSvc, Mailer: &fakeMailer{},
		Limiter: &countingLimiter{calls: map[string]int{}}, Now: now, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return svc, ledgerR, settingsSvc
}

func addExpense(l *ledgerRepo, date, category, amount string) {
	l.rows = append(l.rows, ledger.Movement{ID: len(l.rows) + 1, Date: date, Category: category, Kind: ledger.KindExpense, Amount: d(amount), AmountMXN: d(amount)})
}

func newPending(t *testing.T, svc *app.Service, category, amount, date string) domain.Request {
	t.Helper()
	r, err := svc.Create(context.Background(), spouse, domain.Input{Amount: d(amount), Description: "x", SuggestedCategory: category, Date: date})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return r
}

func TestBudgetCheckResolvesBudgetsLikeTheDashboard(t *testing.T) {
	svc, ledgerR, settingsSvc := realEnv(t, 0)
	ctx := context.Background()
	addExpense(ledgerR, "2026-10-05", "Transporte", "700") // budget 1000
	r := newPending(t, svc, "Transporte", "300", "2026-10-20")

	// Exactly fits: 700 + 300 = 1000.
	got, err := svc.BudgetCheck(ctx, owner, r.ID, "Transporte", "")
	if err != nil || got.Fits == nil || !*got.Fits || !got.OverBy.IsZero() || !got.Budget.Equal(d("1000")) || !got.Spent.Equal(d("700")) || !got.ProjectedRemaining.IsZero() {
		t.Errorf("exactly fits = %+v, %v", got, err)
	}
	// One cent more exceeds.
	r2 := newPending(t, svc, "Transporte", "300.01", "2026-10-20")
	got, err = svc.BudgetCheck(ctx, owner, r2.ID, "Transporte", "")
	if err != nil || got.Fits == nil || *got.Fits || !got.OverBy.Equal(d("0.01")) {
		t.Errorf("exceeds by a cent = %+v, %v", got, err)
	}
	// A category without budget.
	r3 := newPending(t, svc, "Ocio", "50", "")
	got, err = svc.BudgetCheck(ctx, owner, r3.ID, "Ocio", "")
	if err != nil || got.Budget != nil || got.Fits != nil {
		t.Errorf("no budget = %+v, %v", got, err)
	}

	// A computed budget (Impuestos has none configured) comes from the same
	// resolver the dashboard uses, month by month.
	budgets, err := settingsSvc.MonthBudgets(ctx, "2026-10")
	if err != nil {
		t.Fatal(err)
	}
	var taxes *settingsapp.CategoryBudget
	for i := range budgets {
		if budgets[i].Name == "Impuestos" {
			taxes = &budgets[i]
		}
	}
	if taxes == nil || taxes.Budget == nil {
		t.Fatal("the fixture must give Impuestos a computed budget")
	}
	r4 := newPending(t, svc, "Impuestos", "10", "")
	got, err = svc.BudgetCheck(ctx, owner, r4.ID, "Impuestos", "2026-10-20")
	if err != nil || got.Budget == nil || !got.Budget.Equal(*taxes.Budget) {
		t.Errorf("Impuestos budget = %v (%v), want the computed %v", got.Budget, err, taxes.Budget)
	}
}

func TestBudgetCheckCycleBoundaries(t *testing.T) {
	svc, ledgerR, _ := realEnv(t, 15) // a cycle starts on the 15th: 2026-10-15 belongs to 2026-11
	ctx := context.Background()
	addExpense(ledgerR, "2026-10-14", "Transporte", "400") // cycle 2026-10 (09-15..10-14)
	addExpense(ledgerR, "2026-10-15", "Transporte", "90")  // cycle 2026-11 (10-15..11-14)
	addExpense(ledgerR, "2026-11-14", "Transporte", "10")  // cycle 2026-11
	addExpense(ledgerR, "2026-11-15", "Transporte", "5")   // cycle 2026-12

	r := newPending(t, svc, "Transporte", "100", "2026-10-14")
	for date, spent := range map[string]string{"2026-10-14": "400", "2026-10-15": "100", "2026-11-14": "100", "2026-11-15": "5"} {
		got, err := svc.BudgetCheck(ctx, owner, r.ID, "Transporte", date)
		if err != nil || !got.Spent.Equal(d(spent)) || !got.ProjectedSpent.Equal(d(spent).Add(d("100"))) {
			t.Errorf("date %s: spent %s projected %s, want spent %s (%v)", date, got.Spent, got.ProjectedSpent, spent, err)
		}
	}
	// The date of the request is the default one.
	got, err := svc.BudgetCheck(ctx, owner, r.ID, "Transporte", "")
	if err != nil || !got.Spent.Equal(d("400")) {
		t.Errorf("default date: spent %s (%v), want the 2026-10 cycle", got.Spent, err)
	}
}

func TestApproveAsExpenseRegistersTheRealGastoAndEchoesTheFeedback(t *testing.T) {
	svc, ledgerR, _ := realEnv(t, 0)
	ctx := context.Background()
	addExpense(ledgerR, "2026-10-05", "Transporte", "900") // budget 1000
	r := newPending(t, svc, "Transporte", "300", "2026-10-20")

	res, err := svc.Approve(ctx, owner, r.ID, app.ApproveInput{Destination: domain.DestinationExpense, Category: "Transporte"})
	if err != nil {
		t.Fatalf("a request that does not fit must still be approved: %v", err)
	}
	if res.Request.Status != domain.StatusApproved {
		t.Errorf("status = %s", res.Request.Status)
	}
	// The fake repository does not store the Gasto (the real one is tested in the
	// postgres package), so the feedback only counts what the ledger holds.
	if res.Feedback == nil || res.Feedback.Category != "Transporte" || res.Feedback.Budget == nil || !res.Feedback.Budget.Equal(d("1000")) {
		t.Errorf("feedback = %+v", res.Feedback)
	}
}
