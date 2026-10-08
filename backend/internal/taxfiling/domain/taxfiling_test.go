package domain_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
	taxfiling "github.com/valium69mg/finances-app/backend/internal/taxfiling/domain"
)

var d = settingstest.D

func ptr(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

// octoberInvoices are the real October numbers, rounded to cents as stored:
// client A's salary (3,500 USD * 17.74 = 62,090) and client B's 35,000 deposit
// (IVA included: subtotal 30,172.41 and IVA 4,827.59), both issued, plus a
// prepared invoice, a cancelled one and one of another period that must not count.
func octoberInvoices() []invoices.Invoice {
	usa := invoices.ComputeUSAInvoice(d("3500"), d("17.74")).Rounded()
	b := clientAmounts("35000")
	return []invoices.Invoice{
		{ID: 1, ClientID: "usa", Period: "2026-10", Currency: "USD", ExchangeRate: ptr("17.74"), Status: invoices.StatusIssued, Amounts: usa},
		{ID: 2, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusIssued, Amounts: b},
		{ID: 3, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusCancelled, Amounts: clientAmounts("999")},
		{ID: 4, ClientID: "usa", Period: "2026-11", Currency: "USD", ExchangeRate: ptr("17.74"), Status: invoices.StatusIssued, Amounts: usa},
		{ID: 5, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusPrepared, Amounts: b},
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
	// Only the issued invoices count; the prepared one is reported apart.
	if len(got.Invoices) != 2 || got.Invoices[0].ID != 1 || got.Invoices[1].ID != 2 {
		t.Fatalf("included invoices = %+v, want #1 and #2", got.Invoices)
	}
	if len(got.PreparedIDs) != 1 || got.PreparedIDs[0] != 5 {
		t.Errorf("PreparedIDs = %v, want [5]", got.PreparedIDs)
	}
	// Income 62,090 + 30,172.41 = 92,262.41 -> 2% bracket; 2% is 1,845.2482 -> 1,845.25.
	checks := []struct{ name, got, want string }{
		{"income", got.IncomeCollected.String(), "92262.41"},
		{"rate", got.ISRRate.String(), "0.02"},
		{"accrued", got.ISRAccrued.String(), "1845.25"},
		{"withheld", got.ISRWithheld.String(), "0"},
		{"toPay", got.ISRToPay.String(), "1845.25"},
		{"ivaTransferred", got.IVATransferred.String(), "4827.59"},
		{"ivaPayable", got.IVAPayable.String(), "4827.59"},
		{"totalToPay", got.TotalToPay().String(), "6672.84"},
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
	one := func(subtotal string) []invoices.Invoice {
		return []invoices.Invoice{{
			ID: 1, ClientID: "b", Period: "2026-10", Currency: "MXN", Status: invoices.StatusIssued,
			Amounts: invoices.Amounts{SubtotalMXN: d(subtotal)},
		}}
	}

	t.Run("no income uses the first bracket", func(t *testing.T) {
		got, err := taxfiling.ComputeDeclaration(cfg, nil, "2026-10", d("0"))
		if err != nil || !got.ISRRate.Equal(d("0.01")) || !got.ISRToPay.IsZero() || !got.IncomeCollected.IsZero() {
			t.Errorf("got %+v, %v", got, err)
		}
		if len(got.Invoices) != 0 || len(got.PreparedIDs) != 0 {
			t.Errorf("no invoices expected, got %+v", got)
		}
	})

	t.Run("only prepared invoices leave an empty declaration", func(t *testing.T) {
		inv := one("10000")
		inv[0].Status = invoices.StatusPrepared
		got, err := taxfiling.ComputeDeclaration(cfg, inv, "2026-10", d("0"))
		if err != nil || !got.IncomeCollected.IsZero() || len(got.PreparedIDs) != 1 {
			t.Errorf("got %+v, %v", got, err)
		}
	})

	t.Run("the bracket upper bound is inclusive", func(t *testing.T) {
		at, _ := taxfiling.ComputeDeclaration(cfg, one("25000"), "2026-10", d("0"))
		over, _ := taxfiling.ComputeDeclaration(cfg, one("25000.01"), "2026-10", d("0"))
		if !at.ISRRate.Equal(d("0.01")) || !over.ISRRate.Equal(d("0.011")) {
			t.Errorf("rates = %s and %s, want 0.01 and 0.011", at.ISRRate, over.ISRRate)
		}
	})

	t.Run("the accrued ISR rounds half away from zero to cents", func(t *testing.T) {
		got, err := taxfiling.ComputeDeclaration(cfg, one("10000.50"), "2026-10", d("0"))
		// 1% of 10,000.50 is 100.005.
		if err != nil || !got.ISRAccrued.Equal(d("100.01")) {
			t.Errorf("accrued = %s, %v; want 100.01", got.ISRAccrued, err)
		}
	})

	t.Run("withheld ISR above the accrued ISR pays zero", func(t *testing.T) {
		inv := one("10000")
		inv[0].ISRWithheld = d("2000")
		got, err := taxfiling.ComputeDeclaration(cfg, inv, "2026-10", d("0"))
		// 10,000 at 1% = 100 accrued, 2,000 withheld.
		if err != nil || !got.ISRAccrued.Equal(d("100")) || !got.ISRToPay.IsZero() || !got.ISRWithheld.Equal(d("2000")) {
			t.Errorf("got %+v, %v", got, err)
		}
	})

	t.Run("IVA in favor is not clamped and does not reduce the total", func(t *testing.T) {
		got, err := taxfiling.ComputeDeclaration(cfg, octoberInvoices(), "2026-10", d("5000"))
		if err != nil || !got.IVAPayable.Equal(d("-172.41")) || !got.IVACreditable.Equal(d("5000")) {
			t.Fatalf("IVAPayable = %s, %v; want -172.41", got.IVAPayable, err)
		}
		if !got.TotalToPay().Equal(got.ISRToPay) {
			t.Errorf("TotalToPay = %s, want the ISR only (%s)", got.TotalToPay(), got.ISRToPay)
		}
	})

	t.Run("USD invoice IVA is converted with the exchange rate", func(t *testing.T) {
		inv := invoices.Invoice{
			ID: 1, ClientID: "usa", Period: "2026-10", Currency: "USD", ExchangeRate: ptr("17.74"), Status: invoices.StatusIssued,
			Amounts: invoices.Amounts{SubtotalMXN: d("1774"), IVA: d("10"), ISRWithheld: d("1"), IVAWithheld: d("2")},
		}
		got, err := taxfiling.ComputeDeclaration(cfg, []invoices.Invoice{inv}, "2026-10", d("0"))
		if err != nil || !got.IVATransferred.Equal(d("177.4")) || !got.ISRWithheld.Equal(d("17.74")) ||
			!got.IVAWithheld.Equal(d("35.48")) || !got.IncomeCollected.Equal(d("1774")) {
			t.Errorf("got %+v, %v", got, err)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		for _, tc := range []struct {
			period, creditable string
		}{{"2026-13", "0"}, {"202610", "0"}, {"", "0"}, {"2026-10", "-1"}, {"2026-10", "1000000000000"}} {
			if _, err := taxfiling.ComputeDeclaration(cfg, nil, tc.period, d(tc.creditable)); !errors.Is(err, taxfiling.ErrInvalidInput) {
				t.Errorf("ComputeDeclaration(%q, %s) err = %v, want ErrInvalidInput", tc.period, tc.creditable, err)
			}
		}
	})

	t.Run("missing brackets", func(t *testing.T) {
		bad := cfg
		bad.Brackets = nil
		if _, err := taxfiling.ComputeDeclaration(bad, octoberInvoices(), "2026-10", d("0")); err == nil {
			t.Error("expected a missing configuration error")
		}
	})
}

func TestPendingPeriods(t *testing.T) {
	all := octoberInvoices() // issued in 2026-10 and 2026-11, prepared and cancelled ones do not count
	all = append(all,
		invoices.Invoice{ID: 6, Period: "2026-08", Status: invoices.StatusCancelled},
		invoices.Invoice{ID: 7, Period: "2026-09", Status: invoices.StatusPrepared},
	)

	t.Run("lists unfiled periods with issued invoices", func(t *testing.T) {
		got, err := taxfiling.PendingPeriods(all, nil, "2026-11-18")
		if err != nil || len(got) != 2 {
			t.Fatalf("got %+v, %v", got, err)
		}
		if got[0].Period != "2026-10" || got[0].DueDate != "2026-11-17" || !got[0].Overdue {
			t.Errorf("first = %+v, want 2026-10 due 2026-11-17 overdue", got[0])
		}
		if got[1].Period != "2026-11" || got[1].DueDate != "2026-12-17" || got[1].Overdue {
			t.Errorf("second = %+v, want 2026-11 due 2026-12-17 not overdue", got[1])
		}
	})

	t.Run("the due date itself is not overdue", func(t *testing.T) {
		got, _ := taxfiling.PendingPeriods(all, nil, "2026-11-17")
		if got[0].Overdue {
			t.Errorf("%+v should not be overdue on its due date", got[0])
		}
	})

	t.Run("filed periods are removed", func(t *testing.T) {
		got, err := taxfiling.PendingPeriods(all, []taxfiling.Filing{{Period: "2026-10"}}, "2026-11-18")
		if err != nil || len(got) != 1 || got[0].Period != "2026-11" {
			t.Errorf("got %+v, %v", got, err)
		}
	})

	t.Run("nothing pending is an empty list", func(t *testing.T) {
		got, err := taxfiling.PendingPeriods(nil, nil, "2026-11-18")
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("got %#v, %v; want an empty non-nil list", got, err)
		}
	})
}

func TestUnfiledInvoices(t *testing.T) {
	all := []invoices.Invoice{
		{ID: 1, Period: "2026-10", Status: invoices.StatusIssued, DeclarationPeriod: "2026-10"}, // linked to its filing
		{ID: 2, Period: "2026-10", Status: invoices.StatusIssued},                               // issued after the filing
		{ID: 3, Period: "2026-10", Status: invoices.StatusCancelled},                            // cancelled
		{ID: 4, Period: "2026-10", Status: invoices.StatusPrepared},                             // not issued yet
		{ID: 5, Period: "2026-11", Status: invoices.StatusIssued},                               // period not filed: pending, not late
		{ID: 6, Period: "2026-09", Status: invoices.StatusIssued},
		{ID: 7, Period: "2026-10", Status: invoices.StatusIssued},
	}
	filings := []taxfiling.Filing{{Period: "2026-10"}, {Period: "2026-09"}}
	got := taxfiling.UnfiledInvoices(all, filings)
	ids := make([]int, len(got))
	for i, inv := range got {
		ids[i] = inv.ID
	}
	if len(ids) != 3 || ids[0] != 6 || ids[1] != 2 || ids[2] != 7 {
		t.Errorf("ids = %v, want [6 2 7] (by period then id)", ids)
	}
	if got := taxfiling.UnfiledInvoices(all, nil); len(got) != 0 {
		t.Errorf("without filings nothing is late: %+v", got)
	}
}

func TestNewFiling(t *testing.T) {
	cfg := settingstest.RealConfig()
	in := taxfiling.FilingInput{Period: "2026-10", Date: "2026-11-10", Folio: "  ACUSE-123 "}

	t.Run("registers a pending filing from the computed declaration", func(t *testing.T) {
		f, err := taxfiling.NewFiling(cfg, octoberInvoices(), nil, in)
		if err != nil {
			t.Fatal(err)
		}
		if f.Period != "2026-10" || f.FilingDate != "2026-11-10" || f.Folio != "ACUSE-123" {
			t.Errorf("filing = %+v", f)
		}
		if !f.IncomeCollected.Equal(d("92262.41")) || !f.ISRRate.Equal(d("0.02")) || !f.ISRAccrued.Equal(d("1845.25")) ||
			!f.ISRDue.Equal(d("1845.25")) || !f.IVATransferred.Equal(d("4827.59")) || !f.IVADue.Equal(d("4827.59")) {
			t.Errorf("amounts = %+v", f)
		}
		if len(f.InvoiceIDs) != 2 || f.InvoiceIDs[0] != 1 || f.InvoiceIDs[1] != 2 {
			t.Errorf("InvoiceIDs = %v, want [1 2]", f.InvoiceIDs)
		}
		if f.Payment != nil || f.PaymentStatus() != taxfiling.PaymentPending {
			t.Errorf("payment = %+v, status %s; want pending", f.Payment, f.PaymentStatus())
		}
		if !f.TotalToPay().Equal(d("6672.84")) {
			t.Errorf("TotalToPay = %s", f.TotalToPay())
		}
	})

	t.Run("stores the creditable IVA that was used", func(t *testing.T) {
		withCredit := in
		withCredit.IVACreditable = d("800.005")
		f, err := taxfiling.NewFiling(cfg, octoberInvoices(), nil, withCredit)
		if err != nil || !f.IVACreditable.Equal(d("800.01")) || !f.IVADue.Equal(d("4027.58")) {
			t.Errorf("filing = %+v, %v; want creditable 800.01 and IVA due 4027.58", f, err)
		}
	})

	t.Run("registers a paid filing", func(t *testing.T) {
		paid := in
		paid.Payment = &taxfiling.PaymentInput{Date: "2026-11-12", ISRPaid: d("1845.25"), IVAPaid: d("4827.59")}
		f, err := taxfiling.NewFiling(cfg, octoberInvoices(), nil, paid)
		if err != nil || f.PaymentStatus() != taxfiling.PaymentPaid || !f.Payment.Total().Equal(d("6672.84")) || f.Payment.Date != "2026-11-12" {
			t.Errorf("filing = %+v, %v", f, err)
		}
	})

	t.Run("a period without invoices is filed with zeros", func(t *testing.T) {
		empty := in
		empty.Period = "2026-05"
		f, err := taxfiling.NewFiling(cfg, octoberInvoices(), nil, empty)
		if err != nil || !f.IncomeCollected.IsZero() || len(f.InvoiceIDs) != 0 || !f.TotalToPay().IsZero() {
			t.Errorf("filing = %+v, %v", f, err)
		}
	})

	t.Run("one filing per period", func(t *testing.T) {
		_, err := taxfiling.NewFiling(cfg, octoberInvoices(), []taxfiling.Filing{{Period: "2026-10"}}, in)
		if !errors.Is(err, taxfiling.ErrAlreadyFiled) {
			t.Errorf("err = %v, want ErrAlreadyFiled", err)
		}
	})

	t.Run("invalid input", func(t *testing.T) {
		bad := []taxfiling.FilingInput{
			{Period: "2026-13", Date: "2026-11-10"},
			{Period: "2026-10", Date: "10/11/2026"},
			{Period: "2026-10", Date: "2026-11-10", Folio: string(make([]byte, taxfiling.MaxFolioLength+1))},
			{Period: "2026-10", Date: "2026-11-10", IVACreditable: d("-1")},
			{Period: "2026-10", Date: "2026-11-10", Payment: &taxfiling.PaymentInput{Date: "2026-11-10", ISRPaid: d("-1")}},
			{Period: "2026-10", Date: "2026-11-10", Payment: &taxfiling.PaymentInput{Date: "nope"}},
		}
		for i, b := range bad {
			if _, err := taxfiling.NewFiling(cfg, octoberInvoices(), nil, b); !errors.Is(err, taxfiling.ErrInvalidInput) {
				t.Errorf("case %d: err = %v, want ErrInvalidInput", i, err)
			}
		}
	})
}

func TestPay(t *testing.T) {
	pending := taxfiling.Filing{Period: "2026-10", ISRDue: d("100"), IVADue: d("50")}

	t.Run("records the payment", func(t *testing.T) {
		got, err := pending.Pay(taxfiling.PaymentInput{Date: "2026-11-12", ISRPaid: d("100.004"), IVAPaid: d("50")})
		if err != nil || got.PaymentStatus() != taxfiling.PaymentPaid {
			t.Fatalf("got %+v, %v", got, err)
		}
		if !got.Payment.ISRPaid.Equal(d("100")) || !got.Payment.Total().Equal(d("150")) {
			t.Errorf("payment = %+v", got.Payment)
		}
		if pending.Payment != nil {
			t.Error("Pay must not mutate the receiver")
		}
	})

	t.Run("zero payments are valid", func(t *testing.T) {
		if _, err := pending.Pay(taxfiling.PaymentInput{Date: "2026-11-12"}); err != nil {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("a paid filing cannot be paid again", func(t *testing.T) {
		paid, _ := pending.Pay(taxfiling.PaymentInput{Date: "2026-11-12"})
		if _, err := paid.Pay(taxfiling.PaymentInput{Date: "2026-11-13"}); !errors.Is(err, taxfiling.ErrAlreadyPaid) {
			t.Errorf("err = %v, want ErrAlreadyPaid", err)
		}
	})

	t.Run("invalid payments", func(t *testing.T) {
		for _, in := range []taxfiling.PaymentInput{
			{Date: "2026-13-01"},
			{Date: "2026-11-12", IVAPaid: d("-0.01")},
			{Date: "2026-11-12", ISRPaid: d("1000000000000")},
		} {
			if _, err := pending.Pay(in); !errors.Is(err, taxfiling.ErrInvalidInput) {
				t.Errorf("Pay(%+v) err = %v, want ErrInvalidInput", in, err)
			}
		}
	})
}

func TestStatusOf(t *testing.T) {
	all := octoberInvoices()
	paid := taxfiling.Filing{Period: "2026-10", Payment: &taxfiling.Payment{Date: "2026-11-12"}}
	pending := taxfiling.Filing{Period: "2026-10"}

	tests := []struct {
		name        string
		filings     []taxfiling.Filing
		month       string
		wantPayment taxfiling.PaymentStatus
		wantPrev    string
		wantPending bool
	}{
		{"no filing, previous period has issued invoices", nil, "2026-11", taxfiling.PaymentNone, "2026-10", true},
		{"previous period filed and paid", []taxfiling.Filing{paid}, "2026-11", taxfiling.PaymentNone, "2026-10", false},
		{"previous period filed with payment pending", []taxfiling.Filing{pending}, "2026-11", taxfiling.PaymentNone, "2026-10", true},
		{"month with a pending filing", []taxfiling.Filing{pending}, "2026-10", taxfiling.PaymentPending, "2026-09", false},
		{"month with a paid filing", []taxfiling.Filing{paid}, "2026-10", taxfiling.PaymentPaid, "2026-09", false},
		{"previous period without issued invoices", nil, "2026-12", taxfiling.PaymentNone, "2026-11", true},
		{"previous period only has prepared invoices", nil, "2026-10", taxfiling.PaymentNone, "2026-09", false},
		{"January looks at the previous December", nil, "2027-01", taxfiling.PaymentNone, "2026-12", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := taxfiling.StatusOf(all, tc.filings, tc.month)
			if err != nil {
				t.Fatal(err)
			}
			if got.Payment != tc.wantPayment || got.PreviousPeriod != tc.wantPrev || got.PreviousPending != tc.wantPending {
				t.Errorf("StatusOf = %+v, want payment %s previous %s pending %v", got, tc.wantPayment, tc.wantPrev, tc.wantPending)
			}
		})
	}
	if _, err := taxfiling.StatusOf(all, nil, "2026-13"); !errors.Is(err, taxfiling.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

// User decimals are range-checked before any comparison or rounding: a huge
// exponent must be a plain invalid-input error, not an allocation blow-up.
func TestUserAmountsAreRangeChecked(t *testing.T) {
	cfg := settingstest.RealConfig()
	bad := []string{"1e2000000000", "1e999999999", "10000000000000", "-1", "1e-2000000000"}

	for _, raw := range bad {
		if _, err := taxfiling.ComputeDeclaration(cfg, octoberInvoices(), "2026-10", d(raw)); !errors.Is(err, taxfiling.ErrInvalidInput) {
			t.Errorf("ComputeDeclaration iva_acreditable %s: err = %v, want ErrInvalidInput", raw, err)
		}
		in := taxfiling.FilingInput{Period: "2026-10", Date: "2026-11-05", IVACreditable: d(raw)}
		if _, err := taxfiling.NewFiling(cfg, octoberInvoices(), nil, in); !errors.Is(err, taxfiling.ErrInvalidInput) {
			t.Errorf("NewFiling iva_acreditable %s: err = %v, want ErrInvalidInput", raw, err)
		}
		for name, p := range map[string]taxfiling.PaymentInput{
			"isr": {Date: "2026-11-07", ISRPaid: d(raw)},
			"iva": {Date: "2026-11-07", IVAPaid: d(raw)},
		} {
			if _, err := taxfiling.NewPayment(p); !errors.Is(err, taxfiling.ErrInvalidInput) {
				t.Errorf("NewPayment %s_paid %s: err = %v, want ErrInvalidInput", name, raw, err)
			}
		}
	}

	// Zero is a valid creditable IVA and a valid (empty) payment.
	if _, err := taxfiling.ComputeDeclaration(cfg, octoberInvoices(), "2026-10", d("0")); err != nil {
		t.Errorf("zero creditable IVA: %v", err)
	}
	if _, err := taxfiling.NewPayment(taxfiling.PaymentInput{Date: "2026-11-07"}); err != nil {
		t.Errorf("zero payment: %v", err)
	}
}

func TestPaymentStatusIsValid(t *testing.T) {
	if !taxfiling.PaymentPending.IsValid() || !taxfiling.PaymentPaid.IsValid() || taxfiling.PaymentNone.IsValid() || taxfiling.PaymentStatus("x").IsValid() {
		t.Error("only pendiente and pagada are valid filing statuses")
	}
}

// clientAmounts computes the amounts of a client that takes 16% IVA with no retentions.
func clientAmounts(net string) invoices.Amounts {
	a, _ := invoices.ComputeClientInvoice(d(net), d("0.16"), d("0"), d("0"))
	return a
}
