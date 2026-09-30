package domain_test

import (
	"testing"

	dashboard "github.com/valium69mg/finances-app/backend/internal/dashboard/domain"
)

func TestNewCycleProgress(t *testing.T) {
	tests := []struct {
		name, from, to, today string
		day, days             int
	}{
		{"first day", "2026-10-01", "2026-10-31", "2026-10-01", 1, 31},
		{"middle", "2026-10-01", "2026-10-31", "2026-10-15", 15, 31},
		{"last day", "2026-09-30", "2026-10-30", "2026-10-30", 31, 31},
		{"before the cycle", "2026-10-01", "2026-10-31", "2026-08-01", 0, 31},
		{"after the cycle", "2026-10-01", "2026-10-31", "2026-12-01", 31, 31},
		{"leap february", "2028-02-01", "2028-02-29", "2028-02-29", 29, 29},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dashboard.NewCycleProgress(tt.from, tt.to, tt.today)
			if err != nil || got.Day != tt.day || got.Days != tt.days || got.Today != tt.today {
				t.Errorf("progress = %+v, %v, want day %d of %d", got, err, tt.day, tt.days)
			}
		})
	}
	if _, err := dashboard.NewCycleProgress("x", "2026-10-31", "2026-10-01"); err == nil {
		t.Error("want an error for a malformed date")
	}
}
