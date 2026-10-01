package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/platform/clock"
	"github.com/valium69mg/finances-app/backend/internal/taxfiling/app"
)

// Mexico City has no daylight saving time since 2022: a fixed -6h zone is exact.
var mexico = time.FixedZone("America/Mexico_City", -6*60*60)

// boundaries straddle local midnight (and the local month end) while UTC is
// already a day (or a month) ahead.
var boundaries = []struct {
	name, instant, wantDate, wantPreviousPeriod string
	utcDiffers                                  bool
}{
	{"evening before local midnight", "2026-10-01T02:30:00Z", "2026-09-30", "2026-08", true},
	{"after local midnight", "2026-10-01T06:30:00Z", "2026-10-01", "2026-09", false},
	{"evening of the last day of the month", "2026-11-01T05:30:00Z", "2026-10-31", "2026-09", true},
	{"after the local month change", "2026-11-01T06:30:00Z", "2026-11-01", "2026-10", false},
}

// fixtureAt is newFixture with its service rebuilt on the given clock.
func fixtureAt(now func() time.Time) *fixture {
	fx := newFixture()
	fx.svc = app.NewService(fx.repo, fx.invoices, fx.expenses, fx.settings, now, nil)
	return fx
}

func TestDefaultDatesAndPeriodFollowTheLocalClock(t *testing.T) {
	ctx := context.Background()
	for _, tc := range boundaries {
		t.Run(tc.name, func(t *testing.T) {
			instant, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			if differs := instant.UTC().Format("2006-01-02") != tc.wantDate; differs != tc.utcDiffers {
				t.Fatalf("fixture: UTC date differs from local = %v, want %v", differs, tc.utcDiffers)
			}

			fx := fixtureAt(clock.Fixed(instant, mexico))
			prev, err := fx.svc.Preview(ctx, "", d("0"))
			if err != nil {
				t.Fatal(err)
			}
			if prev.Declaration.Period != tc.wantPreviousPeriod {
				t.Errorf("default preview period = %s, want %s", prev.Declaration.Period, tc.wantPreviousPeriod)
			}
			res, err := fx.svc.Register(ctx, app.RegisterInput{
				Period: "2026-10", Payment: &app.PaymentInput{ISRPaid: d("1845.25"), IVAPaid: d("4827.59")},
			})
			if err != nil {
				t.Fatal(err)
			}
			if res.Filing.FilingDate != tc.wantDate || res.Filing.Payment == nil || res.Filing.Payment.Date != tc.wantDate {
				t.Errorf("default filing date = %s (payment %+v), want %s", res.Filing.FilingDate, res.Filing.Payment, tc.wantDate)
			}

			// The later payment of a pending filing defaults to today too.
			fx = fixtureAt(clock.Fixed(instant, mexico))
			if _, err := fx.svc.Register(ctx, app.RegisterInput{Period: "2026-10", Date: "2026-10-05"}); err != nil {
				t.Fatal(err)
			}
			paid, err := fx.svc.Pay(ctx, "2026-10", app.PaymentInput{ISRPaid: d("1845.25"), IVAPaid: d("4827.59")})
			if err != nil {
				t.Fatal(err)
			}
			if paid.Filing.Payment == nil || paid.Filing.Payment.Date != tc.wantDate {
				t.Errorf("default payment date = %+v, want %s", paid.Filing.Payment, tc.wantDate)
			}

			// The UTC clock this fix replaces gives the wrong day exactly when UTC
			// is ahead of the local calendar.
			wrong, err := fixtureAt(clock.Fixed(instant, time.UTC)).svc.Register(ctx, app.RegisterInput{Period: "2026-10"})
			if err != nil {
				t.Fatal(err)
			}
			if (wrong.Filing.FilingDate != tc.wantDate) != tc.utcDiffers {
				t.Errorf("UTC clock date = %s, local = %s, utcDiffers = %v", wrong.Filing.FilingDate, tc.wantDate, tc.utcDiffers)
			}
		})
	}
}
