package importer

import (
	"context"
	"fmt"

	"github.com/cosmofactory/recetas-backend/internal/catalog/catalogdb"
)

// writeVariations first moves the kept variations whose position changes,
// all at once, so their new positions are free for the variations created.
func (w *writer) writeVariations(ctx context.Context, writes []variationWrite) error {
	if err := w.moveVariations(ctx, writes); err != nil {
		return err
	}
	for _, vw := range writes {
		if vw.id != 0 && !vw.edited { // only moved, which is done
			continue
		}
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

func (w *writer) moveVariations(ctx context.Context, writes []variationWrite) error {
	var moves catalogdb.ImportMoveVariationsParams
	for _, vw := range writes {
		if vw.moved {
			moves.Ids = append(moves.Ids, vw.id)
			moves.Positions = append(moves.Positions, int64(vw.position))
		}
	}
	if len(moves.Ids) == 0 {
		return nil
	}
	if err := w.q.ImportMoveVariations(ctx, moves); err != nil {
		return fmt.Errorf("move variations: %w", err)
	}
	return nil
}

func (w *writer) saveVariationRow(ctx context.Context, vw variationWrite) (int64, error) {
	v, photos := vw.variation, photoKeys(vw.variation.Photos)
	if vw.id == 0 {
		regionID, slug := w.placeOf(v)
		return w.q.ImportInsertVariation(ctx, catalogdb.ImportInsertVariationParams{
			DishID:     w.dishes[vw.dishKey],
			RegionID:   regionID,
			Slug:       slug,
			Position:   int64(vw.position),
			RecipeKey:  v.RecipeKey,
			Score:      v.Score,
			PhotoThumb: photos.Thumb, PhotoCard: photos.Card, PhotoHero: photos.Hero,
		})
	}
	return vw.id, w.q.ImportUpdateVariation(ctx, catalogdb.ImportUpdateVariationParams{
		ID:         vw.id,
		Score:      v.Score,
		PhotoThumb: photos.Thumb, PhotoCard: photos.Card, PhotoHero: photos.Hero,
	})
}

// placeOf is a variation row's region, or for a Spanish variation no region
// and its slug (D-ESP-1).
func (w *writer) placeOf(v Variation) (regionID *int64, slug *string) {
	if v.spainWide() {
		return nil, &v.VariationSlug
	}
	id := w.regions[v.RegionKey]
	return &id, nil
}
