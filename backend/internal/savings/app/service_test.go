package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/savings/app"
	savings "github.com/valium69mg/finances-app/backend/internal/savings/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

type fakeRepo struct {
	rows      map[int]ledger.Movement
	nextID    int
	listLimit int
	listKind  ledger.Kind
	listFrom  string
	listTo    string
	batches   int
	failBatch error

	transferDeletes int
}

func newFakeRepo() *fakeRepo { return &fakeRepo{rows: map[int]ledger.Movement{}, nextID: 1} }

func (f *fakeRepo) Create(_ context.Context, m ledger.Movement) (ledger.Movement, error) {
	m.ID = f.nextID
	f.nextID++
	f.rows[m.ID] = m
	return m, nil
}

// CreateMany is all or nothing, like the transactional adapter.
func (f *fakeRepo) CreateMany(_ context.Context, ms []ledger.Movement) ([]ledger.Movement, error) {
	f.batches++
	if f.failBatch != nil {
		return nil, f.failBatch
	}
	out := make([]ledger.Movement, len(ms))
	for i, m := range ms {
		m.ID = f.nextID
		f.nextID++
		f.rows[m.ID] = m
		out[i] = m
	}
	return out, nil
}
func (f *fakeRepo) Update(_ context.Context, m ledger.Movement) error {
	if cur, ok := f.rows[m.ID]; !ok || cur.Kind != m.Kind {
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
func (f *fakeRepo) DeleteByTransfer(_ context.Context, transferID string) error {
	f.transferDeletes++
	found := false
	for id, m := range f.rows {
		if m.TransferID == transferID {
			delete(f.rows, id)
			found = true
		}
	}
	if !found {
		return ledger.ErrNotFound
	}
	return nil
}
func (f *fakeRepo) GetByID(_ context.Context, id int) (ledger.Movement, error) {
	m, ok := f.rows[id]
	if !ok {
		return ledger.Movement{}, ledger.ErrNotFound
	}
	return m, nil
}
func (f *fakeRepo) ListByRange(_ context.Context, from, to string, kind ledger.Kind, limit int) ([]ledger.Movement, error) {
	f.listFrom, f.listTo, f.listKind, f.listLimit = from, to, kind, limit
	var out []ledger.Movement
	for _, m := range f.rows {
		if m.Date >= from && m.Date <= to && (kind == "" || m.Kind == kind) {
			out = append(out, m)
		}
	}
	return out, nil
}
func (f *fakeRepo) ListAllByKind(_ context.Context, kind ledger.Kind) ([]ledger.Movement, error) {
	var out []ledger.Movement
	for id := 1; id < f.nextID; id++ {
		if m, ok := f.rows[id]; ok && m.Kind == kind {
			out = append(out, m)
		}
	}
	return out, nil
}

type fakeValuations struct{ rows []ledger.Valuation }

func (f *fakeValuations) Add(_ context.Context, v ledger.Valuation) error {
	f.rows = append(f.rows, v)
	return nil
}
func (f *fakeValuations) List(context.Context) ([]ledger.Valuation, error) { return f.rows, nil }

type fakeSettings struct{ cfg settings.Config }

func (f *fakeSettings) Get(context.Context) (settings.Config, error) { return f.cfg, nil }

func newService(t *testing.T) (*app.Service, *fakeRepo, *fakeValuations, *fakeSettings) {
	t.Helper()
	cfg := settingstest.RealConfig()
	cfg.PaymentMethods = []string{"Efectivo", "Débito", "Crédito", "Transferencia"}
	repo, vals, st := newFakeRepo(), &fakeValuations{}, &fakeSettings{cfg: cfg}
	now := func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	return app.NewService(repo, vals, st, now), repo, vals, st
}

func TestCreateSavingDefaults(t *testing.T) {
	svc, repo, _, _ := newService(t)
	m, err := svc.CreateSaving(context.Background(), app.Input{Category: "Inversiones", Amount: d("1000")})
	if err != nil {
		t.Fatal(err)
	}
	// The instrument comes from instrumento_por_categoria; everything else from ledger defaults.
	if m.ID != 1 || m.Kind != ledger.KindSavings || m.Instrument != "voo" || m.PaymentMethod != "Transferencia" ||
		m.Currency != "MXN" || m.Date != "2026-10-15" || !m.AmountMXN.Equal(d("1000")) {
		t.Errorf("unexpected movement %+v", m)
	}
	if _, ok := repo.rows[1]; !ok {
		t.Error("movement was not stored")
	}

	// Explicit instrument, USD rate from fx_rate_applied, canonical category from a sloppy name.
	m, err = svc.CreateSaving(context.Background(), app.Input{Category: "fondo de emergencia", Instrument: "cetes-91", Currency: "USD", Amount: d("10")})
	if err != nil {
		t.Fatal(err)
	}
	if m.Category != "Fondo de emergencia" || m.Instrument != "cetes-91" || !m.ExchangeRate.Equal(d("17.74")) || !m.AmountMXN.Equal(d("177.40")) {
		t.Errorf("explicit %+v", m)
	}
}

func TestCreateSavingInfersCategoryAndAllowsWithdrawals(t *testing.T) {
	svc, _, _, _ := newService(t)
	m, err := svc.CreateSaving(context.Background(), app.Input{Description: "Viaje a Oaxaca", Amount: d("-500")})
	if err != nil {
		t.Fatal(err)
	}
	if m.Category != "Aguinaldo y vacaciones" || m.Instrument != "cetes-28" || !m.Amount.Equal(d("-500")) || !m.AmountMXN.Equal(d("-500")) {
		t.Errorf("withdrawal %+v", m)
	}
	if _, err := svc.CreateSaving(context.Background(), app.Input{Description: "xyzzy", Amount: d("1")}); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("uninferable: err = %v, want ErrInvalid", err)
	}
}

func TestCreateSavingWithoutDefaultInstrument(t *testing.T) {
	svc, _, _, st := newService(t)
	delete(st.cfg.InstrumentByCategory, "Inversiones")
	m, err := svc.CreateSaving(context.Background(), app.Input{Category: "Inversiones", Amount: d("1")})
	if err != nil || m.Instrument != "" {
		t.Errorf("no default instrument: %+v, %v", m, err)
	}
}

func TestCreateSavingRejectsInvalid(t *testing.T) {
	svc, repo, _, st := newService(t)
	bad := map[string]app.Input{
		"expense category":   {Category: "Vivienda", Amount: d("10")},
		"income category":    {Category: "Sueldo", Amount: d("10")},
		"unknown instrument": {Category: "Inversiones", Instrument: "nope", Amount: d("10")},
		"zero":               {Category: "Inversiones", Amount: d("0")},
		"payment method":     {Category: "Inversiones", Amount: d("5"), PaymentMethod: "X"},
		"currency":           {Category: "Inversiones", Amount: d("5"), Currency: "EUR"},
		"date":               {Category: "Inversiones", Amount: d("5"), Date: "2026-13-01"},
		"usd rate":           {Category: "Inversiones", Currency: "USD", Amount: d("5"), ExchangeRate: ptr("0")},
	}
	for name, in := range bad {
		if _, err := svc.CreateSaving(context.Background(), in); !errors.Is(err, ledger.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	st.cfg.InstrumentByCategory["Inversiones"] = "ghost"
	if _, err := svc.CreateSaving(context.Background(), app.Input{Category: "Inversiones", Amount: d("1")}); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("dangling default instrument: err = %v, want ErrInvalid", err)
	}
	if len(repo.rows) != 0 {
		t.Error("invalid input must not be stored")
	}
}

func ptr(s string) *decimal.Decimal { v := d(s); return &v }

func TestUpdateSaving(t *testing.T) {
	svc, repo, _, _ := newService(t)
	ctx := context.Background()
	created, _ := svc.CreateSaving(ctx, app.Input{Category: "Inversiones", Amount: d("1000")})

	got, err := svc.UpdateSaving(ctx, created.ID, app.Input{Category: "Gastos futuros", Amount: d("-200"), Date: "2026-10-20"})
	if err != nil {
		t.Fatalf("UpdateSaving: %v", err)
	}
	stored := repo.rows[created.ID]
	if got.ID != created.ID || stored.Category != "Gastos futuros" || stored.Instrument != "liquidez-gbm" ||
		!stored.Amount.Equal(d("-200")) || stored.Date != "2026-10-20" {
		t.Errorf("stored %+v", stored)
	}
	if _, err := svc.UpdateSaving(ctx, created.ID, app.Input{Category: "Inversiones", Amount: d("0")}); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("zero amount: err = %v, want ErrInvalid", err)
	}
	if _, err := svc.UpdateSaving(ctx, 999, app.Input{Category: "Inversiones", Amount: d("1")}); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("missing id: err = %v, want ErrNotFound", err)
	}
}

