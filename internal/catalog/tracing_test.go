package catalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/cosmofactory/recetas-backend/internal/testutil/spantest"
)

type spanResult struct {
	rec  *httptest.ResponseRecorder
	span sdktrace.ReadOnlySpan
}

// getInSpan serves target with a recording server span in the request
// context, as the HTTP tracing middleware would, and returns that span ended.
func (s apiServer) getInSpan(t *testing.T, target string) spanResult {
	t.Helper()
	tp, spans := spantest.New(t)
	ctx, span := spantest.StartServerSpan(context.Background(), tp)
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx))
	span.End()
	return spanResult{rec: rec, span: spantest.SingleServer(t, spans)}
}

func TestHandler_FailureMarksServerSpanErrorWithException(t *testing.T) {
	t.Parallel()
	for _, fc := range failingCalls() {
		t.Run(fc.failsAt+fc.target, func(t *testing.T) {
			t.Parallel()
			repo := newFakeRepo()
			repo.failWith(fc.failsAt, errRepoDown)

			got := newAPIServer(t, repo).getInSpan(t, fc.target)

			assertStatus(t, got.rec, http.StatusInternalServerError)
			spantest.AssertErrorWithException(t, got.span, errRepoDown.Error())
		})
	}
}

func TestHandler_ClientErrorsLeaveServerSpanUnmarked(t *testing.T) {
	t.Parallel()
	tests := []struct {
		target     string
		wantStatus int
	}{
		{"/v1/dishes/tortilla?lang=ru", http.StatusOK},
		{"/v1/dishes/no-such-dish?lang=ru", http.StatusNotFound},
		{"/v1/dishes/Bad_Slug?lang=ru", http.StatusNotFound},
		{"/v1/regions?lang=xx", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			t.Parallel()
			got := newAPIServer(t, newFakeRepo()).getInSpan(t, tt.target)

			assertStatus(t, got.rec, tt.wantStatus)
			spantest.AssertNoError(t, got.span)
		})
	}
}
