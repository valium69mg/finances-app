// Package procfs reads the host counters from the Linux /proc filesystem.
// These files are not namespaced by Docker, so inside a container they still
// describe the whole machine.
package procfs

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/valium69mg/finances-app/backend/internal/system/domain"
)

// DefaultRoot is where the kernel mounts procfs.
const DefaultRoot = "/proc"

// maxFileBytes bounds what is read from a proc file (they are a few KB).
const maxFileBytes = 1 << 20

// Reader reads proc files under a root directory, which tests point at fixtures.
type Reader struct{ root string }

// New builds a Reader. An empty root selects /proc.
func New(root string) *Reader {
	if root == "" {
		root = DefaultRoot
	}
	return &Reader{root: root}
}

func (r *Reader) read(name string) ([]byte, error) {
	f, err := os.Open(filepath.Join(r.root, name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxFileBytes))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return b, nil
}

// CPUTimes parses the aggregate "cpu" line of /proc/stat.
func (r *Reader) CPUTimes() (domain.CPUTimes, error) {
	b, err := r.read("stat")
	if err != nil {
		return domain.CPUTimes{}, err
	}
	return ParseStat(b)
}

// Memory parses /proc/meminfo: RAM from MemTotal and MemAvailable, swap from
// SwapTotal and SwapFree (nil when the machine has no swap).
func (r *Reader) Memory() (domain.Memory, *domain.Swap, error) {
	b, err := r.read("meminfo")
	if err != nil {
		return domain.Memory{}, nil, err
	}
	return ParseMeminfo(b)
}

// UptimeSeconds parses the first field of /proc/uptime.
func (r *Reader) UptimeSeconds() (int64, error) {
	b, err := r.read("uptime")
	if err != nil {
		return 0, err
	}
	return ParseUptime(b)
}

// Load parses the three load averages of /proc/loadavg.
func (r *Reader) Load() (domain.Load, error) {
	b, err := r.read("loadavg")
	if err != nil {
		return domain.Load{}, err
	}
	return ParseLoadavg(b)
}

// ParseStat reads the "cpu " line: user nice system idle iowait irq softirq
// steal [guest guest_nice]. Busy is total - idle - iowait; guest time is left
// out of the total because the kernel already counts it in user and nice.
func ParseStat(b []byte) (domain.CPUTimes, error) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || f[0] != "cpu" {
			continue
		}
		if len(f) < 5 { // user nice system idle are the minimum
			return domain.CPUTimes{}, errors.New("stat: short cpu line")
		}
		var v [8]uint64 // user nice system idle iowait irq softirq steal; missing = 0
		for i := 0; i < len(v) && i+1 < len(f); i++ {
			n, err := strconv.ParseUint(f[i+1], 10, 64)
			if err != nil {
				return domain.CPUTimes{}, fmt.Errorf("stat: bad counter %q", f[i+1])
			}
			v[i] = n
		}
		var total uint64
		for _, n := range v {
			total += n
		}
		idle := v[3] + v[4]
		if idle > total {
			return domain.CPUTimes{}, errors.New("stat: inconsistent counters")
		}
		return domain.CPUTimes{Busy: total - idle, Total: total}, nil
	}
	return domain.CPUTimes{}, errors.New("stat: no cpu line")
}

// ParseMeminfo reads MemTotal, MemAvailable, SwapTotal and SwapFree (in kB).
func ParseMeminfo(b []byte) (domain.Memory, *domain.Swap, error) {
	vals := map[string]uint64{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		switch key {
		case "MemTotal", "MemAvailable", "SwapTotal", "SwapFree":
			f := strings.Fields(rest)
			if len(f) == 0 {
				return domain.Memory{}, nil, fmt.Errorf("meminfo: empty %s", key)
			}
			n, err := strconv.ParseUint(f[0], 10, 64)
			if err != nil || n > math.MaxUint64/1024 {
				return domain.Memory{}, nil, fmt.Errorf("meminfo: bad %s", key)
			}
			vals[key] = n * 1024
		}
	}
	total, okT := vals["MemTotal"]
	avail, okA := vals["MemAvailable"]
	if !okT || !okA || total == 0 {
		return domain.Memory{}, nil, errors.New("meminfo: MemTotal or MemAvailable missing")
	}
	mem := domain.NewMemory(total, avail)
	var swap *domain.Swap
	if st := vals["SwapTotal"]; st > 0 {
		swap = &domain.Swap{TotalBytes: st, UsedBytes: st - min(vals["SwapFree"], st)}
	}
	return mem, swap, nil
}

// ParseUptime reads the seconds since boot (the first field).
func ParseUptime(b []byte) (int64, error) {
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0, errors.New("uptime: empty")
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) || v > math.MaxInt64/2 {
		return 0, errors.New("uptime: bad value")
	}
	return int64(v), nil
}

// ParseLoadavg reads "1min 5min 15min running/total lastpid".
func ParseLoadavg(b []byte) (domain.Load, error) {
	f := strings.Fields(string(b))
	if len(f) < 3 {
		return domain.Load{}, errors.New("loadavg: short line")
	}
	var v [3]float64
	for i := range v {
		n, err := strconv.ParseFloat(f[i], 64)
		if err != nil || n < 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			return domain.Load{}, errors.New("loadavg: bad value")
		}
		v[i] = n
	}
	return domain.Load{One: v[0], Five: v[1], Fifteen: v[2]}, nil
}
