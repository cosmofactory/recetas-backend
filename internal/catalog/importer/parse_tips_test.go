package importer_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// tipsPointer is the first recipe's tips in minimalDoc.
const tipsPointer = "/recipes/0/tips"

func tipDoc(en, es, ru string) jsonDoc {
	return jsonDoc{"en": en, "es": es, "ru": ru}
}

func firstRecipeTips(t *testing.T, b importer.Bundle) []importer.Text {
	t.Helper()
	if len(b.Recipes) != 1 {
		t.Fatalf("got %d recipes, want 1", len(b.Recipes))
	}
	return b.Recipes[0].Tips
}

// A bundle written before tips existed is still schemaVersion 1 and parses
// with no tips.
func TestParse_MissingTipsIsEmpty(t *testing.T) {
	b := parseString(t, minimalBundleJSON)

	if got := firstRecipeTips(t, b); len(got) != 0 {
		t.Errorf("tips: got %+v, want none", got)
	}
	if err := importer.Validate(b); err != nil {
		t.Errorf("Validate: got %v, want nil", err)
	}
}

func TestParse_ReadsTipsInOrder(t *testing.T) {
	doc := minimalDoc(t)
	member(doc, "recipes")["tips"] = []any{
		tipDoc("Use a small pan.", "Usa una sartén pequeña.", "Возьмите маленькую сковороду."),
		tipDoc("Serve at once.", "Sirve enseguida.", "Подавайте сразу."),
	}

	b := parseString(t, encode(t, doc))

	want := []importer.Text{
		{En: "Use a small pan.", Es: "Usa una sartén pequeña.", Ru: "Возьмите маленькую сковороду."},
		{En: "Serve at once.", Es: "Sirve enseguida.", Ru: "Подавайте сразу."},
	}
	if got := firstRecipeTips(t, b); !reflect.DeepEqual(got, want) {
		t.Errorf("tips: got %+v, want %+v", got, want)
	}
}

func TestParse_AcceptsUpToThreeTips(t *testing.T) {
	for n := range 4 {
		t.Run(fmt.Sprintf("%d tips", n), func(t *testing.T) {
			doc := minimalDoc(t)
			member(doc, "recipes")["tips"] = tipDocs(n)

			if err := parseAndValidate(encode(t, doc)); err != nil {
				t.Errorf("got %v, want nil", err)
			}
		})
	}
}

func TestParse_RejectsBadTips(t *testing.T) {
	tests := []struct {
		name        string
		tips        any
		wantPointer string
		wantInMsg   string // the offending field name, when the pointer is its parent
	}{
		{"four tips", tipDocs(4), tipsPointer, ""},
		{"tip missing a locale", []any{jsonDoc{"en": "Rest it.", "es": "Repósala."}}, tipsPointer + "/0", "ru"},
		{"tip with an empty text", []any{tipDoc("", "Repósala.", "Дайте отдохнуть.")}, tipsPointer + "/0/en", ""},
		{"tip with a whitespace-only text", []any{tipDoc("Rest it.", " \t", "Дайте отдохнуть.")}, tipsPointer + "/0/es", ""},
		{"tip with an unknown locale", []any{jsonDoc{"en": "a", "es": "b", "ru": "c", "fr": "d"}}, tipsPointer + "/0", "fr"},
		{"tip as a plain string", []any{"Rest it."}, tipsPointer + "/0", ""},
		{"tips as an object", tipDoc("Rest it.", "Repósala.", "Дайте отдохнуть."), tipsPointer, ""},
		{"tips null", nil, tipsPointer, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := minimalDoc(t)
			member(doc, "recipes")["tips"] = tt.tips

			err := parseAndValidate(encode(t, doc))

			assertHasProblem(t, validationProblems(t, err), tt.wantPointer, tt.wantInMsg)
		})
	}
}

// Every tip-less bundle keeps its checksum: an omitted tips list and an
// explicit empty one are the same content.
func TestParse_EmptyTipsChecksumLikeNoTips(t *testing.T) {
	doc := minimalDoc(t)
	member(doc, "recipes")["tips"] = []any{}

	if got, want := importer.Checksum(parseString(t, encode(t, doc))), importer.Checksum(parseString(t, minimalBundleJSON)); got != want {
		t.Errorf("checksum: got %s, want %s as without tips", got, want)
	}
}

// tipDocs returns n distinct, complete tips.
func tipDocs(n int) []any {
	out := make([]any, 0, n)
	for i := range n {
		suffix := strings.Repeat("!", i+1)
		out = append(out, tipDoc("Tip"+suffix, "Truco"+suffix, "Совет"+suffix))
	}
	return out
}