func TestUpdateSavingKeepsOmittedFields(t *testing.T) {
	svc, repo, _, _ := newService(t)
	ctx := context.Background()
	created, err := svc.CreateSaving(ctx, app.Input{
		Category: "Inversiones", Amount: d("10"), Currency: "USD", ExchangeRate: ptr("20.5"),
		PaymentMethod: "Efectivo", Date: "2026-09-03", Description: "before",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Only the description (and the required amount) is sent, as an edit form might.
	if _, err := svc.UpdateSaving(ctx, created.ID, app.Input{Category: "Inversiones", Amount: d("10"), Description: "after"}); err != nil {
		t.Fatal(err)
	}
	got := repo.rows[created.ID]
	if got.Description != "after" || got.Currency != "USD" || got.ExchangeRate == nil || !got.ExchangeRate.Equal(d("20.5")) ||
		got.PaymentMethod != "Efectivo" || got.Date != "2026-09-03" || !got.AmountMXN.Equal(d("205")) {
		t.Errorf("omitted fields were reset: %+v", got)
	}

	// An explicit value still wins, and leaving USD drops the rate.
	if _, err := svc.UpdateSaving(ctx, created.ID, app.Input{Category: "Inversiones", Amount: d("10"), Currency: "MXN", PaymentMethod: "Débito"}); err != nil {
		t.Fatal(err)
	}
	got = repo.rows[created.ID]
	if got.Currency != "MXN" || got.ExchangeRate != nil || got.PaymentMethod != "Débito" || !got.AmountMXN.Equal(d("10")) {
		t.Errorf("explicit values: %+v", got)
	}

	// Switching MXN -> USD without a rate takes the configured one, not a stale rate.
	if _, err := svc.UpdateSaving(ctx, created.ID, app.Input{Category: "Inversiones", Amount: d("10"), Currency: "USD"}); err != nil {
		t.Fatal(err)
	}
	if got = repo.rows[created.ID]; got.ExchangeRate == nil || !got.ExchangeRate.Equal(d("17.74")) {
		t.Errorf("switch to USD: %+v", got)
	}
}

func TestUpdateAndDeleteIgnoreOtherKinds(t *testing.T) {
	svc, repo, _, _ := newService(t)
	ctx := context.Background()
	expense, _ := repo.Create(ctx, ledger.Movement{Date: "2026-10-01", Category: "Ocio", Kind: ledger.KindExpense, Amount: decimal.NewFromInt(1)})

	if _, err := svc.UpdateSaving(ctx, expense.ID, app.Input{Category: "Inversiones", Amount: d("1")}); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("Update expense: err = %v, want ErrNotFound", err)
	}
	if err := svc.DeleteSaving(ctx, expense.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("Delete expense: err = %v, want ErrNotFound", err)
	}
	if _, ok := repo.rows[expense.ID]; !ok {
		t.Error("expense must not be deleted through savings")
	}
}

func TestDeleteSaving(t *testing.T) {
	svc, repo, _, _ := newService(t)
	ctx := context.Background()
	m, _ := svc.CreateSaving(ctx, app.Input{Category: "Inversiones", Amount: d("100")})
	if err := svc.DeleteSaving(ctx, m.ID); err != nil || len(repo.rows) != 0 {
		t.Errorf("DeleteSaving: %v rows=%d", err, len(repo.rows))
	}
	if err := svc.DeleteSaving(ctx, m.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("second delete: err = %v, want ErrNotFound", err)
	}
}

func TestListSavings(t *testing.T) {
	svc, repo, _, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.ListSavings(ctx, "", 0); err != nil {
		t.Fatal(err)
	}
	if repo.listFrom != "2026-10-01" || repo.listTo != "2026-10-31" || repo.listLimit != app.DefaultListLimit || repo.listKind != ledger.KindSavings {
		t.Errorf("defaults: range=%q..%q limit=%d kind=%q", repo.listFrom, repo.listTo, repo.listLimit, repo.listKind)
	}
	if _, err := svc.ListSavings(ctx, "2026-09", 5); err != nil || repo.listFrom != "2026-09-01" || repo.listTo != "2026-09-30" || repo.listLimit != 5 {
		t.Errorf("explicit: range=%q..%q limit=%d err=%v", repo.listFrom, repo.listTo, repo.listLimit, err)
	}
	if _, err := svc.ListSavings(ctx, "2026-9", 5); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("bad month: err = %v, want ErrInvalid", err)
	}
}

