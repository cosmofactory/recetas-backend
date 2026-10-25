package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/testutil/spantest"
)

// A handler error the adapter turns into a 500 is recorded on the server
// span in the request context, as the HTTP tracing middleware creates it.
func TestRegister_ResponseErrorMarksServerSpanErrorWithException(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, failingRegions{})
	tp, spans := spantest.New(t)
	ctx, span := spantest.StartServerSpan(context.Background(), tp)

	rec := httptest.NewRecorder()
	srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/regions?lang=es", nil).WithContext(ctx))
	span.End()

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	spantest.AssertErrorWithException(t, spantest.SingleServer(t, spans), internalErrText)
}
