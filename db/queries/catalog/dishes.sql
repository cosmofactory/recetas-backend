-- name: ListDishes :many
SELECT
    d.key,
    d.sort_order,
    d.course,
    d.season,
    d.is_vegetarian,
    d.is_seafood,
    d.has_pork,
    d.is_quick,
    d.is_slow_cooked,
    d.photo_thumb,
    d.photo_card,
    d.photo_hero,
    t.name        AS name,
    t.description AS description,
    es.name       AS name_es
FROM dishes d
LEFT JOIN dish_translations t ON t.dish_id = d.id AND t.locale = sqlc.arg(locale)::text
LEFT JOIN dish_translations es ON es.dish_id = d.id AND es.locale = sqlc.arg(spanish)::text
ORDER BY d.sort_order;

-- name: ListVariations :many
SELECT
    d.key         AS dish_key,
    r.key         AS region_key,
    v.recipe_key,
    v.photo_thumb,
    v.photo_card,
    v.photo_hero,
    t.name        AS name,
    t.description AS description,
    es.name       AS name_es,
    rt.name       AS region_name
FROM variations v
JOIN dishes d ON d.id = v.dish_id
JOIN regions r ON r.id = v.region_id
LEFT JOIN variation_translations t ON t.variation_id = v.id AND t.locale = sqlc.arg(locale)::text
LEFT JOIN variation_translations es ON es.variation_id = v.id AND es.locale = sqlc.arg(spanish)::text
LEFT JOIN region_translations rt ON rt.region_id = r.id AND rt.locale = sqlc.arg(locale)::text
ORDER BY d.sort_order, r.sort_order;
