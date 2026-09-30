package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/valium69mg/finances-app/backend/internal/income/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
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
func (f *fakeRepo) ListByMonth(_ context.Context, month string, kind ledger.Kind, limit int) ([]ledger.Movement, error) {
	f.listMonth, f.listKind, f.listLimit = month, kind, limit
	var out []ledger.Movement
	for _, m := range f.rows {
		if ledger.MonthOf(m.Date) == month && (kind == "" || m.Kind == kind) {
			out = append(out, m)
		}
	}
	return out, nil
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

type fakeSettings struct{ cfg settings.Config }

func (f *fakeSettings) Get(context.Context) (settings.Config, error) { return f.cfg, nil }

func newService(t *testing.T) (*app.Service, *fakeRepo, *fakeSettings) {
	t.Helper()
	cfg := settingstest.RealConfig()
	cfg.PaymentMethods = []string{"Efectivo", "Débito", "Crédito", "Transferencia"}
	repo, st := newFakeRepo(), &fakeSettings{cfg: cfg}
	now := func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	return app.NewService(repo, st, now), repo, st
}

func TestCreateSueldoUSD(t *testing.T) {
	svc, repo, _ := newService(t)
	res, err := svc.Create(context.Background(), app.Input{
		Date: "2026-10-01", Description: "Sueldo octubre", Category: "Sueldo", Currency: "USD", Amount: d("3383.33"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	m := res.Movement
	if m.ID != 1 || m.Kind != ledger.KindIncome || m.PaymentMethod != "Transferencia" || !m.ExchangeRate.Equal(d("17.74")) ||
		!m.AmountMXN.Equal(d("60020.27")) {
		t.Errorf("unexpected movement %+v", m)
	}
	if _, ok := repo.rows[1]; !ok {
		t.Error("movement was not stored")
	}
	sum := res.Summary
	if sum.Month != "2026-10" || !sum.MonthTotalMXN.Equal(d("60020.27")) {
		t.Errorf("summary %+v", sum)
	}
	// 60,020.27 falls in the 1.5% bracket; an empty month is at the first (1%).
	r := sum.Resico
	if r == nil || !r.Rate.Equal(d("0.015")) || !r.EstimatedISR.Equal(d("900.30405")) || !r.RateIncreased ||
		r.PreviousRate == nil || !r.PreviousRate.Equal(d("0.01")) {
		t.Errorf("resico %+v", r)
	}
	if res.Split != nil {
		t.Errorf("Sueldo must not get a split: %+v", res.Split)
	}
	if repo.listKind != ledger.KindIncome {
		t.Errorf("month total listed kind %q", repo.listKind)
	}
}

func TestCreateAppliesDefaultsAndInfersCategory(t *testing.T) {
	svc, _, _ := newService(t)
	res, err := svc.Create(context.Background(), app.Input{Description: "Pago USA octubre", Amount: d("100")})
	if err != nil {
		t.Fatal(err)
	}
	m := res.Movement
	if m.Category != "Sueldo" || m.Date != "2026-10-15" || m.PaymentMethod != "Transferencia" || m.Currency != "MXN" {
		t.Errorf("defaults %+v", m)
	}
	if _, err := svc.Create(context.Background(), app.Input{Description: "xyzzy", Amount: d("1")}); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("uninferable description: err = %v, want ErrInvalid", err)
	}
}

func TestRateIncreaseAcrossBrackets(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	repo.Create(ctx, ledger.Movement{Date: "2026-10-02", Kind: ledger.KindIncome, Category: "Sueldo", AmountMXN: d("24000"), Amount: d("24000")})
	repo.Create(ctx, ledger.Movement{Date: "2026-09-02", Kind: ledger.KindIncome, Category: "Sueldo", AmountMXN: d("90000"), Amount: d("90000")})
	repo.Create(ctx, ledger.Movement{Date: "2026-10-03", Kind: ledger.KindExpense, Category: "Ocio", AmountMXN: d("5000"), Amount: d("5000")})

	res, err := svc.Create(ctx, app.Input{Category: "Otros ingresos", Amount: d("2000")})
	if err != nil {
		t.Fatal(err)
	}
	r := res.Summary.Resico
	// October: 24,000 (1%) -> 26,000 (1.1%); September income and expenses do not count.
	if !res.Summary.MonthTotalMXN.Equal(d("26000")) || !r.Rate.Equal(d("0.011")) || !r.EstimatedISR.Equal(d("286")) ||
		!r.RateIncreased || !r.PreviousRate.Equal(d("0.01")) {
		t.Errorf("summary %+v resico %+v", res.Summary, r)
	}

	// Staying in the same bracket: no increase, no previous rate.
	res, err = svc.Create(ctx, app.Input{Category: "Otros ingresos", Amount: d("100")})
	if err != nil {
		t.Fatal(err)
	}
	if r := res.Summary.Resico; r.RateIncreased || r.PreviousRate != nil || !r.Rate.Equal(d("0.011")) {
		t.Errorf("same bracket resico %+v", r)
	}
}

func TestExtraContractSplit(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	res, err := svc.Create(ctx, app.Input{Category: "Contrato extra", Amount: d("1000")})
	if err != nil {
		t.Fatal(err)
	}
	s := res.Split
	// 1,000: SAT 165; remainder 835 -> 417.5 (418, half-even), 292.25 (292), 125.25 (125).
	if s == nil || !s.SATReserve.Equal(d("165")) || !s.EmergencyFund.Equal(d("418")) || !s.Investments.Equal(d("292")) ||
		!s.AguinaldoVacation.Equal(d("125")) || s.GoalReached {
		t.Fatalf("split %+v", s)
	}
	if len(s.Breakdown) != 1 || s.Breakdown[0].Key != "voo" || !s.Breakdown[0].Amount.Equal(d("292")) {
		t.Errorf("breakdown %+v", s.Breakdown)
	}
	if len(repo.rows) != 1 {
		t.Errorf("the split must not create movements, rows = %d", len(repo.rows))
	}

	// The emergency fund goal (6 x 27,820.44 = 166,922.64) is reached: its share moves to investments.
	repo.Create(ctx, ledger.Movement{Date: "2026-01-01", Kind: ledger.KindSavings, Category: "Fondo de emergencia", Amount: d("170000"), AmountMXN: d("170000")})
	res, err = svc.Create(ctx, app.Input{Category: "Contrato extra", Amount: d("1000")})
	if err != nil {
		t.Fatal(err)
	}
	s = res.Split
	if !s.GoalReached || !s.EmergencyFund.IsZero() || !s.Investments.Equal(d("710")) {
		t.Errorf("goal reached split %+v", s)
	}
}

func TestSplitUsesAmountMXNAndInvestmentWeights(t *testing.T) {
	svc, _, st := newService(t)
	st.cfg.InvestmentAllocation = []settings.Weight{{Key: "voo", Value: d("0.6")}, {Key: "vxus", Value: d("0.4")}, {Key: "cetes-28", Value: d("0")}}
	// 100 USD * 17.74 = 1,774 MXN: SAT 292.71 -> 293; remainder 1,481 -> 740.5 (740), 518.35 (518), 222.15 (222); diff 1 to the fund.
	res, err := svc.Create(context.Background(), app.Input{Category: "Contrato extra", Currency: "USD", Amount: d("100")})
	if err != nil {
		t.Fatal(err)
	}
	s := res.Split
	if !s.SATReserve.Equal(d("293")) || !s.EmergencyFund.Equal(d("741")) || !s.Investments.Equal(d("518")) || !s.AguinaldoVacation.Equal(d("222")) {
		t.Fatalf("split %+v", s)
	}
	// 518 * 0.6 = 310.8 (311), 518 * 0.4 = 207.2 (207); zero weights are skipped.
	if len(s.Breakdown) != 2 || s.Breakdown[0].Key != "voo" || !s.Breakdown[0].Amount.Equal(d("311")) ||
		s.Breakdown[1].Key != "vxus" || !s.Breakdown[1].Amount.Equal(d("207")) {
		t.Errorf("breakdown %+v", s.Breakdown)
	}

	st.cfg.InvestmentAllocation = nil
	res, err = svc.Create(context.Background(), app.Input{Category: "Contrato extra", Amount: d("100")})
	if err != nil || res.Split == nil || len(res.Split.Breakdown) != 0 {
		t.Errorf("no allocation: %+v, %v", res.Split, err)
	}
}

func TestMonthSummary(t *testing.T) {
	svc, repo, st := newService(t)
	ctx := context.Background()
	repo.Create(ctx, ledger.Movement{Date: "2026-10-02", Kind: ledger.KindIncome, Category: "Sueldo", AmountMXN: d("24000"), Amount: d("24000")})
	repo.Create(ctx, ledger.Movement{Date: "2026-10-09", Kind: ledger.KindIncome, Category: "Otros ingresos", AmountMXN: d("2000"), Amount: d("2000")})
	repo.Create(ctx, ledger.Movement{Date: "2026-09-02", Kind: ledger.KindIncome, Category: "Sueldo", AmountMXN: d("90000"), Amount: d("90000")})
	repo.Create(ctx, ledger.Movement{Date: "2026-10-03", Kind: ledger.KindExpense, Category: "Ocio", AmountMXN: d("5000"), Amount: d("5000")})

	// The empty month defaults to the current one; only its income counts.
	sum, err := svc.MonthSummary(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	r := sum.Resico
	if sum.Month != "2026-10" || !sum.MonthTotalMXN.Equal(d("26000")) || r == nil || !r.Rate.Equal(d("0.011")) ||
		!r.EstimatedISR.Equal(d("286")) || r.RateIncreased || r.PreviousRate != nil {
		t.Errorf("summary %+v resico %+v", sum, r)
	}

	// A month without income takes the first bracket.
	sum, err = svc.MonthSummary(ctx, "2027-01")
	if err != nil || !sum.MonthTotalMXN.IsZero() || sum.Resico == nil || !sum.Resico.Rate.Equal(d("0.01")) || !sum.Resico.EstimatedISR.IsZero() {
		t.Errorf("empty month: %+v, %v", sum, err)
	}

	if _, err := svc.MonthSummary(ctx, "2026-13"); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("bad month: err = %v, want ErrInvalid", err)
	}

	st.cfg.Brackets = nil
	sum, err = svc.MonthSummary(ctx, "2026-10")
	if err != nil || sum.Resico != nil || !sum.MonthTotalMXN.Equal(d("26000")) {
		t.Errorf("no brackets: %+v, %v", sum, err)
	}
}

func TestIncompleteSettingsDegradeGracefully(t *testing.T) {
	svc, repo, st := newService(t)
	st.cfg.Brackets = nil
	res, err := svc.Create(context.Background(), app.Input{Category: "Contrato extra", Amount: d("1000")})
	if err != nil || res.Summary.Resico != nil || len(repo.rows) != 1 {
		t.Errorf("no brackets: res=%+v err=%v rows=%d", res, err, len(repo.rows))
	}
	// The goal needs the tax budgets, so the split is unavailable too, but the total survives.
	if !res.Summary.MonthTotalMXN.Equal(d("1000")) || res.Split != nil {
		t.Errorf("missing brackets: %+v", res)
	}

	st.cfg = settingstest.RealConfig()
	st.cfg.PaymentMethods = []string{"Transferencia"}
	st.cfg.EmergencyMonths = nil
	res, err = svc.Create(context.Background(), app.Input{Category: "Contrato extra", Amount: d("1000")})
	if err != nil || res.Split != nil || res.Summary.Resico == nil {
		t.Errorf("no emergency months: split=%+v resico=%+v err=%v", res.Split, res.Summary.Resico, err)
	}
}

func TestCreateRejectsInvalid(t *testing.T) {
	svc, repo, _ := newService(t)
	bad := []app.Input{
		{Category: "Vivienda", Amount: d("10")},                                       // expense category
		{Category: "Sueldo", Amount: d("0")},                                          // zero
		{Category: "Sueldo", Amount: d("-5")},                                         // negative
		{Category: "Sueldo", Amount: d("5"), PaymentMethod: "X"},                      // payment method
		{Category: "Sueldo", Amount: d("5"), Currency: "EUR"},                         // currency
		{Category: "Sueldo", Amount: d("5"), Date: "2026-13-01"},                      // date
		{Category: "Sueldo", Currency: "USD", Amount: d("5"), ExchangeRate: ptr("0")}, // rate
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

func ptr(s string) *decimal.Decimal { v := d(s); return &v }

func TestUpdateRecomputesMonthWithoutDoubleCounting(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	created, _ := svc.Create(ctx, app.Input{Category: "Otros ingresos", Amount: d("1000")})
	id := created.Movement.ID

	res, err := svc.Update(ctx, id, app.Input{Category: "Contrato extra", Amount: d("1500"), Date: "2026-10-20", PaymentMethod: "Efectivo"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := repo.rows[id]
	if got.Category != "Contrato extra" || !got.Amount.Equal(d("1500")) || got.PaymentMethod != "Efectivo" || got.Date != "2026-10-20" {
		t.Errorf("stored %+v", got)
	}
	if !res.Summary.MonthTotalMXN.Equal(d("1500")) || res.Split == nil {
		t.Errorf("summary %+v split %+v", res.Summary, res.Split)
	}

	// Moving it to another month re-totals that month.
	res, err = svc.Update(ctx, id, app.Input{Category: "Otros ingresos", Amount: d("1500"), Date: "2026-11-02"})
	if err != nil || res.Summary.Month != "2026-11" || !res.Summary.MonthTotalMXN.Equal(d("1500")) {
		t.Errorf("moved: %+v, %v", res.Summary, err)
	}

	for name, in := range map[string]app.Input{
		"payment method": {Category: "Sueldo", Amount: d("1"), PaymentMethod: "Cheque"},
		"amount":         {Category: "Sueldo", Amount: d("0")},
	} {
		if _, err := svc.Update(ctx, id, in); !errors.Is(err, ledger.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := svc.Update(ctx, 999, app.Input{Category: "Sueldo", Amount: d("1")}); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("missing id: err = %v, want ErrNotFound", err)
	}
}

func TestUpdateAndDeleteIgnoreOtherKinds(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	expense, _ := repo.Create(ctx, ledger.Movement{Date: "2026-10-01", Category: "Ocio", Kind: ledger.KindExpense, Amount: decimal.NewFromInt(1)})

	if _, err := svc.Update(ctx, expense.ID, app.Input{Category: "Sueldo", Amount: d("1")}); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("Update expense: err = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, expense.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("Delete expense: err = %v, want ErrNotFound", err)
	}
	if _, ok := repo.rows[expense.ID]; !ok {
		t.Error("expense must not be deleted through income")
	}
}

func TestDelete(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	res, _ := svc.Create(ctx, app.Input{Category: "Sueldo", Amount: d("100")})
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
	if repo.listMonth != "2026-10" || repo.listLimit != app.DefaultListLimit || repo.listKind != ledger.KindIncome {
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
	name, ok, err := svc.InferCategory(context.Background(), "Quincena cliente B")
	if err != nil || !ok || name != "Contrato extra" {
		t.Errorf("got %q %v %v", name, ok, err)
	}
	if _, ok, _ := svc.InferCategory(context.Background(), "nada conocido"); ok {
		t.Error("unknown description should not infer")
	}
	// Expense keywords must not leak into income inference.
	if name, ok, _ := svc.InferCategory(context.Background(), "walmart"); ok {
		t.Errorf("expense keyword inferred income %q", name)
	}
}
