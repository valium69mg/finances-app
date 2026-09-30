// Package domain holds the server status figures and the pure math behind them.
package domain

import "math"

// CPUTimes are the cumulative jiffies of the aggregate "cpu" line of /proc/stat.
// Busy and Total only make sense as a difference between two samples.
type CPUTimes struct {
	// Busy is every non-idle jiffy: Total - idle - iowait.
	Busy uint64
	// Total is user + nice + system + idle + iowait + irq + softirq + steal
	// (guest time is already inside user and nice).
	Total uint64
}

// CPUPercent is the share of busy time between two samples, from 0 to 100 with
// one decimal. No elapsed jiffies, or counters that went backwards (wrap or a
// reset), cannot be measured and report 0.
func CPUPercent(before, after CPUTimes) float64 {
	if after.Total <= before.Total || after.Busy < before.Busy {
		return 0
	}
	busy := float64(after.Busy - before.Busy)
	total := float64(after.Total - before.Total)
	return Round1(math.Min(100, busy/total*100))
}

// Memory is the RAM of the machine. Used is Total - Available, so the page
// cache (reclaimable) does not count as used.
type Memory struct {
	TotalBytes     uint64
	UsedBytes      uint64
	AvailableBytes uint64
	UsedPercent    float64
}

// NewMemory derives the used figures from MemTotal and MemAvailable.
func NewMemory(total, available uint64) Memory {
	available = min(available, total)
	m := Memory{TotalBytes: total, AvailableBytes: available, UsedBytes: total - available}
	m.UsedPercent = Percent(m.UsedBytes, total)
	return m
}

// Swap is the swap space; absent (nil in Status) when the machine has none.
type Swap struct {
	TotalBytes uint64
	UsedBytes  uint64
}

// Disk is the filesystem of the monitored path.
type Disk struct {
	TotalBytes  uint64
	UsedBytes   uint64
	UsedPercent float64
	// PathMonitored is true when the probe path was measured.
	PathMonitored bool
}

// NewDisk derives the used percentage from the byte counts.
func NewDisk(total, used uint64) Disk {
	used = min(used, total)
	return Disk{TotalBytes: total, UsedBytes: used, UsedPercent: Percent(used, total), PathMonitored: true}
}

// Load holds the 1, 5 and 15 minute load averages.
type Load struct{ One, Five, Fifteen float64 }

// Status is the current state of the server.
type Status struct {
	CPUPercent    float64
	Memory        Memory
	Swap          *Swap
	Disk          *Disk
	UptimeSeconds int64
	Load          Load
}

// Percent is part/total as a percentage with one decimal; 0 when total is 0.
func Percent(part, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return Round1(float64(part) / float64(total) * 100)
}

// Round1 rounds to one decimal.
func Round1(v float64) float64 { return math.Round(v*10) / 10 }
