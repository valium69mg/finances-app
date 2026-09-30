// Package disk measures the usage of a filesystem with statfs.
package disk

import (
	"errors"
	"fmt"
	"syscall"
)

// Probe measures the filesystem that holds a directory. Point it at an empty
// directory on the volume to watch: statfs reports the whole volume and the
// caller never sees its contents.
type Probe struct{ path string }

// NewProbe builds a Probe for the directory at path.
func NewProbe(path string) *Probe { return &Probe{path: path} }

// UsedPercent returns the used share of the filesystem, from 0 to 100, as
// (total - available) / total, where available is what unprivileged users can
// still use (Bavail).
func (p *Probe) UsedPercent() (float64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(p.path, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", p.path, err)
	}
	return PercentUsed(uint64(st.Blocks), uint64(st.Bavail))
}

// PercentUsed computes the used percentage from the total and available block
// counts. A filesystem without blocks cannot be measured.
func PercentUsed(total, avail uint64) (float64, error) {
	if total == 0 {
		return 0, errors.New("the filesystem reports no blocks")
	}
	if avail >= total {
		return 0, nil
	}
	return float64(total-avail) / float64(total) * 100, nil
}
