package importer_test

import (
	"slices"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// fabadaTipsIndex is fabada's recipe in validBundle, the one with tips.
const fabadaTipsIndex = 3

var (
	fabadaPair   = pairKey{"cocido", "ast"}
	betanzosPair = pairKey{"tortilla", "gal"}
)

func fabadaTips(b *importer.Bundle) *[]importer.Text {
	return &b.Recipes[fabadaTipsIndex].Tips
}

func TestApply_StoresTipsInOrderPerLocale(t *testing.T) {
	tx := emptyTx(t)
	b := validBundle()
	mustApply(t, tx, b, realImport("tips"))
	svc := newService(tx)

	for _, l := range locales {
		var want []string
		for _, tip := range *fabadaTips(&b) {
			want = append(want, pick(tip, l))
		}
		if got := mustRecipe(t, svc, fabadaPair, l).Tips; !slices.Equal(got, want) {
			t.Errorf("fabada tips in %s: got %q, want %q", l, got, want)
		}
		if got := mustRecipe(t, svc, betanzosPair, l).Tips; len(got) != 0 {
			t.Errorf("betanzos tips in %s: got %q, want none", l, got)
		}
	}
	assertCatalogMatches(t, tx, b)
}

func TestApply_StoresOneRowPerTipAndLocale(t *testing.T) {
	tx := emptyTx(t)
	b := validBundle()
	mustApply(t, tx, b, realImport("tips"))

	assertTipRows(t, tableCounts(t, tx), b)
}

func TestApply_ReimportingABundleWithTipsMakesNoChanges(t *testing.T) {
	tx := emptyTx(t)
	first := mustApply(t, tx, validBundle(), realImport("first"))
	before := readState(t, tx)

	again := mustApply(t, tx, validBundle(), realImport("second"))

	if !again.NoChanges {
		t.Errorf("got changes %+v, want NoChanges", again.Changes)
	}
	if again.Version != first.Version {
		t.Errorf("got version %d, want the unchanged %d", again.Version, first.Version)
	}
	assertStateUnchanged(t, tx, before)
}

// A tip-less recipe stores no tips whether the bundle omits the list or sends
// it empty, so switching between the two is not a change.
func TestApply_EmptyTipsAreTheSameAsNone(t *testing.T) {
	tests := []struct {
		name        string
		first, then []importer.Text
	}{
		{"omitted, then empty", nil, []importer.Text{}},
		{"empty, then omitted", []importer.Text{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := emptyTx(t)
			b := validBundle()
			*fabadaTips(&b) = tt.first
			mustApply(t, tx, b, realImport("first"))

			*fabadaTips(&b) = tt.then
			s := mustApply(t, tx, b, realImport("again"))

			if !s.NoChanges {
				t.Errorf("got changes %+v, want NoChanges", s.Changes)
			}
		})
	}
}

func TestApply_ChangedTipsUpdateOnlyThatRecipe(t *testing.T) {
	tests := []struct {
		name string
		edit func(b *importer.Bundle)
	}{
		{"tip text", func(b *importer.Bundle) {
			(*fabadaTips(b))[1].Ru = "Пугайте фасоль холодной водой."
		}},
		{"tip removed", func(b *importer.Bundle) { *fabadaTips(b) = (*fabadaTips(b))[:2] }},
		{"every tip removed", func(b *importer.Bundle) { *fabadaTips(b) = nil }},
		{"tips reordered", func(b *importer.Bundle) {
			tips := *fabadaTips(b)
			tips[0], tips[2] = tips[2], tips[0]
		}},
		{"tip added to a recipe without tips", func(b *importer.Bundle) {
			b.Recipes[0].Tips = []importer.Text{txt("Rest it for a minute.")}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tx := emptyTx(t)
			mustApply(t, tx, validBundle(), realImport("base"))
			idsBefore := rowIDs(t, tx)

			b := validBundle()
			tt.edit(&b)
			s := mustApply(t, tx, b, realImport("edit"))

			assertChanges(t, s, map[string]importer.Counts{"recipes": {Updated: 1}})
			assertDeepEqual(t, "row ids", rowIDs(t, tx), idsBefore)
			assertTipRows(t, tableCounts(t, tx), b)
			assertCatalogMatches(t, tx, b)
		})
	}
}

// assertTipRows checks that the stored tip rows are exactly the bundle's,
// so removed tips leave nothing behind.
func assertTipRows(t *testing.T, counts map[string]int64, b importer.Bundle) {
	t.Helper()
	var tips int64
	for _, rc := range b.Recipes {
		tips += int64(len(rc.Tips))
	}
	if counts["recipe_tips"] != tips || counts["recipe_tip_translations"] != 3*tips {
		t.Errorf("got %d tips and %d tip translations, want %d and %d",
			counts["recipe_tips"], counts["recipe_tip_translations"], tips, 3*tips)
	}
}
