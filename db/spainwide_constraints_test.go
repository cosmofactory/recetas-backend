package db_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
)

// Spanish (Spain-wide) variations (BE-9, D-ESP-1, D-ESP-4): region_id NULL
// exactly when slug is set, slugs unique within a dish, and every variation
// at a unique positive position within its dish.

const insertPlacedVariationSQL = `
	INSERT INTO variations (dish_id, region_id, slug, recipe_key, position)
	VALUES ($1, $2, $3, $4, $5)`

const (
	takenSlug     = "allioli-de-morter"
	takenPosition = 1
	freePosition  = 50
)

// spainWideBaseline is allioli with the Spanish variation takenSlug at
// takenPosition and a regional one in gal at position 2, plus another dish.
type spainWideBaseline struct {
	allioli, other, gal, ast int64
}

func newSpainWideBaseline(t *testing.T, tx pgx.Tx) spainWideBaseline {
	t.Helper()
	b := spainWideBaseline{
		allioli: insertDish(t, tx), other: insertDish(t, tx),
		gal: insertRegion(t, tx), ast: insertRegion(t, tx),
	}
	mustExec(t, tx, insertPlacedVariationSQL, b.allioli, nil, takenSlug, uniqKey(t, "allioli-esp"), takenPosition)
	mustExec(t, tx, insertPlacedVariationSQL, b.allioli, b.gal, nil, uniqKey(t, "allioli-gal"), takenPosition+1)
	return b
}

type placedVariationCase struct {
	name     string
	dish     int64
	region   any // nil for a Spanish variation
	slug     any
	position any
	want     string
}

func placedVariationCases(t *testing.T, b spainWideBaseline) []placedVariationCase {
	return []placedVariationCase{
		{"second spanish variation of a dish", b.allioli, nil, "allioli-de-codony", freePosition, codeOK},
		{"spanish variation without a slug", b.allioli, nil, nil, freePosition, codeCheck},
		{"regional variation with a slug", b.allioli, b.ast, "allioli-asturiano", freePosition, codeCheck},
		{"regional variation without a slug", b.allioli, b.ast, nil, freePosition, codeOK},
		{"duplicate slug in a dish", b.allioli, nil, takenSlug, freePosition, codeUnique},
		{"same slug in another dish", b.other, nil, takenSlug, freePosition, codeOK},
		{"duplicate dish and region", b.allioli, b.gal, nil, freePosition, codeUnique},
		{"slug uppercase", b.allioli, nil, "Allioli-" + randHex(t, 3), freePosition, codeCheck},
		{"slug space", b.allioli, nil, "allioli de " + randHex(t, 3), freePosition, codeCheck},
		{"slug accented", b.allioli, nil, "allioli-codoñy" + randHex(t, 3), freePosition, codeCheck},
		{"slug trailing hyphen", b.allioli, nil, "allioli-" + randHex(t, 3) + "-", freePosition, codeCheck},
		{"slug empty", b.allioli, nil, "", freePosition, codeCheck},
		{"duplicate position in a dish", b.allioli, nil, uniqKey(t, "allioli"), takenPosition, codeUnique},
		{"same position in another dish", b.other, nil, uniqKey(t, "allioli"), takenPosition, codeOK},
		{"position zero", b.allioli, nil, uniqKey(t, "allioli"), 0, codeCheck},
		{"position negative", b.allioli, nil, uniqKey(t, "allioli"), -1, codeCheck},
		{"position null", b.allioli, nil, uniqKey(t, "allioli"), nil, codeNotNull},
	}
}

func TestIntegrationSpainWideVariationConstraints(t *testing.T) {
	tx := testdb.Tx(t)
	b := newSpainWideBaseline(t, tx)

	for i, tc := range placedVariationCases(t, b) {
		t.Run(tc.name, func(t *testing.T) {
			position := tc.position
			if position == freePosition {
				position = freePosition + i // each accepted row takes its own position
			}
			err := try(t, tx, insertPlacedVariationSQL, tc.dish, tc.region, tc.slug, uniqKey(t, "rec"), position)
			wantCode(t, err, tc.want)
		})
	}
}

// A regional variation can't drop its region, nor a Spanish one gain one,
// without the slug following.
func TestIntegrationSpainWideVariationUpdates(t *testing.T) {
	tx := testdb.Tx(t)
	b := newSpainWideBaseline(t, tx)
	spanish := insertSpainWideVariation(t, tx, b.other, "allioli-de-codony")
	regional := insertVariation(t, tx, b.other, b.ast)

	t.Run("spanish variation gains a region", func(t *testing.T) {
		wantCode(t, try(t, tx, `UPDATE variations SET region_id = $2 WHERE id = $1`, spanish, b.gal), codeCheck)
	})
	t.Run("regional variation loses its region", func(t *testing.T) {
		wantCode(t, try(t, tx, `UPDATE variations SET region_id = NULL WHERE id = $1`, regional), codeCheck)
	})
	t.Run("regional variation becomes spanish with a slug", func(t *testing.T) {
		wantCode(t, try(t, tx, `UPDATE variations SET region_id = NULL, slug = $2 WHERE id = $1`, regional, uniqKey(t, "allioli")), codeOK)
	})
}

// Deleting a dish removes its Spanish variations and their recipes; deleting
// a region leaves every Spanish variation alone.
func TestIntegrationSpainWideVariationCascades(t *testing.T) {
	tx := testdb.Tx(t)
	b := newSpainWideBaseline(t, tx)
	spanish := insertSpainWideVariation(t, tx, b.other, "allioli-de-codony")
	recipe := insertRecipe(t, tx, spanish)

	mustExec(t, tx, `DELETE FROM regions WHERE id = $1 OR id = $2`, b.gal, b.ast)
	if n := countRows(t, tx, `SELECT count(*) FROM variations WHERE dish_id = $1 AND region_id IS NULL`, b.allioli); n != 1 {
		t.Errorf("allioli spanish variations after deleting regions: got %d, want 1", n)
	}

	mustExec(t, tx, `DELETE FROM dishes WHERE id = $1`, b.other)
	if n := countRows(t, tx, `SELECT count(*) FROM variations WHERE id = $1`, spanish); n != 0 {
		t.Errorf("spanish variation after deleting its dish: got %d rows, want 0", n)
	}
	if n := countRows(t, tx, `SELECT count(*) FROM recipes WHERE id = $1`, recipe); n != 0 {
		t.Errorf("spanish recipe after deleting its dish: got %d rows, want 0", n)
	}
}
