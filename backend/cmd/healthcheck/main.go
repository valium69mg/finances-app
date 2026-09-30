// Command healthcheck probes the local API for the container health check. The
// runtime image is distroless (no shell, curl or wget), so the probe ships as its
// own static binary. It exits 0 when GET /healthz answers 200 and 1 otherwise.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

const timeout = 3 * time.Second

func main() {
	if err := check(context.Background(), probeURL(os.Getenv("HTTP_ADDR")), http.DefaultClient); err != nil {
		fmt.Fprintln(os.Stderr, "unhealthy:", err)
		os.Exit(1)
	}
}

// probeURL builds the loopback URL of the API from HTTP_ADDR (":8080" by default).
func probeURL(httpAddr string) string {
	port := "8080"
	if _, p, err := net.SplitHostPort(httpAddr); err == nil && p != "" {
		port = p
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port) + "/healthz"
}

func check(ctx context.Context, url string, client *http.Client) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
