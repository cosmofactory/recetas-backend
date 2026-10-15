package importer

import (
	"context"
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

func (w *writer) writeVariations(ctx context.Context, writes []variationWrite) error {
	for _, vw := range writes {
		if err := w.writeVariation(ctx, vw); err != nil {
			return fmt.Errorf("write variation %s: %w", vw.variation.RecipeKey, err)
		}
	}
	return nil
}

func (w *writer) writeVariation(ctx context.Context, vw variationWrite) error {
	id, err := w.saveVariationRow(ctx, vw)
	if err != nil {
		return err
	}
	v := vw.variation
	w.variations[v.RecipeKey] = id
	err = w.q.ImportUpsertVariationTranslations(ctx, catalogdb.ImportUpsertVariationTranslationsParams{
		VariationID:  id,
		Locales:      locales,
		Names:        v.Name.values(),
		Descriptions: v.Description.values(),
	})
	if err != nil {
		return fmt.Errorf("translations: %w", err)
	}
	return nil
}

func (w *writer) saveVariationRow(ctx context.Context, vw variationWrite) (int64, error) {
	v, photos := vw.variation, photoKeys(vw.variation.Photos)
	if vw.id == 0 {
		return w.q.ImportInsertVariation(ctx, catalogdb.ImportInsertVariationParams{
			DishID:     w.dishes[vw.dishKey],
			RegionID:   w.regions[v.RegionKey],
			RecipeKey:  v.RecipeKey,
			PhotoThumb: photos.Thumb, PhotoCard: photos.Card, PhotoHero: photos.Hero,
		})
	}
	return vw.id, w.q.ImportUpdateVariation(ctx, catalogdb.ImportUpdateVariationParams{
		ID:         vw.id,
		PhotoThumb: photos.Thumb, PhotoCard: photos.Card, PhotoHero: photos.Hero,
	})
}
