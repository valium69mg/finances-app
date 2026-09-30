package app_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	expensesapp "github.com/valium69mg/finances-app/backend/internal/expenses/app"
	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
	"github.com/valium69mg/finances-app/backend/internal/taxfiling/app"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

var d = settingstest.D

func ptr(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

type fakeSettings struct {
	cfg settings.Config
	err error
}

func (f *fakeSettings) Get(context.Context) (settings.Config, error) { return f.cfg, f.err }

type fakeInvoices struct {
	rows []invoices.Invoice
	err  error
}

func (f *fakeInvoices) List(_ context.Context, period string, status invoices.Status) ([]invoices.Invoice, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []invoices.Invoice
	for _, inv := range f.rows {
		if (period == "" || inv.Period == period) && (status == "" || inv.Status == status) {
			out = append(out, inv)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID }) // newest first
	return out, nil
}

type fakeExpenses struct {
	created   []expensesapp.Input
	deleted   []int
	createErr error
	nextID    int
}

func (f *fakeExpenses) Create(_ context.Context, in expensesapp.Input) (expensesapp.Result, error) {
	if f.createErr != nil {
		return expensesapp.Result{}, f.createErr
	}
	f.created = append(f.created, in)
	f.nextID++
	return expensesapp.Result{Movement: ledger.Movement{ID: 100 + f.nextID, Date: in.Date, Category: in.Category, Kind: ledger.KindExpense, Amount: in.Amount, AmountMXN: in.Amount}}, nil
}

func (f *fakeExpenses) Delete(_ context.Context, id int) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeRepo struct {
	filings   map[string]taxfiling.Filing
	createErr error
	payErr    error
}

func (f *fakeRepo) Create(_ context.Context, fl taxfiling.Filing) (taxfiling.Filing, error) {
	if f.createErr != nil {
		return taxfiling.Filing{}, f.createErr
	}
	if _, ok := f.filings[fl.Period]; ok {
		return taxfiling.Filing{}, taxfiling.ErrAlreadyFiled
	}
	f.filings[fl.Period] = fl
	return fl, nil
}

func (f *fakeRepo) Get(_ context.Context, period string) (taxfiling.Filing, error) {
	fl, ok := f.filings[period]
	if !ok {
		return taxfiling.Filing{}, taxfiling.ErrNotFound
	}
	return fl, nil
}

func (f *fakeRepo) List(context.Context) ([]taxfiling.Filing, error) {
	out := make([]taxfiling.Filing, 0, len(f.filings))
	for _, fl := range f.filings {
		out = append(out, fl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Period > out[j].Period })
	return out, nil
}

func (f *fakeRepo) MarkPaid(_ context.Context, period string, p taxfiling.Payment, expenseID *int) (taxfiling.Filing, error) {
	if f.payErr != nil {
		return taxfiling.Filing{}, f.payErr
	}
	fl, ok := f.filings[period]
	if !ok {
		return taxfiling.Filing{}, taxfiling.ErrNotFound
	}
	fl.Payment, fl.ExpenseMovementID = &p, expenseID
	f.filings[period] = fl
	return fl, nil
}

func (f *fakeRepo) Delete(_ context.Context, period string) error {
	fl, ok := f.filings[period]
	if !ok {
		return taxfiling.ErrNotFound
	}
	if fl.Payment != nil {
		return taxfiling.ErrFilingPaid
	}
	delete(f.filings, period)
	return nil
}

type fixture struct {
	svc      *app.Service
	repo     *fakeRepo
	invoices *fakeInvoices
	expenses *fakeExpenses
	settings *fakeSettings
}

func newFixture() *fixture {
	usa := invoices.ComputeUSAInvoice(d("3500"), d("17.74")).Rounded()
	b := invoices.ComputeClientBInvoice(d("35000"), d("0.16")).Rounded()
	fx := &fixture{
		repo: &fakeRepo{filings: map[string]taxfiling.Filing{}},
		invoices: &fakeInvoices{rows: []invoices.Invoice{
			{ID: 1, ClientID: "usa", Period: "2026-10", Currency: "USD", ExchangeRate: ptr("17.74"), Status: invoices.StatusIssued, Amounts: usa},
			{ID: 2, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusIssued, Amounts: b},
			{ID: 3, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusPrepared, Amounts: b},
			{ID: 4, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusCancelled, Amounts: b},
			{ID: 5, ClientID: "usa", Period: "2026-11", Currency: "USD", ExchangeRate: ptr("17.74"), Status: invoices.StatusIssued, Amounts: usa},
		}},
		expenses: &fakeExpenses{},
		settings: &fakeSettings{cfg: settingstest.RealConfig()},
	}
	now := func() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) }
	fx.svc = app.NewService(fx.repo, fx.invoices, fx.expenses, fx.settings, now, nil)
	return fx
}

