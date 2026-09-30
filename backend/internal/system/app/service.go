// Package app reads the current server status. It is view only: the disk email
// alert lives in the reminders runner and does not depend on this module.
package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/system/domain"
)

// ErrUnavailable means the host figures cannot be read (for example /proc is
// not mounted). The cause is logged, never returned to callers.
var ErrUnavailable = errors.New("system status is unavailable")

// DefaultSampleInterval is the gap between the two CPU readings.
const DefaultSampleInterval = 250 * time.Millisecond

// Proc is the host's kernel counters (Linux /proc).
type Proc interface {
	CPUTimes() (domain.CPUTimes, error)
	// Memory returns the RAM and, when the machine has swap, the swap space (nil otherwise).
	Memory() (domain.Memory, *domain.Swap, error)
	UptimeSeconds() (int64, error)
	Load() (domain.Load, error)
}

// DiskUsage measures the filesystem of the monitored path: its size, the bytes
// in use and the bytes available to unprivileged users (df's figures). It is
// optional: a nil DiskUsage or a failing one leaves the disk out of the status.
type DiskUsage interface {
	Usage() (total, used, available uint64, err error)
}

// Options configure the service.
type Options struct {
	// DiskAlertPercent and RemindersEnabled echo the e-mail alert configuration
	// so the page can say when a warning is sent.
	DiskAlertPercent int
	RemindersEnabled bool
	// SampleInterval is the gap between the CPU readings (default 250 ms).
	SampleInterval time.Duration
	Now            func() time.Time
	// Wait blocks for d or until ctx is done; tests replace it.
	Wait   func(ctx context.Context, d time.Duration) error
	Logger *slog.Logger
}

// Report is the status plus the alert configuration and the sampling time.
type Report struct {
	domain.Status
	DiskAlertPercent int
	RemindersEnabled bool
	SampledAt        time.Time
}

// Service builds the status report.
type Service struct {
	proc Proc
	disk DiskUsage
	opts Options
}

// NewService builds a Service. disk may be nil.
func NewService(proc Proc, disk DiskUsage, opts Options) *Service {
	if opts.SampleInterval <= 0 {
		opts.SampleInterval = DefaultSampleInterval
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Wait == nil {
		opts.Wait = wait
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Service{proc: proc, disk: disk, opts: opts}
}

func wait(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Status samples the CPU twice (stateless: nothing is kept between calls) and
// reads the rest of the figures. A host read failure is ErrUnavailable; a
// cancelled context returns its error. A disk that cannot be measured is left
// out (Disk nil) and the rest still works.
func (s *Service) Status(ctx context.Context) (Report, error) {
	first, err := s.proc.CPUTimes()
	if err != nil {
		return Report{}, s.unavailable("read cpu", err)
	}
	if err := s.opts.Wait(ctx, s.opts.SampleInterval); err != nil {
		return Report{}, err
	}
	second, err := s.proc.CPUTimes()
	if err != nil {
		return Report{}, s.unavailable("read cpu", err)
	}
	mem, swap, err := s.proc.Memory()
	if err != nil {
		return Report{}, s.unavailable("read memory", err)
	}
	uptime, err := s.proc.UptimeSeconds()
	if err != nil {
		return Report{}, s.unavailable("read uptime", err)
	}
	load, err := s.proc.Load()
	if err != nil {
		return Report{}, s.unavailable("read load", err)
	}
	return Report{
		Status: domain.Status{
			CPUPercent: domain.CPUPercent(first, second), Memory: mem, Swap: swap,
			Disk: s.diskStatus(), UptimeSeconds: uptime, Load: load,
		},
		DiskAlertPercent: s.opts.DiskAlertPercent,
		RemindersEnabled: s.opts.RemindersEnabled,
		SampledAt:        s.opts.Now().UTC(),
	}, nil
}

func (s *Service) diskStatus() *domain.Disk {
	if s.disk == nil {
		return nil
	}
	total, used, available, err := s.disk.Usage()
	if err != nil || total == 0 {
		// Debug: the page polls, and dev has no probe mount, so this would repeat.
		s.opts.Logger.Debug("system status: the disk cannot be measured", "error", err)
		return nil
	}
	d := domain.NewDisk(total, used, available)
	return &d
}

func (s *Service) unavailable(what string, err error) error {
	s.opts.Logger.Error("system status: "+what+" failed", "error", err)
	return ErrUnavailable
}
