package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"

	bills "github.com/valium69mg/finances-app/backend/internal/bills/domain"
)

const (
	servicesCategory = "Servicios"
	reminderLeadDays = 3
)

// parseServices reads the Servicios category of the legacy config.json and turns
// its free-text `includes` note ("megacable 550, agua 400, ...") into monthly
// bills. Only that category is read: the Vivienda note (predial) and every
// other `includes` are not bills. The parse is defensive: the category must
// exist and be a Gasto, and every comma-separated item must be "<name> <amount>"
// with a positive amount and a unique name, otherwise nothing is imported.
//
// The real due day is unknown (the note has no dates), so every bill is due on
// the 1st of the month after today; the user adjusts the dates in the app.
func parseServices(data []byte, today time.Time) ([]bills.Validated, error) {
	var file struct {
		Categories []struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Includes string `json:"includes"`
		} `json:"categories"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("parse config.json: %w", err)
	}
	note, found := "", false
	for _, c := range file.Categories {
		if c.Name != servicesCategory {
			continue
		}
		if c.Type != "Gasto" {
			return nil, fmt.Errorf("category %q must be of type Gasto, got %q", servicesCategory, c.Type)
		}
		note, found = c.Includes, true
	}
	if !found {
		return nil, fmt.Errorf("category %q not found in config.json", servicesCategory)
	}
	if strings.TrimSpace(note) == "" {
		return nil, fmt.Errorf("category %q has no `includes` note to import", servicesCategory)
	}

	due := time.Date(today.Year(), today.Month()+1, 1, 0, 0, 0, 0, time.UTC).Format(bills.DateLayout)
	seen := map[string]bool{}
	var out []bills.Validated
	for _, raw := range strings.Split(note, ",") {
		name, amount, err := parseItem(raw)
		if err != nil {
			return nil, fmt.Errorf("`includes` of %q: %w", servicesCategory, err)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("`includes` of %q: %q appears twice", servicesCategory, name)
		}
		seen[key] = true
		v, err := bills.Validate(bills.Input{
			Name: name, Category: servicesCategory, Amount: &amount, Currency: bills.CurrencyMXN, Recurrence: bills.Monthly,
			NextDueDate: due, ReminderLeadDays: intPtr(reminderLeadDays), Active: true,
			Notes: "Imported from the Servicios note; the due day is a placeholder (the 1st), set the real one.",
		})
		if err != nil {
			return nil, fmt.Errorf("bill %q: %w", name, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func intPtr(n int) *int { return &n }

// parseItem splits "gas LP 500" into the name "Gas LP" and the amount 500. The
// name gets an upper-case first letter; the rest is kept as written.
func parseItem(raw string) (string, decimal.Decimal, error) {
	item := strings.TrimSpace(raw)
	i := strings.LastIndexFunc(item, unicode.IsSpace)
	if i < 0 {
		return "", decimal.Zero, fmt.Errorf("item %q must be \"<name> <amount>\"", item)
	}
	name, amountText := strings.Join(strings.Fields(item[:i]), " "), strings.TrimSpace(item[i:])
	amount, err := decimal.NewFromString(amountText)
	if err != nil {
		return "", decimal.Zero, fmt.Errorf("item %q: %q is not an amount", item, amountText)
	}
	if !amount.IsPositive() {
		return "", decimal.Zero, fmt.Errorf("item %q: the amount must be greater than zero", item)
	}
	if name == "" {
		return "", decimal.Zero, fmt.Errorf("item %q has no name", item)
	}
	runes := []rune(name)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes), amount, nil
}