func TestPreviewComputesWithoutPersisting(t *testing.T) {
	fx := newFixture()
	got, err := fx.svc.Preview(context.Background(), "2026-10", d("0"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Declaration.IncomeCollected.Equal(d("92262.41")) || !got.Declaration.ISRAccrued.Equal(d("1845.25")) {
		t.Errorf("declaration = %+v", got.Declaration)
	}
	if len(got.Declaration.Invoices) != 2 {
		t.Errorf("included = %d, want the 2 issued ones", len(got.Declaration.Invoices))
	}
	if len(got.Warnings) != 1 || got.Warnings[0].Code != app.WarningPreparedInvoices || len(got.Warnings[0].InvoiceIDs) != 1 || got.Warnings[0].InvoiceIDs[0] != 3 {
		t.Errorf("warnings = %+v, want one prepared_invoices warning with #3", got.Warnings)
	}
	if got.Filing != nil || len(fx.repo.filings) != 0 || len(fx.expenses.created) != 0 {
		t.Error("a preview must not store anything")
	}
}

func TestPreviewDefaultsToThePreviousMonth(t *testing.T) {
	got, err := newFixture().svc.Preview(context.Background(), "", d("0"))
	if err != nil || got.Declaration.Period != "2026-10" {
		t.Errorf("period = %q, %v; want 2026-10 (now is 2026-11-10)", got.Declaration.Period, err)
	}
}

func TestPreviewWarnsWhenAlreadyFiled(t *testing.T) {
	fx := newFixture()
	fx.repo.filings["2026-10"] = taxfiling.Filing{Period: "2026-10", FilingDate: "2026-11-05"}
	got, err := fx.svc.Preview(context.Background(), "2026-10", d("0"))
	if err != nil || got.Filing == nil {
		t.Fatalf("got %+v, %v", got, err)
	}
	found := false
	for _, w := range got.Warnings {
		found = found || w.Code == app.WarningAlreadyFiled
	}
	if !found {
		t.Errorf("warnings = %+v, want already_filed", got.Warnings)
	}
}

func TestPreviewErrors(t *testing.T) {
	fx := newFixture()
	if _, err := fx.svc.Preview(context.Background(), "2026-13", d("0")); !errors.Is(err, taxfiling.ErrInvalidInput) {
		t.Errorf("bad period err = %v", err)
	}
	if _, err := fx.svc.Preview(context.Background(), "2026-10", d("-1")); !errors.Is(err, taxfiling.ErrInvalidInput) {
		t.Errorf("negative creditable err = %v", err)
	}
	fx.settings.cfg.Brackets = nil
	if _, err := fx.svc.Preview(context.Background(), "2026-10", d("0")); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("missing brackets err = %v, want ErrMissingConfig", err)
	}
	boom := errors.New("boom")
	fx.settings.cfg = settingstest.RealConfig()
	fx.invoices.err = boom
	if _, err := fx.svc.Preview(context.Background(), "2026-10", d("0")); !errors.Is(err, boom) {
		t.Errorf("invoices err = %v, want boom", err)
	}
}

func TestRegisterPending(t *testing.T) {
	fx := newFixture()
	res, err := fx.svc.Register(context.Background(), app.RegisterInput{Period: "2026-10", Date: "2026-11-05", Folio: "A1"})
	if err != nil {
		t.Fatal(err)
	}
	f := res.Filing
	if f.PaymentStatus() != taxfiling.PaymentPending || f.Folio != "A1" || !f.ISRDue.Equal(d("1845.25")) || len(f.InvoiceIDs) != 2 {
		t.Errorf("filing = %+v", f)
	}
	if res.Expense != nil || len(fx.expenses.created) != 0 {
		t.Error("no expense must be created for a pending filing")
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Code != app.WarningPreparedInvoices {
		t.Errorf("warnings = %+v", res.Warnings)
	}
	if _, ok := fx.repo.filings["2026-10"]; !ok {
		t.Error("the filing was not stored")
	}
}

func TestRegisterDefaultsTheDateToToday(t *testing.T) {
	fx := newFixture()
	res, err := fx.svc.Register(context.Background(), app.RegisterInput{Period: "2026-10"})
	if err != nil || res.Filing.FilingDate != "2026-11-10" {
		t.Errorf("date = %q, %v; want 2026-11-10", res.Filing.FilingDate, err)
	}
}

func TestRegisterWithPaymentDoesNotCreateAnExpenseUnlessAsked(t *testing.T) {
	fx := newFixture()
	res, err := fx.svc.Register(context.Background(), app.RegisterInput{
		Period: "2026-10", Date: "2026-11-05",
		Payment: &app.PaymentInput{ISRPaid: d("1845.25"), IVAPaid: d("4827.59")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Filing.PaymentStatus() != taxfiling.PaymentPaid || res.Filing.Payment.Date != "2026-11-05" {
		t.Errorf("payment = %+v, want paid on the filing date", res.Filing.Payment)
	}
	if res.Expense != nil || len(fx.expenses.created) != 0 || res.Filing.ExpenseMovementID != nil {
		t.Error("an expense was created without record_expense")
	}
}

func TestRegisterWithPaymentAndExpense(t *testing.T) {
	fx := newFixture()
	res, err := fx.svc.Register(context.Background(), app.RegisterInput{
		Period: "2026-10", Date: "2026-11-05",
		Payment: &app.PaymentInput{Date: "2026-11-07", ISRPaid: d("1845.25"), IVAPaid: d("4827.59"), RecordExpense: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fx.expenses.created) != 1 {
		t.Fatalf("expenses created = %d, want 1", len(fx.expenses.created))
	}
	e := fx.expenses.created[0]
	if e.Category != "Impuestos" || e.Currency != "MXN" || e.Date != "2026-11-07" || !e.Amount.Equal(d("6672.84")) ||
		e.Description != "Pago SAT ISR+IVA periodo 2026-10" || e.PaymentMethod != "Transferencia" {
		t.Errorf("expense input = %+v", e)
	}
	if res.Expense == nil || res.Filing.ExpenseMovementID == nil || *res.Filing.ExpenseMovementID != res.Expense.ID {
		t.Errorf("expense link = %+v / %+v", res.Expense, res.Filing.ExpenseMovementID)
	}
}

func TestRegisterFailuresLeaveNoOrphanExpense(t *testing.T) {
	fx := newFixture()
	fx.repo.createErr = errors.New("db down")
	_, err := fx.svc.Register(context.Background(), app.RegisterInput{
		Period: "2026-10", Payment: &app.PaymentInput{ISRPaid: d("10"), RecordExpense: true},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(fx.expenses.created) != 1 || len(fx.expenses.deleted) != 1 || fx.expenses.deleted[0] != 101 {
		t.Errorf("created %d, deleted %v; want the created expense rolled back", len(fx.expenses.created), fx.expenses.deleted)
	}
}

func TestRegisterRefusesToRecordAZeroExpense(t *testing.T) {
	fx := newFixture()
	_, err := fx.svc.Register(context.Background(), app.RegisterInput{
		Period: "2026-10", Payment: &app.PaymentInput{RecordExpense: true},
	})
	if !errors.Is(err, taxfiling.ErrInvalidInput) || len(fx.expenses.created) != 0 || len(fx.repo.filings) != 0 {
		t.Errorf("err = %v, expenses %d, filings %d", err, len(fx.expenses.created), len(fx.repo.filings))
	}
}

func TestRegisterDuplicatePeriod(t *testing.T) {
	fx := newFixture()
	in := app.RegisterInput{Period: "2026-10", Date: "2026-11-05"}
	if _, err := fx.svc.Register(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.svc.Register(context.Background(), in); !errors.Is(err, taxfiling.ErrAlreadyFiled) {
		t.Errorf("err = %v, want ErrAlreadyFiled", err)
	}
}

func TestRegisterValidation(t *testing.T) {
	fx := newFixture()
	for _, in := range []app.RegisterInput{
		{Period: "bad"},
		{Period: "2026-10", Date: "05/11/2026"},
		{Period: "2026-10", IVACreditable: d("-5")},
	} {
		if _, err := fx.svc.Register(context.Background(), in); !errors.Is(err, taxfiling.ErrInvalidInput) {
			t.Errorf("Register(%+v) err = %v, want ErrInvalidInput", in, err)
		}
	}
	fx.settings.err = settings.ErrMissingConfig
	if _, err := fx.svc.Register(context.Background(), app.RegisterInput{Period: "2026-10"}); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("settings err = %v", err)
	}
}

func TestPay(t *testing.T) {
	ctx := context.Background()
	register := func(fx *fixture) {
		if _, err := fx.svc.Register(ctx, app.RegisterInput{Period: "2026-10", Date: "2026-11-05"}); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("records the payment without an expense", func(t *testing.T) {
		fx := newFixture()
		register(fx)
		res, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{Date: "2026-11-12", ISRPaid: d("1845.25"), IVAPaid: d("4827.59")})
		if err != nil || res.Filing.PaymentStatus() != taxfiling.PaymentPaid || res.Expense != nil || len(fx.expenses.created) != 0 {
			t.Fatalf("res = %+v, %v, expenses %d", res, err, len(fx.expenses.created))
		}
	})

	t.Run("the date defaults to today", func(t *testing.T) {
		fx := newFixture()
		register(fx)
		res, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{ISRPaid: d("1")})
		if err != nil || res.Filing.Payment.Date != "2026-11-10" {
			t.Errorf("payment = %+v, %v", res.Filing.Payment, err)
		}
	})

	t.Run("records the expense when asked and links it", func(t *testing.T) {
		fx := newFixture()
		register(fx)
		res, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{Date: "2026-11-12", ISRPaid: d("1845.25"), IVAPaid: d("4827.59"), RecordExpense: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(fx.expenses.created) != 1 || !fx.expenses.created[0].Amount.Equal(d("6672.84")) || fx.expenses.created[0].Date != "2026-11-12" {
			t.Errorf("expenses = %+v", fx.expenses.created)
		}
		if res.Expense == nil || fx.repo.filings["2026-10"].ExpenseMovementID == nil || *fx.repo.filings["2026-10"].ExpenseMovementID != res.Expense.ID {
			t.Errorf("the stored filing does not link the expense: %+v", fx.repo.filings["2026-10"])
		}
	})

	t.Run("a failing expense stops the payment", func(t *testing.T) {
		fx := newFixture()
		register(fx)
		fx.expenses.createErr = ledger.ErrInvalid
		_, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{ISRPaid: d("10"), RecordExpense: true})
		if !errors.Is(err, ledger.ErrInvalid) || fx.repo.filings["2026-10"].Payment != nil {
			t.Errorf("err = %v, payment = %+v; want the filing to stay pending", err, fx.repo.filings["2026-10"].Payment)
		}
	})

	t.Run("a failing store removes the expense", func(t *testing.T) {
		fx := newFixture()
		register(fx)
		fx.repo.payErr = taxfiling.ErrAlreadyPaid
		_, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{ISRPaid: d("10"), RecordExpense: true})
		if !errors.Is(err, taxfiling.ErrAlreadyPaid) || len(fx.expenses.deleted) != 1 {
			t.Errorf("err = %v, deleted = %v", err, fx.expenses.deleted)
		}
	})

	t.Run("paying twice is rejected", func(t *testing.T) {
		fx := newFixture()
		register(fx)
		if _, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{ISRPaid: d("10")}); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{ISRPaid: d("10")}); !errors.Is(err, taxfiling.ErrAlreadyPaid) {
			t.Errorf("err = %v, want ErrAlreadyPaid", err)
		}
	})

	t.Run("unknown period and bad input", func(t *testing.T) {
		fx := newFixture()
		if _, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{}); !errors.Is(err, taxfiling.ErrNotFound) {
			t.Errorf("unknown period err = %v", err)
		}
		if _, err := fx.svc.Pay(ctx, "nope", app.PaymentInput{}); !errors.Is(err, taxfiling.ErrInvalidInput) {
			t.Errorf("bad period err = %v", err)
		}
		register(fx)
		if _, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{ISRPaid: d("-1")}); !errors.Is(err, taxfiling.ErrInvalidInput) {
			t.Errorf("negative err = %v", err)
		}
	})
}

func TestGetListsTheLinkedInvoices(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	fx.repo.filings["2026-10"] = taxfiling.Filing{Period: "2026-10", InvoiceIDs: []int{1, 2}}
	fx.invoices.rows[0].DeclarationPeriod = "2026-10"
	fx.invoices.rows[1].DeclarationPeriod = "2026-10"

	got, err := fx.svc.Get(ctx, "2026-10")
	if err != nil || len(got.Invoices) != 2 || got.Invoices[0].ID != 1 || got.Invoices[1].ID != 2 {
		t.Errorf("got %+v, %v; want invoices #1 and #2 oldest first", got.Invoices, err)
	}
	if _, err := fx.svc.Get(ctx, "2026-09"); !errors.Is(err, taxfiling.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if _, err := fx.svc.Get(ctx, "x"); !errors.Is(err, taxfiling.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestListFilters(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	paid := &taxfiling.Payment{Date: "2026-11-12"}
	fx.repo.filings["2025-12"] = taxfiling.Filing{Period: "2025-12", Payment: paid}
	fx.repo.filings["2026-09"] = taxfiling.Filing{Period: "2026-09"}
	fx.repo.filings["2026-10"] = taxfiling.Filing{Period: "2026-10", Payment: paid}

	periods := func(year int, status taxfiling.PaymentStatus) []string {
		list, err := fx.svc.List(ctx, year, status)
		if err != nil {
			t.Fatal(err)
		}
		out := []string{}
		for _, f := range list {
			out = append(out, f.Period)
		}
		return out
	}
	eq := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}
	if got := periods(0, ""); !eq(got, []string{"2026-10", "2026-09", "2025-12"}) {
		t.Errorf("all = %v", got)
	}
	if got := periods(2026, ""); !eq(got, []string{"2026-10", "2026-09"}) {
		t.Errorf("2026 = %v", got)
	}
	if got := periods(0, taxfiling.PaymentPending); !eq(got, []string{"2026-09"}) {
		t.Errorf("pending = %v", got)
	}
	if got := periods(2026, taxfiling.PaymentPaid); !eq(got, []string{"2026-10"}) {
		t.Errorf("2026 paid = %v", got)
	}
	if got := periods(2030, ""); len(got) != 0 {
		t.Errorf("2030 = %v, want none", got)
	}
	if _, err := fx.svc.List(ctx, 0, "ninguna"); !errors.Is(err, taxfiling.ErrInvalidInput) {
		t.Errorf("bad status err = %v", err)
	}
	if _, err := fx.svc.List(ctx, -1, ""); !errors.Is(err, taxfiling.ErrInvalidInput) {
		t.Errorf("bad year err = %v", err)
	}
}

func TestPending(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	got, err := fx.svc.Pending(ctx)
	if err != nil || len(got) != 2 || got[0].Period != "2026-10" || got[0].Overdue || got[1].Period != "2026-11" || got[1].Overdue {
		t.Fatalf("pending = %+v, %v; now is 2026-11-10 so 2026-10 (due 11-17) is not overdue", got, err)
	}
}

func TestPendingOverdueUsesToday(t *testing.T) {
	fx := newFixture()
	late := func() time.Time { return time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC) }
	svc := app.NewService(fx.repo, fx.invoices, fx.expenses, fx.settings, late, nil)
	got, err := svc.Pending(context.Background())
	if err != nil || !got[0].Overdue {
		t.Errorf("pending = %+v, %v; want 2026-10 overdue on 2026-11-18", got, err)
	}
}

func TestDelete(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	fx.repo.filings["2026-09"] = taxfiling.Filing{Period: "2026-09"}
	fx.repo.filings["2026-10"] = taxfiling.Filing{Period: "2026-10", Payment: &taxfiling.Payment{}}

	if err := fx.svc.Delete(ctx, "2026-09"); err != nil {
		t.Errorf("deleting a pending filing: %v", err)
	}
	if err := fx.svc.Delete(ctx, "2026-10"); !errors.Is(err, taxfiling.ErrFilingPaid) {
		t.Errorf("deleting a paid filing err = %v, want ErrFilingPaid", err)
	}
	if err := fx.svc.Delete(ctx, "2026-08"); !errors.Is(err, taxfiling.ErrNotFound) {
		t.Errorf("deleting an unknown filing err = %v", err)
	}
	if err := fx.svc.Delete(ctx, "x"); !errors.Is(err, taxfiling.ErrInvalidInput) {
		t.Errorf("bad period err = %v", err)
	}
}

func TestMonthStatus(t *testing.T) {
	fx := newFixture()
	ctx := context.Background()
	got, err := fx.svc.MonthStatus(ctx, "2026-11")
	if err != nil || got.Payment != taxfiling.PaymentNone || got.PreviousPeriod != "2026-10" || !got.PreviousPending {
		t.Errorf("status = %+v, %v", got, err)
	}
	fx.repo.filings["2026-10"] = taxfiling.Filing{Period: "2026-10", Payment: &taxfiling.Payment{}}
	got, _ = fx.svc.MonthStatus(ctx, "2026-11")
	if got.PreviousPending {
		t.Errorf("a paid previous period is not pending: %+v", got)
	}
	got, _ = fx.svc.MonthStatus(ctx, "2026-10")
	if got.Payment != taxfiling.PaymentPaid {
		t.Errorf("status = %+v, want paid", got)
	}
	if _, err := fx.svc.MonthStatus(ctx, "x"); !errors.Is(err, taxfiling.ErrInvalidInput) {
		t.Errorf("err = %v", err)
	}
}
