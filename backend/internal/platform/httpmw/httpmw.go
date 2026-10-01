// Package httpmw holds the cross-cutting HTTP middleware of the API: panic
// recovery, structured request logging, request ids, a JSON error envelope for
// the router's own 404 and 405 answers, and a global request body cap.
package httpmw

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/valium69mg/finances-app/backend/internal/platform/clientip"
)

// RequestIDHeader carries the request id in and out. nginx sets it from its own
// $request_id, so a request can be followed from the proxy log to the API log.
const RequestIDHeader = "X-Request-ID"

// MaxRequestBytes is the global request body ceiling. Handlers apply tighter
// limits of their own (JSON bodies 1 MiB, invoice uploads about 12 MiB); this is
// the backstop for any route that forgets one.
const MaxRequestBytes = 14 << 20

// healthPath is probed by the container health check every few seconds; it is
// logged at debug level so it does not drown the request log.
const healthPath = "/healthz"

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// Chain wraps h so that the first middleware is the outermost.
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// recorder captures the status and size of a response, and rewrites the router's
// plain-text 404 and 405 answers into the JSON error envelope.
type recorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
	discard     bool
}

func (rec *recorder) WriteHeader(code int) {
	if rec.wroteHeader {
		return
	}
	rec.wroteHeader = true
	rec.status = code
	h := rec.Header()
	if (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) &&
		strings.HasPrefix(h.Get("Content-Type"), "text/plain") {
		errCode := "not_found"
		if code == http.StatusMethodNotAllowed {
			errCode = "method_not_allowed"
		}
		h.Set("Content-Type", "application/json")
		h.Set("Cache-Control", "no-store")
		h.Del("Content-Length")
		rec.ResponseWriter.WriteHeader(code)
		body, _ := json.Marshal(map[string]string{"error": errCode})
		n, _ := rec.ResponseWriter.Write(append(body, '\n'))
		rec.bytes += n
		rec.discard = true // the router's text body is dropped
		return
	}
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *recorder) Write(p []byte) (int, error) {
	if !rec.wroteHeader {
		rec.WriteHeader(http.StatusOK)
	}
	if rec.discard {
		return len(p), nil
	}
	n, err := rec.ResponseWriter.Write(p)
	rec.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (rec *recorder) Unwrap() http.ResponseWriter { return rec.ResponseWriter }

// Observe records the response, writes the JSON envelope for the router's own
// 404 and 405, sets nosniff on every response, propagates the request id, caps
// the request body and logs one structured line per request. The line holds the
// method, path (never the query string), route pattern, status, size, duration,
// client IP and request id: no headers, tokens, credentials or bodies.
func Observe(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			id := r.Header.Get(RequestIDHeader)
			if !validRequestID.MatchString(id) {
				id = newRequestID()
			}
			w.Header().Set(RequestIDHeader, id)
			w.Header().Set("X-Content-Type-Options", "nosniff")

			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			r.Body = http.MaxBytesReader(rec, r.Body, MaxRequestBytes)
			next.ServeHTTP(rec, r)

			level := slog.LevelInfo
			switch {
			case rec.status >= 500:
				level = slog.LevelError
			case r.URL.Path == healthPath && rec.status < 400:
				level = slog.LevelDebug
			}
			logger.Log(r.Context(), level, "request",
				"method", r.Method,
				"path", r.URL.Path,
				"pattern", r.Pattern,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"client_ip", clientip.FromRequest(r),
				"request_id", id,
			)
		})
	}
}

// Recover turns a panic in a handler into a logged 500 internal_error, so one
// bad request never takes the process down or leaks a stack trace to the client.
// When the response has already started the connection is aborted instead.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler { // compared by identity, as net/http does
					panic(v)
				}
				logger.Error("panic serving request",
					"method", r.Method, "path", r.URL.Path,
					"client_ip", clientip.FromRequest(r),
					"panic", v, "stack", string(debug.Stack()))
				rec, ok := w.(*recorder)
				if ok && rec.wroteHeader {
					panic(http.ErrAbortHandler)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal_error"}` + "\n"))
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Router is the part of *http.ServeMux the module adapters use to mount their
// routes. Taking the interface lets a test record every route registered by
// the application (see cmd/api).
type Router interface {
	Handle(pattern string, handler http.Handler)
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
}
