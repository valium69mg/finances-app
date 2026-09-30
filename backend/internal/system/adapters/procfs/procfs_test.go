package procfs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/system/adapters/procfs"
)

func TestReaderOnFixtures(t *testing.T) {
	r := procfs.New(filepath.Join("testdata", "ok"))

	cpu, err := r.CPUTimes()
	if err != nil {
		t.Fatal(err)
	}
	// total = user+nice+system+idle+iowait+irq+softirq+steal; guest is not added.
	if cpu.Total != 98315055 || cpu.Busy != 6468558 {
		t.Errorf("cpu = %+v, want Total 98315055 Busy 6468558", cpu)
	}

	mem, swap, err := r.Memory()
	if err != nil {
		t.Fatal(err)
	}
	if mem.TotalBytes != 2005412*1024 || mem.AvailableBytes != 812348*1024 || mem.UsedBytes != (2005412-812348)*1024 {
		t.Errorf("memory = %+v", mem)
	}
	if mem.UsedPercent != 59.5 {
		t.Errorf("used percent = %v, want 59.5 (the page cache is not used memory)", mem.UsedPercent)
	}
	if swap == nil || swap.TotalBytes != 1048572*1024 || swap.UsedBytes != (1048572-991228)*1024 {
		t.Errorf("swap = %+v", swap)
	}

	up, err := r.UptimeSeconds()
	if err != nil || up != 1234567 {
		t.Errorf("uptime = %d, %v", up, err)
	}

	load, err := r.Load()
	if err != nil || load.One != 0.52 || load.Five != 0.58 || load.Fifteen != 0.59 {
		t.Errorf("load = %+v, %v", load, err)
	}
}

func TestMemoryWithoutSwap(t *testing.T) {
	_, swap, err := procfs.New(filepath.Join("testdata", "noswap")).Memory()
	if err != nil {
		t.Fatal(err)
	}
	if swap != nil {
		t.Errorf("swap = %+v, want nil", swap)
	}
}

func TestMissingRootFails(t *testing.T) {
	r := procfs.New(filepath.Join(t.TempDir(), "nope"))
	if _, err := r.CPUTimes(); err == nil {
		t.Error("CPUTimes: expected an error")
	}
	if _, _, err := r.Memory(); err == nil {
		t.Error("Memory: expected an error")
	}
	if _, err := r.UptimeSeconds(); err == nil {
		t.Error("UptimeSeconds: expected an error")
	}
	if _, err := r.Load(); err == nil {
		t.Error("Load: expected an error")
	}
}

func TestDefaultRootIsProc(t *testing.T) {
	if _, err := os.Stat("/proc/stat"); err != nil {
		t.Skip("no /proc on this host")
	}
	cpu, err := procfs.New("").CPUTimes()
	if err != nil || cpu.Total == 0 || cpu.Busy > cpu.Total {
		t.Fatalf("cpu = %+v, %v", cpu, err)
	}
}

func TestParseStatRejectsMalformed(t *testing.T) {
	for name, in := range map[string]string{
		"empty":        "",
		"no cpu line":  "cpu0 1 2 3 4\nintr 1\n",
		"short":        "cpu 1 2 3\n",
		"not a number": "cpu 1 2 x 4 5 6 7 8\n",
		"negative":     "cpu 1 2 -3 4 5 6 7 8\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := procfs.ParseStat([]byte(in)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestParseStatOldKernelWithoutSteal(t *testing.T) {
	cpu, err := procfs.ParseStat([]byte("cpu  10 0 10 70 10\n"))
	if err != nil || cpu.Total != 100 || cpu.Busy != 20 {
		t.Errorf("cpu = %+v, %v", cpu, err)
	}
}

func TestParseMeminfoRejectsMalformed(t *testing.T) {
	for name, in := range map[string]string{
		"empty":            "",
		"no available":     "MemTotal: 1000 kB\nMemFree: 10 kB\n",
		"zero total":       "MemTotal: 0 kB\nMemAvailable: 0 kB\n",
		"bad number":       "MemTotal: lots kB\nMemAvailable: 1 kB\n",
		"missing value":    "MemTotal:\nMemAvailable: 1 kB\n",
		"overflow on kB":   "MemTotal: 18446744073709551615 kB\nMemAvailable: 1 kB\n",
		"negative counter": "MemTotal: -5 kB\nMemAvailable: 1 kB\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := procfs.ParseMeminfo([]byte(in)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestParseMeminfoSwapFreeAboveTotalIsClamped(t *testing.T) {
	_, swap, err := procfs.ParseMeminfo([]byte("MemTotal: 10 kB\nMemAvailable: 5 kB\nSwapTotal: 4 kB\nSwapFree: 9 kB\n"))
	if err != nil || swap == nil || swap.UsedBytes != 0 {
		t.Errorf("swap = %+v, %v", swap, err)
	}
}

func TestParseUptimeAndLoadavgRejectMalformed(t *testing.T) {
	for _, in := range []string{"", "abc 1.0", "-5.0 1.0", "NaN 1", "+Inf 1"} {
		if _, err := procfs.ParseUptime([]byte(in)); err == nil {
			t.Errorf("uptime %q: expected an error", in)
		}
	}
	for _, in := range []string{"", "0.1 0.2", "a b c", "0.1 -0.2 0.3", "NaN 0 0"} {
		if _, err := procfs.ParseLoadavg([]byte(in)); err == nil {
			t.Errorf("loadavg %q: expected an error", in)
		}
	}
	if _, err := procfs.ParseLoadavg([]byte(strings.Repeat("0.00 ", 3))); err != nil {
		t.Errorf("idle machine: %v", err)
	}
}
