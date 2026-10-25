package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
)

const (
	RequestIDHeader = "X-Request-ID"
	requestIDAttr   = "request_id"
	maxRequestIDLen = 64
	// generatedIDBytes encodes to 32 hex characters.
	generatedIDBytes = 16
)

// RequestID honours a well-formed X-Request-ID from upstream (Caddy,
// SvelteKit) or generates one, echoes it in the response, and puts it on the
// span and in the context, so every log line of the request carries it. Malformed IDs are replaced so a
// client can't inject arbitrary text into logs.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		trace.SpanFromContext(r.Context()).SetAttributes(attribute.String(requestIDAttr, id))
		next.ServeHTTP(w, r.WithContext(logging.WithRequestID(r.Context(), id)))
	})
}

// validRequestID accepts 1–64 characters of [A-Za-z0-9._-].
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		if !requestIDChar(id[i]) {
			return false
		}
	}
	return true
}

func requestIDChar(c byte) bool {
	switch {
	case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		return true
	default:
		return c == '.' || c == '_' || c == '-'
	}
}

func newRequestID() string {
	b := make([]byte, generatedIDBytes)
	// crypto/rand.Read never returns an error; it crashes the program instead.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
