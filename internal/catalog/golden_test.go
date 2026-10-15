package catalog_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/*.golden.json from the current responses")

// TestGolden_RussianListBodies pins the /v1/regions and /v1/dishes bodies for
// the catalogtest fixture in ru, served end to end from Postgres. ETags hold
// database ids, so only bodies are golden. Regenerate with
// `go test ./internal/catalog -run Golden -update` (needs TEST_DATABASE_URL).
func TestGolden_RussianListBodies(t *testing.T) {
	cases := []struct {
		target string
		golden string
	}{
		{"/v1/regions?lang=ru", "regions_ru.golden.json"},
		{"/v1/dishes?lang=ru", "dishes_ru.golden.json"},
	}
	p := loadPGFixture(t)
	srv := newAPIServer(t, p.repo)
	for _, tc := range cases {
		t.Run(tc.golden, func(t *testing.T) {
			rec := srv.get(t, tc.target)
			assertStatus(t, rec, http.StatusOK)
			assertGolden(t, filepath.Join("testdata", tc.golden), indentJSON(t, rec.Body.Bytes()))
		})
	}
}

func indentJSON(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Indent(&buf, bytes.TrimSpace(raw), "", "  "); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	buf.WriteByte('\n')
	return buf.Bytes()
}

func assertGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		writeGolden(t, path, got)
		return
	}
	want, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("golden file %s is missing; create it with `go test ./internal/catalog -run Golden -update` and review it", path)
	}
	if err != nil {
		t.Fatalf("read golden file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("body differs from %s (rerun with -update if the change is intended):\ngot  %s\nwant %s", path, got, want)
	}
}

func writeGolden(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create testdata dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write golden file: %v", err)
	}
}