func TestTransfer(t *testing.T) {
	svc, repo, _, _ := newService(t)
	res, err := svc.Transfer(context.Background(), app.TransferInput{From: "cetes-28", To: "voo", Amount: d("1500"), Date: "2026-10-03"})
	if err != nil {
		t.Fatal(err)
	}
	// One atomic batch with the two legs.
	if repo.batches != 1 || len(repo.rows) != 2 {
		t.Fatalf("batches=%d rows=%d", repo.batches, len(repo.rows))
	}
	out, in := res.Out, res.In
	if out.Instrument != "cetes-28" || !out.Amount.Equal(d("-1500")) || !out.AmountMXN.Equal(d("-1500")) ||
		in.Instrument != "voo" || !in.Amount.Equal(d("1500")) || !in.AmountMXN.Equal(d("1500")) {
		t.Errorf("legs out=%+v in=%+v", out, in)
	}
	for _, m := range []ledger.Movement{out, in} {
		if m.Kind != ledger.KindSavings || m.PaymentMethod != "Transferencia" || m.Currency != "MXN" || m.Date != "2026-10-03" ||
			m.Description != "Traspaso cetes-28 -> voo" {
			t.Errorf("leg %+v", m)
		}
	}
	// The source instrument is cetes-28: the last category with that default wins, like fin.py.
	if out.Category != "Aguinaldo y vacaciones" || in.Category != out.Category {
		t.Errorf("category %q / %q", out.Category, in.Category)
	}
}

