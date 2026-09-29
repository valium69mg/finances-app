// Package health exposes the liveness/readiness endpoint.
package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

const pingTimeout = 2 * time.Second

// Pinger reports whether a dependency (the database) is reachable.
type Pinger interface {
	Ping(ctx context.Context) error
}

type response struct {
	Status string `json:"status"`
}

// Handler answers 200 {"status":"ok"} when the pinger succeeds and
// 503 {"status":"unavailable"} otherwise.
func Handler(p Pinger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
		defer cancel()

		status, body := http.StatusOK, response{Status: "ok"}
		if err := p.Ping(ctx); err != nil {
			slog.Warn("health check failed", "error", err)
			status, body = http.StatusServiceUnavailable, response{Status: "unavailable"}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	})
}
