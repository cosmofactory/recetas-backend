package telemetry_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// exportRequest is one OTLP/HTTP export the fake collector received.
type exportRequest struct {
	path          string
	authorization string
	contentType   string
	body          []byte
}

// collector is a fake OTLP/HTTP endpoint over TLS. Requests block while
// the collector is held, to show exports never run on the caller's path.
type collector struct {
	srv     *httptest.Server
	mu      sync.Mutex
	got     []exportRequest
	release chan struct{}
	once    sync.Once
}

func newCollector(t *testing.T) *collector {
	t.Helper()
	c := &collector{release: make(chan struct{})}
	c.unhold()
	c.srv = httptest.NewTLSServer(http.HandlerFunc(c.serve))
	t.Cleanup(c.srv.Close)
	return c
}

// newHeldCollector returns a collector whose requests block until unhold.
func newHeldCollector(t *testing.T) *collector {
	t.Helper()
	c := &collector{release: make(chan struct{})}
	c.srv = httptest.NewTLSServer(http.HandlerFunc(c.serve))
	t.Cleanup(c.srv.Close)
	t.Cleanup(c.unhold)
	return c
}

func (c *collector) unhold() {
	c.once.Do(func() { close(c.release) })
}

func (c *collector) serve(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	c.got = append(c.got, exportRequest{
		path:          r.URL.Path,
		authorization: r.Header.Get("Authorization"),
		contentType:   r.Header.Get("Content-Type"),
		body:          body,
	})
	c.mu.Unlock()
	<-c.release
	w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
	w.WriteHeader(http.StatusOK)
}

func readBody(r *http.Request) ([]byte, error) {
	var src io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
		src = gz
	}
	return io.ReadAll(src)
}

func (c *collector) requests() []exportRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]exportRequest(nil), c.got...)
}

func (c *collector) requestsTo(path string) []exportRequest {
	var out []exportRequest
	for _, r := range c.requests() {
		if r.path == path {
			out = append(out, r)
		}
	}
	return out
}

func (c *collector) paths() []string {
	var out []string
	for _, r := range c.requests() {
		out = append(out, r.path)
	}
	return out
}

// decodeTraces decodes an OTLP trace export in either OTLP/HTTP encoding.
func decodeTraces(t *testing.T, r exportRequest) *coltracepb.ExportTraceServiceRequest {
	t.Helper()
	var msg coltracepb.ExportTraceServiceRequest
	var err error
	if strings.HasPrefix(r.contentType, "application/json") {
		err = protojson.Unmarshal(r.body, &msg)
	} else {
		err = proto.Unmarshal(r.body, &msg)
	}
	if err != nil {
		t.Fatalf("decode trace export (Content-Type %q): %v", r.contentType, err)
	}
	return &msg
}

// resourceAttrs returns the string resource attributes of every exported
// resource; the last resource wins on a repeated key.
func resourceAttrs(msg *coltracepb.ExportTraceServiceRequest) map[string]string {
	out := map[string]string{}
	for _, rs := range msg.GetResourceSpans() {
		for _, kv := range rs.GetResource().GetAttributes() {
			out[kv.GetKey()] = kv.GetValue().GetStringValue()
		}
	}
	return out
}

func spanNames(msg *coltracepb.ExportTraceServiceRequest) []string {
	var out []string
	for _, rs := range msg.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			for _, s := range ss.GetSpans() {
				out = append(out, s.GetName())
			}
		}
	}
	return out
}

func containsBytes(haystack []byte, needle string) bool {
	return bytes.Contains(haystack, []byte(needle))
}
