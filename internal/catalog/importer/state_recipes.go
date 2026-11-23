package importer

import (
	"context"
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

func loadRecipes(ctx context.Context, q *catalogdb.Queries) (map[string]storedRecipe, error) {
	rows, err := q.ImportListRecipes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	ingredients, err := loadIngredients(ctx, q)
	if err != nil {
		return nil, err
	}
	lists, err := loadTextLists(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make(map[string]storedRecipe, len(rows))
	for _, r := range rows {
		rc := Recipe{
			RecipeKey:    r.RecipeKey,
			TimeMinutes:  int(r.TimeMinutes),
			BaseServings: int(r.BaseServings),
			Ingredients:  ingredients[r.ID],
			Steps:        lists.steps[r.ID],
			Tips:         lists.tips[r.ID],
		}
		if r.HasTimeNote {
			rc.TimeNote = &Text{r.TimeNoteEn, r.TimeNoteEs, r.TimeNoteRu}
		}
		out[r.RecipeKey] = storedRecipe{id: r.ID, recipe: rc}
	}
	return out, nil
}

// textLists are every recipe's steps and tips in order, by recipe id.
type textLists struct {
	steps map[int64][]Text
	tips  map[int64][]Text
}

func loadTextLists(ctx context.Context, q *catalogdb.Queries) (textLists, error) {
	steps, err := loadSteps(ctx, q)
	if err != nil {
		return textLists{}, err
	}
	tips, err := loadTips(ctx, q)
	if err != nil {
		return textLists{}, err
	}
	return textLists{steps: steps, tips: tips}, nil
}

// loadIngredients returns every recipe's ingredients in order, by recipe id.
func loadIngredients(ctx context.Context, q *catalogdb.Queries) (map[int64][]Ingredient, error) {
	rows, err := q.ImportListIngredients(ctx)
	if err != nil {
		return nil, fmt.Errorf("list ingredients: %w", err)
	}
	out := map[int64][]Ingredient{}
	for _, r := range rows {
		ing := Ingredient{Quantity: r.Quantity, Unit: r.Unit, Name: Text{r.NameEn, r.NameEs, r.NameRu}}
		out[r.RecipeID] = append(out[r.RecipeID], ing)
	}
	return out, nil
}

// loadSteps returns every recipe's steps in order, by recipe id.
func loadSteps(ctx context.Context, q *catalogdb.Queries) (map[int64][]Text, error) {
	rows, err := q.ImportListSteps(ctx)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}
	out := map[int64][]Text{}
	for _, r := range rows {
		out[r.RecipeID] = append(out[r.RecipeID], Text{r.TextEn, r.TextEs, r.TextRu})
	}
	return out, nil
}

// loadTips returns every recipe's tips in order, by recipe id.
func loadTips(ctx context.Context, q *catalogdb.Queries) (map[int64][]Text, error) {
	rows, err := q.ImportListTips(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tips: %w", err)
	}
	out := map[int64][]Text{}
	for _, r := range rows {
		out[r.RecipeID] = append(out[r.RecipeID], Text{r.TextEn, r.TextEs, r.TextRu})
	}
	return out, nil
}
