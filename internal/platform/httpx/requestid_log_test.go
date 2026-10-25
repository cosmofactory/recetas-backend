package httpx_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/platform/httpx"
	"github.com/cosmofactory/recetas-backend/internal/platform/logging"
	"github.com/cosmofactory/recetas-backend/internal/testutil/spantest"
)

const handlerErrorMsg = "lookup failed"

// newLoggingStack serves a route that logs an Error with the request context,
// as catalog and api handlers do.
func newLoggingStack(t *testing.T) (http.Handler, *bytes.Buffer) {
	t.Helper()
	tp, _ := spantest.New(t)
	var logs bytes.Buffer
	logger := logging.New(&logs, slog.LevelDebug)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/regions", func(w http.ResponseWriter, r *http.Request) {
		logger.ErrorContext(r.Context(), handlerErrorMsg, "err", "db down")
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	})
	return httpx.Stack(mux, logger, tp), &logs
}

func TestStack_HandlerLogLinesCarryTheRequestID(t *testing.T) {
	t.Parallel()
	for _, incoming := range []string{"caddy-0123456789", ""} {
		t.Run("incoming="+incoming, func(t *testing.T) {
			t.Parallel()
			h, logs := newLoggingStack(t)
			req := httptest.NewRequest(http.MethodGet, "/v1/regions", nil)
			req.Header.Set(requestIDHeader, incoming)
			rec := httptest.NewRecorder()
			serve(t, h, rec, req)

			id := rec.Header().Get(requestIDHeader)
			line := lineWithMsg(t, logs, handlerErrorMsg)
			if line["level"] != "ERROR" || line[requestIDAttr] != id {
				t.Errorf("got handler line %v, want an ERROR with %s=%q", line, requestIDAttr, id)
			}
		})
	}
}

func TestStack_AccessLineHasExactlyOneRequestID(t *testing.T) {
	t.Parallel()
	h, logs := newLoggingStack(t)
	serve(t, h, httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/regions", nil))

	for _, raw := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if !strings.Contains(raw, `"msg":"request"`) {
			continue
		}
		if n := strings.Count(raw, `"`+requestIDAttr+`":`); n != 1 {
			t.Errorf("got %d %s keys in access line %s, want 1", n, requestIDAttr, raw)
		}
		return
	}
	t.Fatalf("got logs %q, want an access line", logs.String())
}

func lineWithMsg(t *testing.T, buf *bytes.Buffer, msg string) map[string]any {
	t.Helper()
	for _, rec := range decodeLines(t, buf) {
		if rec["msg"] == msg {
			return rec
		}
	}
	t.Fatalf("got logs %q, want a line with msg %q", buf.String(), msg)
	return nil
}
