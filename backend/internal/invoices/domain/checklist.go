package domain

import (
	"fmt"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
)

// Periodicity of a global invoice (InformacionGlobal), as in the SAT catalog.
const (
	PeriodicityBiweekly = "quincenal" // 03
	PeriodicityMonthly  = "mensual"   // 04
)

// Fixed CFDI values the portal asks for.
const (
	VoucherType   = "I"  // Ingreso
	PaymentForm   = "03" // Transferencia electronica de fondos
	PaymentMethod = "PUE"
	// ForeignGenericRFC is the generic RFC of a foreign receiver.
	ForeignGenericRFC = "XEXX010101000"
	// PublicGeneralRFC is the generic RFC of público en general, the only
	// receiver of a global invoice (InformacionGlobal).
	PublicGeneralRFC = "XAXX010101000"
	// Regimen fiscal 626 is RESICO, the default issuer regime.
	defaultIssuerRegimen = "626"
)

// Party is the issuer or the receiver of the CFDI. An empty field is a pending
// value the user must fill in Settings.
type Party struct {
	RFC        string
	Name       string
	Regimen    string
	PostalCode string
	UsoCFDI    string // receiver only
	// InternalNote is the real payer of a public-in-general client. It is not
	// part of the CFDI.
	InternalNote string
}

// Voucher is the Comprobante section.
type Voucher struct {
	Type          string
	Currency      string
	ExchangeRate  *decimal.Decimal
	PaymentForm   string
	PaymentMethod string
	// Export is true for the export of services (client USA), where the portal
	// asks for the export key at 0% IVA.
	Export bool
	// Global is set for public-in-general invoices (InformacionGlobal).
	Global *GlobalInfo
}

// GlobalInfo is the InformacionGlobal block.
type GlobalInfo struct {
	Periodicity string // PeriodicityBiweekly or PeriodicityMonthly
	Code        string // 03 or 04
	Months      string // MM
	Year        string // YYYY
}

// Concept is the single line item of the invoice.
type Concept struct {
	ProdServKey string
	UnitKey     string
	Description string
	Quantity    int
	UnitValue   decimal.Decimal
}

// Taxes is the Impuestos section. IVAIncluded is true for domestic clients (the
// IVA is part of the CFDI total; the amount received is net of the retentions)
// and false for the 0% export.
type Taxes struct {
	IVAIncluded bool
	IVA         decimal.Decimal
	ISRWithheld decimal.Decimal
	IVAWithheld decimal.Decimal
}

// Checklist is the data the user copies into the SAT portal "Genera tu
// factura" form. The app does not stamp invoices. Items in ToConfirm are the
// ones the original workflow marks "confirm with the accountant".
type Checklist struct {
	Issuer   Party
	Receiver Party
	Voucher  Voucher
	Concept  Concept
	Taxes    Taxes
	Totals   Totals
	Period   string
	DueDate  string
	// MissingConfig lists the settings keys that are empty and needed.
	MissingConfig []string
	// ToConfirm lists the items to confirm with the accountant.
	ToConfirm []string
}

// Totals is the Totales section.
type Totals struct {
	Currency           string
	Subtotal           decimal.Decimal
	Total              decimal.Decimal
	ExpectedDepositMXN decimal.Decimal
}

// Items of Checklist.ToConfirm.
const (
	ConfirmExport     = "export_key"
	ConfirmGlobalInfo = "global_info"
	ConfirmProdServ   = "prod_serv_key"
	ConfirmUnitKey    = "unit_key"
	ConfirmTaxObject  = "tax_object"
	ConfirmFXRateDOF  = "fx_rate_dof"
)

