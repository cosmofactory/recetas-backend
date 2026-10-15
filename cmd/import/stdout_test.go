package main

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// summaryLine matches every line run may write to stdout.
var summaryLine = regexp.MustCompile(`^(` +
	`imported content version \d+ \(source ".*"\):` +
	`|dry run, rolled back; changes against content version \d+:` +
	`|(dry run: )?no changes \(content version \d+ is current\)` +
	`|  [a-z]+ +created +\d+ +updated +\d+ +deleted +\d+` +
	`)$`)

// Scripts capture stdout as the import's result, so diagnostics, usage and
// errors all go to stderr.
func TestRun_FailuresWriteNothingToStdout(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		db    importer.Beginner
		code  int
	}{
		{"help", []string{"--help"}, "", unusedDB(), 0},
		{"usage error", []string{"--file", designBundlePath}, "", unusedDB(), 1},
		{"invalid bundle", []string{"--source", "x"}, "{not json", unusedDB(), 1},
		{"database error", []string{"--source", "x", "--file", designBundlePath}, "", &beginner{err: errors.New("connection refused")}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runCLI(t, tt.args, strings.NewReader(tt.stdin), tt.db)
			r.requireCode(t, tt.code)
			if r.stdout != "" {
				t.Errorf("got stdout %q, want nothing", r.stdout)
			}
			if strings.TrimSpace(r.stderr) == "" {
				t.Error("got empty stderr, want the diagnostic")
			}
		})
	}
}

func TestRun_SuccessWritesOnlyTheSummaryToStdout(t *testing.T) {
	tx := emptyTx(t)
	for _, args := range [][]string{
		{"--source", "x", "--dry-run", "--file", designBundlePath}, // dry run with changes
		{"--source", "x", "--file", designBundlePath},              // import
		{"--source", "x", "--file", designBundlePath},              // no changes
		{"--source", "x", "--dry-run", "--file", designBundlePath}, // dry run, no changes
	} {
		r := runCLI(t, args, strings.NewReader(""), tx)
		r.requireCode(t, 0)
		assertSummaryOnly(t, r)
	}
}

func assertSummaryOnly(t *testing.T, r cliResult) {
	t.Helper()
	if r.stderr != "" {
		t.Errorf("got stderr %q, want nothing on success", r.stderr)
	}
	for _, line := range strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n") {
		if !summaryLine.MatchString(line) {
			t.Errorf("got stdout line %q, want only summary lines", line)
		}
	}
}
