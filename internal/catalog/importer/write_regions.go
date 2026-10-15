package importer

import (
	"context"
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

func (w *writer) writeRegions(ctx context.Context, writes []regionWrite) error {
	if moved := movedIDs(writes); len(moved) > 0 {
		if err := w.q.ImportParkRegionOrders(ctx, moved); err != nil {
			return fmt.Errorf("park region orders: %w", err)
		}
	}
	for _, rw := range writes {
		if err := w.writeRegion(ctx, rw); err != nil {
			return fmt.Errorf("write region %s: %w", rw.value.Key, err)
		}
	}
	return nil
}

func (w *writer) writeRegion(ctx context.Context, rw regionWrite) error {
	id, err := w.saveRegionRow(ctx, rw)
	if err != nil {
		return err
	}
	w.regions[rw.value.Key] = id
	r := rw.value
	err = w.q.ImportUpsertRegionTranslations(ctx, catalogdb.ImportUpsertRegionTranslationsParams{
		RegionID:     id,
		Locales:      locales,
		Names:        r.Name.values(),
		MapLabels:    r.MapLabel.values(),
		Descriptions: r.Description.values(),
	})
	if err != nil {
		return fmt.Errorf("translations: %w", err)
	}
	if err := w.q.ImportDeleteRegionalDishes(ctx, id); err != nil {
		return fmt.Errorf("clear regional dishes: %w", err)
	}
	err = w.q.ImportInsertRegionalDishes(ctx, catalogdb.ImportInsertRegionalDishesParams{RegionID: id, Names: r.RegionalDishes})
	if err != nil {
		return fmt.Errorf("regional dishes: %w", err)
	}
	return nil
}

func (w *writer) saveRegionRow(ctx context.Context, rw regionWrite) (int64, error) {
	if rw.id == 0 {
		return w.q.ImportInsertRegion(ctx, catalogdb.ImportInsertRegionParams{
			Key: rw.value.Key, SortOrder: int64(rw.order), TileColor: rw.value.TileColor,
		})
	}
	return rw.id, w.q.ImportUpdateRegion(ctx, catalogdb.ImportUpdateRegionParams{
		ID: rw.id, SortOrder: int64(rw.order), TileColor: rw.value.TileColor,
	})
}
