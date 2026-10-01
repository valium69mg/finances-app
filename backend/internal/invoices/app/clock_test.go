package app_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/invoices/app"
	ledger "github.com/valium69mg/finances-app/backend/internal/ledger/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/clock"
	settings "github.com/valium69mg/finances-app/backend/internal/settings/domain"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

// Mexico City has no daylight saving time since 2022: a fixed -6h zone is exact.
var mexico = time.FixedZone("America/Mexico_City", -6*60*60)

// boundaries straddle local midnight (and the local month end) while UTC is
// already a day (or a month) ahead.
var boundaries = []struct {
	name, instant, wantDate, wantPeriod string
	utcDiffers                          bool
}{
	{"evening before local midnight", "2026-10-01T02:30:00Z", "2026-09-30", "2026-09", true},
	{"after local midnight", "2026-10-01T06:30:00Z", "2026-10-01", "2026-10", false},
	{"evening of the last day of the month", "2026-11-01T05:30:00Z", "2026-10-31", "2026-10", true},
	{"after the local month change", "2026-11-01T06:30:00Z", "2026-11-01", "2026-11", false},
}

func prepareAt(t *testing.T, now func() time.Time) app.Result {
	t.Helper()
	cfg := settingstest.RealConfig()
	cfg.Issuer = settings.Issuer{RFC: "AAA010101AAA", Name: "Juan", PostalCode: "64000"}
	svc := app.NewService(newRepo(), newStore(), fakeMovements{5: {ID: 5, Kind: ledger.KindIncome}}, fakeSettings{cfg}, now,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	res, err := svc.Prepare(context.Background(), app.PrepareInput{ClientID: "usa"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestPrepareDefaultsToTheLocalDay(t *testing.T) {
	for _, tc := range boundaries {
		t.Run(tc.name, func(t *testing.T) {
			instant, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			if differs := instant.UTC().Format("2006-01-02") != tc.wantDate; differs != tc.utcDiffers {
				t.Fatalf("fixture: UTC date differs from local = %v, want %v", differs, tc.utcDiffers)
			}

			inv := prepareAt(t, clock.Fixed(instant, mexico)).Invoice
			if inv.CollectionDate != tc.wantDate || inv.Period != tc.wantPeriod {
				t.Errorf("collection date/period = %s/%s, want %s/%s", inv.CollectionDate, inv.Period, tc.wantDate, tc.wantPeriod)
			}

			// The UTC clock this fix replaces gives the wrong day exactly when UTC
			// is ahead of the local calendar.
			wrong := prepareAt(t, clock.Fixed(instant, time.UTC)).Invoice
			if (wrong.CollectionDate != tc.wantDate) != tc.utcDiffers {
				t.Errorf("UTC clock date = %s, local = %s, utcDiffers = %v", wrong.CollectionDate, tc.wantDate, tc.utcDiffers)
			}
		})
	}
}
