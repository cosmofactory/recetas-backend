-- name: ListRegions :many
SELECT
    r.key,
    r.sort_order,
    r.tile_color,
    t.name        AS name,
    t.map_label   AS map_label,
    t.description AS description,
    es.name       AS name_es
FROM regions r
LEFT JOIN region_translations t ON t.region_id = r.id AND t.locale = sqlc.arg(locale)::text
LEFT JOIN region_translations es ON es.region_id = r.id AND es.locale = sqlc.arg(spanish)::text
ORDER BY r.sort_order;

-- name: ListRegionalDishes :many
SELECT r.key AS region_key, d.name
FROM region_regional_dishes d
JOIN regions r ON r.id = d.region_id
ORDER BY r.sort_order, d.position;