func TestTransferLinksLegsAndLocksThem(t *testing.T) {
	svc, repo, _, _ := newService(t)
	ctx := context.Background()
	res, err := svc.Transfer(ctx, app.TransferInput{From: "cetes-28", To: "voo", Amount: d("100")})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.Transfer(ctx, app.TransferInput{From: "cetes-28", To: "voo", Amount: d("50")})
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := svc.CreateSaving(ctx, app.Input{Category: "Inversiones", Amount: d("9")})
	if res.Out.TransferID == "" || res.Out.TransferID != res.In.TransferID || res.Out.TransferID == other.Out.TransferID {
		t.Fatalf("transfer ids: %q %q %q", res.Out.TransferID, res.In.TransferID, other.Out.TransferID)
	}
	if plain.TransferID != "" {
		t.Errorf("plain saving got transfer id %q", plain.TransferID)
	}

	// A leg cannot be edited on its own.
	before := repo.rows[res.Out.ID]
	if _, err := svc.UpdateSaving(ctx, res.Out.ID, app.Input{Category: "Inversiones", Amount: d("-1")}); !errors.Is(err, savings.ErrTransferLegLocked) {
		t.Errorf("update leg: err = %v, want ErrTransferLegLocked", err)
	}
	if repo.rows[res.Out.ID].Amount.String() != before.Amount.String() {
		t.Error("locked leg was modified")
	}

	// Deleting either leg removes both, and only that transfer.
	if err := svc.DeleteSaving(ctx, res.In.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := repo.rows[res.In.ID]; ok {
		t.Error("deleted leg still there")
	}
	if _, ok := repo.rows[res.Out.ID]; ok {
		t.Error("the other leg survived")
	}
	if _, ok := repo.rows[other.Out.ID]; !ok || len(repo.rows) != 3 || repo.transferDeletes != 1 {
		t.Errorf("rows=%d transferDeletes=%d", len(repo.rows), repo.transferDeletes)
	}
	if err := svc.DeleteSaving(ctx, plain.ID); err != nil || repo.transferDeletes != 1 {
		t.Errorf("plain delete: %v (transfer deletes %d)", err, repo.transferDeletes)
	}
}

func TestTransferCategoryInference(t *testing.T) {
	svc, _, _, _ := newService(t)
	ctx := context.Background()

	// Source has no default category: the target's is used.
	res, err := svc.Transfer(ctx, app.TransferInput{From: "vxus", To: "voo", Amount: d("10")})
	if err != nil || res.Out.Category != "Inversiones" {
		t.Errorf("target fallback: %+v, %v", res, err)
	}
	// Neither instrument maps to a category.
	if _, err := svc.Transfer(ctx, app.TransferInput{From: "vxus", To: "cetes-91", Amount: d("10")}); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("no category: err = %v, want ErrInvalid", err)
	}
	// An explicit category overrides the inference and a description is kept.
	res, err = svc.Transfer(ctx, app.TransferInput{From: "vxus", To: "cetes-91", Amount: d("10"), Category: "gastos futuros", Description: "Apartado"})
	if err != nil || res.Out.Category != "Gastos futuros" || res.In.Description != "Apartado" {
		t.Errorf("explicit category: %+v, %v", res, err)
	}
	// The category must be an Ahorro category.
	if _, err := svc.Transfer(ctx, app.TransferInput{From: "voo", To: "vxus", Amount: d("10"), Category: "Vivienda"}); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("expense category: err = %v, want ErrInvalid", err)
	}
}

