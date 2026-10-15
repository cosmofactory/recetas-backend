package catalog_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/cosmofactory/recetas-backend/internal/catalog"
	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogtest"
	"github.com/cosmofactory/recetas-backend/internal/platform/testdb"
)

// countingDB counts every statement sent through it.
type countingDB struct {
	db      catalogdb.DBTX
	queries int
}

func (c *countingDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	c.queries++
	return c.db.Exec(ctx, sql, args...)
}

func (c *countingDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.queries++
	return c.db.Query(ctx, sql, args...)
}

func (c *countingDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	c.queries++
	return c.db.QueryRow(ctx, sql, args...)
}

// pgFixture is a loaded fixture in a rolled-back transaction, with a
// repository reading through a statement counter.
type pgFixture struct {
	tx      pgx.Tx
	f       catalogtest.Fixture
	counter *countingDB
	repo    catalog.Repository
}

func loadPGFixture(t *testing.T) pgFixture {
	t.Helper()
	tx := testdb.Tx(t)
	f := catalogtest.Load(t, tx)
	counter := &countingDB{db: tx}
	return pgFixture{tx: tx, f: f, counter: counter, repo: catalog.NewPGRepository(counter)}
}

func (p pgFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := p.tx.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

// Rows outside the fixture may exist in the shared test database; compare
// only the fixture's own keys.
func fixtureRegionsOnly(got []catalog.Region) []catalog.Region {
	return slices.DeleteFunc(got, func(r catalog.Region) bool { return r.Key != "gal" && r.Key != "ast" })
}

func fixtureDishesOnly(got []catalog.Dish) []catalog.Dish {
	return slices.DeleteFunc(got, func(d catalog.Dish) bool { return d.Key != "tortilla" && d.Key != "cocido" })
}

func TestPGRepository_RegionsPerLocale(t *testing.T) {
	p := loadPGFixture(t)
	for _, l := range locales {
		t.Run(string(l), func(t *testing.T) {
			got, err := p.repo.Regions(context.Background(), l)
			if err != nil {
				t.Fatalf("Regions(%s): %v", l, err)
			}
			assertDeepEqual(t, "regions", fixtureRegionsOnly(got), domainRegions(l))
		})
	}
}

func TestPGRepository_DishesPerLocale(t *testing.T) {
	p := loadPGFixture(t)
	for _, l := range locales {
		t.Run(string(l), func(t *testing.T) {
			got, err := p.repo.Dishes(context.Background(), l)
			if err != nil {
				t.Fatalf("Dishes(%s): %v", l, err)
			}
			assertDeepEqual(t, "dishes", fixtureDishesOnly(got), domainDishes(l))
		})
	}
}

func TestPGRepository_RecipePerLocale(t *testing.T) {
	p := loadPGFixture(t)
	for _, l := range locales {
		for _, pair := range fixtureRecipePairs {
			t.Run(fmt.Sprintf("%s/%s/%s", l, pair.dish, pair.region), func(t *testing.T) {
				got, err := p.repo.Recipe(context.Background(), pair.dish, pair.region, l)
				if err != nil {
					t.Fatalf("Recipe: %v", err)
				}
				assertDeepEqual(t, "recipe", got, mustDomainRecipe(t, pair, l))
			})
		}
	}
}

func TestPGRepository_RecipeMapsNullableAndFractionalValues(t *testing.T) {
	p := loadPGFixture(t)
	ctx := context.Background()
	fabada, err := p.repo.Recipe(ctx, "cocido", "ast", "es")
	if err != nil {
		t.Fatalf("Recipe fabada: %v", err)
	}
	if fabada.TimeNote == nil || *fabada.TimeNote != "+ remojo" {
		t.Errorf("fabada: got time note %v, want %q", fabada.TimeNote, "+ remojo")
	}
	chorizo, err := p.repo.Recipe(ctx, "tortilla", "ast", "es")
	if err != nil {
		t.Fatalf("Recipe chorizo: %v", err)
	}
	if chorizo.TimeNote != nil {
		t.Errorf("chorizo: got time note %q, want nil", *chorizo.TimeNote)
	}
	requireLen(t, "chorizo ingredients", len(chorizo.Ingredients), 5)
	salt := chorizo.Ingredients[4]
	if salt.Quantity != 0.5 || salt.Unit == nil || *salt.Unit != "tsp" {
		t.Errorf("chorizo salt: got %v %v, want 0.5 tsp", salt.Quantity, salt.Unit)
	}
	if eggs := chorizo.Ingredients[0]; eggs.Unit != nil || eggs.Quantity != 6 {
		t.Errorf("chorizo eggs: got %v %v, want 6 with nil unit", eggs.Quantity, eggs.Unit)
	}
}

func TestPGRepository_PhotosMapToTheirRoles(t *testing.T) {
	p := loadPGFixture(t)
	dishes, err := p.repo.Dishes(context.Background(), "en")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	dishes = fixtureDishesOnly(dishes)
	requireLen(t, "fixture dishes", len(dishes), 2)
	tortilla := dishes[0]
	requireLen(t, "tortilla variations", len(tortilla.Variations), 2)
	assertDeepEqual(t, "tortilla photos", tortilla.Photos,
		photos("dishes/tortilla/thumb-1a2b3c4d", "dishes/tortilla/card-5e6f7a8b", "dishes/tortilla/hero-9c0d1e2f"))
	assertDeepEqual(t, "tortilla/gal photos", tortilla.Variations[0].Photos,
		photos("dishes/tortilla/gal/thumb-3a4b5c6d", "dishes/tortilla/gal/card-7e8f9a0b", "dishes/tortilla/gal/hero-1c2d3e4f"))
	assertDeepEqual(t, "tortilla/ast photos", tortilla.Variations[1].Photos, catalog.Photos{})
}

func requireLen(t *testing.T, what string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("got %d %s, want %d", got, what, want)
	}
}

