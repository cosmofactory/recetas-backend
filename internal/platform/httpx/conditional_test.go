package httpx_test

import (
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/platform/httpx"
)

const (
	testETag    = `"v1-abc"`
	testBody    = `{"items":[]}`
	testCaching = "public, max-age=60"
)

// fakeResponse describes what the wrapped handler writes.
type fakeResponse struct {
	status   int    // 0 means never call WriteHeader (implicit 200)
	etag     string // empty means no ETag header
	body     string
	noLength bool // skip the explicit Content-Length header
}

// handlerFor returns a handler that writes resp with typical cache headers.
func handlerFor(resp fakeResponse) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		h := w.Header()
		h.Set("Content-Type", "application/json")
		h.Set("Cache-Control", testCaching)
		h.Set("Vary", "Accept-Language")
		if !resp.noLength {
			h.Set("Content-Length", strconv.Itoa(len(resp.body)))
		}
		if resp.etag != "" {
			h.Set("ETag", resp.etag)
		}
		if resp.status != 0 {
			w.WriteHeader(resp.status)
		}
		_, _ = io.WriteString(w, resp.body)
	})
}

// conditionalRequest builds a request with one If-None-Match line per value.
func conditionalRequest(method string, ifNoneMatch ...string) *http.Request {
	r := httptest.NewRequest(method, "/dishes", nil)
	for _, v := range ifNoneMatch {
		r.Header.Add("If-None-Match", v)
	}
	return r
}

func record(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// assertNotModified checks a bodyless 304 that keeps the validator headers.
func assertNotModified(t *testing.T, rec *httptest.ResponseRecorder, etag string) {
	t.Helper()
	if rec.Code != http.StatusNotModified {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotModified)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body: got %q, want empty", rec.Body.String())
	}
	want := map[string]string{"ETag": etag, "Cache-Control": testCaching, "Vary": "Accept-Language"}
	for name, value := range want {
		if got := rec.Header().Get(name); got != value {
			t.Errorf("%s: got %q, want %q", name, got, value)
		}
	}
	for _, name := range []string{"Content-Type", "Content-Length"} {
		if got, ok := rec.Header()[name]; ok {
			t.Errorf("%s: got %q, want absent", name, got)
		}
	}
}

// assertPassThrough checks that the wrapped response equals the bare one.
func assertPassThrough(t *testing.T, resp fakeResponse, r *http.Request) {
	t.Helper()
	want := record(handlerFor(resp), r.Clone(r.Context()))
	got := record(httpx.ConditionalGET(handlerFor(resp)), r)
	if got.Code != want.Code {
		t.Errorf("status: got %d, want %d", got.Code, want.Code)
	}
	if got.Body.String() != want.Body.String() {
		t.Errorf("body: got %q, want %q", got.Body.String(), want.Body.String())
	}
	if !maps.EqualFunc(got.Header(), want.Header(), slices.Equal) {
		t.Errorf("headers: got %v, want %v", got.Header(), want.Header())
	}
}

func TestConditionalGET_MatchReturns304(t *testing.T) {
	t.Parallel()
	explicit := fakeResponse{status: http.StatusOK, etag: testETag, body: testBody}
	implicit := fakeResponse{etag: testETag, body: testBody, noLength: true}
	cases := []struct {
		name        string
		method      string
		resp        fakeResponse
		ifNoneMatch []string
	}{
		{"GET exact", http.MethodGet, explicit, []string{testETag}},
		{"HEAD exact", http.MethodHead, explicit, []string{testETag}},
		{"comma list", http.MethodGet, explicit, []string{`"other", ` + testETag + `, "third"`}},
		{"comma list without spaces", http.MethodGet, explicit, []string{`"other",` + testETag}},
		{"second header line", http.MethodGet, explicit, []string{`"other"`, testETag}},
		{"star", http.MethodGet, explicit, []string{"*"}},
		{"star in list", http.MethodGet, explicit, []string{`"other", *`}},
		{"weak request, strong etag", http.MethodGet, explicit, []string{"W/" + testETag}},
		{"strong request, weak etag", http.MethodGet, fakeResponse{status: http.StatusOK, etag: "W/" + testETag, body: testBody}, []string{testETag}},
		{"implicit 200 via Write only", http.MethodGet, implicit, []string{testETag}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := record(httpx.ConditionalGET(handlerFor(tc.resp)), conditionalRequest(tc.method, tc.ifNoneMatch...))
			assertNotModified(t, rec, tc.resp.etag)
		})
	}
}

func TestConditionalGET_PassesThroughUnchanged(t *testing.T) {
	t.Parallel()
	ok := fakeResponse{status: http.StatusOK, etag: testETag, body: testBody}
	cases := []struct {
		name        string
		method      string
		resp        fakeResponse
		ifNoneMatch []string
	}{
		{"no If-None-Match", http.MethodGet, ok, nil},
		{"non-matching tag", http.MethodGet, ok, []string{`"v2-def"`}},
		{"non-matching list", http.MethodGet, ok, []string{`"a", "b"`, `W/"c"`}},
		{"HEAD non-matching", http.MethodHead, ok, []string{`"v2-def"`}},
		{"unquoted tag", http.MethodGet, ok, []string{"v1-abc"}},
		{"weak unquoted tag", http.MethodGet, ok, []string{"W/v1-abc"}},
		{"lone quote", http.MethodGet, ok, []string{`"`}},
		{"half-quoted tag", http.MethodGet, ok, []string{`"v1-abc`}},
		{"empty header", http.MethodGet, ok, []string{""}},
		{"garbage", http.MethodGet, ok, []string{"!!, ;;, W/"}},
		{"star on 404", http.MethodGet, fakeResponse{status: http.StatusNotFound, body: `{"error":"not found"}`}, []string{"*"}},
		{"star on 404 with ETag", http.MethodGet, fakeResponse{status: http.StatusNotFound, etag: testETag, body: `{"error":"not found"}`}, []string{"*"}},
		{"star on 500", http.MethodGet, fakeResponse{status: http.StatusInternalServerError, body: `{"error":"internal"}`}, []string{"*"}},
		{"matching tag on 500", http.MethodGet, fakeResponse{status: http.StatusInternalServerError, etag: testETag, body: "x"}, []string{testETag}},
		{"200 without ETag under star", http.MethodGet, fakeResponse{status: http.StatusOK, body: testBody}, []string{"*"}},
		{"200 without ETag under tag", http.MethodGet, fakeResponse{status: http.StatusOK, body: testBody}, []string{testETag}},
		{"POST with matching tag", http.MethodPost, ok, []string{testETag}},
		{"POST with star", http.MethodPost, ok, []string{"*"}},
		{"implicit 200 non-matching", http.MethodGet, fakeResponse{etag: testETag, body: testBody, noLength: true}, []string{`"v2"`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertPassThrough(t, tc.resp, conditionalRequest(tc.method, tc.ifNoneMatch...))
		})
	}
}

// TestConditionalGET_POSTIsNotBuffered pins that non-GET/HEAD requests reach
// the real writer directly, so streaming and flushing keep working.
func TestConditionalGET_POSTIsNotBuffered(t *testing.T) {
	t.Parallel()
	var sawRecorder bool
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, sawRecorder = w.(*httptest.ResponseRecorder)
	})
	record(httpx.ConditionalGET(h), conditionalRequest(http.MethodPost, testETag))
	if !sawRecorder {
		t.Errorf("POST writer: got a wrapped writer, want the original ResponseWriter")
	}
}