func TestTransferRejectsInvalidAndStoresNothing(t *testing.T) {
	svc, repo, _, _ := newService(t)
	bad := map[string]app.TransferInput{
		"same instrument":    {From: "voo", To: "voo", Amount: d("1")},
		"unknown source":     {From: "nope", To: "voo", Amount: d("1")},
		"unknown target":     {From: "voo", To: "nope", Amount: d("1")},
		"empty source":       {To: "voo", Amount: d("1")},
		"zero amount":        {From: "cetes-28", To: "voo", Amount: d("0")},
		"negative amount":    {From: "cetes-28", To: "voo", Amount: d("-5")},
		"bad date":           {From: "cetes-28", To: "voo", Amount: d("5"), Date: "2026-02-30"},
		"unknown category":   {From: "cetes-28", To: "voo", Amount: d("5"), Category: "Nada"},
		"no inferable label": {From: "vxus", To: "cetes-91", Amount: d("5")},
	}
	for name, in := range bad {
		if _, err := svc.Transfer(context.Background(), in); !errors.Is(err, ledger.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if len(repo.rows) != 0 || repo.batches != 0 {
		t.Errorf("rows=%d batches=%d, want nothing stored", len(repo.rows), repo.batches)
	}
}

func TestTransferStorageFailureIsPropagated(t *testing.T) {
	svc, repo, _, _ := newService(t)
	boom := errors.New("db down")
	repo.failBatch = boom
	if _, err := svc.Transfer(context.Background(), app.TransferInput{From: "cetes-28", To: "voo", Amount: d("1")}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
	if len(repo.rows) != 0 {
		t.Error("a failed batch must store nothing")
	}
}

func TestAddAndListValuations(t *testing.T) {
	svc, _, vals, _ := newService(t)
	ctx := context.Background()
	v, err := svc.AddValuation(ctx, app.ValuationInput{Instrument: "voo", ValueMXN: d("12345.67"), Note: " statement "})
	if err != nil {
		t.Fatal(err)
	}
	if v.Date != "2026-10-15" || v.Instrument != "voo" || !v.ValueMXN.Equal(d("12345.67")) || v.Note != "statement" {
		t.Errorf("valuation %+v", v)
	}
	if _, err := svc.AddValuation(ctx, app.ValuationInput{Date: "2026-10-01", Instrument: "voo", ValueMXN: d("100")}); err != nil {
		t.Fatal(err)
	}
	// Append-only: both valuations are kept, in insertion order.
	got, err := svc.ListValuations(ctx)
	if err != nil || len(got) != 2 || len(vals.rows) != 2 || got[1].Date != "2026-10-01" {
		t.Errorf("list %+v, %v", got, err)
	}
}

func TestAddValuationRoundsToCents(t *testing.T) {
	svc, _, vals, _ := newService(t)
	v, err := svc.AddValuation(context.Background(), app.ValuationInput{Instrument: "voo", ValueMXN: d("100.555")})
	if err != nil {
		t.Fatal(err)
	}
	if !v.ValueMXN.Equal(d("100.56")) || !vals.rows[0].ValueMXN.Equal(d("100.56")) || v.ValueMXN.Exponent() != -2 {
		t.Errorf("returned %s, stored %s, want 100.56", v.ValueMXN, vals.rows[0].ValueMXN)
	}
}

func TestAddValuationRejectsInvalid(t *testing.T) {
	svc, _, vals, _ := newService(t)
	bad := map[string]app.ValuationInput{
		"zero":               {Instrument: "voo", ValueMXN: d("0")},
		"negative":           {Instrument: "voo", ValueMXN: d("-1")},
		"bad date":           {Date: "nope", Instrument: "voo", ValueMXN: d("1")},
		"missing instrument": {ValueMXN: d("1")},
		"unknown instrument": {Instrument: "ghost", ValueMXN: d("1")},
		"rounds to zero":     {Instrument: "voo", ValueMXN: d("0.004")},
		"at the cap":         {Instrument: "voo", ValueMXN: d("1e12")},
		"absurd exponent":    {Instrument: "voo", ValueMXN: d("1e999999999")},
	}
	for name, in := range bad {
		if _, err := svc.AddValuation(context.Background(), in); !errors.Is(err, savings.ErrInvalidValuation) {
			t.Errorf("%s: err = %v, want ErrInvalidValuation", name, err)
		}
	}
	if len(vals.rows) != 0 {
		t.Error("invalid valuations must not be stored")
	}
}

func TestPortfolio(t *testing.T) {
	svc, _, _, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.CreateSaving(ctx, app.Input{Category: "Inversiones", Amount: d("1000")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Transfer(ctx, app.TransferInput{From: "voo", To: "cetes-28", Amount: d("400"), Category: "Inversiones"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddValuation(ctx, app.ValuationInput{Instrument: "voo", ValueMXN: d("900")}); err != nil {
		t.Fatal(err)
	}

	p, err := svc.Portfolio(ctx)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]savings.PortfolioRow{}
	for _, r := range p.Rows {
		rows[r.ID] = r
	}
	voo, cetes := rows["voo"], rows["cetes-28"]
	if !voo.Contributed.Equal(d("600")) || !voo.Value.Equal(d("900")) || voo.Unvalued || !voo.Gain.Equal(d("300")) ||
		!cetes.Contributed.Equal(d("400")) || !cetes.Unvalued {
		t.Errorf("rows voo=%+v cetes=%+v", voo, cetes)
	}
	if !p.TotalContributed.Equal(d("1000")) || !p.TotalValue.Equal(d("1300")) {
		t.Errorf("totals %s / %s", p.TotalContributed, p.TotalValue)
	}
	// 6 x 27,820.44 goal; nothing accumulated in the emergency fund.
	if !p.Emergency.Goal.Equal(d("166922.64")) || !p.Emergency.Accumulated.IsZero() {
		t.Errorf("emergency %+v", p.Emergency)
	}
	if len(p.ByDestination) != len(savings.DestinationCategories) {
		t.Errorf("by destination %+v", p.ByDestination)
	}
}

func TestPortfolioNeedsEmergencyMonths(t *testing.T) {
	svc, _, _, st := newService(t)
	st.cfg.EmergencyMonths = nil
	if _, err := svc.Portfolio(context.Background()); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("err = %v, want ErrMissingConfig", err)
	}
}

func TestListSavingsFollowsTheCycle(t *testing.T) {
	svc, repo, _, st := newService(t)
	st.cfg.CycleStartDay = 31
	ctx := context.Background()
	if _, err := svc.ListSavings(ctx, "2026-10", 0); err != nil {
		t.Fatal(err)
	}
	if repo.listFrom != "2026-09-30" || repo.listTo != "2026-10-30" {
		t.Errorf("range = %s..%s, want 2026-09-30..2026-10-30", repo.listFrom, repo.listTo)
	}
	if _, err := svc.ListSavings(ctx, "", 0); err != nil || repo.listFrom != "2026-09-30" || repo.listTo != "2026-10-30" {
		t.Errorf("default cycle of 2026-10-15: range = %s..%s, err = %v", repo.listFrom, repo.listTo, err)
	}
}

func TestFutureExpenseLinkIsCarriedAndKept(t *testing.T) {
	svc, repo, _, _ := newService(t)
	ctx := context.Background()
	m, err := svc.CreateSaving(ctx, app.Input{Category: "Gastos futuros", Amount: d("100"), FutureExpenseID: 7})
	if err != nil || m.FutureExpenseID != 7 || repo.rows[m.ID].FutureExpenseID != 7 {
		t.Fatalf("CreateSaving = %+v, %v, want the link stored", m, err)
	}
	// An edit made without the link (the savings page) never drops it.
	updated, err := svc.UpdateSaving(ctx, m.ID, app.Input{Category: "Gastos futuros", Amount: d("150")})
	if err != nil || updated.FutureExpenseID != 7 {
		t.Errorf("UpdateSaving = %+v, %v, want the link kept", updated, err)
	}
}

func TestBuildValidatesWithoutStoring(t *testing.T) {
	svc, repo, _, _ := newService(t)
	m, err := svc.Build(context.Background(), app.Input{Category: "Gastos futuros", Amount: d("-40"), FutureExpenseID: 3})
	if err != nil || m.FutureExpenseID != 3 || !m.AmountMXN.Equal(d("-40")) || len(repo.rows) != 0 {
		t.Errorf("Build = %+v, %v, stored %d", m, err, len(repo.rows))
	}
	if _, err := svc.Build(context.Background(), app.Input{Category: "Nope", Amount: d("1")}); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

func TestCreateManyIsAllOrNothing(t *testing.T) {
	svc, repo, _, _ := newService(t)
	ctx := context.Background()
	out, err := svc.CreateMany(ctx, []app.Input{
		{Category: "Gastos futuros", Amount: d("-50")},
		{Category: "Gastos futuros", Amount: d("50"), FutureExpenseID: 2},
	})
	if err != nil || len(out) != 2 || out[0].FutureExpenseID != 0 || out[1].FutureExpenseID != 2 || repo.batches != 1 {
		t.Fatalf("CreateMany = %+v, %v", out, err)
	}
	before := len(repo.rows)
	if _, err := svc.CreateMany(ctx, []app.Input{
		{Category: "Gastos futuros", Amount: d("10")},
		{Category: "Nope", Amount: d("10")},
	}); !errors.Is(err, ledger.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
	if len(repo.rows) != before {
		t.Errorf("an invalid second row left %d rows stored", len(repo.rows)-before)
	}
}
