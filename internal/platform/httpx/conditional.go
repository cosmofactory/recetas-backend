package httpx

import (
	"bytes"
	"net/http"
	"strings"
)

// ConditionalGET answers a GET or HEAD with 304 Not Modified when the
// handler's 200 carries an ETag that an If-None-Match value matches (RFC 9110
// 13.1.2: weak comparison, comma lists across every header line, and "*").
// It buffers the response, so it suits small bodies only. Other statuses,
// such as a 404 under "*", pass through unchanged.
func ConditionalGET(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		buf := &bufferedResponse{header: http.Header{}}
		next.ServeHTTP(buf, r)
		buf.replay(w, ifNoneMatchHits(r.Header.Values("If-None-Match"), buf.header.Get("ETag")))
	})
}

// bufferedResponse holds a handler's response until ConditionalGET decides
// between sending it and sending 304.
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedResponse) Write(p []byte) (int, error) {
	b.WriteHeader(http.StatusOK)
	return b.body.Write(p)
}

// replay copies the buffered response to w, as a bodyless 304 when
// notModified holds for a 200.
func (b *bufferedResponse) replay(w http.ResponseWriter, notModified bool) {
	status := b.status
	if status == 0 {
		status = http.StatusOK
	}
	for name, values := range b.header {
		w.Header()[name] = values
	}
	if status == http.StatusOK && notModified {
		w.Header().Del("Content-Type")
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(b.body.Bytes())
}

// ifNoneMatchHits reports whether any entity tag in the If-None-Match header
// lines matches etag.
func ifNoneMatchHits(lines []string, etag string) bool {
	if etag == "" {
		return false
	}
	for _, line := range lines {
		if listMatches(line, etag) {
			return true
		}
	}
	return false
}

func listMatches(list, etag string) bool {
	for _, candidate := range strings.Split(list, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || weakMatch(candidate, etag) {
			return true
		}
	}
	return false
}

// weakMatch compares two entity tags ignoring the W/ prefix; anything that
// isn't a quoted tag matches nothing.
func weakMatch(a, b string) bool {
	opaqueA, opaqueB := opaqueTag(a), opaqueTag(b)
	return opaqueA != "" && opaqueA == opaqueB
}

func opaqueTag(tag string) string {
	tag = strings.TrimPrefix(tag, "W/")
	if len(tag) < 2 || !strings.HasPrefix(tag, `"`) || !strings.HasSuffix(tag, `"`) {
		return ""
	}
	return tag
}
