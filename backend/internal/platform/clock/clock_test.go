package clock_test

import (
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/platform/clock"
)

// Mexico City has no daylight saving time since 2022: a fixed -6h zone is exact.
var mexico = time.FixedZone("America/Mexico_City", -6*60*60)

func TestFixedFollowsTheLocalCalendarDay(t *testing.T) {
	for _, tc := range []struct {
		name      string
		instant   string
		wantLocal string
		wantUTC   string
	}{
		{"evening before midnight UTC", "2026-10-01T02:30:00Z", "2026-09-30", "2026-10-01"},
		{"after local midnight", "2026-10-01T06:30:00Z", "2026-10-01", "2026-10-01"},
		{"evening of the last day of the month", "2026-11-01T05:30:00Z", "2026-10-31", "2026-11-01"},
		{"after local month change", "2026-11-01T06:30:00Z", "2026-11-01", "2026-11-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instant, err := time.Parse(time.RFC3339, tc.instant)
			if err != nil {
				t.Fatal(err)
			}
			now := clock.Fixed(instant, mexico)()
			if got := now.Format("2006-01-02"); got != tc.wantLocal {
				t.Errorf("local date = %s, want %s", got, tc.wantLocal)
			}
			if got := instant.UTC().Format("2006-01-02"); got != tc.wantUTC {
				t.Errorf("UTC date = %s, want %s", got, tc.wantUTC)
			}
			if !now.Equal(instant) {
				t.Errorf("clock moved the instant: %s != %s", now, instant)
			}
		})
	}
}

func TestInReportsTheGivenZone(t *testing.T) {
	if got := clock.In(mexico)().Location(); got != mexico {
		t.Errorf("location = %s, want %s", got, mexico)
	}
}
