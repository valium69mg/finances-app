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
