package disk_test

import (
	"path/filepath"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/reminders/adapters/disk"
)

func TestProbeOnTempDir(t *testing.T) {
	used, err := disk.NewProbe(t.TempDir()).UsedPercent()
	if err != nil {
		t.Fatal(err)
	}
	if used < 0 || used > 100 {
		t.Fatalf("used = %v, want a percentage between 0 and 100", used)
	}
}

func TestProbeUsage(t *testing.T) {
	total, used, err := disk.NewProbe(t.TempDir()).Usage()
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 || used > total {
		t.Fatalf("usage = %d of %d, want 0 < used <= total", used, total)
	}
	if _, _, err := disk.NewProbe(filepath.Join(t.TempDir(), "missing")).Usage(); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestProbeMissingPath(t *testing.T) {
	if _, err := disk.NewProbe(filepath.Join(t.TempDir(), "missing")).UsedPercent(); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestPercentUsed(t *testing.T) {
	tests := []struct {
		name         string
		total, avail uint64
		want         float64
		wantErr      bool
	}{
		{"half used", 200, 100, 50, false},
		{"empty", 200, 200, 0, false},
		{"full", 200, 0, 100, false},
		{"eighty percent", 1000, 200, 80, false},
		{"available above total is clamped", 100, 150, 0, false},
		{"no blocks", 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := disk.PercentUsed(tt.total, tt.avail)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("PercentUsed = %v, want %v", got, tt.want)
			}
		})
	}
}
