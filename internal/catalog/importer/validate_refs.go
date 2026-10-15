package importer

// recipeRef is a variation's reference to its recipe.
type recipeRef struct {
	key string
	at  pointer
}

// recipeRefs are the variations' recipe keys, in bundle order.
type recipeRefs struct {
	keys keySet
	refs []recipeRef
}

func validateDishes(ps *problems, dishes []Dish, regionKeys keySet) recipeRefs {
	keys := newKeySet("dish")
	refs := recipeRefs{keys: newKeySet("variation recipe")}
	for i, d := range dishes {
		at := pointer("/dishes").at(i)
		keys.check(ps, at.at("key"), d.Key)
		checkText(ps, at.at("name"), d.Name)
		checkText(ps, at.at("description"), d.Description)
		checkEnum(ps, at.at("course"), d.Course, courses)
		checkEnum(ps, at.at("season"), d.Season, seasons)
		checkPhotos(ps, at.at("photos"), d.Photos, "dishes/"+d.Key+"/")
		validateVariations(ps, at, d, variationContext{regions: regionKeys, refs: &refs})
	}
	return refs
}

type variationContext struct {
	regions keySet
	refs    *recipeRefs
}

func validateVariations(ps *problems, dishAt pointer, d Dish, vc variationContext) {
	inDish := map[string]bool{}
	for j, v := range d.Variations {
		at := dishAt.at("variations", j)
		switch {
		case !vc.regions.seen[v.RegionKey]:
			ps.add(at.at("regionKey"), "unknown region %q", v.RegionKey)
		case inDish[v.RegionKey]:
			ps.add(at.at("regionKey"), "dish %q already has a variation in region %q", d.Key, v.RegionKey)
		}
		inDish[v.RegionKey] = true
		vc.refs.add(ps, at.at("recipeKey"), v.RecipeKey)
		checkText(ps, at.at("name"), v.Name)
		checkText(ps, at.at("description"), v.Description)
		checkPhotos(ps, at.at("photos"), v.Photos, "dishes/"+d.Key+"/"+v.RegionKey+"/")
	}
}

func (r *recipeRefs) add(ps *problems, at pointer, key string) {
	r.keys.check(ps, at, key)
	r.refs = append(r.refs, recipeRef{key: key, at: at})
}

func validateRecipes(ps *problems, recipes []Recipe, refs recipeRefs) {
	keys := newKeySet("recipe")
	for i, rc := range recipes {
		at := pointer("/recipes").at(i)
		known := keys.seen[rc.RecipeKey]
		keys.check(ps, at.at("recipeKey"), rc.RecipeKey)
		if !known && validSlug(rc.RecipeKey) && !refs.keys.seen[rc.RecipeKey] {
			ps.add(at.at("recipeKey"), "recipe %q is not used by any variation", rc.RecipeKey)
		}
		validateRecipe(ps, at, rc)
	}
	for _, ref := range refs.refs {
		if validSlug(ref.key) && !keys.seen[ref.key] {
			ps.add(ref.at, "no recipe with key %q", ref.key)
		}
	}
}

func validateRecipe(ps *problems, at pointer, rc Recipe) {
	if rc.TimeMinutes < 1 || rc.TimeMinutes > maxTimeMinutes {
		ps.add(at.at("timeMinutes"), "time %d is not 1-%d minutes", rc.TimeMinutes, maxTimeMinutes)
	}
	if rc.BaseServings < minServings || rc.BaseServings > maxServings {
		ps.add(at.at("baseServings"), "base servings %d is not %d-%d", rc.BaseServings, minServings, maxServings)
	}
	if rc.TimeNote != nil {
		checkText(ps, at.at("timeNote"), *rc.TimeNote)
	}
	if len(rc.Ingredients) == 0 {
		ps.add(at.at("ingredients"), "a recipe needs at least one ingredient")
	}
	for j, ing := range rc.Ingredients {
		validateIngredient(ps, at.at("ingredients", j), ing)
	}
	if len(rc.Steps) == 0 {
		ps.add(at.at("steps"), "a recipe needs at least one step")
	}
	for j, step := range rc.Steps {
		checkText(ps, at.at("steps", j), step)
	}
}

func validateIngredient(ps *problems, at pointer, ing Ingredient) {
	if !validQuantity(ing.Quantity) {
		ps.add(at.at("quantity"), "quantity %v is not above 0 and at most %d at 3 decimal places", ing.Quantity, maxQuantity)
	}
	if ing.Unit != nil {
		checkEnum(ps, at.at("unit"), *ing.Unit, units)
	}
	checkText(ps, at.at("name"), ing.Name)
}
