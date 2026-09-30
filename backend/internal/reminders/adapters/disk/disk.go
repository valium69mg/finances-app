// Package disk measures the usage of a filesystem with statfs.
package disk

import (
	"errors"
	"fmt"
	"math/bits"
	"syscall"
)

// Stat is the subset of statfs the probe needs, in filesystem blocks.
type Stat struct {
	Blocks uint64 // total data blocks
	Bfree  uint64 // free blocks, including the ones reserved for root
	Bavail uint64 // free blocks available to unprivileged users
	Bsize  uint64 // block size in bytes
}

// StatFunc reads the Stat of the filesystem that holds path.
type StatFunc func(path string) (Stat, error)

// Usage is the measured state of a filesystem, computed as df does.
type Usage struct {
	// TotalBytes is Blocks * Bsize, reserved blocks included.
	TotalBytes uint64
	// UsedBytes is (Blocks - Bfree) * Bsize.
	UsedBytes uint64
	// AvailBytes is Bavail * Bsize: what unprivileged users can still write.
	AvailBytes uint64
	// UsedPercent is used / (used + avail) * 100, from 0 to 100. It is df's
	// formula: the blocks reserved for root count neither as used nor as
	// available, so it can exceed UsedBytes / TotalBytes on a volume with
	// reserved blocks (ext4 reserves 5% by default).
	UsedPercent float64
}

// Probe measures the filesystem that holds a directory. Point it at an empty
// directory on the volume to watch: statfs reports the whole volume and the
// caller never sees its contents.
type Probe struct {
	path string
	stat StatFunc
}

// NewProbe builds a Probe for the directory at path, backed by statfs(2).
func NewProbe(path string) *Probe { return NewProbeWith(path, statfs) }

// NewProbeWith builds a Probe that reads the filesystem through stat, so tests
// can feed explicit values.
func NewProbeWith(path string, stat StatFunc) *Probe { return &Probe{path: path, stat: stat} }

func statfs(path string) (Stat, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Stat{}, err
	}
	// Field widths and signedness differ per platform.
	return Stat{Blocks: uint64(st.Blocks), Bfree: uint64(st.Bfree), Bavail: uint64(st.Bavail), Bsize: uint64(st.Bsize)}, nil
}

func (p *Probe) measure() (Usage, error) {
	st, err := p.stat(p.path)
	if err != nil {
		return Usage{}, fmt.Errorf("statfs %s: %w", p.path, err)
	}
	return Measure(st)
}

// UsedPercent returns the used share of the filesystem, from 0 to 100, as df
// reports it (see Usage.UsedPercent).
func (p *Probe) UsedPercent() (float64, error) {
	u, err := p.measure()
	if err != nil {
		return 0, err
	}
	return u.UsedPercent, nil
}

// Usage returns the size of the filesystem, the bytes in use and the bytes
// available to unprivileged users, on the same basis as UsedPercent. The system
// status module reads it through its own port.
func (p *Probe) Usage() (total, used, avail uint64, err error) {
	u, err := p.measure()
	if err != nil {
		return 0, 0, 0, err
	}
	return u.TotalBytes, u.UsedBytes, u.AvailBytes, nil
}

// Measure derives the usage from a Stat with df's formulas. A filesystem
// without blocks or a block size cannot be measured. Inconsistent counters are
// clamped (Bfree to Blocks, Bavail to Bfree) and byte counts that do not fit in
// 64 bits are an error.
func Measure(st Stat) (Usage, error) {
	if st.Blocks == 0 || st.Bsize == 0 {
		return Usage{}, errors.New("the filesystem reports no blocks")
	}
	bfree := min(st.Bfree, st.Blocks)
	bavail := min(st.Bavail, bfree)
	usedBlocks := st.Blocks - bfree

	total, err := blocksToBytes(st.Blocks, st.Bsize)
	if err != nil {
		return Usage{}, err
	}
	used, err := blocksToBytes(usedBlocks, st.Bsize)
	if err != nil {
		return Usage{}, err
	}
	avail, err := blocksToBytes(bavail, st.Bsize)
	if err != nil {
		return Usage{}, err
	}

	u := Usage{TotalBytes: total, UsedBytes: used, AvailBytes: avail}
	// usedBlocks + bavail <= Blocks, so the sum cannot overflow; the block size
	// cancels out, so the ratio is computed in blocks.
	if denom := usedBlocks + bavail; denom > 0 {
		u.UsedPercent = float64(usedBlocks) / float64(denom) * 100
	}
	return u, nil
}

func blocksToBytes(blocks, bsize uint64) (uint64, error) {
	hi, lo := bits.Mul64(blocks, bsize)
	if hi != 0 {
		return 0, errors.New("the filesystem size overflows 64 bits")
	}
	return lo, nil
}