// BuildChecklist assembles the SAT-portal data for an invoice from the issuer
// and client settings. periodicity applies to global (público en general) invoices only
// and defaults to monthly. The due date of the declaration is supplied by the
// caller (it belongs to the tax filing rules).
func BuildChecklist(cfg settings.Config, inv Invoice, periodicity, dueDate string) (Checklist, error) {
	client, ok := cfg.FindClient(inv.ClientID)
	if !ok {
		return Checklist{}, fmt.Errorf("%w: %q", ErrUnknownClient, inv.ClientID)
	}
	isUSA := inv.ClientID == settings.ClientUSA

	c := Checklist{
		Period:  inv.Period,
		DueDate: dueDate,
		Issuer: Party{
			RFC: cfg.Issuer.RFC, Name: cfg.Issuer.Name, Regimen: cfg.Issuer.Regimen, PostalCode: cfg.Issuer.PostalCode,
		},
		Receiver: Party{
			RFC: client.RFC, Name: client.Name, Regimen: client.Regimen,
			PostalCode: client.PostalCode,
			UsoCFDI:    client.UsoCFDI, InternalNote: client.RealPayer,
		},
		Voucher: Voucher{
			Type: VoucherType, Currency: inv.Currency, ExchangeRate: inv.ExchangeRate,
			PaymentForm: PaymentForm, PaymentMethod: PaymentMethod, Export: isUSA,
		},
		Concept: Concept{
			ProdServKey: client.ClaveProdServ, UnitKey: client.ClaveUnidad, Description: client.Concepto,
			Quantity: 1, UnitValue: inv.Subtotal,
		},
		Taxes:  Taxes{IVAIncluded: !isUSA, IVA: inv.IVA, ISRWithheld: inv.ISRWithheld, IVAWithheld: inv.IVAWithheld},
		Totals: Totals{Currency: inv.Currency, Subtotal: inv.Subtotal, Total: inv.Total, ExpectedDepositMXN: inv.ExpectedDepositMXN},
	}
	if c.Issuer.Regimen == "" {
		c.Issuer.Regimen = defaultIssuerRegimen
	}
	if c.Issuer.RFC == "" {
		c.MissingConfig = append(c.MissingConfig, "issuer.rfc")
	}
	if c.Issuer.Name == "" {
		c.MissingConfig = append(c.MissingConfig, "issuer.name")
	}
	if c.Issuer.PostalCode == "" {
		c.MissingConfig = append(c.MissingConfig, "issuer.postal_code")
	}

	// The portal takes the issuer's postal code for the generic RFCs (público en
	// general and foreign receivers); any other client needs its own.
	if isGenericRFC(client.RFC) {
		c.Receiver.PostalCode = cfg.Issuer.PostalCode
	} else if c.Receiver.PostalCode == "" {
		c.MissingConfig = append(c.MissingConfig, "client.postal_code")
	}

	if inv.Currency == ledger.CurrencyUSD {
		c.ToConfirm = append(c.ToConfirm, ConfirmFXRateDOF)
	}
	c.ToConfirm = append(c.ToConfirm, ConfirmProdServ)
	if isUSA {
		c.ToConfirm = append(c.ToConfirm, ConfirmExport, ConfirmTaxObject)
	} else {
		if client.RFC == PublicGeneralRFC {
			if periodicity == "" {
				periodicity = PeriodicityMonthly
			}
			g, err := globalInfo(periodicity, inv.Period)
			if err != nil {
				return Checklist{}, err
			}
			c.Voucher.Global = &g
			c.ToConfirm = append(c.ToConfirm, ConfirmGlobalInfo)
		}
		c.ToConfirm = append(c.ToConfirm, ConfirmUnitKey)
	}
	return c, nil
}

func isGenericRFC(rfc string) bool { return rfc == PublicGeneralRFC || rfc == ForeignGenericRFC }

func globalInfo(periodicity, period string) (GlobalInfo, error) {
	var code string
	switch periodicity {
	case PeriodicityBiweekly:
		code = "03"
	case PeriodicityMonthly:
		code = "04"
	default:
		return GlobalInfo{}, fmt.Errorf("%w: periodicity must be %q or %q", ErrInvalidInput, PeriodicityBiweekly, PeriodicityMonthly)
	}
	if len(period) != 7 || period[4] != '-' {
		return GlobalInfo{}, fmt.Errorf("%w: invalid period %q", ErrInvalidInput, period)
	}
	return GlobalInfo{Periodicity: periodicity, Code: code, Months: period[5:], Year: period[:4]}, nil
}
