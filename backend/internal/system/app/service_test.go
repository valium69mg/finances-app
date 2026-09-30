package app_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/system/app"
	"github.com/valium69mg/finances-app/backend/internal/system/domain"
)

type fakeProc struct {
	cpu      []domain.CPUTimes // returned in order, one per call
	cpuCalls int
	cpuErr   error
	memErr   error
	swap     *domain.Swap
}

func (f *fakeProc) CPUTimes() (domain.CPUTimes, error) {
	if f.cpuErr != nil {
		return domain.CPUTimes{}, f.cpuErr
	}
	v := f.cpu[min(f.cpuCalls, len(f.cpu)-1)]
	f.cpuCalls++
	return v, nil
}

func (f *fakeProc) Memory() (domain.Memory, *domain.Swap, error) {
	return domain.NewMemory(2000, 500), f.swap, f.memErr
}
func (f *fakeProc) UptimeSeconds() (int64, error) { return 3600, nil }
func (f *fakeProc) Load() (domain.Load, error) {
	return domain.Load{One: 0.5, Five: 0.4, Fifteen: 0.3}, nil
}

type fakeDisk struct {
	total, used uint64
	err         error
}

func (f fakeDisk) Usage() (uint64, uint64, error) { return f.total, f.used, f.err }

var now = time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

func options() app.Options {
	return app.Options{
		DiskAlertPercent: 80, RemindersEnabled: true,
		Now:    func() time.Time { return now.In(time.FixedZone("x", -6*3600)) },
		Wait:   func(context.Context, time.Duration) error { return nil },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func proc() *fakeProc {
	return &fakeProc{cpu: []domain.CPUTimes{{Busy: 100, Total: 1000}, {Busy: 150, Total: 1200}}}
}

func TestStatusComposesEveryFigure(t *testing.T) {
	p := proc()
	p.swap = &domain.Swap{TotalBytes: 1000, UsedBytes: 100}
	r, err := app.NewService(p, fakeDisk{total: 1000, used: 850}, options()).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.CPUPercent != 25 {
		t.Errorf("cpu = %v, want 25", r.CPUPercent)
	}
	if r.Memory.UsedBytes != 1500 || r.Memory.UsedPercent != 75 {
		t.Errorf("memory = %+v", r.Memory)
	}
	if r.Swap == nil || r.Swap.UsedBytes != 100 {
		t.Errorf("swap = %+v", r.Swap)
	}
	if r.Disk == nil || r.Disk.UsedPercent != 85 || !r.Disk.PathMonitored {
		t.Errorf("disk = %+v", r.Disk)
	}
	if r.UptimeSeconds != 3600 || r.Load.One != 0.5 {
		t.Errorf("uptime/load = %d %+v", r.UptimeSeconds, r.Load)
	}
	if r.DiskAlertPercent != 80 || !r.RemindersEnabled {
		t.Errorf("alert config = %d %v", r.DiskAlertPercent, r.RemindersEnabled)
	}
	if !r.SampledAt.Equal(now) || r.SampledAt.Location() != time.UTC {
		t.Errorf("sampled_at = %v, want %v in UTC", r.SampledAt, now)
	}
}

func TestStatusWaitsBetweenCPUReadings(t *testing.T) {
	var waited time.Duration
	var cpuCallsAtWait int
	p := proc()
	o := options()
	o.Wait = func(_ context.Context, d time.Duration) error {
		waited = d
		cpuCallsAtWait = p.cpuCalls
		return nil
	}
	if _, err := app.NewService(p, nil, o).Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	if waited != app.DefaultSampleInterval || cpuCallsAtWait != 1 || p.cpuCalls != 2 {
		t.Errorf("waited %v after %d reads, %d reads total", waited, cpuCallsAtWait, p.cpuCalls)
	}
}

func TestStatusStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	o := options()
	o.Wait = nil // the real wait must notice the cancelled context
	start := time.Now()
	_, err := app.NewService(proc(), nil, o).Status(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if time.Since(start) > app.DefaultSampleInterval/2 {
		t.Errorf("the wait did not stop on cancel")
	}
}

func TestStatusDiskFailureLeavesDiskOut(t *testing.T) {
	for name, d := range map[string]app.DiskUsage{
		"error":    fakeDisk{err: errors.New("statfs /probe: no such file")},
		"no bytes": fakeDisk{},
		"nil":      nil,
	} {
		t.Run(name, func(t *testing.T) {
			r, err := app.NewService(proc(), d, options()).Status(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if r.Disk != nil {
				t.Errorf("disk = %+v, want nil", r.Disk)
			}
			if r.Memory.TotalBytes == 0 {
				t.Error("the rest of the status must still work")
			}
		})
	}
}

func TestStatusHostFailureIsUnavailable(t *testing.T) {
	secret := errors.New("open /proc/stat: permission denied")
	for name, p := range map[string]*fakeProc{
		"cpu":    {cpuErr: secret},
		"memory": {cpu: proc().cpu, memErr: secret},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := app.NewService(p, nil, options()).Status(context.Background())
			if !errors.Is(err, app.ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
			if errors.Is(err, secret) || err.Error() == secret.Error() {
				t.Error("the raw cause must not be part of the error")
			}
		})
	}
}
