package api_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/cosmofactory/recetas-backend/internal/api"
)

// normalizedSpec loads a document through the kin-openapi loader and returns it
// re-marshalled as generic JSON values, so YAML and JSON sources compare equal
// whenever they describe the same document.
func normalizedSpec(t *testing.T, doc *openapi3.T) any {
	t.Helper()
	raw, err := doc.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("unmarshal spec: %v", err)
	}
	return v
}

func loadFileSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(specPath(t))
	if err != nil {
		t.Fatalf("load %s: %v", specPath(t), err)
	}
	return doc
}

func assertSameSpec(t *testing.T, served []byte) {
	t.Helper()
	var generic map[string]any
	if err := json.Unmarshal(served, &generic); err != nil {
		t.Fatalf("served spec is not a JSON object: %v", err)
	}
	if got, want := generic["openapi"], "3.0.3"; got != want {
		t.Errorf("got openapi %v, want %q", got, want)
	}
	info, _ := generic["info"].(map[string]any)
	if got, want := info["version"], "1.0.0"; got != want {
		t.Errorf("got info.version %v, want %q", got, want)
	}

	servedDoc, err := openapi3.NewLoader().LoadFromData(served)
	if err != nil {
		t.Fatalf("load served spec: %v", err)
	}
	got, want := normalizedSpec(t, servedDoc), normalizedSpec(t, loadFileSpec(t))
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(want)
		t.Errorf("served spec differs from api/openapi.yaml\ngot  %.2000s\nwant %.2000s", gotJSON, wantJSON)
	}
}

func TestOpenAPI_ServedSpecMatchesFile(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, api.Stub{})

	rec := srv.get(t, "/openapi.json")

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d (body=%.200q)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("got Content-Type %q, want application/json", ct)
	}
	assertSameSpec(t, rec.Body.Bytes())
}

func TestSpecJSON_MatchesFile(t *testing.T) {
	t.Parallel()
	raw, err := api.SpecJSON()
	if err != nil {
		t.Fatalf("SpecJSON: got error %v, want nil", err)
	}
	assertSameSpec(t, raw)
}

func TestOpenAPI_ServedBodyEqualsSpecJSON(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t, api.Stub{})
	want, err := api.SpecJSON()
	if err != nil {
		t.Fatalf("SpecJSON: got error %v, want nil", err)
	}

	rec := srv.get(t, "/openapi.json")

	var got, wantV any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("served body is not JSON: %v", err)
	}
	if err := json.Unmarshal(want, &wantV); err != nil {
		t.Fatalf("SpecJSON is not JSON: %v", err)
	}
	if !reflect.DeepEqual(got, wantV) {
		t.Error("got /openapi.json body different from SpecJSON(), want the same document")
	}
}
