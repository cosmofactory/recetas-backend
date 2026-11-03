package importer_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/cosmofactory/recetas-backend/internal/catalog/importer"
)

// scorePointer is the first variation's score in minimalDoc.
const scorePointer = "/dishes/0/variations/0/score"

func firstScore(t *testing.T, b importer.Bundle) float64 {
	t.Helper()
	if len(b.Dishes) != 1 || len(b.Dishes[0].Variations) != 1 {
		t.Fatalf("got %+v, want one dish with one variation", b)
	}
	return b.Dishes[0].Variations[0].Score
}

func TestParse_MissingScoreIsZero(t *testing.T) {
	b := parseString(t, minimalBundleJSON)

	if got := firstScore(t, b); got != 0 {
		t.Errorf("score: got %v, want 0", got)
	}
}

func TestParse_ReadsScore(t *testing.T) {
	for _, score := range []float64{0, 1, 17.25, 999999.999, 1000000} {
		t.Run(strconv.FormatFloat(score, 'g', -1, 64), func(t *testing.T) {
			doc := minimalDoc(t)
			firstVariation(doc)["score"] = score

			b := parseString(t, encode(t, doc))

			if got := firstScore(t, b); got != score {
				t.Errorf("score: got %v, want %v", got, score)
			}
		})
	}
}

func TestParse_RoundsScoreToThreeDecimals(t *testing.T) {
	cases := []struct {
		name  string
		score float64
		want  float64
	}{
		{"float noise", noisyThreeTenths(), 0.3},
		{"rounds half up", 2.0005, 2.001},
		{"rounds down", 41.23449, 41.234},
		{"below the third decimal is 0", 0.0004, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := minimalDoc(t)
			firstVariation(doc)["score"] = tc.score

			b := parseString(t, encode(t, doc))

			if got := firstScore(t, b); got != tc.want {
				t.Errorf("score: got %v, want %v", got, tc.want)
			}
		})
	}
}

// A rounded score checksums like the value it rounds to, so float noise from
// the pipeline doesn't look like new content.
func TestParse_RoundedScoreChecksumsLikeItsValue(t *testing.T) {
	doc := minimalDoc(t)
	firstVariation(doc)["score"] = noisyThreeTenths()
	noisy := parseString(t, encode(t, doc))
	firstVariation(doc)["score"] = 0.3
	clean := parseString(t, encode(t, doc))

	if got, want := importer.Checksum(noisy), importer.Checksum(clean); got != want {
		t.Errorf("checksum: got %s, want %s as for 0.3", got, want)
	}
}

// Every score-less bundle keeps its checksum: an omitted score and an explicit
// 0 are the same content.
func TestParse_ZeroScoreChecksumsLikeNoScore(t *testing.T) {
	doc := minimalDoc(t)
	firstVariation(doc)["score"] = 0

	if got, want := importer.Checksum(parseString(t, encode(t, doc))), importer.Checksum(parseString(t, minimalBundleJSON)); got != want {
		t.Errorf("checksum: got %s, want %s as without a score", got, want)
	}
}

func TestParse_RejectsOutOfRangeOrMistypedScore(t *testing.T) {
	cases := []struct {
		name  string
		score any
	}{
		{"negative", -1},
		{"slightly negative", -0.001},
		{"above one million", 1000000.5},
		{"far above one million", 1e9},
		{"string", "12"},
		{"null", nil},
		{"boolean", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := minimalDoc(t)
			firstVariation(doc)["score"] = tc.score

			_, err := importer.Parse(strings.NewReader(encode(t, doc)))

			assertHasProblem(t, validationProblems(t, err), scorePointer, "")
		})
	}
}
