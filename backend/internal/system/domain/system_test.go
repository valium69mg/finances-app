package domain_test

import (
	"math"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/system/domain"
)

func TestCPUPercent(t *testing.T) {
	tests := []struct {
		name          string
		before, after domain.CPUTimes
		want          float64
	}{
		{"quarter busy", domain.CPUTimes{Busy: 100, Total: 1000}, domain.CPUTimes{Busy: 150, Total: 1200}, 25},
		{"idle", domain.CPUTimes{Busy: 100, Total: 1000}, domain.CPUTimes{Busy: 100, Total: 1100}, 0},
		{"fully busy", domain.CPUTimes{Busy: 0, Total: 0}, domain.CPUTimes{Busy: 50, Total: 50}, 100},
		{"rounds to one decimal", domain.CPUTimes{}, domain.CPUTimes{Busy: 1, Total: 3}, 33.3},
		{"zero delta", domain.CPUTimes{Busy: 10, Total: 100}, domain.CPUTimes{Busy: 10, Total: 100}, 0},
		{"total went backwards", domain.CPUTimes{Busy: 10, Total: 100}, domain.CPUTimes{Busy: 5, Total: 50}, 0},
		{"busy went backwards", domain.CPUTimes{Busy: 10, Total: 100}, domain.CPUTimes{Busy: 5, Total: 200}, 0},
		{"busy above total is clamped", domain.CPUTimes{}, domain.CPUTimes{Busy: 120, Total: 100}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.CPUPercent(tt.before, tt.after); got != tt.want {
				t.Errorf("CPUPercent = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewMemoryDoesNotCountCacheAsUsed(t *testing.T) {
	// 2 GiB total, 1.5 GiB available (free plus reclaimable cache).
	m := domain.NewMemory(2<<30, 3<<29)
	if m.UsedBytes != 1<<29 || m.UsedPercent != 25 || m.AvailableBytes != 3<<29 {
		t.Errorf("memory = %+v", m)
	}
}

func TestNewMemoryClampsAvailable(t *testing.T) {
	m := domain.NewMemory(100, 150)
	if m.UsedBytes != 0 || m.AvailableBytes != 100 || m.UsedPercent != 0 {
		t.Errorf("memory = %+v", m)
	}
}

func TestNewDisk(t *testing.T) {
	d := domain.NewDisk(1000, 800, 200)
	if d.UsedPercent != 80 || !d.PathMonitored || d.UsedBytes != 800 {
		t.Errorf("disk = %+v", d)
	}
	// df's formula: 50 reserved bytes are neither used nor available, so the
	// percent is 750/950, not 750/1000.
	if got := domain.NewDisk(1000, 750, 200); got.UsedBytes != 750 || got.UsedPercent != 78.9 {
		t.Errorf("reserved blocks = %+v", got)
	}
	if got := domain.NewDisk(100, 500, 50); got.UsedBytes != 100 || got.UsedPercent != 100 {
		t.Errorf("used above total = %+v", got)
	}
	if got := domain.NewDisk(100, 0, 0); got.UsedPercent != 0 {
		t.Errorf("nothing used or available = %+v", got)
	}
}

func TestPercent(t *testing.T) {
	if got := domain.Percent(1, 0); got != 0 {
		t.Errorf("Percent(1, 0) = %v, want 0", got)
	}
	if got := domain.Percent(2, 3); math.Abs(got-66.7) > 1e-9 {
		t.Errorf("Percent(2, 3) = %v, want 66.7", got)
	}
}
