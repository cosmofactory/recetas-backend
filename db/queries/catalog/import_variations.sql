-- name: ImportInsertVariation :one
INSERT INTO variations (dish_id, region_id, recipe_key, photo_thumb, photo_card, photo_hero)
VALUES (
    sqlc.arg(dish_id), sqlc.arg(region_id), sqlc.arg(recipe_key),
    sqlc.narg(photo_thumb), sqlc.narg(photo_card), sqlc.narg(photo_hero)
)
RETURNING id;

-- name: ImportUpdateVariation :exec
UPDATE variations
SET photo_thumb = sqlc.narg(photo_thumb), photo_card = sqlc.narg(photo_card), photo_hero = sqlc.narg(photo_hero)
WHERE id = sqlc.arg(id);

-- name: ImportDeleteVariations :exec
DELETE FROM variations WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ImportUpsertVariationTranslations :exec
INSERT INTO variation_translations (variation_id, locale, name, description)
SELECT
    sqlc.arg(variation_id)::bigint,
    unnest(sqlc.arg(locales)::text[]),
    unnest(sqlc.arg(names)::text[]),
    unnest(sqlc.arg(descriptions)::text[])
ON CONFLICT (variation_id, locale) DO UPDATE
SET name = EXCLUDED.name, description = EXCLUDED.description;
