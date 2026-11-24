package importer

import "slices"

// placedVariation is a bundle variation with the key of its dish and its
// 1-based position there.
type placedVariation struct {
	dishKey   string
	position  int
	variation Variation
}

func placed(dishes []Dish) []placedVariation {
	var out []placedVariation
	for _, d := range dishes {
		for j, v := range d.Variations {
			out = append(out, placedVariation{dishKey: d.Key, position: j + 1, variation: v})
		}
	}
	return out
}

// samePlace is true when the stored variation is still the bundle one: the
// same dish and region, or for a Spanish variation the same slug.
func (s storedVariation) samePlace(pv placedVariation) bool {
	return s.dishKey == pv.dishKey && s.variation.RegionKey == pv.variation.RegionKey &&
		s.variation.VariationSlug == pv.variation.VariationSlug
}

// variationChanges tells the recipe diff which recipes lose or gain their
// variation row, by recipe key.
type variationChanges struct {
	created map[string]bool
	deleted []string
}

// diffVariations matches variations by recipe key. A variation that moved to
// another dish or region, or changed its slug, is deleted and created again,
// so the unique (dish, region) and (dish, slug) never collide while rows are
// being written.
func (p *plan) diffVariations(cur map[string]storedVariation, dishes []Dish) variationChanges {
	var c Counts
	vc := variationChanges{created: map[string]bool{}}
	keep := map[string]bool{}
	for _, pv := range placed(dishes) {
		key := pv.variation.RecipeKey
		old, ok := cur[key]
		keep[key] = ok && old.samePlace(pv)
		w := variationWriteFor(old, keep[key], pv)
		if !w.needed() {
			continue
		}
		p.variations = append(p.variations, w)
		if !keep[key] {
			vc.created[key] = true
		}
		if w.reported() {
			c.add(keep[key])
		}
	}
	vc.deleted = missingKeys(cur, keep)
	for _, key := range vc.deleted {
		p.deleteVariations = append(p.deleteVariations, cur[key].id)
	}
	c.Deleted = len(vc.deleted)
	p.changes[EntityVariations] = c
	return vc
}

// variationWriteFor writes pv over old when kept, else as a new row.
func variationWriteFor(old storedVariation, kept bool, pv placedVariation) variationWrite {
	w := variationWrite{dishKey: pv.dishKey, position: pv.position, variation: pv.variation}
	if kept {
		w.id = old.id
		w.edited = !sameVariation(old.variation, pv.variation)
		w.moved = old.position != pv.position
	}
	return w
}

// missingKeys returns the stored keys that aren't kept, sorted.
func missingKeys[T any](cur map[string]T, keep map[string]bool) []string {
	var keys []string
	for key := range cur {
		if !keep[key] {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

// diffRecipes matches recipes by recipe key; a recipe whose variation row is
// new is new too (the old one went with its variation).
func (p *plan) diffRecipes(cur map[string]storedRecipe, recipes []Recipe, vc variationChanges) {
	var c Counts
	for _, key := range vc.deleted {
		if _, ok := cur[key]; ok {
			c.Deleted++
		}
	}
	for _, rc := range recipes {
		old, ok := cur[rc.RecipeKey]
		ok = ok && !vc.created[rc.RecipeKey]
		if ok && sameRecipe(old.recipe, rc) {
			continue
		}
		w := recipeWrite{recipe: rc}
		if ok {
			w.id = old.id
		}
		c.add(ok)
		p.recipes = append(p.recipes, w)
	}
	p.changes[EntityRecipes] = c
}
