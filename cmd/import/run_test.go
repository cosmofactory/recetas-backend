package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
)

const designBundlePath = "../../testdata/content.design.json"

type cliResult struct {
	code           int
	stdout, stderr string
}

// runCLI is the only caller of run, so a signature change touches one line.
func runCLI(t *testing.T, args []string, stdin io.Reader, db importer.Beginner) cliResult {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), args, stdio{in: stdin, out: &stdout, err: &stderr}, db)
	return cliResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func (r cliResult) requireCode(t *testing.T, want int) {
	t.Helper()
	if r.code != want {
		t.Fatalf("got exit code %d, want %d\nstdout: %s\nstderr: %s", r.code, want, r.stdout, r.stderr)
	}
}

// beginner records Begin calls and fails them with err.
type beginner struct {
	err   error
	calls int
}

func (b *beginner) Begin(context.Context) (pgx.Tx, error) {
	b.calls++
	return nil, b.err
}

// unusedDB fails a test that reaches the database.
func unusedDB() *beginner { return &beginner{err: errors.New("unexpected Begin")} }

func (b *beginner) requireUnused(t *testing.T) {
	t.Helper()
	if b.calls != 0 {
		t.Errorf("got %d Begin calls, want none before the input is valid", b.calls)
	}
}

// emptyTx is a rolled-back transaction with no catalog and no content versions.
func emptyTx(t *testing.T) pgx.Tx {
	t.Helper()
	tx := testdb.Tx(t)
	for _, sql := range []string{"DELETE FROM dishes", "DELETE FROM regions", "DELETE FROM content_versions"} {
		if _, err := tx.Exec(context.Background(), sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	return tx
}

func designJSON(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(designBundlePath)
	if err != nil {
		t.Fatalf("read design bundle: %v", err)
	}
	return data
}

// editedDesignJSON re-encodes the design bundle after edit.
func editedDesignJSON(t *testing.T, edit func(b *importer.Bundle)) string {
	t.Helper()
	var b importer.Bundle
	if err := json.Unmarshal(designJSON(t), &b); err != nil {
		t.Fatalf("decode design bundle: %v", err)
	}
	edit(&b)
	out, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("encode bundle: %v", err)
	}
	return string(out)
}

func count(t *testing.T, tx pgx.Tx, table string) int64 {
	t.Helper()
	queries := map[string]string{
		"regions":          "SELECT count(*) FROM regions",
		"recipes":          "SELECT count(*) FROM recipes",
		"content_versions": "SELECT count(*) FROM content_versions",
	}
	var n int64
	if err := tx.QueryRow(context.Background(), queries[table]).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func latestSource(t *testing.T, tx pgx.Tx) string {
	t.Helper()
	var source string
	err := tx.QueryRow(context.Background(), "SELECT source FROM content_versions ORDER BY id DESC LIMIT 1").Scan(&source)
	if err != nil {
		t.Fatalf("latest content version: %v", err)
	}
	return source
}

func assertContains(t *testing.T, what, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(strings.ToLower(got), strings.ToLower(want)) {
			t.Errorf("%s: got %q, want it to contain %q", what, got, want)
		}
	}
}

func TestRun_ImportsFileAndPrintsSummary(t *testing.T) {
	tx := emptyTx(t)
	r := runCLI(t, []string{"--source", "local seed", "--file", designBundlePath}, strings.NewReader(""), tx)
	r.requireCode(t, 0)
	assertContains(t, "stdout", r.stdout, "regions", "dishes", "variations", "recipes", "14", "54")
	if got := count(t, tx, "regions"); got != 14 {
		t.Errorf("got %d regions, want 14", got)
	}
	if got := latestSource(t, tx); got != "local seed" {
		t.Errorf("source: got %q, want %q", got, "local seed")
	}
}

func TestRun_ReadsStdinWithoutFile(t *testing.T) {
	tx := emptyTx(t)
	r := runCLI(t, []string{"--source=ssh"}, bytes.NewReader(designJSON(t)), tx)
	r.requireCode(t, 0)
	if got := count(t, tx, "recipes"); got != 54 {
		t.Errorf("got %d recipes, want 54", got)
	}
	if got := latestSource(t, tx); got != "ssh" {
		t.Errorf("source: got %q, want %q", got, "ssh")
	}
}

func TestRun_FileTakesPrecedenceOverStdin(t *testing.T) {
	tx := emptyTx(t)
	r := runCLI(t, []string{"--source", "local", "--file", designBundlePath}, strings.NewReader("{not json"), tx)
	r.requireCode(t, 0)
}

func TestRun_SecondImportPrintsNoChanges(t *testing.T) {
	tx := emptyTx(t)
	args := []string{"--source", "seed", "--file", designBundlePath}
	runCLI(t, args, strings.NewReader(""), tx).requireCode(t, 0)
	versions := count(t, tx, "content_versions")

	r := runCLI(t, args, strings.NewReader(""), tx)
	r.requireCode(t, 0)
	assertContains(t, "stdout", r.stdout, "no changes")
	if got := count(t, tx, "content_versions"); got != versions {
		t.Errorf("got %d content versions, want the unchanged %d", got, versions)
	}
}

func TestRun_DryRunPrintsSummaryAndWritesNothing(t *testing.T) {
	tx := emptyTx(t)
	r := runCLI(t, []string{"--source", "check", "--dry-run", "--file", designBundlePath}, strings.NewReader(""), tx)
	r.requireCode(t, 0)
	assertContains(t, "stdout", r.stdout, "dry run", "regions", "14")
	for _, table := range []string{"regions", "recipes", "content_versions"} {
		if got := count(t, tx, table); got != 0 {
			t.Errorf("%s: got %d rows after a dry run, want 0", table, got)
		}
	}
}

func TestRun_InvalidBundleExitsOne(t *testing.T) {
	tests := []struct {
		name  string
		input func(t *testing.T) string
		want  []string // in stderr
	}{
		{"unknown region", func(t *testing.T) string {
			return editedDesignJSON(t, func(b *importer.Bundle) { b.Dishes[0].Variations[0].RegionKey = "xyz" })
		}, []string{"/dishes/0/variations/0/regionKey"}},
		{"every problem listed", func(t *testing.T) string {
			return editedDesignJSON(t, func(b *importer.Bundle) {
				b.Regions[1].TileColor = "red"
				b.Recipes[0].BaseServings = 21
			})
		}, []string{"/regions/1/tileColor", "/recipes/0/baseServings"}},
		{"schema error", func(t *testing.T) string {
			return editedDesignJSON(t, func(b *importer.Bundle) { b.Dishes[0].Course = "brunch" })
		}, []string{"/dishes/0/course"}},
		{"newer schema version", func(t *testing.T) string {
			return editedDesignJSON(t, func(b *importer.Bundle) { b.SchemaVersion = 2 })
		}, []string{"/schemaVersion"}},
		{"newer schema version with a field this backend doesn't know", func(t *testing.T) string {
			return withTopLevelField(editedDesignJSON(t, func(b *importer.Bundle) { b.SchemaVersion = 2 }), "collections")
		}, []string{"/schemaVersion"}},
		{"quantity rounding to zero", func(t *testing.T) string {
			return editedDesignJSON(t, func(b *importer.Bundle) { b.Recipes[0].Ingredients[0].Quantity = 1e-10 })
		}, []string{"/recipes/0/ingredients/0/quantity"}},
		{"not JSON", func(*testing.T) string { return "{not json" }, nil},
		{"empty input", func(*testing.T) string { return "" }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := unusedDB()
			r := runCLI(t, []string{"--source", "x"}, strings.NewReader(tt.input(t)), db)
			r.requireCode(t, 1)
			if strings.TrimSpace(r.stderr) == "" {
				t.Error("got empty stderr, want the problems")
			}
			assertContains(t, "stderr", r.stderr, tt.want...)
			db.requireUnused(t)
		})
	}
}

