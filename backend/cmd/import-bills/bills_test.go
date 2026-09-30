package main

import (
	"strings"
	"testing"
	"time"

	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
)

var today = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

func config(servicios, vivienda string) []byte {
	return []byte(`{"categories":[
		{"name":"Vivienda","type":"Gasto","includes":"` + vivienda + `"},
		{"name":"Servicios","type":"Gasto","includes":"` + servicios + `"}]}`)
}

func TestParseServicesFromTheRealNote(t *testing.T) {
	list, err := parseServices(config("megacable 550, agua 400, luz 200, telcel 350, gas LP 500", "mantenimiento 1,100 + predial 2,500 oct-ene"), today)
	if err != nil {
		t.Fatalf("parseServices: %v", err)
	}
	want := []struct {
		name   string
		amount string
	}{{"Megacable", "550"}, {"Agua", "400"}, {"Luz", "200"}, {"Telcel", "350"}, {"Gas LP", "500"}}
	if len(list) != len(want) {
		t.Fatalf("bills = %d, want %d", len(list), len(want))
	}
	for i, w := range want {
		b := list[i]
		if b.Name != w.name || b.Amount == nil || b.Amount.String() != w.amount {
			t.Errorf("bill %d = %s %v, want %s %s", i, b.Name, b.Amount, w.name, w.amount)
		}
		if b.Category != "Servicios" || b.Currency != "MXN" || b.Recurrence != bills.Monthly ||
			b.ReminderLeadDays != 3 || b.NextDueDate != "2026-10-01" || b.AnchorDay != 1 || !b.Active {
			t.Errorf("bill %d = %+v", i, b)
		}
	}
}

func TestParseServicesNeverImportsThePredial(t *testing.T) {
	list, err := parseServices(config("agua 400", "mantenimiento 1,100 + predial 2,500 oct-ene"), today)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range list {
		if strings.Contains(strings.ToLower(b.Name), "predial") || strings.Contains(strings.ToLower(b.Name), "mantenimiento") {
			t.Errorf("imported %q from Vivienda", b.Name)
		}
	}
	if len(list) != 1 {
		t.Errorf("bills = %d, want 1", len(list))
	}
}

func TestNextMonthFirstAcrossTheYear(t *testing.T) {
	list, err := parseServices(config("agua 400", ""), time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC))
	if err != nil || list[0].NextDueDate != "2027-01-01" {
		t.Errorf("due = %v, %v; want 2027-01-01", list, err)
	}
}

func TestParseServicesToleratesSpacing(t *testing.T) {
	list, err := parseServices(config("  gas   LP   500.50 ,agua 400 ", ""), today)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Name != "Gas LP" || list[0].Amount.String() != "500.5" || list[1].Name != "Agua" {
		t.Errorf("list = %+v", list)
	}
}

func TestParseServicesRejectsMalformedInput(t *testing.T) {
	for name, data := range map[string][]byte{
		"not json":            []byte(`{`),
		"no categories":       []byte(`{"categories":[]}`),
		"no Servicios":        []byte(`{"categories":[{"name":"Vivienda","type":"Gasto","includes":"x 1"}]}`),
		"Servicios not Gasto": []byte(`{"categories":[{"name":"Servicios","type":"Ingreso","includes":"agua 400"}]}`),
		"empty note":          config("", ""),
		"item without amount": config("megacable", ""),
		"non numeric amount":  config("megacable abc", ""),
		"zero amount":         config("agua 0", ""),
		"negative amount":     config("agua -5", ""),
		"trailing comma":      config("agua 400,", ""),
		"duplicate name":      config("agua 400, Agua 300", ""),
		"amount only":         config(" 400", ""),
	} {
		t.Run(name, func(t *testing.T) {
			if list, err := parseServices(data, today); err == nil {
				t.Errorf("want an error, got %+v", list)
			}
		})
	}
}
