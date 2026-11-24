-- name: ImportInsertVariation :one
-- A Spanish (Spain-wide) variation has no region and a slug.
INSERT INTO variations (dish_id, region_id, slug, position, recipe_key, photo_thumb, photo_card, photo_hero, score)
VALUES (
    sqlc.arg(dish_id), sqlc.narg(region_id), sqlc.narg(slug), sqlc.arg(position)::bigint, sqlc.arg(recipe_key),
    sqlc.narg(photo_thumb), sqlc.narg(photo_card), sqlc.narg(photo_hero), sqlc.arg(score)
)
RETURNING id;

-- name: ImportUpdateVariation :exec
UPDATE variations
SET photo_thumb = sqlc.narg(photo_thumb), photo_card = sqlc.narg(photo_card), photo_hero = sqlc.narg(photo_hero),
    score = sqlc.arg(score)
WHERE id = sqlc.arg(id);

-- name: ImportMoveVariations :exec
-- One statement, so the deferrable UNIQUE (dish_id, position) is checked
-- only once every position has moved.
UPDATE variations v
SET position = m.position
FROM (
    SELECT unnest(sqlc.arg(ids)::bigint[]) AS id, unnest(sqlc.arg(positions)::bigint[]) AS position
) m
WHERE v.id = m.id;

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