func photos(thumb, card, hero string) catalog.Photos {
	return catalog.Photos{Thumb: &thumb, Card: &card, Hero: &hero}
}

func TestPGRepository_RecipeOfUnknownPairIsNotFound(t *testing.T) {
	p := loadPGFixture(t)
	for _, pair := range []recipePair{{"nope", "gal"}, {"tortilla", "nope"}, {"cocido", "gal"}} {
		t.Run(pair.dish+"/"+pair.region, func(t *testing.T) {
			_, err := p.repo.Recipe(context.Background(), pair.dish, pair.region, "en")
			assertNotFound(t, err)
		})
	}
}

// Swapping sort orders inside the transaction makes insertion order and
// editorial order disagree, so only an explicit ORDER BY passes.
func TestPGRepository_ListsFollowEditorialOrder(t *testing.T) {
	p := loadPGFixture(t)
	reorderFixture(t, p)
	ctx := context.Background()

	regions, err := p.repo.Regions(ctx, "en")
	if err != nil {
		t.Fatalf("Regions: %v", err)
	}
	dishes, err := p.repo.Dishes(ctx, "en")
	if err != nil {
		t.Fatalf("Dishes: %v", err)
	}
	regions, dishes = fixtureRegionsOnly(regions), fixtureDishesOnly(dishes)
	requireLen(t, "fixture regions", len(regions), 2)
	requireLen(t, "fixture dishes", len(dishes), 2)
	assertDeepEqual(t, "region keys", regionKeys(regions), []string{"ast", "gal"})
	assertDeepEqual(t, "galicia regional dishes", regions[1].RegionalDishes,
		[]string{"Lacón con grelos", "Empanada gallega", "Pulpo á feira"})
	assertDeepEqual(t, "dish keys", dishKeys(dishes), []string{"cocido", "tortilla"})
	assertDeepEqual(t, "tortilla variation regions", variationRegions(dishes[1]), []string{"ast", "gal"})
}

// reorderFixture puts Asturias before Galicia, cocido before tortilla and
// reverses Galicia's regional dishes, in two steps to dodge unique keys.
func reorderFixture(t *testing.T, p pgFixture) {
	t.Helper()
	gal, ast := p.f.RegionIDs["gal"], p.f.RegionIDs["ast"]
	tortilla, cocido := p.f.DishIDs["tortilla"], p.f.DishIDs["cocido"]
	p.exec(t, `UPDATE regions SET sort_order = sort_order + 1000 WHERE id = ANY($1)`, []int64{gal, ast})
	p.exec(t, `UPDATE regions SET sort_order = CASE id WHEN $1 THEN 2 ELSE 1 END WHERE id = ANY($2)`, gal, []int64{gal, ast})
	p.exec(t, `UPDATE dishes SET sort_order = sort_order + 1000 WHERE id = ANY($1)`, []int64{tortilla, cocido})
	p.exec(t, `UPDATE dishes SET sort_order = CASE id WHEN $1 THEN 3 ELSE 1 END WHERE id = ANY($2)`, tortilla, []int64{tortilla, cocido})
	p.exec(t, `UPDATE region_regional_dishes SET position = position + 10 WHERE region_id = $1`, gal)
	p.exec(t, `UPDATE region_regional_dishes SET position = 14 - position WHERE region_id = $1`, gal)
}

func regionKeys(rs []catalog.Region) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Key)
	}
	return out
}

func dishKeys(ds []catalog.Dish) []string {
	var out []string
	for _, d := range ds {
		out = append(out, d.Key)
	}
	return out
}

func variationRegions(d catalog.Dish) []string {
	var out []string
	for _, v := range d.Variations {
		out = append(out, v.RegionKey)
	}
	return out
}

func TestPGRepository_ContentVersionIsTheLatestImport(t *testing.T) {
	p := loadPGFixture(t)

	got, err := p.repo.ContentVersion(context.Background())

	if err != nil {
		t.Fatalf("ContentVersion: %v", err)
	}
	if got != p.f.ContentVersionID {
		t.Errorf("got content version %d, want %d", got, p.f.ContentVersionID)
	}
}

func TestPGRepository_ContentVersionIsZeroBeforeAnyImport(t *testing.T) {
	p := loadPGFixture(t)
	p.exec(t, `DELETE FROM content_versions`)

	got, err := p.repo.ContentVersion(context.Background())

	if err != nil {
		t.Fatalf("ContentVersion: got error %v, want nil", err)
	}
	if got != 0 {
		t.Errorf("got content version %d, want 0", got)
	}
}
