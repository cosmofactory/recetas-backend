package importer

import "slices"

// placedVariation is a bundle variation with the key of its dish.
type placedVariation struct {
	dishKey   string
	variation Variation
}

func placed(dishes []Dish) []placedVariation {
	var out []placedVariation
	for _, d := range dishes {
		for _, v := range d.Variations {
			out = append(out, placedVariation{dishKey: d.Key, variation: v})
		}
	}
	return out
}

// variationChanges tells the recipe diff which recipes lose or gain their
// variation row, by recipe key.
type variationChanges struct {
	created map[string]bool
	deleted []string
}

// diffVariations matches variations by recipe key. A variation that moved to
// another dish or region is deleted and created again, so the unique
// (dish, region) pair never collides while rows are being written.
func (p *plan) diffVariations(cur map[string]storedVariation, dishes []Dish) variationChanges {
	var c Counts
	vc := variationChanges{created: map[string]bool{}}
	keep := map[string]bool{}
	for _, pv := range placed(dishes) {
		key := pv.variation.RecipeKey
		old, ok := cur[key]
		keep[key] = ok && old.dishKey == pv.dishKey && old.variation.RegionKey == pv.variation.RegionKey
		if keep[key] && sameVariation(old.variation, pv.variation) {
			continue
		}
		w := variationWrite{dishKey: pv.dishKey, variation: pv.variation}
		if keep[key] {
			w.id = old.id
		} else {
			vc.created[key] = true
		}
		c.add(keep[key])
		p.variations = append(p.variations, w)
	}
	vc.deleted = missingKeys(cur, keep)
	for _, key := range vc.deleted {
		p.deleteVariations = append(p.deleteVariations, cur[key].id)
	}
	c.Deleted = len(vc.deleted)
	p.changes[EntityVariations] = c
	return vc
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
