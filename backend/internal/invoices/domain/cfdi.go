package domain

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/shopspring/decimal"
)

const (
	nsCFDI3 = "http://www.sat.gob.mx/cfd/3"
	nsCFDI4 = "http://www.sat.gob.mx/cfd/4"
	nsTFD   = "http://www.sat.gob.mx/TimbreFiscalDigital"

	maxXMLDepth = 32
)

// CFDI holds the only fields read from an issued CFDI XML.
type CFDI struct {
	UUID     string // normalized (upper case)
	Total    decimal.Decimal
	SubTotal decimal.Decimal
	Currency string // Moneda
	// Comprobante-level taxes (the Impuestos child of the root; the per-Concepto
	// ones are not read). Absent values are zero.
	IVATransferred decimal.Decimal // Traslado, Impuesto 002
	ISRWithheld    decimal.Decimal // Retencion, Impuesto 001
	IVAWithheld    decimal.Decimal // Retencion, Impuesto 002
}

// ParseCFDI extracts the stamp UUID, the Total, SubTotal and Moneda of the
// Comprobante and its document-level taxes (IVA transferred, ISR and IVA
// withheld) from a stamped CFDI XML (versions 3.3 and 4.0).
//
// The parse is deliberately narrow and safe: input above MaxXMLBytes is
// rejected, any DOCTYPE or other directive is rejected (encoding/xml never
// resolves external entities or expands custom ones, and unknown entities are
// an error in strict mode), nesting is bounded, and only the needed attributes
// are read. The root must be a cfd/3 or cfd/4 Comprobante and the stamp a
// TimbreFiscalDigital under Comprobante/Complemento; an XML without the stamp is
// ErrNotStamped.
func ParseCFDI(data []byte) (CFDI, error) {
	if len(data) > MaxXMLBytes {
		return CFDI{}, fmt.Errorf("%w: file exceeds %d bytes", ErrInvalidCFDI, MaxXMLBytes)
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true

	var (
		out      CFDI
		stack    []string
		gotRoot  bool
		gotStamp bool
		stampID  string
	)
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return CFDI{}, fmt.Errorf("%w: malformed XML", ErrInvalidCFDI)
		}
		switch t := tok.(type) {
		case xml.Directive:
			return CFDI{}, fmt.Errorf("%w: DOCTYPE and other directives are not allowed", ErrInvalidCFDI)
		case xml.StartElement:
			if len(stack) >= maxXMLDepth {
				return CFDI{}, fmt.Errorf("%w: XML is nested too deeply", ErrInvalidCFDI)
			}
			switch {
			case len(stack) == 0:
				if t.Name.Local != "Comprobante" || (t.Name.Space != nsCFDI3 && t.Name.Space != nsCFDI4) {
					return CFDI{}, fmt.Errorf("%w: the root element is not a CFDI Comprobante", ErrInvalidCFDI)
				}
				if err := readComprobante(t, &out); err != nil {
					return CFDI{}, err
				}
				gotRoot = true
			case t.Name.Local == "TimbreFiscalDigital" && t.Name.Space == nsTFD &&
				len(stack) == 2 && stack[1] == "Complemento":
				if gotStamp {
					return CFDI{}, fmt.Errorf("%w: more than one TimbreFiscalDigital", ErrInvalidCFDI)
				}
				gotStamp = true
				stampID = attr(t, "UUID")
			}
			if err := readTax(stack, t, &out); err != nil {
				return CFDI{}, err
			}
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if !gotRoot {
		return CFDI{}, fmt.Errorf("%w: no Comprobante found", ErrInvalidCFDI)
	}
	if !gotStamp {
		return CFDI{}, ErrNotStamped
	}
	uuid, err := NormalizeUUID(stampID)
	if err != nil {
		return CFDI{}, err
	}
	out.UUID = uuid
	return out, nil
}

func attr(el xml.StartElement, local string) string {
	for _, a := range el.Attr {
		if a.Name.Space == "" && a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// readTax adds the Importe of a Comprobante-level tax element. stack is the
// path of local names above el, so only Comprobante/Impuestos/Traslados/Traslado
// and Comprobante/Impuestos/Retenciones/Retencion are read; the Impuestos
// inside each Concepto sit deeper and are ignored. Several lines of the same
// tax add up.
func readTax(stack []string, el xml.StartElement, out *CFDI) error {
	if len(stack) != 3 || stack[1] != "Impuestos" || (el.Name.Space != nsCFDI3 && el.Name.Space != nsCFDI4) {
		return nil
	}
	var dst *decimal.Decimal
	switch {
	case stack[2] == "Traslados" && el.Name.Local == "Traslado" && attr(el, "Impuesto") == "002":
		dst = &out.IVATransferred
	case stack[2] == "Retenciones" && el.Name.Local == "Retencion" && attr(el, "Impuesto") == "001":
		dst = &out.ISRWithheld
	case stack[2] == "Retenciones" && el.Name.Local == "Retencion" && attr(el, "Impuesto") == "002":
		dst = &out.IVAWithheld
	default:
		return nil
	}
	if attr(el, "Importe") == "" { // an exempt Traslado has no Importe
		return nil
	}
	v, err := requiredAmount(el, "Importe")
	if err != nil {
		return err
	}
	*dst = dst.Add(v)
	if dst.GreaterThanOrEqual(MaxAmount) {
		return fmt.Errorf("%w: Importe is missing or invalid", ErrInvalidCFDI)
	}
	return nil
}

func readComprobante(el xml.StartElement, out *CFDI) error {
	var err error
	if out.Total, err = requiredAmount(el, "Total"); err != nil {
		return err
	}
	if out.SubTotal, err = requiredAmount(el, "SubTotal"); err != nil {
		return err
	}
	if out.Currency = attr(el, "Moneda"); out.Currency == "" || len(out.Currency) > 3 {
		return fmt.Errorf("%w: Moneda is missing or invalid", ErrInvalidCFDI)
	}
	return nil
}

func requiredAmount(el xml.StartElement, name string) (decimal.Decimal, error) {
	raw := attr(el, name)
	// Plain decimals only: an exponent form such as 1e999999999 would make
	// decimal comparisons allocate enormous numbers.
	if !amountPattern.MatchString(raw) {
		return decimal.Zero, fmt.Errorf("%w: %s is missing or invalid", ErrInvalidCFDI, name)
	}
	v, err := decimal.NewFromString(raw)
	if err != nil || v.GreaterThanOrEqual(MaxAmount) {
		return decimal.Zero, fmt.Errorf("%w: %s is missing or invalid", ErrInvalidCFDI, name)
	}
	return v, nil
}

var amountPattern = regexp.MustCompile(`^[0-9]{1,15}(\.[0-9]{1,6})?$`)
