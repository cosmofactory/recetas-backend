-- name: GetRecipeByDishRegion :one
-- has_time_note is true when the recipe has a note in any locale, so a NULL
-- time_note then means this locale's row is missing.
SELECT
    rc.id,
    rc.time_minutes,
    rc.base_servings,
    d.key          AS dish_key,
    r.key          AS region_key,
    v.recipe_key,
    v.photo_thumb,
    v.photo_card,
    v.photo_hero,
    dt.name        AS dish_name,
    rt.name        AS region_name,
    vt.name        AS name,
    vt.description AS description,
    es.name        AS name_es,
    nt.time_note   AS time_note,
    EXISTS (SELECT 1 FROM recipe_translations n WHERE n.recipe_id = rc.id) AS has_time_note
FROM recipes rc
JOIN variations v ON v.id = rc.variation_id
JOIN dishes d ON d.id = v.dish_id
JOIN regions r ON r.id = v.region_id
LEFT JOIN dish_translations dt ON dt.dish_id = d.id AND dt.locale = sqlc.arg(locale)::text
LEFT JOIN region_translations rt ON rt.region_id = r.id AND rt.locale = sqlc.arg(locale)::text
LEFT JOIN variation_translations vt ON vt.variation_id = v.id AND vt.locale = sqlc.arg(locale)::text
LEFT JOIN variation_translations es ON es.variation_id = v.id AND es.locale = sqlc.arg(spanish)::text
LEFT JOIN recipe_translations nt ON nt.recipe_id = rc.id AND nt.locale = sqlc.arg(locale)::text
WHERE d.key = sqlc.arg(dish_key) AND r.key = sqlc.arg(region_key);

-- name: ListRecipeIngredients :many
SELECT i.quantity::float8 AS quantity, i.unit, t.name AS name
FROM recipe_ingredients i
LEFT JOIN recipe_ingredient_translations t
    ON t.recipe_id = i.recipe_id AND t.position = i.position AND t.locale = sqlc.arg(locale)::text
WHERE i.recipe_id = sqlc.arg(recipe_id)
ORDER BY i.position;

-- name: ListRecipeSteps :many
SELECT t.text AS text
FROM recipe_steps s
LEFT JOIN recipe_step_translations t
    ON t.recipe_id = s.recipe_id AND t.position = s.position AND t.locale = sqlc.arg(locale)::text
WHERE s.recipe_id = sqlc.arg(recipe_id)
ORDER BY s.position;
