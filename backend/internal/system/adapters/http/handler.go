// Package systemhttp exposes the server status over HTTP, behind the
// authentication middleware passed to Register.
package systemhttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/platform/httpjson"
	"github.com/valium69mg/finances-app/backend/internal/system/app"
)

// Service is the use case the handler needs.
type Service interface {
	Status(ctx context.Context) (app.Report, error)
}

// Handler serves the /system routes.
type Handler struct {
	svc    Service
	logger *slog.Logger
}

// New builds a Handler. A nil logger selects slog.Default().
func New(svc Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{svc: svc, logger: logger}
}

// Register mounts the routes on mux, wrapped by requireAuth.
//
// TODO(phase 9, Household): there are no roles yet, so any authenticated user
// can read this. When Household lands, restrict it to the owner role.
func (h *Handler) Register(mux *http.ServeMux, requireAuth func(http.Handler) http.Handler) {
	mux.Handle("GET /system/status", requireAuth(http.HandlerFunc(h.status)))
}

type memoryDTO struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

type swapDTO struct {
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
}

type diskDTO struct {
	TotalBytes    uint64  `json:"total_bytes"`
	UsedBytes     uint64  `json:"used_bytes"`
	UsedPercent   float64 `json:"used_percent"`
	PathMonitored bool    `json:"path_monitored"`
}

type loadDTO struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

type statusDTO struct {
	CPUPercent       float64   `json:"cpu_percent"`
	Memory           memoryDTO `json:"memory"`
	Swap             *swapDTO  `json:"swap"`
	Disk             *diskDTO  `json:"disk"`
	UptimeSeconds    int64     `json:"uptime_seconds"`
	LoadAverage      loadDTO   `json:"load_average"`
	DiskAlertPercent int       `json:"disk_alert_percent"`
	RemindersEnabled bool      `json:"reminders_enabled"`
	SampledAt        string    `json:"sampled_at"`
}

func toDTO(r app.Report) statusDTO {
	dto := statusDTO{
		CPUPercent: r.CPUPercent,
		Memory: memoryDTO{
			TotalBytes: r.Memory.TotalBytes, UsedBytes: r.Memory.UsedBytes,
			AvailableBytes: r.Memory.AvailableBytes, UsedPercent: r.Memory.UsedPercent,
		},
		UptimeSeconds:    r.UptimeSeconds,
		LoadAverage:      loadDTO{One: r.Load.One, Five: r.Load.Five, Fifteen: r.Load.Fifteen},
		DiskAlertPercent: r.DiskAlertPercent,
		RemindersEnabled: r.RemindersEnabled,
		SampledAt:        r.SampledAt.Format(time.RFC3339),
	}
	if r.Swap != nil {
		dto.Swap = &swapDTO{TotalBytes: r.Swap.TotalBytes, UsedBytes: r.Swap.UsedBytes}
	}
	if r.Disk != nil {
		dto.Disk = &diskDTO{
			TotalBytes: r.Disk.TotalBytes, UsedBytes: r.Disk.UsedBytes,
			UsedPercent: r.Disk.UsedPercent, PathMonitored: r.Disk.PathMonitored,
		}
	}
	return dto
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	report, err := h.svc.Status(r.Context())
	switch {
	case err == nil:
		httpjson.WriteJSON(w, http.StatusOK, toDTO(report))
	case errors.Is(err, app.ErrUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// A cancelled request (client gone or timed out while sampling) has no
		// reader left; the same generic answer keeps the path simple.
		httpjson.WriteError(w, http.StatusServiceUnavailable, "system_unavailable")
	default:
		h.logger.Error("system status failed", "error", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "internal_error")
	}
}
