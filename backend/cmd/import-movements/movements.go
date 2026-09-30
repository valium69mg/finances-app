package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	ledgerpg "github.com/valium69mg/finances-app/backend/internal/ledger/adapters/postgres"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

// csvColumns are the columns of the legacy movimientos.csv, in file order.
var csvColumns = []string{
	"id", "fecha", "descripcion", "categoria", "instrumento", "tipo", "metodo", "moneda",
	"monto", "tipo_cambio", "monto_mxn", "creado",
}

var createdLayouts = []string{"2006-01-02T15:04:05", time.RFC3339}

// parseMovements maps the rows of the legacy movimientos.csv to movements. The
// legacy id is dropped (the database generates new ones) and the naive
// creado timestamps are read in loc, as fin.py wrote them in local time.
//
// amount_mxn is recomputed with ledger.AmountMXN instead of being copied: the
// CSV keeps the unrounded product (60020.27419999999 for 3383.33 USD at
// 17.74) while the app stores it rounded to cents (60020.27), a 0.0042 peso
// difference per row that is intentional.
//
// Only structure is checked here (kind, currency, date, numbers, the USD rate
// pairing); amount signs are left to the movements table CHECK constraints.
func parseMovements(r io.Reader, loc *time.Location) ([]ledgerpg.ImportedMovement, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = len(csvColumns)
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	for i, want := range csvColumns {
		if strings.TrimSpace(strings.TrimPrefix(header[i], "\ufeff")) != want {
			return nil, fmt.Errorf("header column %d is %q, want %q", i+1, header[i], want)
		}
	}

	var out []ledgerpg.ImportedMovement
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		row, err := parseRow(rec, loc)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		out = append(out, row)
	}
	return out, nil
}

func parseRow(rec []string, loc *time.Location) (ledgerpg.ImportedMovement, error) {
	field := func(name string) string {
		for i, c := range csvColumns {
			if c == name {
				return strings.TrimSpace(rec[i])
			}
		}
		return ""
	}

	kind := ledger.Kind(field("tipo"))
	switch kind {
	case ledger.KindIncome, ledger.KindExpense, ledger.KindSavings:
	default:
		return ledgerpg.ImportedMovement{}, fmt.Errorf("tipo %q must be Ingreso, Gasto or Ahorro", kind)
	}
	currency := field("moneda")
	if currency != ledger.CurrencyMXN && currency != ledger.CurrencyUSD {
		return ledgerpg.ImportedMovement{}, fmt.Errorf("moneda %q must be MXN or USD", currency)
	}
	date, err := ledger.ParseDate(field("fecha"))
	if err != nil {
		return ledgerpg.ImportedMovement{}, err
	}
	if field("categoria") == "" || field("metodo") == "" {
		return ledgerpg.ImportedMovement{}, errors.New("categoria and metodo are required")
	}
	amount, err := decimal.NewFromString(field("monto"))
	if err != nil {
		return ledgerpg.ImportedMovement{}, fmt.Errorf("monto: %w", err)
	}

	var rate *decimal.Decimal
	if raw := field("tipo_cambio"); raw != "" {
		r, err := decimal.NewFromString(raw)
		if err != nil {
			return ledgerpg.ImportedMovement{}, fmt.Errorf("tipo_cambio: %w", err)
		}
		rate = &r
	}
	if (currency == ledger.CurrencyUSD) != (rate != nil) {
		return ledgerpg.ImportedMovement{}, errors.New("tipo_cambio is required for USD and must be empty for MXN")
	}

	created, err := parseCreated(field("creado"), loc)
	if err != nil {
		return ledgerpg.ImportedMovement{}, err
	}

	return ledgerpg.ImportedMovement{
		Movement: ledger.Movement{
			Date:          date,
			Description:   field("descripcion"),
			Category:      field("categoria"),
			Instrument:    field("instrumento"),
			Kind:          kind,
			PaymentMethod: field("metodo"),
			Currency:      currency,
			Amount:        amount,
			ExchangeRate:  rate,
			AmountMXN:     ledger.AmountMXN(amount, currency, rate),
		},
		CreatedAt: created,
	}, nil
}

func parseCreated(s string, loc *time.Location) (time.Time, error) {
	for _, layout := range createdLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("creado %q must be YYYY-MM-DDTHH:MM:SS", s)
}
