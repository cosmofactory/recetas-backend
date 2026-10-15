package importer

import (
	"context"
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

func (w *writer) writeDishes(ctx context.Context, writes []dishWrite) error {
	if moved := movedIDs(writes); len(moved) > 0 {
		if err := w.q.ImportParkDishOrders(ctx, moved); err != nil {
			return fmt.Errorf("park dish orders: %w", err)
		}
	}
	for _, dw := range writes {
		if err := w.writeDish(ctx, dw); err != nil {
			return fmt.Errorf("write dish %s: %w", dw.value.Key, err)
		}
	}
	return nil
}

func (w *writer) writeDish(ctx context.Context, dw dishWrite) error {
	id, err := w.saveDishRow(ctx, dw)
	if err != nil {
		return err
	}
	w.dishes[dw.value.Key] = id
	err = w.q.ImportUpsertDishTranslations(ctx, catalogdb.ImportUpsertDishTranslationsParams{
		DishID:       id,
		Locales:      locales,
		Names:        dw.value.Name.values(),
		Descriptions: dw.value.Description.values(),
	})
	if err != nil {
		return fmt.Errorf("translations: %w", err)
	}
	return nil
}

func (w *writer) saveDishRow(ctx context.Context, dw dishWrite) (int64, error) {
	d, photos := dw.value, photoKeys(dw.value.Photos)
	if dw.id == 0 {
		return w.q.ImportInsertDish(ctx, catalogdb.ImportInsertDishParams{
			Key: d.Key, SortOrder: int64(dw.order), Course: d.Course, Season: d.Season,
			IsVegetarian: d.Flags.Vegetarian, IsSeafood: d.Flags.Seafood, HasPork: d.Flags.Pork,
			IsQuick: d.Flags.Quick, IsSlowCooked: d.Flags.SlowCooked,
			PhotoThumb: photos.Thumb, PhotoCard: photos.Card, PhotoHero: photos.Hero,
		})
	}
	return dw.id, w.q.ImportUpdateDish(ctx, catalogdb.ImportUpdateDishParams{
		ID: dw.id, SortOrder: int64(dw.order), Course: d.Course, Season: d.Season,
		IsVegetarian: d.Flags.Vegetarian, IsSeafood: d.Flags.Seafood, HasPork: d.Flags.Pork,
		IsQuick: d.Flags.Quick, IsSlowCooked: d.Flags.SlowCooked,
		PhotoThumb: photos.Thumb, PhotoCard: photos.Card, PhotoHero: photos.Hero,
	})
}

// photoKeys reads nil photos as no photo in any role.
func photoKeys(p *Photos) Photos {
	if p == nil {
		return Photos{}
	}
	return *p
}
