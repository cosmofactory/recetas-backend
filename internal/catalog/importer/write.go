package importer

import (
	"context"
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

// writer applies a plan, tracking the ids rows get by key.
type writer struct {
	q          *catalogdb.Queries
	regions    map[string]int64 // by key
	dishes     map[string]int64 // by key
	variations map[string]int64 // by recipe key
}

func newWriter(q *catalogdb.Queries, cur state) *writer {
	w := &writer{q: q, regions: map[string]int64{}, dishes: map[string]int64{}, variations: map[string]int64{}}
	for key, r := range cur.regions {
		w.regions[key] = r.id
	}
	for key, d := range cur.dishes {
		w.dishes[key] = d.id
	}
	for key, v := range cur.variations {
		w.variations[key] = v.id
	}
	return w
}

func (w *writer) execute(ctx context.Context, p plan) error {
	if err := w.deleteRows(ctx, p); err != nil {
		return err
	}
	if err := w.writeRegions(ctx, p.regions); err != nil {
		return err
	}
	if err := w.writeDishes(ctx, p.dishes); err != nil {
		return err
	}
	if err := w.writeVariations(ctx, p.variations); err != nil {
		return err
	}
	return w.writeRecipes(ctx, p.recipes)
}

// deleteRows removes variations first, then dishes and regions; children
// cascade.
func (w *writer) deleteRows(ctx context.Context, p plan) error {
	for _, del := range []struct {
		what string
		ids  []int64
		run  func(context.Context, []int64) error
	}{
		{EntityVariations, p.deleteVariations, w.q.ImportDeleteVariations},
		{EntityDishes, p.deleteDishes, w.q.ImportDeleteDishes},
		{EntityRegions, p.deleteRegions, w.q.ImportDeleteRegions},
	} {
		if len(del.ids) == 0 {
			continue
		}
		if err := del.run(ctx, del.ids); err != nil {
			return fmt.Errorf("delete %s: %w", del.what, err)
		}
	}
	return nil
}

// movedIDs are the rows whose sort order changes. They are parked on negative
// orders first, so no new order collides with an old one under the unique
// constraint.
func movedIDs[T interface{ row() ordered }](writes []T) []int64 {
	var ids []int64
	for _, w := range writes {
		if r := w.row(); r.moved() {
			ids = append(ids, r.id)
		}
	}
	return ids
}

func (o ordered) row() ordered { return o }
