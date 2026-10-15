package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

const MaxBodyBytes int64 = 64 << 10

// HandlerTimeout stays below the server's WriteTimeout so a handler can still
// write its error response before the connection is cut.
const HandlerTimeout = 12 * time.Second

type Middleware func(http.Handler) http.Handler

// Chain wraps h so that mws[0] is the outermost middleware.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw := &responseWriter{ResponseWriter: w}
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler {
					panic(v)
				}
				logger.ErrorContext(r.Context(), "panic recovered",
					"panic", fmt.Sprint(v),
					"stack", string(debug.Stack()),
				)
				if rw.wroteHeader {
					panic(http.ErrAbortHandler)
				}
				h := w.Header()
				for k := range h {
					delete(h, k)
				}
				WriteError(w, http.StatusInternalServerError, "internal", "internal error")
			}()
			next.ServeHTTP(rw, r)
		})
	}
}

type routeHolder struct {
	pattern string
}

type routeHolderKey struct{}

const unmatchedRoute = "unmatched"

func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			entry := &accessEntry{start: time.Now(), rw: &responseWriter{ResponseWriter: w}, route: &routeHolder{}}
			r = r.WithContext(context.WithValue(r.Context(), routeHolderKey{}, entry.route))
			completed := false
			defer func() {
				if completed {
					entry.log(logger, r, false)
					return
				}
				v := recover()
				entry.log(logger, r, true)
				panic(v)
			}()
			next.ServeHTTP(entry.rw, r)
			completed = true
		})
	}
}

// accessEntry collects what AccessLog records about one request.
type accessEntry struct {
	start time.Time
	rw    *responseWriter
	route *routeHolder
}

func (e *accessEntry) log(logger *slog.Logger, r *http.Request, aborted bool) {
	attrs := []any{
		"method", r.Method,
		"route", e.routeName(r),
		"status", e.rw.statusCode(),
		"duration_ms", time.Since(e.start).Milliseconds(),
		"bytes", e.rw.bytes,
	}
	if aborted {
		attrs = append(attrs, "aborted", true)
	}
	logger.InfoContext(r.Context(), "request", attrs...)
}

// routeName prefers the pattern Routed recorded, then the request's own.
func (e *accessEntry) routeName(r *http.Request) string {
	if e.route.pattern != "" {
		return e.route.pattern
	}
	if r.Pattern != "" {
		return r.Pattern
	}
	return unmatchedRoute
}

// Routed serves mux and records the matched pattern for AccessLog, which
// cannot see r.Pattern once a middleware in between has cloned the request.
func Routed(mux http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if holder, ok := r.Context().Value(routeHolderKey{}).(*routeHolder); ok {
			defer func() { holder.pattern = r.Pattern }()
		}
		mux.ServeHTTP(w, r)
	})
}

func Deadline(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func MaxBody(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	informational := status >= 100 && status < 200 && status != http.StatusSwitchingProtocols
	if !informational {
		w.status = status
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *responseWriter) statusCode() int {
	if !w.wroteHeader {
		return http.StatusOK
	}
	return w.status
}
