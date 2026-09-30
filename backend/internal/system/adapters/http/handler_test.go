package systemhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	systemhttp "github.com/valium69mg/finances-app/backend/internal/system/adapters/http"
	"github.com/valium69mg/finances-app/backend/internal/system/app"
	"github.com/valium69mg/finances-app/backend/internal/system/domain"
)

type fakeService struct {
	out   app.Report
	err   error
	calls int
}

func (f *fakeService) Status(context.Context) (app.Report, error) {
	f.calls++
	return f.out, f.err
}

func requireGoodToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func get(svc *fakeService, auth bool) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	systemhttp.New(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux, requireGoodToken)
	req := httptest.NewRequest(http.MethodGet, "/system/status", nil)
	if auth {
		req.Header.Set("Authorization", "Bearer good")
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func report() app.Report {
	d := domain.NewDisk(50_000_000_000, 41_000_000_000)
	return app.Report{
		Status: domain.Status{
			CPUPercent:    12.5,
			Memory:        domain.NewMemory(2_000_000_000, 500_000_000),
			Swap:          &domain.Swap{TotalBytes: 1000, UsedBytes: 100},
			Disk:          &d,
			UptimeSeconds: 90061,
			Load:          domain.Load{One: 0.52, Five: 0.58, Fifteen: 0.59},
		},
		DiskAlertPercent: 80,
		RemindersEnabled: true,
		SampledAt:        time.Date(2026, 9, 30, 18, 0, 5, 0, time.UTC),
	}
}

func TestRequiresAuth(t *testing.T) {
	svc := &fakeService{out: report()}
	if rec := get(svc, false); rec.Code != http.StatusUnauthorized || svc.calls != 0 {
		t.Fatalf("status = %d, service calls = %d", rec.Code, svc.calls)
	}
}

func TestStatusJSONShape(t *testing.T) {
	rec := get(&fakeService{out: report()}, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := `{"cpu_percent":12.5,"memory":{"total_bytes":2000000000,"used_bytes":1500000000,"available_bytes":500000000,"used_percent":75},` +
		`"swap":{"total_bytes":1000,"used_bytes":100},` +
		`"disk":{"total_bytes":50000000000,"used_bytes":41000000000,"used_percent":82,"path_monitored":true},` +
		`"uptime_seconds":90061,"load_average":{"one":0.52,"five":0.58,"fifteen":0.59},` +
		`"disk_alert_percent":80,"reminders_enabled":true,"sampled_at":"2026-09-30T18:00:05Z"}`
	var wantMap map[string]any
	if err := json.Unmarshal([]byte(want), &wantMap); err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(wantMap)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("body = %s\nwant   %s", gotJSON, wantJSON)
	}
}

func TestStatusNullDiskAndSwap(t *testing.T) {
	r := report()
	r.Disk, r.Swap = nil, nil
	rec := get(&fakeService{out: r}, true)
	var got struct {
		Disk   *json.RawMessage `json:"disk"`
		Swap   *json.RawMessage `json:"swap"`
		Memory struct{ TotalBytes uint64 }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Disk != nil || got.Swap != nil {
		t.Errorf("disk/swap = %v / %v, want null", got.Disk, got.Swap)
	}
	if !strings.Contains(rec.Body.String(), `"disk":null`) {
		t.Errorf("the keys must be present as null: %s", rec.Body)
	}
}

func TestUnavailableIs503WithoutLeakingPaths(t *testing.T) {
	// Even if a cause slipped into the error text it must not reach the client.
	err := errors.Join(app.ErrUnavailable, errors.New("open /proc/stat: permission denied"))
	rec := get(&fakeService{err: err}, true)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != `{"error":"system_unavailable"}` {
		t.Errorf("body = %s", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Error("errors must not be cached either")
	}
}

func TestCancelledAnd500(t *testing.T) {
	if rec := get(&fakeService{err: context.Canceled}, true); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("cancelled: status = %d", rec.Code)
	}
	rec := get(&fakeService{err: errors.New("boom /etc/secret")}, true)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "secret") {
		t.Errorf("unexpected error: %d %s", rec.Code, rec.Body)
	}
}
