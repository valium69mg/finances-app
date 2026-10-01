package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/income/app"
	"github.com/valium69mg/finances-app/backend/internal/platform/clock"
	"github.com/valium69mg/finances-app/backend/internal/settings/domain/settingstest"
)

// Mexico City has no daylight saving time since 2022: a fixed -6h zone is exact.
var mexico = time.FixedZone("America/Mexico_City", -6*60*60)

// boundaries straddle local midnight (and the local month end) while UTC is
// already a day (or a month) ahead.
var boundaries = []struct {
	name, instant, wantDate, wantMonth string
	utcDiffers                         bool
}{
	{"evening before local midnight", "2026-10-01T02:30:00Z", "2026-09-30", "2026-09", true},
	{"after local midnight", "2026-10-01T06:30:00Z", "2026-10-01", "2026-10", false},
	{"evening of the last day of the month", "2026-11-01T05:30:00Z", "2026-10-31", "2026-10", true},
	{"after the local month change", "2026-11-01T06:30:00Z", "2026-11-01", "2026-11", false},
}

func serviceAt(now func() time.Time) (*app.Service, *fakeRepo) {
	cfg := settingstest.RealConfig()
	cfg.PaymentMethods = []string{"Efectivo", "Débito", "Crédito", "Transferencia"}
	repo := newFakeRepo()
	return app.NewService(repo, &fakeSettings{cfg: cfg}, now), repo
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDefaultDateAndMonthFollowTheLocalClock(t *testing.T) {
	ctx := context.Background()
	for _, tc := range boundaries {
		t.Run(tc.name, func(t *testing.T) {
			instant := mustParse(t, tc.instant)
			if differs := instant.UTC().Format("2006-01-02") != tc.wantDate; differs != tc.utcDiffers {
				t.Fatalf("fixture: UTC date differs from local = %v, want %v", differs, tc.utcDiffers)
			}

			svc, repo := serviceAt(clock.Fixed(instant, mexico))
			res, err := svc.Create(ctx, app.Input{Description: "Sueldo", Category: "Sueldo", Amount: d("1000")})
			if err != nil {
				t.Fatal(err)
			}
			if res.Movement.Date != tc.wantDate {
				t.Errorf("default date = %s, want %s", res.Movement.Date, tc.wantDate)
			}
			if res.Summary.Month != tc.wantMonth {
				t.Errorf("summary month = %s, want %s", res.Summary.Month, tc.wantMonth)
			}
			if _, err := svc.List(ctx, "", 0); err != nil {
				t.Fatal(err)
			}
			if got := repo.listFrom[:7]; got != tc.wantMonth {
				t.Errorf("default list month = %s, want %s", got, tc.wantMonth)
			}
			sum, err := svc.MonthSummary(ctx, "")
			if err != nil || sum.Month != tc.wantMonth {
				t.Errorf("default month summary = %q, %v; want %s", sum.Month, err, tc.wantMonth)
			}

			// The UTC clock this fix replaces gives the wrong day exactly when UTC
			// is ahead of the local calendar.
			utcSvc, _ := serviceAt(clock.Fixed(instant, time.UTC))
			wrong, err := utcSvc.Create(ctx, app.Input{Description: "Sueldo", Category: "Sueldo", Amount: d("1000")})
			if err != nil {
				t.Fatal(err)
			}
			if (wrong.Movement.Date != tc.wantDate) != tc.utcDiffers {
				t.Errorf("UTC clock date = %s, local = %s, utcDiffers = %v", wrong.Movement.Date, tc.wantDate, tc.utcDiffers)
			}
		})
	}
}
