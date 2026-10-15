-- name: ImportInsertRegion :one
INSERT INTO regions (key, sort_order, tile_color)
VALUES (sqlc.arg(key), sqlc.arg(sort_order)::bigint, sqlc.arg(tile_color))
RETURNING id;

-- name: ImportUpdateRegion :exec
UPDATE regions
SET sort_order = sqlc.arg(sort_order)::bigint, tile_color = sqlc.arg(tile_color)
WHERE id = sqlc.arg(id);

-- name: ImportParkRegionOrders :exec
-- Negates the sort orders about to change, so the new ones can't collide
-- with them under the unique constraint.
UPDATE regions SET sort_order = -sort_order WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ImportDeleteRegions :exec
DELETE FROM regions WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ImportUpsertRegionTranslations :exec
INSERT INTO region_translations (region_id, locale, name, map_label, description)
SELECT
    sqlc.arg(region_id)::bigint,
    unnest(sqlc.arg(locales)::text[]),
    unnest(sqlc.arg(names)::text[]),
    unnest(sqlc.arg(map_labels)::text[]),
    unnest(sqlc.arg(descriptions)::text[])
ON CONFLICT (region_id, locale) DO UPDATE
SET name = EXCLUDED.name, map_label = EXCLUDED.map_label, description = EXCLUDED.description;

-- name: ImportDeleteRegionalDishes :exec
DELETE FROM region_regional_dishes WHERE region_id = sqlc.arg(region_id);

-- name: ImportInsertRegionalDishes :exec
INSERT INTO region_regional_dishes (region_id, position, name)
SELECT
    sqlc.arg(region_id)::bigint,
    generate_series(1, cardinality(sqlc.arg(names)::text[])),
    unnest(sqlc.arg(names)::text[]);
