package importer

import (
	"context"
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

// positioned flattens texts at 1-based positions into one row per locale.
type positioned struct {
	positions []int64
	locales   []string
	texts     []string
}

func (p *positioned) add(position int, t Text) {
	for i, v := range t.values() {
		p.positions = append(p.positions, int64(position))
		p.locales = append(p.locales, locales[i])
		p.texts = append(p.texts, v)
	}
}

func (w *writer) insertIngredients(ctx context.Context, id int64, ingredients []Ingredient) error {
	quantities := make([]float64, 0, len(ingredients))
	units := make([]string, 0, len(ingredients))
	var names positioned
	for i, ing := range ingredients {
		quantities = append(quantities, ing.Quantity)
		units = append(units, unitOrEmpty(ing.Unit))
		names.add(i+1, ing.Name)
	}
	err := w.q.ImportInsertIngredients(ctx, catalogdb.ImportInsertIngredientsParams{
		RecipeID: id, Quantities: quantities, Units: units,
	})
	if err != nil {
		return fmt.Errorf("ingredients: %w", err)
	}
	err = w.q.ImportInsertIngredientTranslations(ctx, catalogdb.ImportInsertIngredientTranslationsParams{
		RecipeID: id, Positions: names.positions, Locales: names.locales, Names: names.texts,
	})
	if err != nil {
		return fmt.Errorf("ingredient names: %w", err)
	}
	return nil
}

// unitOrEmpty encodes a counted item as "", which the query stores as NULL.
func unitOrEmpty(unit *string) string {
	if unit == nil {
		return ""
	}
	return *unit
}

func (w *writer) insertSteps(ctx context.Context, id int64, steps []Text) error {
	var texts positioned
	for i, s := range steps {
		texts.add(i+1, s)
	}
	err := w.q.ImportInsertSteps(ctx, catalogdb.ImportInsertStepsParams{RecipeID: id, Count: int64(len(steps))})
	if err != nil {
		return fmt.Errorf("steps: %w", err)
	}
	err = w.q.ImportInsertStepTranslations(ctx, catalogdb.ImportInsertStepTranslationsParams{
		RecipeID: id, Positions: texts.positions, Locales: texts.locales, Texts: texts.texts,
	})
	if err != nil {
		return fmt.Errorf("step texts: %w", err)
	}
	return nil
}
