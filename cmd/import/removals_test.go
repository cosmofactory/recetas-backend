package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// The design bundle has 57 recipes; removedPath is the variation the tests
// remove from it.
const (
	designRecipes = 57
	removedPath   = "/dishes/tortilla/gal"
)

// allowFile writes an --allow-removals file and returns its path.
func allowFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "allow-removals.txt")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write allow file: %v", err)
	}
	return path
}

// importedDesign is a transaction holding the design bundle.
func importedDesign(t *testing.T) pgx.Tx {
	t.Helper()
	tx := emptyTx(t)
	runCLI(t, []string{"--source", "base", "--file", designBundlePath}, strings.NewReader(""), tx).requireCode(t, exitOK)
	return tx
}

// withoutVariations is the design bundle without the listed dish/region
// variations and their recipes.
func withoutVariations(t *testing.T, pairs ...[2]string) string {
	t.Helper()
	return editedDesignJSON(t, func(b *importer.Bundle) {
		for _, p := range pairs {
			dropVariation(b, p[0], p[1])
		}
	})
}

func dropVariation(b *importer.Bundle, dishKey, regionKey string) {
	var gone string
	for i := range b.Dishes {
		if b.Dishes[i].Key != dishKey {
			continue
		}
		b.Dishes[i].Variations = slices.DeleteFunc(b.Dishes[i].Variations, func(v importer.Variation) bool {
			if v.RegionKey == regionKey {
				gone = v.RecipeKey
			}
			return v.RegionKey == regionKey
		})
	}
	b.Recipes = slices.DeleteFunc(b.Recipes, func(rc importer.Recipe) bool { return rc.RecipeKey == gone })
}

func assertNotContains(t *testing.T, what, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Errorf("%s: got %q, want it not to contain %q", what, got, unwanted)
	}
}

func assertCount(t *testing.T, tx pgx.Tx, table string, want int64) {
	t.Helper()
	if got := count(t, tx, table); got != want {
		t.Errorf("%s: got %d rows, want %d", table, got, want)
	}
}

// An import that removes a public path not in the allowance is refused: exit
// 1, the unlisted path on stderr, nothing on stdout and nothing written.
// Without --allow-removals the allowance is empty, and a dry run is refused
// too, so a CI plan check fails on it.
func TestRun_UnlistedRemovalIsRefused(t *testing.T) {
	otherPath := allowFile(t, "/dishes/paella/cat\n")
	tests := []struct {
		name string
		args []string
	}{
		{"no allowance", nil},
		{"no allowance, dry run", []string{"--dry-run"}},
		{"another path listed", []string{"--allow-removals", otherPath}},
		{"another path listed, dry run", []string{"--dry-run", "--allow-removals", otherPath}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := importedDesign(t)
			versions := count(t, tx, "content_versions")
			args := append([]string{"--source", "unlisted"}, tt.args...)

			r := runCLI(t, args, strings.NewReader(withoutVariations(t, [2]string{"tortilla", "gal"})), tx)

			r.requireCode(t, exitInvalid)
			assertContains(t, "stderr", r.stderr, removedPath)
			if r.stdout != "" {
				t.Errorf("got stdout %q, want nothing", r.stdout)
			}
			assertCount(t, tx, "recipes", designRecipes)
			assertCount(t, tx, "content_versions", versions)
		})
	}
}

// stderr summarises only the removals still missing from the allowance.
func TestRun_RefusalListsOnlyTheUnlistedPaths(t *testing.T) {
	tx := importedDesign(t)
	bundle := withoutVariations(t, [2]string{"tortilla", "gal"}, [2]string{"croquetas", "gal"})
	args := []string{"--source", "partly listed", "--allow-removals", allowFile(t, removedPath+"\n")}

	r := runCLI(t, args, strings.NewReader(bundle), tx)

	r.requireCode(t, exitInvalid)
	assertContains(t, "stderr", r.stderr, "/dishes/croquetas/gal")
	assertNotContains(t, "stderr", r.stderr, removedPath)
	assertCount(t, tx, "recipes", designRecipes)
}

// The allowance file has one public path per line; surrounding whitespace
// (CRLF included), blank lines and # comment lines are ignored, and listed
// paths that aren't removed are fine.
func TestRun_ListedRemovalIsImportedAndReported(t *testing.T) {
	file := allowFile(t, "# release 2026-11-24\n\n   "+removedPath+"  \r\n\t\n# /dishes/croquetas/gal\n/regions/cat\n")
	tx := importedDesign(t)

	r := runCLI(t, []string{"--source", "listed", "--allow-removals", file}, strings.NewReader(withoutVariations(t, [2]string{"tortilla", "gal"})), tx)

	r.requireCode(t, exitOK)
	assertContains(t, "stdout", r.stdout, "imported content version", "removed "+removedPath)
	assertNotContains(t, "stdout", r.stdout, "/regions/cat")
	assertSummaryOnly(t, r)
	assertCount(t, tx, "recipes", designRecipes-1)
}

// A commented-out path is not an allowance.
func TestRun_CommentedOutPathIsNotAllowed(t *testing.T) {
	tx := importedDesign(t)
	args := []string{"--source", "commented", "--allow-removals", allowFile(t, "# "+removedPath+"\n")}

	r := runCLI(t, args, strings.NewReader(withoutVariations(t, [2]string{"tortilla", "gal"})), tx)

	r.requireCode(t, exitInvalid)
	assertContains(t, "stderr", r.stderr, removedPath)
	assertCount(t, tx, "recipes", designRecipes)
}

func TestRun_DryRunListsRemovedPaths(t *testing.T) {
	tx := importedDesign(t)
	args := []string{"--source", "plan", "--dry-run", "--allow-removals", allowFile(t, removedPath+"\n")}

	r := runCLI(t, args, strings.NewReader(withoutVariations(t, [2]string{"tortilla", "gal"})), tx)

	r.requireCode(t, exitOK)
	assertContains(t, "stdout", r.stdout, "dry run", "deleted", "removed "+removedPath)
	assertSummaryOnly(t, r)
	assertCount(t, tx, "recipes", designRecipes)
}

// An empty allowance file is valid: it is the state a promote PR resets to.
func TestRun_EmptyAllowFileImportsWithoutRemovals(t *testing.T) {
	tx := emptyTx(t)
	args := []string{"--source", "first", "--file", designBundlePath, "--allow-removals", allowFile(t, "")}

	r := runCLI(t, args, strings.NewReader(""), tx)

	r.requireCode(t, exitOK)
	assertNotContains(t, "stdout", r.stdout, "removed")
	assertCount(t, tx, "recipes", designRecipes)
}

// An allowance that can't be read stops the import before the database.
func TestRun_UnreadableAllowFileExitsOne(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-allow.txt")
	dir := t.TempDir()
	tests := []struct {
		name string
		file string
	}{
		{"missing file", missing},
		{"directory", dir},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := unusedDB()
			args := []string{"--source", "x", "--file", designBundlePath, "--allow-removals", tt.file}

			r := runCLI(t, args, strings.NewReader(""), db)

			r.requireCode(t, exitInvalid)
			assertContains(t, "stderr", r.stderr, tt.file)
			if r.stdout != "" {
				t.Errorf("got stdout %q, want nothing", r.stdout)
			}
			db.requireUnused(t)
		})
	}
}

func TestRun_HelpDocumentsAllowRemovals(t *testing.T) {
	r := runCLI(t, []string{"--help"}, strings.NewReader(""), unusedDB())

	r.requireCode(t, exitOK)
	assertContains(t, "stderr", r.stderr, "--allow-removals")
}
