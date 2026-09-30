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

func TestComputeClientBInvoice(t *testing.T) {
	a := invoices.ComputeClientBInvoice(d("35000"), d("0.16"))
	// 35,000 / 1.16 = 30,172.4137931034 (10 decimals); IVA is the difference.
	if !a.Subtotal.Equal(d("30172.4137931034")) || !a.SubtotalMXN.Equal(d("30172.4137931034")) ||
		!a.IVA.Equal(d("4827.5862068966")) || !a.Total.Equal(d("35000")) || !a.ExpectedDepositMXN.Equal(d("35000")) {
		t.Errorf("amounts = %+v", a)
	}
	if !a.Subtotal.Add(a.IVA).Equal(a.Total) {
		t.Error("subtotal + IVA must equal the total exactly")
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
		if inv.Currency != "MXN" || inv.ExchangeRate != nil || !inv.IVA.Equal(d("4827.5862068966")) {
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

func TestStatusLifecycle(t *testing.T) {
	prepared := invoices.Invoice{ID: 7, Status: invoices.StatusPrepared}

	issued, err := prepared.MarkIssued("UUID-1")
	if err != nil || issued.Status != invoices.StatusIssued || issued.UUID != "UUID-1" {
		t.Fatalf("MarkIssued = %+v, %v", issued, err)
	}
	if reissued, err := issued.MarkIssued("UUID-2"); err != nil || reissued.UUID != "UUID-2" {
		t.Errorf("re-issuing should replace the UUID, got %+v, %v", reissued, err)
	}

	cancelled := issued.Cancel()
	if cancelled.Status != invoices.StatusCancelled {
		t.Errorf("Cancel status = %s", cancelled.Status)
	}
	if _, err := cancelled.MarkIssued("UUID-3"); !errors.Is(err, invoices.ErrCancelled) {
		t.Errorf("issuing a cancelled invoice error = %v, want ErrCancelled", err)
	}
	if again := cancelled.Cancel(); again.Status != invoices.StatusCancelled {
		t.Error("cancelling twice must stay cancelled")
	}
	if prepared.Status != invoices.StatusPrepared {
		t.Error("state changes must not mutate the original")
	}
}

func TestStatusIsActive(t *testing.T) {
	want := map[invoices.Status]bool{
		invoices.StatusPrepared: true, invoices.StatusIssued: true, invoices.StatusCancelled: false,
	}
	for s, active := range want {
		if s.IsActive() != active {
			t.Errorf("%s.IsActive() = %v, want %v", s, s.IsActive(), active)
		}
	}
}

func TestMarkDeclared(t *testing.T) {
	list := []invoices.Invoice{
		{ID: 1, Period: "2026-10", Status: invoices.StatusIssued},
		{ID: 2, Period: "2026-10", Status: invoices.StatusPrepared},
		{ID: 3, Period: "2026-10", Status: invoices.StatusCancelled},
		{ID: 4, Period: "2026-11", Status: invoices.StatusIssued},
	}
	got := invoices.MarkDeclared(list, "2026-10")
	want := []string{"2026-10", "2026-10", "", ""}
	for i, inv := range got {
		if inv.DeclarationPeriod != want[i] {
			t.Errorf("invoice %d declaration = %q, want %q", inv.ID, inv.DeclarationPeriod, want[i])
		}
	}
	if list[0].DeclarationPeriod != "" {
		t.Error("MarkDeclared must not mutate its input")
	}
}
