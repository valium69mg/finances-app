package main

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
)

const header = "id,fecha,descripcion,categoria,instrumento,tipo,metodo,moneda,monto,tipo_cambio,monto_mxn,creado\n"

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestParseMovementsRealRow(t *testing.T) {
	csv := header + `1,2026-10-01,"Sueldo octubre (pagado 30-sep, contado el 1-oct)",Sueldo,,Ingreso,Transferencia,USD,3383.33,17.74,60020.27419999999,2026-09-28T00:00:00` + "\n"
	rows, err := parseMovements(strings.NewReader(csv), time.UTC)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	m := rows[0].Movement
	if m.ID != 0 || m.Date != "2026-10-01" || m.Description != "Sueldo octubre (pagado 30-sep, contado el 1-oct)" ||
		m.Category != "Sueldo" || m.Instrument != "" || m.Kind != ledger.KindIncome || m.PaymentMethod != "Transferencia" ||
		m.Currency != "USD" || !m.Amount.Equal(dec("3383.33")) || m.ExchangeRate == nil || !m.ExchangeRate.Equal(dec("17.74")) {
		t.Errorf("unexpected movement %+v", m)
	}
	// Recomputed and rounded to cents, not the CSV's 60020.27419999999.
	if !m.AmountMXN.Equal(dec("60020.27")) {
		t.Errorf("AmountMXN = %s, want 60020.27", m.AmountMXN)
	}
	if want := time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC); !rows[0].CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %s, want %s", rows[0].CreatedAt, want)
	}
}

func TestParseMovementsOtherKinds(t *testing.T) {
	csv := header +
		"2,2026-10-02,Super,Mandado,,Gasto,Débito,MXN,350.5,,350.5,2026-10-02T09:30:00\n" +
		"3,2026-10-03,Retiro,Inversiones,voo,Ahorro,Transferencia,MXN,-500,,-500,2026-10-03T10:00:00\n"
	rows, err := parseMovements(strings.NewReader(csv), time.UTC)
	if err != nil || len(rows) != 2 {
		t.Fatalf("parse: %v (%d rows)", err, len(rows))
	}
	gasto, ahorro := rows[0].Movement, rows[1].Movement
	if gasto.Kind != ledger.KindExpense || gasto.ExchangeRate != nil || !gasto.AmountMXN.Equal(dec("350.5")) {
		t.Errorf("gasto %+v", gasto)
	}
	if ahorro.Instrument != "voo" || !ahorro.AmountMXN.Equal(dec("-500")) {
		t.Errorf("ahorro %+v", ahorro)
	}
}

func TestParseMovementsCreatedInLocation(t *testing.T) {
	loc := time.FixedZone("MX", -6*3600)
	csv := header + "1,2026-10-01,x,Sueldo,,Ingreso,Transferencia,MXN,1,,1,2026-09-28T00:00:00\n"
	rows, err := parseMovements(strings.NewReader(csv), loc)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 9, 28, 6, 0, 0, 0, time.UTC); !rows[0].CreatedAt.Equal(want) {
		t.Errorf("CreatedAt = %s, want %s", rows[0].CreatedAt.UTC(), want)
	}
}

func TestParseMovementsRejectsBadInput(t *testing.T) {
	good := "1,2026-10-01,x,Sueldo,,Ingreso,Transferencia,MXN,1,,1,2026-09-28T00:00:00"
	tests := map[string]string{
		"empty file":       "",
		"wrong header":     "id,fecha\n1,2026-10-01\n",
		"bad kind":         header + "1,2026-10-01,x,Sueldo,,Otro,Transferencia,MXN,1,,1,2026-09-28T00:00:00\n",
		"bad currency":     header + "1,2026-10-01,x,Sueldo,,Ingreso,Transferencia,EUR,1,,1,2026-09-28T00:00:00\n",
		"bad date":         header + "1,2026-13-01,x,Sueldo,,Ingreso,Transferencia,MXN,1,,1,2026-09-28T00:00:00\n",
		"bad amount":       header + "1,2026-10-01,x,Sueldo,,Ingreso,Transferencia,MXN,abc,,1,2026-09-28T00:00:00\n",
		"USD without rate": header + "1,2026-10-01,x,Sueldo,,Ingreso,Transferencia,USD,1,,1,2026-09-28T00:00:00\n",
		"MXN with rate":    header + "1,2026-10-01,x,Sueldo,,Ingreso,Transferencia,MXN,1,17,1,2026-09-28T00:00:00\n",
		"bad created":      header + "1,2026-10-01,x,Sueldo,,Ingreso,Transferencia,MXN,1,,1,yesterday\n",
		"short row":        header + "1,2026-10-01,x\n",
		"missing category": header + "1,2026-10-01,x,,,Ingreso,Transferencia,MXN,1,,1,2026-09-28T00:00:00\n",
	}
	for name, csv := range tests {
		if _, err := parseMovements(strings.NewReader(csv), time.UTC); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// Sanity check of the fixture the negatives derive from.
	if _, err := parseMovements(strings.NewReader(header+good+"\n"), time.UTC); err != nil {
		t.Errorf("good row: %v", err)
	}
}
