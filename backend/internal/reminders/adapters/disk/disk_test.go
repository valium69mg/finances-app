package disk_test

import (
	"errors"
	"math"
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

func TestProbeUsageOnTempDir(t *testing.T) {
	total, used, avail, err := disk.NewProbe(t.TempDir()).Usage()
	if err != nil {
		t.Fatal(err)
	}
	if total == 0 || used > total || avail > total {
		t.Fatalf("usage = used %d, avail %d of %d, want both within total", used, avail, total)
	}
	if _, _, _, err := disk.NewProbe(filepath.Join(t.TempDir(), "missing")).Usage(); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestProbeMissingPath(t *testing.T) {
	if _, err := disk.NewProbe(filepath.Join(t.TempDir(), "missing")).UsedPercent(); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestProbeInjectedStat(t *testing.T) {
	st := disk.Stat{Blocks: 1000, Bfree: 200, Bavail: 200, Bsize: 4096}
	var gotPath string
	p := disk.NewProbeWith("/probe", func(path string) (disk.Stat, error) {
		gotPath = path
		return st, nil
	})
	pct, err := p.UsedPercent()
	if err != nil || pct != 80 || gotPath != "/probe" {
		t.Fatalf("UsedPercent = %v, %v (path %q), want 80", pct, err, gotPath)
	}
	total, used, avail, err := p.Usage()
	if err != nil || total != 4_096_000 || used != 3_276_800 || avail != 819_200 {
		t.Fatalf("Usage = %d, %d, %d, %v", total, used, avail, err)
	}

	boom := errors.New("boom")
	failing := disk.NewProbeWith("/probe", func(string) (disk.Stat, error) { return disk.Stat{}, boom })
	if _, err := failing.UsedPercent(); !errors.Is(err, boom) {
		t.Fatalf("UsedPercent err = %v, want boom", err)
	}
	if _, _, _, err := failing.Usage(); !errors.Is(err, boom) {
		t.Fatalf("Usage err = %v, want boom", err)
	}
}

func TestMeasure(t *testing.T) {
	const bsize = 4096
	tests := []struct {
		name                 string
		st                   disk.Stat
		total, used, avail   uint64
		percent, percentSlop float64
		wantErr              bool
	}{
		{
			// 50 GB ext4, 5% reserved, almost empty: df reads 1%, not 5%.
			name:  "empty volume with 5% reserved",
			st:    disk.Stat{Blocks: 10_000, Bfree: 9_900, Bavail: 9_400, Bsize: bsize},
			total: 10_000 * bsize, used: 100 * bsize, avail: 9_400 * bsize,
			percent: 100.0 / 9_500 * 100, percentSlop: 1e-9,
		},
		{
			name:  "eighty percent used",
			st:    disk.Stat{Blocks: 1000, Bfree: 200, Bavail: 200, Bsize: bsize},
			total: 1000 * bsize, used: 800 * bsize, avail: 200 * bsize,
			percent: 80, percentSlop: 1e-9,
		},
		{
			name:  "reserved blocks are neither used nor available",
			st:    disk.Stat{Blocks: 1000, Bfree: 250, Bavail: 200, Bsize: bsize},
			total: 1000 * bsize, used: 750 * bsize, avail: 200 * bsize,
			percent: 750.0 / 950 * 100, percentSlop: 1e-9,
		},
		{
			name:  "full volume",
			st:    disk.Stat{Blocks: 1000, Bfree: 0, Bavail: 0, Bsize: bsize},
			total: 1000 * bsize, used: 1000 * bsize, avail: 0,
			percent: 100,
		},
		{
			name:  "full for users, root reserve left",
			st:    disk.Stat{Blocks: 1000, Bfree: 50, Bavail: 0, Bsize: bsize},
			total: 1000 * bsize, used: 950 * bsize, avail: 0,
			percent: 100,
		},
		{
			name:  "completely free",
			st:    disk.Stat{Blocks: 1000, Bfree: 1000, Bavail: 1000, Bsize: bsize},
			total: 1000 * bsize, used: 0, avail: 1000 * bsize,
			percent: 0,
		},
		{
			name:  "free above blocks is clamped",
			st:    disk.Stat{Blocks: 100, Bfree: 150, Bavail: 150, Bsize: bsize},
			total: 100 * bsize, used: 0, avail: 100 * bsize,
			percent: 0,
		},
		{
			name:  "available above free is clamped",
			st:    disk.Stat{Blocks: 100, Bfree: 40, Bavail: 90, Bsize: bsize},
			total: 100 * bsize, used: 60 * bsize, avail: 40 * bsize,
			percent: 60, percentSlop: 1e-9,
		},
		{
			name:  "nothing used and nothing available",
			st:    disk.Stat{Blocks: 100, Bfree: 100, Bavail: 0, Bsize: bsize},
			total: 100 * bsize, used: 0, avail: 0,
			percent: 0,
		},
		{name: "zero blocks", st: disk.Stat{Bsize: bsize}, wantErr: true},
		{name: "zero block size", st: disk.Stat{Blocks: 100, Bfree: 50, Bavail: 50}, wantErr: true},
		{
			name:    "byte count overflow",
			st:      disk.Stat{Blocks: math.MaxUint64, Bfree: 0, Bavail: 0, Bsize: 2},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := disk.Measure(tt.st)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.TotalBytes != tt.total || got.UsedBytes != tt.used || got.AvailBytes != tt.avail {
				t.Errorf("bytes = total %d used %d avail %d, want %d %d %d",
					got.TotalBytes, got.UsedBytes, got.AvailBytes, tt.total, tt.used, tt.avail)
			}
			if math.Abs(got.UsedPercent-tt.percent) > tt.percentSlop {
				t.Errorf("UsedPercent = %v, want %v", got.UsedPercent, tt.percent)
			}
		})
	}
}
