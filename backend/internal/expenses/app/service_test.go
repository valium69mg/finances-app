package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/expenses/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settingsapp "github.com/valium69mg/finances-app/backend/internal/settings/app"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

type fakeRepo struct {
	rows      map[int]ledger.Movement
	nextID    int
	listLimit int
	listKind  ledger.Kind
	listMonth string
	listErr   error
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[int]ledger.Movement{}, nextID: 1} }

func (f *fakeRepo) Create(_ context.Context, m ledger.Movement) (ledger.Movement, error) {
	m.ID = f.nextID
	f.nextID++
	f.rows[m.ID] = m
	return m, nil
}
func (f *fakeRepo) Update(_ context.Context, m ledger.Movement) error {
	if _, ok := f.rows[m.ID]; !ok {
		return ledger.ErrNotFound
	}
	f.rows[m.ID] = m
	return nil
}
func (f *fakeRepo) Delete(_ context.Context, id int) error {
	if _, ok := f.rows[id]; !ok {
		return ledger.ErrNotFound
	}
	delete(f.rows, id)
	return nil
}
func (f *fakeRepo) GetByID(_ context.Context, id int) (ledger.Movement, error) {
	m, ok := f.rows[id]
	if !ok {
		return ledger.Movement{}, ledger.ErrNotFound
	}
	return m, nil
}
func (f *fakeRepo) ListAllByKind(_ context.Context, kind ledger.Kind) ([]ledger.Movement, error) {
	var out []ledger.Movement
	for _, m := range f.rows {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListByMonth(_ context.Context, month string, kind ledger.Kind, limit int) ([]ledger.Movement, error) {
	f.listMonth, f.listKind, f.listLimit = month, kind, limit
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []ledger.Movement
	for _, m := range f.rows {
		if ledger.MonthOf(m.Date) == month && (kind == "" || m.Kind == kind) {
			out = append(out, m)
		}
	}
	return out, nil
}

type fakeSettings struct {
	cfg        settings.Config
	budgetsErr error
}

func (f *fakeSettings) Get(context.Context) (settings.Config, error) { return f.cfg, nil }
func (f *fakeSettings) MonthBudgets(_ context.Context, _ string) ([]settingsapp.CategoryBudget, error) {
	if f.budgetsErr != nil {
		return nil, f.budgetsErr
	}
	out := make([]settingsapp.CategoryBudget, len(f.cfg.Categories))
	for i, c := range f.cfg.Categories {
		out[i] = settingsapp.CategoryBudget{Name: c.Name, Kind: string(c.Kind), Budget: c.Budget}
	}
	return out, nil
}

func newService(t *testing.T) (*app.Service, *fakeRepo, *fakeSettings) {
	t.Helper()
	cfg := settingstest.RealConfig()
	cfg.PaymentMethods = []string{"Efectivo", "Débito", "Crédito", "Transferencia"}
	repo, st := newFakeRepo(), &fakeSettings{cfg: cfg}
	now := func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	return app.NewService(repo, st, now), repo, st
}

func TestCreateAppliesDefaultsAndFeedback(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()

	res, err := svc.Create(ctx, app.Input{Category: "Mandado", Amount: d("1000")})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	m := res.Movement
	if m.ID != 1 || m.Kind != ledger.KindExpense || m.Date != "2026-10-15" || m.PaymentMethod != "Débito" || m.Currency != "MXN" {
		t.Errorf("unexpected movement %+v", m)
	}
	if _, ok := repo.rows[1]; !ok {
		t.Error("movement was not stored")
	}
	fb := res.Feedback
	if fb == nil || fb.Month != "2026-10" || !fb.Spent.Equal(d("1000")) || fb.Budget == nil || !fb.Budget.Equal(d("10833")) ||
		!fb.Remaining.Equal(d("9833")) || fb.OverBudget {
		t.Errorf("unexpected feedback %+v", fb)
	}
	if repo.listKind != ledger.KindExpense || repo.listMonth != "2026-10" {
		t.Errorf("feedback listed %q %q", repo.listKind, repo.listMonth)
	}
}

func TestCreateOverBudgetAccumulatesSpent(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	// Transporte budget is 1000.
	if _, err := svc.Create(ctx, app.Input{Category: "Transporte", Amount: d("800")}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Create(ctx, app.Input{Category: "Transporte", Amount: d("300"), Date: "2026-10-20"})
	if err != nil {
		t.Fatal(err)
	}
	fb := res.Feedback
	if !fb.Spent.Equal(d("1100")) || !fb.Remaining.Equal(d("-100")) || !fb.OverBudget {
		t.Errorf("unexpected feedback %+v", fb)
	}
	// Another month does not count.
	res, err = svc.Create(ctx, app.Input{Category: "Transporte", Amount: d("50"), Date: "2026-11-02"})
	if err != nil || !res.Feedback.Spent.Equal(d("50")) || res.Feedback.OverBudget {
		t.Errorf("November feedback %+v, %v", res.Feedback, err)
	}
}

func TestCreateNoBudgetCategory(t *testing.T) {
	svc, _, _ := newService(t)
	res, err := svc.Create(context.Background(), app.Input{Category: "Ocio", Amount: d("500")})
	if err != nil {
		t.Fatal(err)
	}
	fb := res.Feedback
	if fb.Budget != nil || fb.Remaining != nil || fb.OverBudget || !fb.Spent.Equal(d("500")) {
		t.Errorf("no-budget feedback %+v", fb)
	}
}

func TestCreateInfersCategory(t *testing.T) {
	svc, _, _ := newService(t)
	res, err := svc.Create(context.Background(), app.Input{Description: "Compra en Walmart", Amount: d("300")})
	if err != nil || res.Movement.Category != "Mandado" {
		t.Fatalf("got %q, %v; want Mandado", res.Movement.Category, err)
	}
	_, err = svc.Create(context.Background(), app.Input{Description: "xyzzy", Amount: d("300")})
	if !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("uninferable description: err = %v, want ErrInvalid", err)
	}
}

func TestCreateUSDUsesConfiguredRate(t *testing.T) {
	svc, _, _ := newService(t)
	res, err := svc.Create(context.Background(), app.Input{Category: "Suscripciones", Currency: "USD", Amount: d("20")})
	if err != nil {
		t.Fatal(err)
	}
	// 20 USD * 17.74 = 354.80
	if !res.Movement.AmountMXN.Equal(d("354.80")) || !res.Movement.ExchangeRate.Equal(d("17.74")) {
		t.Errorf("unexpected conversion %+v", res.Movement)
	}
	if !res.Feedback.Spent.Equal(d("354.80")) {
		t.Errorf("feedback spent = %s, want the MXN amount", res.Feedback.Spent)
	}
}

func TestCreateRejectsInvalid(t *testing.T) {
	svc, repo, _ := newService(t)
	bad := []app.Input{
		{Category: "Sueldo", Amount: d("10")},                     // income category
		{Category: "Mandado", Amount: d("0")},                     // zero
		{Category: "Mandado", Amount: d("-5")},                    // negative
		{Category: "Mandado", Amount: d("5"), PaymentMethod: "X"}, // payment method
		{Category: "Mandado", Amount: d("5"), Currency: "EUR"},    // currency
		{Category: "Mandado", Amount: d("5"), Date: "2026-13-01"}, // date
	}
	for i, in := range bad {
		if _, err := svc.Create(context.Background(), in); !errors.Is(err, ledger.ErrInvalid) {
			t.Errorf("case %d: err = %v, want ErrInvalid", i, err)
		}
	}
	if len(repo.rows) != 0 {
		t.Error("invalid input must not be stored")
	}
}

func TestMissingTaxConfigDropsFeedbackOnly(t *testing.T) {
	svc, repo, st := newService(t)
	st.budgetsErr = settings.ErrMissingConfig
	res, err := svc.Create(context.Background(), app.Input{Category: "Mandado", Amount: d("10")})
	if err != nil || res.Feedback != nil || len(repo.rows) != 1 {
		t.Errorf("res=%+v err=%v rows=%d", res, err, len(repo.rows))
	}
}

// The expense is already persisted when the feedback is computed: a failing
// lookup must never turn into an error (callers would leave the expense
// unlinked and a retry would duplicate it), only into an omitted feedback.
func TestFeedbackFailureNeverFailsASavedExpense(t *testing.T) {
	ctx := context.Background()

	t.Run("budgets lookup fails on create", func(t *testing.T) {
		svc, repo, st := newService(t)
		st.budgetsErr = errors.New("boom")
		res, err := svc.Create(ctx, app.Input{Category: "Mandado", Amount: d("10")})
		if err != nil || res.Feedback != nil || res.Movement.ID != 1 || len(repo.rows) != 1 {
			t.Errorf("res=%+v err=%v rows=%d", res, err, len(repo.rows))
		}
	})
	t.Run("month listing fails on create", func(t *testing.T) {
		svc, repo, _ := newService(t)
		repo.listErr = errors.New("db down")
		res, err := svc.Create(ctx, app.Input{Category: "Mandado", Amount: d("10")})
		if err != nil || res.Feedback != nil || len(repo.rows) != 1 {
			t.Errorf("res=%+v err=%v rows=%d", res, err, len(repo.rows))
		}
	})
	t.Run("feedback fails on update", func(t *testing.T) {
		svc, repo, st := newService(t)
		created, err := svc.Create(ctx, app.Input{Category: "Mandado", Amount: d("10")})
		if err != nil {
			t.Fatal(err)
		}
		st.budgetsErr = errors.New("boom")
		res, err := svc.Update(ctx, created.Movement.ID, app.Input{Category: "Mandado", Amount: d("20")})
		if err != nil || res.Feedback != nil || !repo.rows[created.Movement.ID].Amount.Equal(d("20")) {
			t.Errorf("res=%+v err=%v stored=%+v", res, err, repo.rows[created.Movement.ID])
		}
	})
}

func TestUpdate(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, app.Input{Category: "Mandado", Amount: d("100")})
	id := created.Movement.ID

	res, err := svc.Update(ctx, id, app.Input{Category: "Ocio", Amount: d("250"), PaymentMethod: "Efectivo", Date: "2026-09-30"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := repo.rows[id]
	if got.Category != "Ocio" || !got.Amount.Equal(d("250")) || got.PaymentMethod != "Efectivo" || got.Date != "2026-09-30" {
		t.Errorf("stored %+v", got)
	}
	if res.Feedback.Month != "2026-09" {
		t.Errorf("feedback month %q, want the new month", res.Feedback.Month)
	}

	for name, in := range map[string]app.Input{
		"payment method": {Category: "Ocio", Amount: d("1"), PaymentMethod: "Cheque"},
		"currency":       {Category: "Ocio", Amount: d("1"), Currency: "EUR"},
		"amount":         {Category: "Ocio", Amount: d("0")},
	} {
		if _, err := svc.Update(ctx, id, in); !errors.Is(err, ledger.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := svc.Update(ctx, 999, app.Input{Category: "Ocio", Amount: d("1")}); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("missing id: err = %v, want ErrNotFound", err)
	}
}

func TestUpdateAndDeleteIgnoreOtherKinds(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	income, _ := repo.Create(ctx, ledger.Movement{Date: "2026-10-01", Category: "Sueldo", Kind: ledger.KindIncome, Amount: decimal.NewFromInt(1)})

	if _, err := svc.Update(ctx, income.ID, app.Input{Category: "Ocio", Amount: d("1")}); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("Update income: err = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, income.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("Delete income: err = %v, want ErrNotFound", err)
	}
	if _, ok := repo.rows[income.ID]; !ok {
		t.Error("income must not be deleted through expenses")
	}
}

func TestDelete(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	res, _ := svc.Create(ctx, app.Input{Category: "Mandado", Amount: d("100")})
	if err := svc.Delete(ctx, res.Movement.ID); err != nil || len(repo.rows) != 0 {
		t.Errorf("Delete: %v rows=%d", err, len(repo.rows))
	}
	if err := svc.Delete(ctx, res.Movement.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("second Delete: err = %v, want ErrNotFound", err)
	}
}

func TestList(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.List(ctx, "", 0); err != nil {
		t.Fatal(err)
	}
	if repo.listMonth != "2026-10" || repo.listLimit != app.DefaultListLimit || repo.listKind != ledger.KindExpense {
		t.Errorf("defaults: month=%q limit=%d kind=%q", repo.listMonth, repo.listLimit, repo.listKind)
	}
	if _, err := svc.List(ctx, "2026-09", 5); err != nil || repo.listMonth != "2026-09" || repo.listLimit != 5 {
		t.Errorf("explicit: month=%q limit=%d err=%v", repo.listMonth, repo.listLimit, err)
	}
	if _, err := svc.List(ctx, "2026-9", 5); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("bad month: err = %v, want ErrInvalid", err)
	}
}

func TestInferCategory(t *testing.T) {
	svc, _, _ := newService(t)
	name, ok, err := svc.InferCategory(context.Background(), "Uber al aeropuerto")
	if err != nil || !ok || name != "Transporte" {
		t.Errorf("got %q %v %v", name, ok, err)
	}
	if _, ok, _ := svc.InferCategory(context.Background(), "nada conocido"); ok {
		t.Error("unknown description should not infer")
	}
	// Savings keywords must not leak into expense inference.
	if name, ok, _ := svc.InferCategory(context.Background(), "cetes"); ok {
		t.Errorf("savings keyword inferred expense %q", name)
	}
}
