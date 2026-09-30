package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbeURL(t *testing.T) {
	for addr, want := range map[string]string{
		"":            "http://127.0.0.1:8080/healthz",
		":8080":       "http://127.0.0.1:8080/healthz",
		":9090":       "http://127.0.0.1:9090/healthz",
		"0.0.0.0:81":  "http://127.0.0.1:81/healthz",
		"not-an-addr": "http://127.0.0.1:8080/healthz",
	} {
		if got := probeURL(addr); got != want {
			t.Errorf("probeURL(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestCheck(t *testing.T) {
	for _, tt := range []struct {
		status  int
		wantErr bool
	}{{http.StatusOK, false}, {http.StatusServiceUnavailable, true}, {http.StatusNotFound, true}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.status) }))
		err := check(context.Background(), srv.URL, srv.Client())
		srv.Close()
		if (err != nil) != tt.wantErr {
			t.Errorf("status %d: err = %v, wantErr %v", tt.status, err, tt.wantErr)
		}
	}
	if err := check(context.Background(), "http://127.0.0.1:1/healthz", http.DefaultClient); err == nil {
		t.Error("an unreachable API must be unhealthy")
	}
}