func TestRun_UsageErrorsExitOne(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string // in stderr
	}{
		{"missing source", []string{"--file", designBundlePath}, "source"},
		{"empty source", []string{"--source=", "--file", designBundlePath}, "source"},
		{"blank source", []string{"--source", "  ", "--file", designBundlePath}, "source"},
		{"unknown flag", []string{"--source", "x", "--force"}, "force"},
		{"missing file", []string{"--source", "x", "--file", "testdata/does-not-exist.json"}, "does-not-exist.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := unusedDB()
			r := runCLI(t, tt.args, strings.NewReader(""), db)
			r.requireCode(t, 1)
			assertContains(t, "stderr", r.stderr, tt.want)
			db.requireUnused(t)
		})
	}
}

func TestRun_DatabaseErrorExitsTwo(t *testing.T) {
	for _, args := range [][]string{
		{"--source", "x", "--file", designBundlePath},
		{"--source", "x", "--dry-run", "--file", designBundlePath},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			db := &beginner{err: errors.New("connection refused")}
			r := runCLI(t, args, strings.NewReader(""), db)
			r.requireCode(t, 2)
			if strings.TrimSpace(r.stderr) == "" {
				t.Error("got empty stderr, want the database error")
			}
			if db.calls == 0 {
				t.Error("got no Begin calls, want the import to reach the database")
			}
		})
	}
}

// withTopLevelField adds an empty array named field to the bundle object.
func withTopLevelField(bundleJSON, field string) string {
	return `{"` + field + `":[],` + strings.TrimPrefix(bundleJSON, "{")
}
