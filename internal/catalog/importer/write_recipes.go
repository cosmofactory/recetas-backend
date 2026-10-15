package importer

import (
	"context"
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

func (w *writer) writeRecipes(ctx context.Context, writes []recipeWrite) error {
	for _, rw := range writes {
		if err := w.writeRecipe(ctx, rw); err != nil {
			return fmt.Errorf("write recipe %s: %w", rw.recipe.RecipeKey, err)
		}
	}
	return nil
}

// writeRecipe saves the recipe row and replaces its time note, ingredients
// and steps, which are keyed by position.
func (w *writer) writeRecipe(ctx context.Context, rw recipeWrite) error {
	id, err := w.saveRecipeRow(ctx, rw)
	if err != nil {
		return err
	}
	if rw.id != 0 {
		if err := w.clearRecipe(ctx, id); err != nil {
			return err
		}
	}
	if err := w.insertTimeNote(ctx, id, rw.recipe.TimeNote); err != nil {
		return err
	}
	if err := w.insertIngredients(ctx, id, rw.recipe.Ingredients); err != nil {
		return err
	}
	return w.insertSteps(ctx, id, rw.recipe.Steps)
}

func (w *writer) saveRecipeRow(ctx context.Context, rw recipeWrite) (int64, error) {
	rc := rw.recipe
	if rw.id == 0 {
		return w.q.ImportInsertRecipe(ctx, catalogdb.ImportInsertRecipeParams{
			VariationID:  w.variations[rc.RecipeKey],
			TimeMinutes:  int64(rc.TimeMinutes),
			BaseServings: int64(rc.BaseServings),
		})
	}
	return rw.id, w.q.ImportUpdateRecipe(ctx, catalogdb.ImportUpdateRecipeParams{
		ID:           rw.id,
		TimeMinutes:  int64(rc.TimeMinutes),
		BaseServings: int64(rc.BaseServings),
	})
}

func (w *writer) clearRecipe(ctx context.Context, id int64) error {
	if err := w.q.ImportDeleteTimeNotes(ctx, id); err != nil {
		return fmt.Errorf("clear time note: %w", err)
	}
	if err := w.q.ImportDeleteIngredients(ctx, id); err != nil {
		return fmt.Errorf("clear ingredients: %w", err)
	}
	if err := w.q.ImportDeleteSteps(ctx, id); err != nil {
		return fmt.Errorf("clear steps: %w", err)
	}
	return nil
}

func (w *writer) insertTimeNote(ctx context.Context, id int64, note *Text) error {
	if note == nil {
		return nil
	}
	err := w.q.ImportInsertTimeNotes(ctx, catalogdb.ImportInsertTimeNotesParams{
		RecipeID: id, Locales: locales, TimeNotes: note.values(),
	})
	if err != nil {
		return fmt.Errorf("time note: %w", err)
	}
	return nil
}
