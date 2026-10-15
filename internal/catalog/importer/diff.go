package importer

import "reflect"

// Counts are the rows of one entity an import creates, updates and deletes.
type Counts struct {
	Created int
	Updated int
	Deleted int
}

func (c *Counts) add(exists bool) {
	if exists {
		c.Updated++
	} else {
		c.Created++
	}
}

// Entity names in Summary.Changes, in print order.
const (
	EntityRegions    = "regions"
	EntityDishes     = "dishes"
	EntityVariations = "variations"
	EntityRecipes    = "recipes"
)

// Entities lists the Summary.Changes keys in print order.
var Entities = []string{EntityRegions, EntityDishes, EntityVariations, EntityRecipes}

// ordered is a region or dish row: id 0 creates it.
type ordered struct {
	id       int64
	oldOrder int
	order    int
}

func (o ordered) moved() bool { return o.id != 0 && o.oldOrder != o.order }

type orderedWrite[T any] struct {
	ordered
	value T
}

type (
	regionWrite = orderedWrite[Region]
	dishWrite   = orderedWrite[Dish]
)

// variationWrite and recipeWrite create their row when id is 0.
type variationWrite struct {
	id        int64
	dishKey   string
	variation Variation
}

type recipeWrite struct {
	id     int64
	recipe Recipe
}

// plan is what turns the stored catalog into the bundle. Deletes run first;
// creates and updates are in bundle order.
type plan struct {
	deleteVariations []int64
	deleteDishes     []int64
	deleteRegions    []int64
	regions          []regionWrite
	dishes           []dishWrite
	variations       []variationWrite
	recipes          []recipeWrite
	changes          map[string]Counts
}

func (p plan) empty() bool {
	for _, c := range p.changes {
		if c != (Counts{}) {
			return false
		}
	}
	return true
}

func diff(cur state, b Bundle) plan {
	p := plan{changes: map[string]Counts{}}
	p.diffRegions(cur.regions, b.Regions)
	p.diffDishes(cur.dishes, b.Dishes)
	vc := p.diffVariations(cur.variations, b.Dishes)
	p.diffRecipes(cur.recipes, b.Recipes, vc)
	return p
}

func (p *plan) diffRegions(cur map[string]storedRegion, regions []Region) {
	d := diffOrdered(cur, regions, func(r Region) string { return r.Key }, sameRegion)
	p.regions, p.deleteRegions, p.changes[EntityRegions] = d.writes, d.deleted, d.counts
}

func (p *plan) diffDishes(cur map[string]storedDish, dishes []Dish) {
	d := diffOrdered(cur, dishes, func(d Dish) string { return d.Key }, sameDish)
	p.dishes, p.deleteDishes, p.changes[EntityDishes] = d.writes, d.deleted, d.counts
}

// orderedDiff turns a stored region or dish list into the bundle's: the
// writes in bundle order and the ids to delete.
type orderedDiff[T any] struct {
	writes  []orderedWrite[T]
	deleted []int64
	counts  Counts
}

// diffOrdered matches rows by key; a row is rewritten when its content or
// its position changed.
func diffOrdered[T any](cur map[string]storedOrdered[T], items []T, key func(T) string, same func(a, b T) bool) orderedDiff[T] {
	var d orderedDiff[T]
	keep := map[string]bool{}
	for i, item := range items {
		k := key(item)
		keep[k] = true
		old, ok := cur[k]
		w := orderedWrite[T]{ordered{id: old.id, oldOrder: old.order, order: i + 1}, item}
		if ok && !w.moved() && same(old.value, item) {
			continue
		}
		d.counts.add(ok)
		d.writes = append(d.writes, w)
	}
	for _, k := range missingKeys(cur, keep) {
		d.deleted = append(d.deleted, cur[k].id)
	}
	d.counts.Deleted = len(d.deleted)
	return d
}

func sameRegion(a, b Region) bool {
	a.RegionalDishes, b.RegionalDishes = nonEmpty(a.RegionalDishes), nonEmpty(b.RegionalDishes)
	return reflect.DeepEqual(a, b)
}

func sameDish(a, b Dish) bool {
	a.Variations, b.Variations = nil, nil
	a.Photos, b.Photos = somePhotos(a.Photos), somePhotos(b.Photos)
	return reflect.DeepEqual(a, b)
}

func sameVariation(a, b Variation) bool {
	a.Photos, b.Photos = somePhotos(a.Photos), somePhotos(b.Photos)
	return reflect.DeepEqual(a, b)
}

func sameRecipe(a, b Recipe) bool {
	return reflect.DeepEqual(a, b)
}

func nonEmpty[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}

// somePhotos is nil when no role has a photo.
func somePhotos(p *Photos) *Photos {
	if p == nil || *p == (Photos{}) {
		return nil
	}
	return p
}
