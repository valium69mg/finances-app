package httpmw_test

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/valium69mg/finances-app/backend/internal/platform/httpmw"
)

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func serve(h http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ok", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("GET /gone", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not_found"}`))
	})
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("database password=hunter2") })
	mux.HandleFunc("GET /late-boom", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("after the header")
	})
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			http.Error(w, "too big", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func handler(buf *bytes.Buffer) http.Handler {
	l := testLogger(buf)
	return httpmw.Chain(newMux(), httpmw.Observe(l), httpmw.Recover(l))
}

func TestUnknownRouteAndMethodUseTheJSONEnvelope(t *testing.T) {
	var buf bytes.Buffer
	h := handler(&buf)

	rec := serve(h, httptest.NewRequest("GET", "/nope", nil))
	if rec.Code != 404 || strings.TrimSpace(rec.Body.String()) != `{"error":"not_found"}` {
		t.Fatalf("404: %d %q", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("404 content type = %q", ct)
	}

	rec = serve(h, httptest.NewRequest("DELETE", "/ok", nil))
	if rec.Code != 405 || strings.TrimSpace(rec.Body.String()) != `{"error":"method_not_allowed"}` {
		t.Fatalf("405: %d %q", rec.Code, rec.Body)
	}
	if rec.Header().Get("Allow") == "" {
		t.Error("405 must keep the Allow header")
	}
}

func TestHandlerJSON404IsLeftAlone(t *testing.T) {
	var buf bytes.Buffer
	rec := serve(handler(&buf), httptest.NewRequest("GET", "/gone", nil))
	if rec.Code != 404 || strings.TrimSpace(rec.Body.String()) != `{"error":"not_found"}` {
		t.Fatalf("%d %q", rec.Code, rec.Body)
	}
}

func TestRecoverHidesPanicDetailsAndLogsThem(t *testing.T) {
	var buf bytes.Buffer
	rec := serve(handler(&buf), httptest.NewRequest("GET", "/boom", nil))
	if rec.Code != 500 || strings.TrimSpace(rec.Body.String()) != `{"error":"internal_error"}` {
		t.Fatalf("%d %q", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatal("panic value leaked to the client")
	}
	if log := buf.String(); !strings.Contains(log, "panic serving request") || !strings.Contains(log, "level=ERROR") {
		t.Fatalf("panic not logged as an error: %s", log)
	}
	if !strings.Contains(buf.String(), "status=500") {
		t.Errorf("request line must record status 500: %s", buf.String())
	}
}

func TestRecoverAbortsWhenResponseStarted(t *testing.T) {
	var buf bytes.Buffer
	defer func() {
		if v := recover(); v != http.ErrAbortHandler {
			t.Fatalf("recovered %v, want http.ErrAbortHandler", v)
		}
	}()
	serve(handler(&buf), httptest.NewRequest("GET", "/late-boom", nil))
	t.Fatal("expected the handler to abort")
}

func TestRequestLogHasNoSensitiveData(t *testing.T) {
	var buf bytes.Buffer
	req := httptest.NewRequest("POST", "/echo?token=SECRET-QUERY", strings.NewReader(`{"password":"SECRET-BODY"}`))
	req.Header.Set("Authorization", "Bearer SECRET-JWT")
	req.RemoteAddr = "203.0.113.9:5555"
	serve(handler(&buf), req)

	log := buf.String()
	for _, secret := range []string{"SECRET-QUERY", "SECRET-BODY", "SECRET-JWT"} {
		if strings.Contains(log, secret) {
			t.Fatalf("log leaks %s: %s", secret, log)
		}
	}
	for _, want := range []string{"method=POST", "path=/echo", "status=204", "client_ip=203.0.113.9", "request_id=", "duration_ms="} {
		if !strings.Contains(log, want) {
			t.Errorf("log missing %s: %s", want, log)
		}
	}
}

func TestRequestID(t *testing.T) {
	var buf bytes.Buffer
	h := handler(&buf)

	rec := serve(h, httptest.NewRequest("GET", "/ok", nil))
	if id := rec.Header().Get(httpmw.RequestIDHeader); len(id) != 16 {
		t.Fatalf("generated id = %q", id)
	}

	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set(httpmw.RequestIDHeader, "abc-123")
	if id := serve(h, req).Header().Get(httpmw.RequestIDHeader); id != "abc-123" {
		t.Errorf("valid incoming id must be kept, got %q", id)
	}

	req = httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set(httpmw.RequestIDHeader, "bad id\nwith newline")
	if id := serve(h, req).Header().Get(httpmw.RequestIDHeader); id == "bad id\nwith newline" || len(id) != 16 {
		t.Errorf("invalid incoming id must be replaced, got %q", id)
	}
}

func TestNosniffOnEveryResponse(t *testing.T) {
	var buf bytes.Buffer
	for _, path := range []string{"/ok", "/nope", "/boom"} {
		rec := serve(handler(&buf), httptest.NewRequest("GET", path, nil))
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: missing nosniff", path)
		}
	}
}

func TestHealthChecksAreLoggedAtDebug(t *testing.T) {
	var buf bytes.Buffer
	serve(handler(&buf), httptest.NewRequest("GET", "/healthz", nil))
	if !strings.Contains(buf.String(), "level=DEBUG") {
		t.Fatalf("healthz should log at debug: %s", buf.String())
	}
}

func TestGlobalBodyCap(t *testing.T) {
	var buf bytes.Buffer
	body := strings.NewReader(strings.Repeat("a", httpmw.MaxRequestBytes+1))
	rec := serve(handler(&buf), httptest.NewRequest("POST", "/echo", body))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
}
