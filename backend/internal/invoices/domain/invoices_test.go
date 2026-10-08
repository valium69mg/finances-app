package domain_test

import (
	"errors"
	"testing"

	"github.com/shopspring/decimal"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

var d = settingstest.D

func ptr(s string) *decimal.Decimal {
	v := d(s)
	return &v
}

func TestComputeUSAInvoice(t *testing.T) {
	a := invoices.ComputeUSAInvoice(d("3383.33"), d("17.74"))
	// 3,383.33 USD * 17.74 = 60,020.2742 MXN, no IVA and no retentions.
	if !a.Subtotal.Equal(d("3383.33")) || !a.SubtotalMXN.Equal(d("60020.2742")) ||
		!a.Total.Equal(d("3383.33")) || !a.ExpectedDepositMXN.Equal(d("60020.2742")) ||
		!a.IVA.IsZero() || !a.ISRWithheld.IsZero() || !a.IVAWithheld.IsZero() {
		t.Errorf("amounts = %+v", a)
	}
}

func compute(t *testing.T, net, iva, isr, retIVA string) invoices.Amounts {
	t.Helper()
	a, ok := invoices.ComputeClientInvoice(d(net), d(iva), d(isr), d(retIVA))
	if !ok {
		t.Fatalf("ComputeClientInvoice(%s, %s, %s, %s) rejected the rates", net, iva, isr, retIVA)
	}
	return a
}

func TestComputeClientInvoiceNoWithholdings(t *testing.T) {
	a := compute(t, "35000", "0.16", "0", "0")
	// 35,000 / 1.16 = 30,172.41; IVA 16% of that is 4,827.59 and they add up exactly.
	if !a.Subtotal.Equal(d("30172.41")) || !a.SubtotalMXN.Equal(d("30172.41")) || !a.IVA.Equal(d("4827.59")) ||
		!a.Total.Equal(d("35000")) || !a.ExpectedDepositMXN.Equal(d("35000")) ||
		!a.ISRWithheld.IsZero() || !a.IVAWithheld.IsZero() {
		t.Errorf("amounts = %+v", a)
	}
}

func TestComputeClientInvoiceWithWithholdings(t *testing.T) {
	// Client IBL: the stamped XML of a 7,318.18 net payment.
	a := compute(t, "7318.18", "0.16", "0.0125", "0.106667")
	if !a.Subtotal.Equal(d("7031.08")) || !a.SubtotalMXN.Equal(d("7031.08")) || !a.IVA.Equal(d("1124.97")) ||
		!a.IVAWithheld.Equal(d("749.98")) || !a.ISRWithheld.Equal(d("87.89")) ||
		!a.Total.Equal(d("7318.18")) || !a.ExpectedDepositMXN.Equal(d("7318.18")) {
		t.Errorf("amounts = %+v", a)
	}
	if got := a.Subtotal.Add(a.IVA).Sub(a.ISRWithheld).Sub(a.IVAWithheld); !got.Equal(a.Total) {
		t.Errorf("subtotal + IVA - withholdings = %s, want %s", got, a.Total)
	}
	if r := a.Rounded(); r != a {
		t.Errorf("Rounded changed exact amounts: %+v vs %+v", r, a)
	}
}

func TestComputeClientInvoiceAlwaysAddsUp(t *testing.T) {
	for _, net := range []string{"0.01", "1", "99.99", "1000", "1234.56", "7318.18", "50000.01", "123456.78"} {
		for _, rates := range [][3]string{{"0.16", "0", "0"}, {"0.16", "0.0125", "0.106667"}, {"0.16", "0.1", "0.106667"}, {"0", "0", "0"}, {"0.08", "0.0125", "0"}} {
			a := compute(t, net, rates[0], rates[1], rates[2])
			if got := a.Subtotal.Add(a.IVA).Sub(a.ISRWithheld).Sub(a.IVAWithheld); !got.Equal(a.Total) || !a.Total.Equal(d(net)) {
				t.Errorf("net %s rates %v: subtotal + IVA - withholdings = %s, total = %s", net, rates, got, a.Total)
			}
		}
	}
}

func TestComputeClientInvoiceZeroRates(t *testing.T) {
	a := compute(t, "1500.50", "0", "0", "0")
	if !a.Subtotal.Equal(d("1500.50")) || !a.IVA.IsZero() || !a.Total.Equal(d("1500.50")) {
		t.Errorf("amounts = %+v", a)
	}
}

func TestComputeClientInvoiceRejectsNoTaxableBase(t *testing.T) {
	if _, ok := invoices.ComputeClientInvoice(d("100"), d("0"), d("0.5"), d("0.5")); ok {
		t.Error("rates that cancel the base must be rejected")
	}
}

func TestRoundedKeepsTheInvariant(t *testing.T) {
	// A correctly computed IVA is not overwritten.
	a := invoices.Amounts{
		Subtotal: d("7031.08"), SubtotalMXN: d("7031.08"), IVA: d("1124.97"), ISRWithheld: d("87.89"),
		IVAWithheld: d("749.98"), Total: d("7318.18"), ExpectedDepositMXN: d("7318.18"),
	}
	if r := a.Rounded(); !r.IVA.Equal(d("1124.97")) {
		t.Errorf("IVA = %s, want 1124.97", r.IVA)
	}
	// An IVA that misses the invariant is the remainder.
	a.IVA = d("1124.50")
	if r := a.Rounded(); !r.IVA.Equal(d("1124.97")) {
		t.Errorf("corrected IVA = %s, want 1124.97", r.IVA)
	}
}

func TestPrepareDefaultsAndValidation(t *testing.T) {
	cfg := settingstest.RealConfig()

	t.Run("USA takes salary and fx defaults", func(t *testing.T) {
		got, err := invoices.Prepare(cfg, nil, invoices.PrepareInput{ClientID: "usa", Date: "2026-10-15"})
		if err != nil {
			t.Fatal(err)
		}
		inv := got.Invoice
		if !inv.Subtotal.Equal(d("3500")) || inv.ExchangeRate == nil || !inv.ExchangeRate.Equal(d("17.74")) ||
			!inv.SubtotalMXN.Equal(d("62090")) || inv.Currency != "USD" || inv.Period != "2026-10" ||
			inv.Status != invoices.StatusPrepared {
			t.Errorf("invoice = %+v", inv)
		}
	})

	t.Run("USA explicit values, zero rate falls back to default", func(t *testing.T) {
		got, err := invoices.Prepare(cfg, nil, invoices.PrepareInput{
			ClientID: "usa", Date: "2026-10-15", Subtotal: ptr("1000"), ExchangeRate: ptr("0"),
		})
		if err != nil || !got.Invoice.SubtotalMXN.Equal(d("17740")) {
			t.Errorf("got %+v, %v", got.Invoice, err)
		}
	})

	t.Run("client B computes IVA from the client rate", func(t *testing.T) {
		got, err := invoices.Prepare(cfg, nil, invoices.PrepareInput{ClientID: "b", Date: "2026-10-31", Amount: ptr("35000")})
		if err != nil {
			t.Fatal(err)
		}
		inv := got.Invoice
		if inv.Currency != "MXN" || inv.ExchangeRate != nil || !inv.IVA.Equal(d("4827.59")) {
			t.Errorf("invoice = %+v", inv)
		}
	})

	t.Run("a client that withholds takes the net amount", func(t *testing.T) {
		withholding := cfg
		withholding.Clients = append([]settings.Client(nil), cfg.Clients...)
		withholding.Clients = append(withholding.Clients, settings.Client{
			ID: "ibl", Name: "IBL", Currency: "MXN", IVARate: d("0.16"), RetISRRate: d("0.0125"), RetIVARate: d("0.106667"),
		})
		got, err := invoices.Prepare(withholding, nil, invoices.PrepareInput{ClientID: "ibl", Date: "2026-10-31", Amount: ptr("7318.18")})
		if err != nil {
			t.Fatal(err)
		}
		inv := got.Invoice
		if !inv.Subtotal.Equal(d("7031.08")) || !inv.IVA.Equal(d("1124.97")) || !inv.ISRWithheld.Equal(d("87.89")) ||
			!inv.IVAWithheld.Equal(d("749.98")) || !inv.Total.Equal(d("7318.18")) || !inv.ExpectedDepositMXN.Equal(d("7318.18")) {
			t.Errorf("invoice = %+v", inv)
		}
	})

	failures := []struct {
		name string
		in   invoices.PrepareInput
		want error
	}{
		{"unknown client", invoices.PrepareInput{ClientID: "zzz", Date: "2026-10-15"}, invoices.ErrUnknownClient},
		{"invalid date", invoices.PrepareInput{ClientID: "usa", Date: "2026-13-01"}, invoices.ErrInvalidInput},
		{"USA rejects an amount", invoices.PrepareInput{ClientID: "usa", Date: "2026-10-15", Amount: ptr("100")}, invoices.ErrInvalidInput},
		{"B rejects a subtotal", invoices.PrepareInput{ClientID: "b", Date: "2026-10-15", Subtotal: ptr("100")}, invoices.ErrInvalidInput},
		{"B requires an amount", invoices.PrepareInput{ClientID: "b", Date: "2026-10-15"}, invoices.ErrInvalidInput},
		{"B amount 1e2000000000", invoices.PrepareInput{ClientID: "b", Date: "2026-10-15", Amount: ptr("1e2000000000")}, invoices.ErrInvalidInput},
		{"B amount 1e999999999", invoices.PrepareInput{ClientID: "b", Date: "2026-10-15", Amount: ptr("1e999999999")}, invoices.ErrInvalidInput},
		{"B amount above the maximum", invoices.PrepareInput{ClientID: "b", Date: "2026-10-15", Amount: ptr("10000000000000")}, invoices.ErrInvalidInput},
		{"USA subtotal 1e2000000000", invoices.PrepareInput{ClientID: "usa", Date: "2026-10-15", Subtotal: ptr("1e2000000000")}, invoices.ErrInvalidInput},
		{"USA rate 1e2000000000", invoices.PrepareInput{ClientID: "usa", Date: "2026-10-15", ExchangeRate: ptr("1e2000000000")}, invoices.ErrInvalidInput},
		{"USA negative subtotal", invoices.PrepareInput{ClientID: "usa", Date: "2026-10-15", Subtotal: ptr("-1")}, invoices.ErrInvalidInput},
	}
	for _, tc := range failures {
		if _, err := invoices.Prepare(cfg, nil, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}

	noDefaults := cfg
	noDefaults.SalaryUSD = nil
	if _, err := invoices.Prepare(noDefaults, nil, invoices.PrepareInput{ClientID: "usa", Date: "2026-10-15"}); !errors.Is(err, settings.ErrMissingConfig) {
		t.Errorf("missing salary_usd error = %v", err)
	}
}

func TestPrepareReportsDuplicates(t *testing.T) {
	cfg := settingstest.RealConfig()
	existing := []invoices.Invoice{
		{ID: 1, ClientID: "b", CollectionDate: "2026-10-31", Status: invoices.StatusPrepared},
		{ID: 2, ClientID: "b", CollectionDate: "2026-10-31", Status: invoices.StatusIssued},
		{ID: 3, ClientID: "b", CollectionDate: "2026-10-31", Status: invoices.StatusCancelled}, // ignored
		{ID: 4, ClientID: "usa", CollectionDate: "2026-10-31", Status: invoices.StatusPrepared},
		{ID: 5, ClientID: "b", CollectionDate: "2026-10-15", Status: invoices.StatusPrepared},
	}
	got, err := invoices.Prepare(cfg, existing, invoices.PrepareInput{ClientID: "b", Date: "2026-10-31", Amount: ptr("100")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Duplicates) != 2 || got.Duplicates[0] != 1 || got.Duplicates[1] != 2 {
		t.Errorf("duplicates = %v, want [1 2]", got.Duplicates)
	}
}
