package domain_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/shopspring/decimal"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

var d = settingstest.D

func ptr(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

// octoberInvoices are the real October numbers: client A's salary
// (3,500 USD * 17.74 = 62,090) and client B's 35,000 deposit (IVA included).
func octoberInvoices() []invoices.Invoice {
	usa := invoices.ComputeUSAInvoice(d("3500"), d("17.74"))
	b := invoices.ComputeClientBInvoice(d("35000"), d("0.16"))
	return []invoices.Invoice{
		{ID: 1, ClientID: "usa", Period: "2026-10", Currency: "USD", ExchangeRate: ptr("17.74"), Status: invoices.StatusIssued, Amounts: usa},
		{ID: 2, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusPrepared, Amounts: b},
		{ID: 3, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusCancelled, Amounts: invoices.ComputeClientBInvoice(d("999"), d("0.16"))},
		{ID: 4, ClientID: "usa", Period: "2026-11", Currency: "USD", ExchangeRate: ptr("17.74"), Status: invoices.StatusIssued, Amounts: usa},
	}
}

func TestDueDate(t *testing.T) {
	tests := []struct{ period, want string }{
		{"2026-10", "2026-11-17"},
		{"2026-12", "2027-01-17"}, // December rolls to January
		{"2027-01", "2027-02-17"},
	}
	for _, tc := range tests {
		if got, err := taxfiling.DueDate(tc.period); err != nil || got != tc.want {
			t.Errorf("DueDate(%s) = %q, %v; want %q", tc.period, got, err, tc.want)
		}
	}
	if _, err := taxfiling.DueDate("2026-13"); err == nil {
		t.Error("DueDate(2026-13) should fail")
	}
}

func TestComputeDeclaration(t *testing.T) {
	cfg := settingstest.RealConfig()
	got, err := taxfiling.ComputeDeclaration(cfg, octoberInvoices(), "2026-10", d("0"))
	if err != nil {
		t.Fatal(err)
	}
	// Cancelled and other-period invoices are excluded.
	if len(got.Invoices) != 2 {
		t.Fatalf("included invoices = %d, want 2", len(got.Invoices))
	}
	// Income 62,090 + 30,172.4137931034 = 92,262.4137931034 -> 2% bracket.
	checks := []struct{ name, got, want string }{
		{"income", got.IncomeCollected.String(), "92262.4137931034"},
		{"rate", got.ISRRate.String(), "0.02"},
		{"accrued", got.ISRAccrued.String(), "1845.248275862068"},
		{"withheld", got.ISRWithheld.String(), "0"},
		{"toPay", got.ISRToPay.String(), "1845.248275862068"},
		{"ivaTransferred", got.IVATransferred.String(), "4827.5862068966"},
		{"ivaPayable", got.IVAPayable.String(), "4827.5862068966"},
		{"exportBase", got.ExportBase.String(), "62090"},
	}
	for _, c := range checks {
		if !d(c.got).Equal(d(c.want)) {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
	}
	if got.DueDate != "2026-11-17" {
		t.Errorf("DueDate = %s", got.DueDate)
	}
}

func TestComputeDeclarationEdgeCases(t *testing.T) {
	cfg := settingstest.RealConfig()

	t.Run("no income uses the first bracket", func(t *testing.T) {
		got, err := taxfiling.ComputeDeclaration(cfg, nil, "2026-10", d("0"))
		if err != nil || !got.ISRRate.Equal(d("0.01")) || !got.ISRToPay.IsZero() || !got.IncomeCollected.IsZero() {
			t.Errorf("got %+v, %v", got, err)
		}
	})

	t.Run("withheld ISR above the accrued ISR pays zero", func(t *testing.T) {
		inv := invoices.Invoice{
			ID: 1, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusIssued,
			Amounts: invoices.Amounts{SubtotalMXN: d("10000"), ISRWithheld: d("2000")},
		}
		got, err := taxfiling.ComputeDeclaration(cfg, []invoices.Invoice{inv}, "2026-10", d("0"))
		// 10,000 at 1% = 100 accrued, 2,000 withheld.
		if err != nil || !got.ISRAccrued.Equal(d("100")) || !got.ISRToPay.IsZero() || !got.ISRWithheld.Equal(d("2000")) {
			t.Errorf("got %+v, %v", got, err)
		}
	})

	t.Run("IVA in favor is not clamped", func(t *testing.T) {
		got, err := taxfiling.ComputeDeclaration(cfg, octoberInvoices(), "2026-10", d("5000"))
		if err != nil || !got.IVAPayable.Equal(d("-172.4137931034")) {
			t.Errorf("IVAPayable = %s, %v; want -172.4137931034", got.IVAPayable, err)
		}
	})

	t.Run("USD invoice IVA is converted with the exchange rate", func(t *testing.T) {
		inv := invoices.Invoice{
			ID: 1, ClientID: "usa", Period: "2026-10", Currency: "USD", ExchangeRate: ptr("17.74"), Status: invoices.StatusIssued,
			Amounts: invoices.Amounts{SubtotalMXN: d("1774"), IVA: d("10")},
		}
		got, err := taxfiling.ComputeDeclaration(cfg, []invoices.Invoice{inv}, "2026-10", d("0"))
		if err != nil || !got.IVATransferred.Equal(d("177.4")) || !got.IncomeCollected.Equal(d("1774")) {
			t.Errorf("got %+v, %v", got, err)
		}
	})
}

func TestAOnlyISR(t *testing.T) {
	cfg := settingstest.RealConfig()
	// Only client A: 62,090 at 1.5% = 931.35, even though client B pushes the month to 2%.
	got, err := taxfiling.AOnlyISR(cfg, octoberInvoices(), "2026-10")
	if err != nil || !got.Equal(d("931.35")) {
		t.Errorf("AOnlyISR = %s, %v; want 931.35", got, err)
	}
	if got, err := taxfiling.AOnlyISR(cfg, octoberInvoices(), "2026-09"); err != nil || !got.IsZero() {
		t.Errorf("AOnlyISR without invoices = %s, %v; want 0", got, err)
	}
}

func TestPendingPeriods(t *testing.T) {
	all := []invoices.Invoice{
		{ID: 1, Period: "2026-12", Status: invoices.StatusIssued},
		{ID: 2, Period: "2026-09", Status: invoices.StatusIssued},
		{ID: 3, Period: "2026-10", Status: invoices.StatusPrepared},
		{ID: 4, Period: "2026-10", Status: invoices.StatusIssued},
		{ID: 5, Period: "2026-11", Status: invoices.StatusCancelled}, // never pending
	}
	filings := []taxfiling.Filing{{Period: "2026-09"}}

	got, err := taxfiling.PendingPeriods(all, filings, "2026-11-20")
	if err != nil {
		t.Fatal(err)
	}
	want := []taxfiling.PendingPeriod{
		{Period: "2026-10", DueDate: "2026-11-17", Overdue: true},
		{Period: "2026-12", DueDate: "2027-01-17", Overdue: false},
	}
	if len(got) != len(want) {
		t.Fatalf("pending = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pending[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// On the due date itself the period is not yet overdue.
	got, _ = taxfiling.PendingPeriods(all[2:3], nil, "2026-11-17")
	if len(got) != 1 || got[0].Overdue {
		t.Errorf("due day = %+v, want not overdue", got)
	}
}

func reserve(amount string) ledger.Movement {
	return ledger.Movement{Date: "2026-10-05", Kind: ledger.KindSavings, Category: ledger.CategorySATReserve, AmountMXN: d(amount)}
}

func TestRegisterFiling(t *testing.T) {
	cfg := settingstest.RealConfig()
	in := taxfiling.FilingInput{Period: "2026-10", Date: "2026-11-15", ISRPaid: d("1845"), IVAPaid: d("4828"), Folio: "F-1"}

	tests := []struct {
		name                                                       string
		reserve                                                    []ledger.Movement
		withdrawal, budgetCovered, shortfall, remaining, movements string
	}{
		// Paid 6,673; client A alone owes 931.35, so 5,741.65 is client B's part.
		{"reserve covers client B's part", []ledger.Movement{reserve("5775")}, "5741.65", "931.35", "0", "33.35", "2"},
		{"reserve is insufficient", []ledger.Movement{reserve("1000")}, "1000", "5673", "4741.65", "0", "2"},
		{"empty reserve creates no withdrawal", nil, "0", "6673", "5741.65", "0", "1"},
		{"reserve net of earlier withdrawals", []ledger.Movement{reserve("5775"), reserve("-5000")}, "775", "5898", "4966.65", "0", "2"},
	}
	for _, tc := range tests {
		got, err := taxfiling.RegisterFiling(cfg, octoberInvoices(), tc.reserve, nil, in)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !got.TotalPaid.Equal(d("6673")) || !got.AOnlyEstimate.Equal(d("931.35")) || !got.BPortionNeeded.Equal(d("5741.65")) {
			t.Errorf("%s: totals = %+v", tc.name, got)
		}
		if !got.ReserveWithdrawal.Equal(d(tc.withdrawal)) || !got.BudgetCovered.Equal(d(tc.budgetCovered)) ||
			!got.Shortfall.Equal(d(tc.shortfall)) || !got.RemainingReserve.Equal(d(tc.remaining)) {
			t.Errorf("%s: withdrawal %s covered %s shortfall %s remaining %s", tc.name,
				got.ReserveWithdrawal, got.BudgetCovered, got.Shortfall, got.RemainingReserve)
		}
		if len(got.NewMovements) != wantMovements(tc.movements) {
			t.Errorf("%s: %d new movements, want %s", tc.name, len(got.NewMovements), tc.movements)
		}
	}
}

func wantMovements(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return n
}

func TestRegisterFilingRecords(t *testing.T) {
	cfg := settingstest.RealConfig()
	in := taxfiling.FilingInput{Period: "2026-10", Date: "2026-11-15", ISRPaid: d("1845"), IVAPaid: d("4828"), Folio: "F-1"}
	got, err := taxfiling.RegisterFiling(cfg, octoberInvoices(), []ledger.Movement{reserve("5775")}, nil, in)
	if err != nil {
		t.Fatal(err)
	}

	f := got.Filing
	if f.Period != "2026-10" || f.FilingDate != "2026-11-15" || f.Folio != "F-1" ||
		!f.ISRPaid.Equal(d("1845")) || !f.IVAPaid.Equal(d("4828")) || !f.IVACreditable.IsZero() ||
		!f.IncomeCollected.Equal(d("92262.4137931034")) || len(f.InvoiceIDs) != 2 || f.InvoiceIDs[0] != 1 || f.InvoiceIDs[1] != 2 {
		t.Errorf("filing = %+v", f)
	}

	expense, withdrawal := got.NewMovements[0], got.NewMovements[1]
	if expense.Kind != ledger.KindExpense || expense.Category != "Impuestos" || !expense.AmountMXN.Equal(d("6673")) ||
		expense.PaymentMethod != "Transferencia" || expense.Currency != "MXN" || expense.Description != "Pago SAT ISR+IVA periodo 2026-10" {
		t.Errorf("expense = %+v", expense)
	}
	if withdrawal.Kind != ledger.KindSavings || withdrawal.Category != "Reserva SAT" || !withdrawal.AmountMXN.Equal(d("-5741.65")) ||
		withdrawal.Description != "Uso de reserva SAT periodo 2026-10" {
		t.Errorf("withdrawal = %+v", withdrawal)
	}

	// Active invoices of the period record the declaration; cancelled and other periods do not.
	wantDeclared := []string{"2026-10", "2026-10", "", ""}
	for i, inv := range got.Invoices {
		if inv.DeclarationPeriod != wantDeclared[i] {
			t.Errorf("invoice %d declaration = %q, want %q", inv.ID, inv.DeclarationPeriod, wantDeclared[i])
		}
	}
}

func TestRegisterFilingGuards(t *testing.T) {
	cfg := settingstest.RealConfig()
	in := taxfiling.FilingInput{Period: "2026-10", Date: "2026-11-15", ISRPaid: d("1"), IVAPaid: d("1")}

	filings := []taxfiling.Filing{{Period: "2026-10"}}
	if _, err := taxfiling.RegisterFiling(cfg, octoberInvoices(), nil, filings, in); !errors.Is(err, taxfiling.ErrAlreadyFiled) {
		t.Errorf("duplicate filing error = %v, want ErrAlreadyFiled", err)
	}
	if _, ok := taxfiling.FindFiling(filings, "2026-09"); ok {
		t.Error("FindFiling(2026-09) should not be found")
	}

	in.Date = "15/11/2026"
	if _, err := taxfiling.RegisterFiling(cfg, octoberInvoices(), nil, nil, in); err == nil {
		t.Error("invalid date should fail")
	}
}

func TestRegisterFilingSmallPayment(t *testing.T) {
	cfg := settingstest.RealConfig()
	// Paying less than client A's own estimate leaves nothing for client B: no withdrawal.
	in := taxfiling.FilingInput{Period: "2026-10", Date: "2026-11-15", ISRPaid: d("500"), IVAPaid: d("0")}
	got, err := taxfiling.RegisterFiling(cfg, octoberInvoices(), []ledger.Movement{reserve("5775")}, nil, in)
	if err != nil {
		t.Fatal(err)
	}
	if !got.BPortionNeeded.IsZero() || !got.ReserveWithdrawal.IsZero() || len(got.NewMovements) != 1 ||
		!got.BudgetCovered.Equal(d("500")) || !got.RemainingReserve.Equal(d("5775")) {
		t.Errorf("got %+v", got)
	}
}
