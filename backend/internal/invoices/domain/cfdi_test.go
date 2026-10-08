package domain_test

import (
	"errors"
	"strings"
	"testing"

	invoices "github.com/valium69mg/finances-app/backend/internal/invoices/domain"
)

const goodUUID = "6F1C2B3A-4D5E-4F60-8A7B-9C0D1E2F3A4B"

func cfdiXML(uuid, total, subtotal, currency string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Version="4.0" Total="` + total + `" SubTotal="` + subtotal + `" Moneda="` + currency + `">
  <cfdi:Emisor Rfc="AAA010101AAA"/>
  <cfdi:Complemento>
    <tfd:TimbreFiscalDigital xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="1.1" UUID="` + uuid + `"/>
  </cfdi:Complemento>
</cfdi:Comprobante>`
}

func TestParseCFDI(t *testing.T) {
	got, err := invoices.ParseCFDI([]byte(cfdiXML(strings.ToLower(goodUUID), "3383.33", "3383.33", "USD")))
	if err != nil {
		t.Fatal(err)
	}
	if got.UUID != goodUUID || !got.Total.Equal(d("3383.33")) || !got.SubTotal.Equal(d("3383.33")) || got.Currency != "USD" {
		t.Errorf("cfdi = %+v", got)
	}

	v33 := strings.Replace(cfdiXML(goodUUID, "100", "86.21", "MXN"), "cfd/4", "cfd/3", 1)
	if got, err := invoices.ParseCFDI([]byte(v33)); err != nil || !got.Total.Equal(d("100")) {
		t.Errorf("v3.3 = %+v, %v", got, err)
	}
}

func TestParseCFDIRejects(t *testing.T) {
	noStamp := `<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Total="1" SubTotal="1" Moneda="MXN"><cfdi:Complemento/></cfdi:Comprobante>`
	wrongRoot := `<Invoice xmlns="urn:other" Total="1" SubTotal="1" Moneda="MXN"/>`
	misplacedStamp := `<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Total="1" SubTotal="1" Moneda="MXN"><tfd:TimbreFiscalDigital UUID="` + goodUUID + `"/></cfdi:Comprobante>`
	doubleStamp := strings.Replace(cfdiXML(goodUUID, "1", "1", "MXN"), "</cfdi:Complemento>",
		`<tfd:TimbreFiscalDigital xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" UUID="`+goodUUID+`"/></cfdi:Complemento>`, 1)
	doctype := `<?xml version="1.0"?><!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>` + cfdiXML(goodUUID, "1", "1", "MXN")
	entity := strings.Replace(cfdiXML(goodUUID, "1", "1", "MXN"), `Version="4.0"`, `Version="&xxe;"`, 1)
	deep := strings.Repeat("<a>", 100) + strings.Repeat("</a>", 100)

	cases := []struct {
		name string
		xml  string
		want error
	}{
		{"not xml", "hello", invoices.ErrInvalidCFDI},
		{"empty", "", invoices.ErrInvalidCFDI},
		{"wrong root", wrongRoot, invoices.ErrInvalidCFDI},
		{"no stamp", noStamp, invoices.ErrNotStamped},
		{"stamp outside Complemento", misplacedStamp, invoices.ErrNotStamped},
		{"two stamps", doubleStamp, invoices.ErrInvalidCFDI},
		{"doctype", doctype, invoices.ErrInvalidCFDI},
		{"undefined entity", entity, invoices.ErrInvalidCFDI},
		{"too deep", `<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Total="1" SubTotal="1" Moneda="MXN">` + deep + `</cfdi:Comprobante>`, invoices.ErrInvalidCFDI},
		{"bad uuid", cfdiXML("not-a-uuid", "1", "1", "MXN"), invoices.ErrInvalidUUID},
		{"missing total", strings.Replace(cfdiXML(goodUUID, "1", "1", "MXN"), `Total="1" `, "", 1), invoices.ErrInvalidCFDI},
		{"exponent total", cfdiXML(goodUUID, "1e999999999", "1", "MXN"), invoices.ErrInvalidCFDI},
		{"negative total", cfdiXML(goodUUID, "-5", "1", "MXN"), invoices.ErrInvalidCFDI},
		{"huge total", cfdiXML(goodUUID, "9999999999999999", "1", "MXN"), invoices.ErrInvalidCFDI},
		{"too big", strings.Repeat(" ", invoices.MaxXMLBytes+1), invoices.ErrInvalidCFDI},
	}
	for _, tc := range cases {
		if _, err := invoices.ParseCFDI([]byte(tc.xml)); !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestNormalizeUUID(t *testing.T) {
	if got, err := invoices.NormalizeUUID("  " + strings.ToLower(goodUUID) + " "); err != nil || got != goodUUID {
		t.Errorf("NormalizeUUID = %q, %v", got, err)
	}
	for _, bad := range []string{"", "123", goodUUID + "0", strings.Replace(goodUUID, "F", "G", 1)} {
		if _, err := invoices.NormalizeUUID(bad); !errors.Is(err, invoices.ErrInvalidUUID) {
			t.Errorf("NormalizeUUID(%q) error = %v", bad, err)
		}
	}
}

func TestCompareCFDI(t *testing.T) {
	inv := invoices.Invoice{Currency: "USD", Amounts: invoices.Amounts{Subtotal: d("3500"), Total: d("3500")}}

	if w := inv.CompareCFDI(invoices.CFDI{Total: d("3500.00"), SubTotal: d("3500"), Currency: "usd"}); len(w) != 0 {
		t.Errorf("matching XML produced warnings: %+v", w)
	}
	w := inv.CompareCFDI(invoices.CFDI{Total: d("3400"), SubTotal: d("3400"), Currency: "MXN"})
	codes := map[string]invoices.Warning{}
	for _, x := range w {
		codes[x.Code] = x
	}
	if len(w) != 3 {
		t.Fatalf("warnings = %+v, want 3", w)
	}
	if got := codes[invoices.WarningTotalMismatch]; got.Expected != "3500.00" || got.Actual != "3400.00" {
		t.Errorf("total warning = %+v", got)
	}
	if codes[invoices.WarningCurrencyMismatch].Actual != "MXN" {
		t.Errorf("currency warning = %+v", codes[invoices.WarningCurrencyMismatch])
	}
}

// iblXML mirrors the stamped XML of a client that withholds taxes: Concepto-level
// taxes (which the parser must ignore) and the Comprobante-level ones it reads.
const iblXML = `<?xml version="1.0" encoding="UTF-8"?>
<cfdi:Comprobante xmlns:cfdi="http://www.sat.gob.mx/cfd/4" Version="4.0" Total="7318.18" SubTotal="7031.08" Moneda="MXN">
  <cfdi:Conceptos>
    <cfdi:Concepto>
      <cfdi:Impuestos>
        <cfdi:Traslados><cfdi:Traslado Impuesto="002" Importe="999.99"/></cfdi:Traslados>
        <cfdi:Retenciones><cfdi:Retencion Impuesto="001" Importe="888.88"/><cfdi:Retencion Impuesto="002" Importe="777.77"/></cfdi:Retenciones>
      </cfdi:Impuestos>
    </cfdi:Concepto>
  </cfdi:Conceptos>
  <cfdi:Impuestos TotalImpuestosRetenidos="837.87" TotalImpuestosTrasladados="1124.97">
    <cfdi:Retenciones>
      <cfdi:Retencion Impuesto="001" Importe="87.89"/>
      <cfdi:Retencion Impuesto="002" Importe="749.98"/>
    </cfdi:Retenciones>
    <cfdi:Traslados>
      <cfdi:Traslado Base="7031.08" Impuesto="002" TipoFactor="Tasa" TasaOCuota="0.160000" Importe="1124.97"/>
      <cfdi:Traslado Base="1" Impuesto="003" TipoFactor="Tasa" TasaOCuota="0.080000" Importe="55.00"/>
    </cfdi:Traslados>
  </cfdi:Impuestos>
  <cfdi:Complemento>
    <tfd:TimbreFiscalDigital xmlns:tfd="http://www.sat.gob.mx/TimbreFiscalDigital" Version="1.1" UUID="` + goodUUID + `"/>
  </cfdi:Complemento>
</cfdi:Comprobante>`

func TestParseCFDIReadsDocumentLevelTaxesOnly(t *testing.T) {
	got, err := invoices.ParseCFDI([]byte(iblXML))
	if err != nil {
		t.Fatal(err)
	}
	if !got.IVATransferred.Equal(d("1124.97")) || !got.ISRWithheld.Equal(d("87.89")) || !got.IVAWithheld.Equal(d("749.98")) {
		t.Errorf("taxes = IVA %s, ISR %s, retIVA %s", got.IVATransferred, got.ISRWithheld, got.IVAWithheld)
	}

	// Without an Impuestos block every tax is zero.
	plain, err := invoices.ParseCFDI([]byte(cfdiXML(goodUUID, "100", "86.21", "MXN")))
	if err != nil || !plain.IVATransferred.IsZero() || !plain.ISRWithheld.IsZero() || !plain.IVAWithheld.IsZero() {
		t.Errorf("plain = %+v, %v", plain, err)
	}

	// An exempt Traslado has no Importe.
	exempt := strings.Replace(iblXML, `TasaOCuota="0.160000" Importe="1124.97"`, `TipoFactor="Exento"`, 1)
	if got, err := invoices.ParseCFDI([]byte(exempt)); err != nil || !got.IVATransferred.IsZero() {
		t.Errorf("exempt = %+v, %v", got, err)
	}

	bad := strings.Replace(iblXML, `Importe="749.98"`, `Importe="1e999999999"`, 1)
	if _, err := invoices.ParseCFDI([]byte(bad)); !errors.Is(err, invoices.ErrInvalidCFDI) {
		t.Errorf("exponent Importe error = %v", err)
	}
}

func TestAmountsFromCFDI(t *testing.T) {
	cfdi, err := invoices.ParseCFDI([]byte(iblXML))
	if err != nil {
		t.Fatal(err)
	}
	// What the app stored before the fix.
	inv := invoices.Invoice{Currency: "MXN", Amounts: invoices.Amounts{
		Subtotal: d("6308.78"), SubtotalMXN: d("6308.78"), IVA: d("1009.40"), Total: d("7318.18"), ExpectedDepositMXN: d("7318.18"),
	}}
	a, warnings, ok, err := inv.AmountsFromCFDI(cfdi)
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if !a.Subtotal.Equal(d("7031.08")) || !a.SubtotalMXN.Equal(d("7031.08")) || !a.IVA.Equal(d("1124.97")) ||
		!a.ISRWithheld.Equal(d("87.89")) || !a.IVAWithheld.Equal(d("749.98")) || !a.Total.Equal(d("7318.18")) ||
		!a.ExpectedDepositMXN.Equal(d("7318.18")) {
		t.Errorf("amounts = %+v", a)
	}
	if len(warnings) != 1 || warnings[0].Code != invoices.WarningAmountsFromXML || len(warnings[0].Changes) != 4 {
		t.Fatalf("warnings = %+v", warnings)
	}
	if c := warnings[0].Changes[0]; c.Field != "subtotal" || c.From != "6308.78" || c.To != "7031.08" {
		t.Errorf("first change = %+v", c)
	}

	// Already in sync: no warning.
	inv.Amounts = a
	if _, w, ok, err := inv.AmountsFromCFDI(cfdi); err != nil || !ok || len(w) != 0 {
		t.Errorf("in sync: warnings = %+v, ok = %v, err = %v", w, ok, err)
	}

	// Another currency is left to CompareCFDI.
	inv.Currency = "USD"
	if _, _, ok, err := inv.AmountsFromCFDI(cfdi); err != nil || ok {
		t.Errorf("currency mismatch: ok = %v, err = %v", ok, err)
	}
}

func TestAmountsFromCFDIUSDUsesTheInvoiceRate(t *testing.T) {
	rate := d("17.74")
	inv := invoices.Invoice{Currency: "USD", ExchangeRate: &rate, Amounts: invoices.ComputeUSAInvoice(d("3500"), rate).Rounded()}
	cfdi, err := invoices.ParseCFDI([]byte(cfdiXML(goodUUID, "3383.33", "3383.33", "USD")))
	if err != nil {
		t.Fatal(err)
	}
	a, warnings, ok, err := inv.AmountsFromCFDI(cfdi)
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if !a.Subtotal.Equal(d("3383.33")) || !a.Total.Equal(d("3383.33")) || !a.SubtotalMXN.Equal(d("60020.27")) || !a.ExpectedDepositMXN.Equal(d("60020.27")) {
		t.Errorf("amounts = %+v", a)
	}
	if len(warnings) != 1 || len(warnings[0].Changes) != 2 { // subtotal and total
		t.Errorf("warnings = %+v", warnings)
	}
}
