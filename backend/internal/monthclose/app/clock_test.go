package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/monthclose/app"
	monthclose "github.com/valium69mg/finances-app/backend/internal/monthclose/domain"
	"github.com/valium69mg/finances-app/backend/internal/platform/clock"
)

// Mexico City has no daylight saving time since 2022: a fixed -6h zone is exact.
var mexico = time.FixedZone("America/Mexico_City", -6*60*60)

// boundaries straddle local midnight (and the local month end) while UTC is
// already a day (or a month) ahead.
var boundaries = []struct {
	name, instant, current, previous, future string
	utcDiffers                               bool
}{
	{"evening before local midnight", "2026-10-01T02:30:00Z", "2026-09", "2026-08", "2026-10", true},
	{"after local midnight", "2026-10-01T06:30:00Z", "2026-10", "2026-09", "2026-11", false},
	{"evening of the last day of the month", "2026-11-01T05:30:00Z", "2026-10", "2026-09", "2026-11", true},
	{"after the local month change", "2026-11-01T06:30:00Z", "2026-11", "2026-10", "2026-12", false},
}

// fixtureAt is newFixture with its service rebuilt on the given clock.
func fixtureAt(now func() time.Time) *fixture {
	fx := newFixture()
	fx.svc = app.NewService(fx.repo, fx.mvs, fx.settings, fx.filings, now, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return fx
}

func TestCurrentAndFuturePeriodsFollowTheLocalClock(t *testing.T) {
	ctx := context.Background()
	for _, tc := range boundaries {
		t.Run(tc.name, func(t *testing.T) {
			instant, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			if differs := instant.UTC().Format("2006-01") != tc.current; differs != tc.utcDiffers {
				t.Fatalf("fixture: UTC month differs from local = %v, want %v", differs, tc.utcDiffers)
			}

			fx := fixtureAt(clock.Fixed(instant, mexico))
			prev, err := fx.svc.Preview(ctx, "")
			if err != nil || prev.Close.Period != tc.previous {
				t.Errorf("default period = %q, %v; want %s", prev.Close.Period, err, tc.previous)
			}
			if _, err := fx.svc.Create(ctx, tc.future); !errors.Is(err, monthclose.ErrInvalidInput) {
				t.Errorf("closing %s: err = %v, want ErrInvalidInput (future)", tc.future, err)
			}
			c, err := fx.svc.Create(ctx, tc.current)
			if err != nil {
				t.Fatalf("closing the current cycle %s: %v", tc.current, err)
			}
			// The stored instant stays the real moment, in UTC.
			if !c.ClosedAt.Equal(instant) || c.ClosedAt.Location() != time.UTC {
				t.Errorf("ClosedAt = %s, want %s in UTC", c.ClosedAt, instant)
			}

			// The UTC clock this fix replaces accepts the local future period
			// exactly when UTC is already in the next month.
			_, err = fixtureAt(clock.Fixed(instant, time.UTC)).svc.Create(ctx, tc.future)
			if accepted := err == nil; accepted != tc.utcDiffers {
				t.Errorf("UTC clock accepted %s = %v (err %v), utcDiffers = %v", tc.future, accepted, err, tc.utcDiffers)
			}
		})
	}
}
