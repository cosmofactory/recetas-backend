package catalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

// noContentVersion is the content version before the first import.
const noContentVersion int64 = 0

// snapshotTx is a read-only REPEATABLE READ transaction: every statement of
// a multi-query read sees the same committed state, even mid-import.
var snapshotTx = pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}

// txBeginner is the part of *pgxpool.Pool a snapshot needs; a pgx.Tx lacks it.
type txBeginner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

type pgRepository struct {
	db catalogdb.DBTX
	q  *catalogdb.Queries
}

var _ Repository = (*pgRepository)(nil)

func NewPGRepository(db catalogdb.DBTX) Repository {
	return &pgRepository{db: db, q: catalogdb.New(db)}
}

func (r *pgRepository) ContentVersion(ctx context.Context) (int64, error) {
	cv, err := r.q.LatestContentVersion(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return noContentVersion, nil
	}
	if err != nil {
		return 0, fmt.Errorf("latest content version: %w", err)
	}
	return cv.ID, nil
}

// snapshot runs fn in one snapshotTx so its queries agree during an import.
// Without a pool, as inside a test's transaction, fn runs on the queries as is.
func (r *pgRepository) snapshot(ctx context.Context, fn func(q *catalogdb.Queries) error) error {
	beginner, ok := r.db.(txBeginner)
	if !ok {
		return fn(r.q)
	}
	return pgx.BeginTxFunc(ctx, beginner, snapshotTx, func(tx pgx.Tx) error {
		return fn(r.q.WithTx(tx))
	})
}

func (r *pgRepository) Regions(ctx context.Context, l Locale) ([]Region, error) {
	var regions []Region
	err := r.snapshot(ctx, func(q *catalogdb.Queries) error {
		var err error
		regions, err = readRegions(ctx, q, l)
		return err
	})
	return regions, err
}

func (r *pgRepository) Dishes(ctx context.Context, l Locale) ([]Dish, error) {
	var dishes []Dish
	err := r.snapshot(ctx, func(q *catalogdb.Queries) error {
		var err error
		dishes, err = readDishes(ctx, q, l)
		return err
	})
	return dishes, err
}

func (r *pgRepository) Recipe(ctx context.Context, dishKey, regionKey string, l Locale) (Recipe, error) {
	var rc Recipe
	err := r.snapshot(ctx, func(q *catalogdb.Queries) error {
		var err error
		rc, err = readRecipe(ctx, q, recipeRef{dishKey: dishKey, regionKey: regionKey}, l)
		return err
	})
	return rc, err
}

// recipeRef names a variation's recipe by its dish and region.
type recipeRef struct {
	dishKey, regionKey string
}

// readRegions takes two queries: the regions, then every regional dish.
func readRegions(ctx context.Context, q *catalogdb.Queries, l Locale) ([]Region, error) {
	rows, err := q.ListRegions(ctx, catalogdb.ListRegionsParams{Locale: string(l), Spanish: string(localeSpanish)})
	if err != nil {
		return nil, fmt.Errorf("list regions: %w", err)
	}
	regions, err := regionsFromRows(rows, l)
	if err != nil {
		return nil, err
	}
	dishes, err := q.ListRegionalDishes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list regional dishes: %w", err)
	}
	attachRegionalDishes(regions, dishes)
	return regions, nil
}

// readDishes takes two queries: the dishes, then every variation.
func readDishes(ctx context.Context, q *catalogdb.Queries, l Locale) ([]Dish, error) {
	rows, err := q.ListDishes(ctx, catalogdb.ListDishesParams{Locale: string(l), Spanish: string(localeSpanish)})
	if err != nil {
		return nil, fmt.Errorf("list dishes: %w", err)
	}
	dishes, err := dishesFromRows(rows, l)
	if err != nil {
		return nil, err
	}
	variations, err := q.ListVariations(ctx, catalogdb.ListVariationsParams{Locale: string(l), Spanish: string(localeSpanish)})
	if err != nil {
		return nil, fmt.Errorf("list variations: %w", err)
	}
	if err := attachVariations(dishes, variations, l); err != nil {
		return nil, err
	}
	return dishes, nil
}

// readRecipe takes three queries: the recipe, its ingredients and its steps.
func readRecipe(ctx context.Context, q *catalogdb.Queries, ref recipeRef, l Locale) (Recipe, error) {
	row, err := q.GetRecipeByDishRegion(ctx, catalogdb.GetRecipeByDishRegionParams{
		Locale: string(l), Spanish: string(localeSpanish), DishKey: ref.dishKey, RegionKey: ref.regionKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Recipe{}, fmt.Errorf("recipe %s/%s: %w", ref.dishKey, ref.regionKey, ErrNotFound)
	}
	if err != nil {
		return Recipe{}, fmt.Errorf("get recipe %s/%s: %w", ref.dishKey, ref.regionKey, err)
	}
	rc, err := recipeFromRow(row, l)
	if err != nil {
		return Recipe{}, err
	}
	stored := storedRecipe{id: row.ID, key: rc.RecipeKey}
	if rc.Ingredients, err = readIngredients(ctx, q, stored, l); err != nil {
		return Recipe{}, err
	}
	if rc.Steps, err = readSteps(ctx, q, stored, l); err != nil {
		return Recipe{}, err
	}
	return rc, nil
}

// storedRecipe is a recipe's row id and its stable key for error messages.
type storedRecipe struct {
	id  int64
	key string
}

func readIngredients(ctx context.Context, q *catalogdb.Queries, rec storedRecipe, l Locale) ([]Ingredient, error) {
	rows, err := q.ListRecipeIngredients(ctx, catalogdb.ListRecipeIngredientsParams{Locale: string(l), RecipeID: rec.id})
	if err != nil {
		return nil, fmt.Errorf("list ingredients of %s: %w", rec.key, err)
	}
	return ingredientsFromRows(rows, rec.key, l)
}

func readSteps(ctx context.Context, q *catalogdb.Queries, rec storedRecipe, l Locale) ([]string, error) {
	rows, err := q.ListRecipeSteps(ctx, catalogdb.ListRecipeStepsParams{Locale: string(l), RecipeID: rec.id})
	if err != nil {
		return nil, fmt.Errorf("list steps of %s: %w", rec.key, err)
	}
	return stepsFromRows(rows, rec.key, l)
}
