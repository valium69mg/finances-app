package domain_test

import (
	"errors"
	"testing"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

func checklistConfig() settings.Config {
	cfg := settingstest.RealConfig()
	cfg.Issuer = settings.Issuer{RFC: "AAA010101AAA", Name: "Juan Perez", PostalCode: "64000"}
	for i := range cfg.Clients {
		cfg.Clients[i].RFC = "XAXX010101000"
		cfg.Clients[i].Regimen = "616"
		cfg.Clients[i].UsoCFDI = "S01"
		cfg.Clients[i].ClaveProdServ = "81111500"
		cfg.Clients[i].ClaveUnidad = "E48"
		cfg.Clients[i].Concepto = "Servicios de software"
	}
	cfg.Clients[1].RealPayer = "Empresa pagadora"
	return cfg
}

func TestBuildChecklistUSA(t *testing.T) {
	cfg := checklistConfig()
	prep, err := invoices.Prepare(cfg, nil, invoices.PrepareInput{ClientID: "usa", Date: "2026-10-15"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := invoices.BuildChecklist(cfg, prep.Invoice, "", "2026-11-17")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Voucher.Export || c.Voucher.Global != nil || c.Taxes.IVAIncluded || c.Voucher.Currency != "USD" ||
		c.Voucher.ExchangeRate == nil || c.Voucher.PaymentForm != "03" || c.Voucher.PaymentMethod != "PUE" {
		t.Errorf("voucher = %+v taxes = %+v", c.Voucher, c.Taxes)
	}
	if c.Issuer.Regimen != "626" || c.Receiver.PostalCode != "64000" || c.Receiver.InternalNote != "" {
		t.Errorf("issuer = %+v receiver = %+v", c.Issuer, c.Receiver)
	}
	if c.Concept.Quantity != 1 || !c.Concept.UnitValue.Equal(d("3500")) || c.DueDate != "2026-11-17" || c.Period != "2026-10" {
		t.Errorf("concept = %+v", c.Concept)
	}
	want := map[string]bool{invoices.ConfirmFXRateDOF: true, invoices.ConfirmProdServ: true, invoices.ConfirmExport: true, invoices.ConfirmTaxObject: true}
	if len(c.ToConfirm) != len(want) {
		t.Errorf("ToConfirm = %v", c.ToConfirm)
	}
	for _, k := range c.ToConfirm {
		if !want[k] {
			t.Errorf("unexpected confirm item %q", k)
		}
	}
	if len(c.MissingConfig) != 0 {
		t.Errorf("MissingConfig = %v", c.MissingConfig)
	}
}

func TestBuildChecklistClientB(t *testing.T) {
	cfg := checklistConfig()
	prep, err := invoices.Prepare(cfg, nil, invoices.PrepareInput{ClientID: "b", Date: "2026-10-31", Amount: ptr("35000")})
	if err != nil {
		t.Fatal(err)
	}
	c, err := invoices.BuildChecklist(cfg, prep.Invoice, invoices.PeriodicityBiweekly, "2026-11-17")
	if err != nil {
		t.Fatal(err)
	}
	g := c.Voucher.Global
	if g == nil || g.Code != "03" || g.Months != "10" || g.Year != "2026" || c.Voucher.Export || !c.Taxes.IVAIncluded {
		t.Errorf("voucher = %+v", c.Voucher)
	}
	if c.Receiver.InternalNote != "Empresa pagadora" || !c.Taxes.IVA.Equal(prep.Invoice.IVA) {
		t.Errorf("receiver = %+v taxes = %+v", c.Receiver, c.Taxes)
	}

	monthly, _ := invoices.BuildChecklist(cfg, prep.Invoice, "", "")
	if monthly.Voucher.Global.Code != "04" {
		t.Errorf("default periodicity code = %q, want 04", monthly.Voucher.Global.Code)
	}
	if _, err := invoices.BuildChecklist(cfg, prep.Invoice, "weekly", ""); !errors.Is(err, invoices.ErrInvalidInput) {
		t.Errorf("bad periodicity error = %v", err)
	}
}

func TestBuildChecklistMissingIssuerAndClient(t *testing.T) {
	cfg := checklistConfig()
	cfg.Issuer = settings.Issuer{}
	prep, _ := invoices.Prepare(cfg, nil, invoices.PrepareInput{ClientID: "usa", Date: "2026-10-15"})
	c, err := invoices.BuildChecklist(cfg, prep.Invoice, "", "")
	if err != nil || len(c.MissingConfig) != 3 { // XAXX uses the issuer's postal code, so none is missing for the client
		t.Errorf("MissingConfig = %v, %v", c.MissingConfig, err)
	}
	if _, err := invoices.BuildChecklist(cfg, invoices.Invoice{ClientID: "zzz"}, "", ""); !errors.Is(err, invoices.ErrUnknownClient) {
		t.Errorf("unknown client error = %v", err)
	}
}

func TestAmountsRoundedAndValidate(t *testing.T) {
	a := compute(t, "35000", "0.16", "0", "0").Rounded()
	if !a.Subtotal.Equal(d("30172.41")) || !a.IVA.Equal(d("4827.59")) || !a.Subtotal.Add(a.IVA).Equal(a.Total) {
		t.Errorf("client B rounded = %+v", a)
	}
	u := invoices.ComputeUSAInvoice(d("3383.33"), d("17.74")).Rounded()
	if !u.SubtotalMXN.Equal(d("60020.27")) || !u.ExpectedDepositMXN.Equal(d("60020.27")) || !u.IVA.IsZero() {
		t.Errorf("USA rounded = %+v", u)
	}
	if err := u.Validate(); err != nil {
		t.Errorf("valid amounts: %v", err)
	}
	if err := invoices.ComputeUSAInvoice(d("0.001"), d("1")).Rounded().Validate(); !errors.Is(err, invoices.ErrInvalidInput) {
		t.Errorf("amount that rounds to zero: %v", err)
	}
	if err := invoices.ComputeUSAInvoice(d("999999999999"), d("2")).Rounded().Validate(); !errors.Is(err, invoices.ErrInvalidInput) {
		t.Errorf("amount over the maximum: %v", err)
	}
}

func TestPrepareRejectsNonPositive(t *testing.T) {
	cfg := settingstest.RealConfig()
	for name, in := range map[string]invoices.PrepareInput{
		"usa zero subtotal":     {ClientID: "usa", Date: "2026-10-15", Subtotal: ptr("0")},
		"usa negative subtotal": {ClientID: "usa", Date: "2026-10-15", Subtotal: ptr("-1")},
		"usa negative rate":     {ClientID: "usa", Date: "2026-10-15", ExchangeRate: ptr("-17")},
		"b zero amount":         {ClientID: "b", Date: "2026-10-15", Amount: ptr("0")},
		"b negative amount":     {ClientID: "b", Date: "2026-10-15", Amount: ptr("-5")},
	} {
		if _, err := invoices.Prepare(cfg, nil, in); !errors.Is(err, invoices.ErrInvalidInput) {
			t.Errorf("%s: error = %v", name, err)
		}
	}
}

func withholdingConfig() (settings.Config, settings.Client) {
	cfg := checklistConfig()
	ibl := settings.Client{
		ID: "ibl", Name: "IBL", Currency: "MXN", IVARate: d("0.16"), RetISRRate: d("0.0125"), RetIVARate: d("0.106667"),
		RFC: "IBL121029ED3", Regimen: "601", UsoCFDI: "G03", PostalCode: "06600",
		ClaveProdServ: "81111500", ClaveUnidad: "E48", Concepto: "Servicios de software",
	}
	cfg.Clients = append(cfg.Clients, ibl)
	return cfg, ibl
}

func TestBuildChecklistWithholdingClient(t *testing.T) {
	cfg, _ := withholdingConfig()
	prep, err := invoices.Prepare(cfg, nil, invoices.PrepareInput{ClientID: "ibl", Date: "2026-10-31", Amount: ptr("7318.18")})
	if err != nil {
		t.Fatal(err)
	}
	// A periodicity is ignored: only público en general issues a global invoice.
	c, err := invoices.BuildChecklist(cfg, prep.Invoice, "weekly", "2026-11-17")
	if err != nil {
		t.Fatal(err)
	}
	if c.Voucher.Global != nil {
		t.Errorf("global info = %+v, want none", c.Voucher.Global)
	}
	for _, k := range c.ToConfirm {
		if k == invoices.ConfirmGlobalInfo {
			t.Error("global_info must not be confirmed for a named receiver")
		}
	}
	if c.Receiver.PostalCode != "06600" || c.Issuer.PostalCode != "64000" || len(c.MissingConfig) != 0 {
		t.Errorf("receiver = %+v missing = %v", c.Receiver, c.MissingConfig)
	}
	if !c.Taxes.IVAIncluded || !c.Taxes.IVA.Equal(d("1124.97")) || !c.Taxes.ISRWithheld.Equal(d("87.89")) || !c.Taxes.IVAWithheld.Equal(d("749.98")) {
		t.Errorf("taxes = %+v", c.Taxes)
	}
}

func TestBuildChecklistClientWithoutPostalCode(t *testing.T) {
	cfg, ibl := withholdingConfig()
	cfg.Clients[len(cfg.Clients)-1].PostalCode = ""
	prep, _ := invoices.Prepare(cfg, nil, invoices.PrepareInput{ClientID: ibl.ID, Date: "2026-10-31", Amount: ptr("7318.18")})
	c, err := invoices.BuildChecklist(cfg, prep.Invoice, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.MissingConfig) != 1 || c.MissingConfig[0] != "client.postal_code" || c.Receiver.PostalCode != "" {
		t.Errorf("missing = %v receiver = %+v", c.MissingConfig, c.Receiver)
	}
}

func TestBuildChecklistPublicGeneralUsesIssuerPostalCode(t *testing.T) {
	cfg := checklistConfig()
	cfg.Clients[1].PostalCode = "99999"
	prep, _ := invoices.Prepare(cfg, nil, invoices.PrepareInput{ClientID: "b", Date: "2026-10-31", Amount: ptr("100")})
	c, err := invoices.BuildChecklist(cfg, prep.Invoice, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Receiver.PostalCode != "64000" || c.Voucher.Global == nil || len(c.MissingConfig) != 0 {
		t.Errorf("receiver = %+v global = %+v missing = %v", c.Receiver, c.Voucher.Global, c.MissingConfig)
	}
}
