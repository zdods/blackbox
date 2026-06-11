package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// setupLogger configures the process-wide slog default from env:
// LOG_FORMAT=json for JSON lines (text otherwise), LOG_LEVEL=debug to
// include per-asset access logs.
func setupLogger(format, level string) {
	lvl := slog.LevelInfo
	if strings.EqualFold(level, "debug") {
		lvl = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: lvl}
	var h slog.Handler
	if strings.EqualFold(format, "json") {
		h = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		h = slog.NewTextHandler(os.Stderr, opts)
	}
	slog.SetDefault(slog.New(h))
}

type ctxKeyRequestID struct{}

// withRequestID tags every request with a unique ID, exposed in the
// X-Request-ID response header, the request context, and logs.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.NewString()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyRequestID{}).(string)
	return id
}

// reqLog returns the default logger scoped with the request ID, for handler
// code that logs mid-request.
func reqLog(ctx context.Context) *slog.Logger {
	if id := requestIDFrom(ctx); id != "" {
		return slog.With("request_id", id)
	}
	return slog.Default()
}

// statusRecorder captures the response status for access logging while
// passing Flusher/Hijacker through (streaming downloads flush; the daemon
// WebSocket upgrade hijacks).
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := sr.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("hijack not supported")
}

// Unwrap supports http.ResponseController lookups through the wrapper.
func (sr *statusRecorder) Unwrap() http.ResponseWriter { return sr.ResponseWriter }

// accessLog emits one structured line per request. /healthz is skipped
// (Docker probes every 30s); static assets log at debug to keep info-level
// output to API traffic.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		level := slog.LevelDebug
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/ws/") {
			level = slog.LevelInfo
		}
		slog.Default().Log(r.Context(), level, "request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", requestIDFrom(r.Context()),
		)
	})
}
